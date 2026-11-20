package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

// Each real helper subprocess has its own stop marker, so stopping a session
// can be verified without real scrcpy windows or modifying a user's device.
func runSessionHelper() {
	// Unix stops real scrcpy with SIGINT; Windows posts WM_CLOSE (our helper
	// has no windows). Keep this fixture alive until its explicit stop marker
	// on every platform so the test can reliably observe the stopping state.
	signal.Ignore(os.Interrupt)
	dir := os.Getenv("GOSCOPY_SESSION_HELPER_DIR")
	pid := strconv.Itoa(os.Getpid())
	data, _ := json.Marshal(os.Args[1:])
	if err := os.WriteFile(filepath.Join(dir, pid+".json"), data, 0600); err != nil {
		os.Exit(2)
	}
	for i, arg := range os.Args {
		if arg == "--record" && i+1 < len(os.Args) {
			if err := os.WriteFile(os.Args[i+1], []byte("helper recording payload"), 0600); err != nil {
				os.Exit(2)
			}
		}
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, pid+".stop")); err == nil {
			os.Exit(0)
		}
		time.Sleep(10 * time.Millisecond)
	}
	os.Exit(3)
}

func waitSessions(t *testing.T, s *Service, count int) map[string]domain.Session {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		sessions := s.State().Sessions
		ready := len(sessions) == count
		for _, session := range sessions {
			ready = ready && session.Status == "running"
		}
		if ready {
			return sessions
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("wanted %d running sessions, got %+v; tasks %+v", count, s.State().Sessions, s.State().Tasks)
	return nil
}

func TestMultipleVirtualSessionsHaveIndependentProcesses(t *testing.T) {
	for _, hosted := range []bool{false, true} {
		t.Run(fmt.Sprint("hosted=", hosted), func(t *testing.T) {
			s := testService(t)
			dir := t.TempDir()
			t.Setenv("GOSCOPY_SESSION_HELPER_DIR", dir)
			// Registered after Service.Close: release the helpers first, even on failure.
			t.Cleanup(func() {
				s.mu.Lock()
				for _, p := range s.processes {
					_ = os.WriteFile(filepath.Join(dir, strconv.Itoa(p.Cmd.Process.Pid)+".stop"), nil, 0600)
				}
				s.mu.Unlock()
				// Wait before TempDir removes the markers.
				s.Close()
			})
			o := s.State().Settings.Defaults
			d := domain.VirtualDisplay{Width: 1080, Height: 2400, DPI: 420, StartApp: "com.example.first", SystemDecorations: true}
			s.Start("test-serial", o, false)
			for i := range 3 {
				if i == 2 {
					d.StartApp = "com.example.second"
				}
				if hosted {
					h := &readyHost{finished: make(chan error, 1)}
					sent := d
					s.StartHosted("test-serial", domain.NewVirtualSessionID("test-serial"), o, &sent, false, h)
					sent.StartApp = "must.not.leak.into.async.launch"
				} else {
					s.StartVirtual("test-serial", o, d, false)
				}
				// Wait for process startup before checking the next launch.
				waitSessions(t, s, i+2)
			}
			// One bad launch must not remove or stop its successful siblings.
			bad := d
			bad.Width = 0
			s.StartVirtual("test-serial", o, bad, false)
			if task := taskResult(t, s); task.Status != "error" {
				t.Fatalf("invalid display was accepted: %+v", task)
			}
			sessions := waitSessions(t, s, 4)
			// Inspect actual subprocess argument boundaries, not only state entries.
			pids := map[int]bool{}
			for id, session := range sessions {
				s.mu.Lock()
				pid := s.processes[id].Cmd.Process.Pid
				s.mu.Unlock()
				if pids[pid] {
					t.Fatal("sessions shared a subprocess")
				}
				pids[pid] = true
				var args []string
				deadline := time.Now().Add(3 * time.Second)
				for {
					data, err := os.ReadFile(filepath.Join(dir, strconv.Itoa(pid)+".json"))
					if err == nil && json.Unmarshal(data, &args) == nil {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("helper did not become ready")
					}
					time.Sleep(5 * time.Millisecond)
				}
				if session.Kind == domain.SessionVirtual {
					joined := strings.Join(args, " ")
					if !strings.Contains(joined, "--new-display=1080x2400/420") || !strings.Contains(joined, "--start-app="+session.StartApp) {
						t.Fatalf("wrong virtual process arguments: %q", args)
					}
				}
			}
			var target string
			packages := map[string]int{}
			for id, session := range sessions {
				if session.Kind == domain.SessionVirtual {
					packages[session.StartApp]++
					target = id
				}
			}
			if packages["com.example.first"] != 2 || packages["com.example.second"] != 1 {
				t.Fatalf("launch metadata: %+v", packages)
			}
			s.mu.Lock()
			pid := s.processes[target].Cmd.Process.Pid
			s.mu.Unlock()
			s.Stop(target)
			deadline := time.Now().Add(5 * time.Second)
			for s.State().Sessions[target].Status != "stopping" {
				if time.Now().After(deadline) {
					t.Fatal("session did not enter stopping state")
				}
				time.Sleep(5 * time.Millisecond)
			}
			if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(pid)+".stop"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			remaining := waitSessions(t, s, 3)
			if _, ok := remaining[target]; ok {
				t.Fatal("stopped virtual session retained")
			}
			for id := range remaining {
				s.mu.Lock()
				p := s.processes[id]
				s.mu.Unlock()
				select {
				case <-p.Done:
					t.Fatalf("stopping %s also ended %s", target, id)
				default:
				}
			}
			// A second mirror must be rejected, not replace the original process.
			s.Start("test-serial", o, false)
			if task := taskResult(t, s); task.Status != "error" || !strings.Contains(task.Detail, "此会话已在运行") {
				t.Fatalf("mirror reuse changed: %+v", task)
			}
			if len(s.State().Sessions) != 3 {
				t.Fatal("duplicate mirror removed another session")
			}
		})
	}
}

type readyHost struct{ finished chan error }

func (*readyHost) Attach(_ context.Context, _ int) error { return nil }
func (*readyHost) Done() <-chan struct{}                 { return nil }
func (h *readyHost) Finished(err error)                  { h.finished <- err }

func TestRecordingSessionFlushesBeforeNonRecordingSessionStarts(t *testing.T) {
	s := testService(t)
	dir := t.TempDir()
	t.Setenv("GOSCOPY_SESSION_HELPER_DIR", dir)
	t.Cleanup(func() {
		s.mu.Lock()
		for _, p := range s.processes {
			_ = os.WriteFile(filepath.Join(dir, strconv.Itoa(p.Cmd.Process.Pid)+".stop"), nil, 0600)
		}
		s.mu.Unlock()
		s.Close()
	})
	id := domain.SessionID("test-serial", domain.SessionMirror)
	options := s.State().Settings.Defaults
	host := &readyHost{finished: make(chan error, 1)}
	s.StartHosted("test-serial", id, options, nil, true, host)
	first := waitSessions(t, s, 1)[id]
	if first.Recording == "" {
		t.Fatal("recording path not published")
	}
	s.mu.Lock()
	pid := s.processes[id].Cmd.Process.Pid
	s.mu.Unlock()
	if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(pid)+".stop"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-host.finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("recording did not finish")
	}
	data, err := os.ReadFile(first.Recording)
	if err != nil || len(data) == 0 {
		t.Fatalf("recording was not saved: %v", err)
	}
	next := &readyHost{finished: make(chan error, 1)}
	s.StartHosted("test-serial", id, options, nil, false, next)
	second := waitSessions(t, s, 1)[id]
	if second.Recording != "" {
		t.Fatal("non-recording restart is still recording")
	}
	after, err := os.ReadFile(first.Recording)
	if err != nil || string(after) != string(data) {
		t.Fatal("restart changed the saved recording")
	}
}
