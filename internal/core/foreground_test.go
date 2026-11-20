package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

func runForegroundHelper(args []string) {
	mode := os.Getenv("GOSCOPY_FOREGROUND_HELPER")
	joined := strings.Join(args, " ")
	dir := os.Getenv("GOSCOPY_SESSION_HELPER_DIR")
	switch {
	case strings.Contains(joined, "dumpsys activity activities"):
		pkg := "com.example.current"
		if mode == "launcher" {
			pkg = "com.example.launcher"
		}
		fmt.Printf("Display #0:\n  mResumedActivity: ActivityRecord{abc u0 %s/.Main}\n", pkg)
		// An existing virtual window of the same app must not satisfy readiness.
		fmt.Println("Display #2:\n  topResumedActivity=ActivityRecord{old u0 com.example.current/.Main}")
		if _, err := os.Stat(filepath.Join(dir, "app.ready")); dir != "" && err == nil {
			fmt.Println("Display #3:\n  topResumedActivity=ActivityRecord{new u0 com.example.current/.Main}")
		}
	case strings.Contains(joined, "resolve-activity"):
		fmt.Println("com.example.launcher/.Home")
	case strings.Contains(joined, "keyevent"):
		if !strings.HasSuffix(joined, "shell input -d 0 keyevent 3") {
			fmt.Fprintln(os.Stderr, "HOME targeted wrong display")
			os.Exit(1)
		}
		if _, err := os.Stat(filepath.Join(dir, "app.ready")); err != nil {
			fmt.Fprintln(os.Stderr, "HOME sent before virtual app ready")
			os.Exit(1)
		}
		if err := os.WriteFile(filepath.Join(dir, "home.sent"), []byte(joined), 0600); err != nil {
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func TestResolveForegroundUsesTaskCenterAndAlwaysCompletes(t *testing.T) {
	for _, mode := range []string{"app", "launcher", "offline", "closed"} {
		t.Run(mode, func(t *testing.T) {
			s := testService(t)
			t.Setenv("GOSCOPY_FOREGROUND_HELPER", mode)
			if mode == "offline" {
				s.state.Devices[0].State = "offline"
			}
			if mode == "closed" {
				s.Close()
			}
			type result struct {
				pkg string
				err error
			}
			done := make(chan result, 2)
			s.ResolveForeground("test-serial", func(pkg string, err error) { done <- result{pkg, err} })
			select {
			case r := <-done:
				if mode == "app" {
					if r.err != nil || r.pkg != "com.example.current" {
						t.Fatalf("result: %+v", r)
					}
				} else if r.err == nil || r.pkg != "" {
					t.Fatalf("expected failure, got %+v", r)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("resolution did not complete")
			}
			if mode != "closed" {
				task := taskResult(t, s)
				if (task.Status == "done") != (mode == "app") {
					t.Fatalf("task: %+v", task)
				}
			}
			select {
			case <-done:
				t.Fatal("callback invoked twice")
			default:
			}
		})
	}
}

type handoffHost struct {
	readyHost
	ready chan struct{}
}

func (h *handoffHost) Ready() { close(h.ready) }

func TestIndependentAppReturnsOnlyMainDisplayHomeAfterReadiness(t *testing.T) {
	s := testService(t)
	dir := t.TempDir()
	t.Setenv("GOSCOPY_FOREGROUND_HELPER", "app")
	t.Setenv("GOSCOPY_SESSION_HELPER_DIR", dir)
	t.Cleanup(func() {
		// Release every real helper before Service.Close and TempDir cleanup.
		s.mu.Lock()
		for _, p := range s.processes {
			_ = os.WriteFile(filepath.Join(dir, strconv.Itoa(p.Cmd.Process.Pid)+".stop"), nil, 0600)
		}
		s.mu.Unlock()
		s.Close()
	})
	options := s.State().Settings.Defaults
	s.Start("test-serial", options, false)
	waitSessions(t, s, 1)
	display := domain.VirtualDisplay{Width: 1080, Height: 2400, DPI: 420, SystemDecorations: true, StartApp: "com.example.current", ReturnHome: true}
	host := &handoffHost{readyHost: readyHost{finished: make(chan error, 1)}, ready: make(chan struct{})}
	s.StartHosted("test-serial", domain.NewVirtualSessionID("test-serial"), options, &display, false, host)
	waitSessions(t, s, 2)
	time.Sleep(250 * time.Millisecond)
	select {
	case <-host.ready:
		t.Fatal("controls became ready before app handoff")
	default:
	}
	if _, err := os.Stat(filepath.Join(dir, "home.sent")); !os.IsNotExist(err) {
		t.Fatal("sent HOME before new virtual app was resumed")
	}
	if err := os.WriteFile(filepath.Join(dir, "app.ready"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		if _, err := os.Stat(filepath.Join(dir, "home.sent")); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("HOME never sent; tasks: %+v", s.State().Tasks)
		case <-time.After(10 * time.Millisecond):
		}
	}
	select {
	case <-host.ready:
	case <-time.After(time.Second):
		t.Fatal("controls were not enabled after app handoff")
	}
	if len(s.State().Sessions) != 2 {
		t.Fatal("shortcut stopped the original mirror")
	}
	select {
	case err := <-host.finished:
		t.Fatalf("virtual session exited unexpectedly: %v", err)
	default:
	}
}

func TestIndependentAppStartupFailureDoesNotSendHome(t *testing.T) {
	s := testService(t)
	t.Setenv("GOSCOPY_FOREGROUND_HELPER", "app")
	// Without SESSION_HELPER_DIR the fake scrcpy deliberately fails startup.
	t.Setenv("GOSCOPY_SESSION_HELPER_DIR", "")
	display := domain.VirtualDisplay{Width: 1080, Height: 2400, DPI: 420, SystemDecorations: true, StartApp: "com.example.current", ReturnHome: true}
	host := &readyHost{finished: make(chan error, 1)}
	s.StartHosted("test-serial", domain.NewVirtualSessionID("test-serial"), s.State().Settings.Defaults, &display, false, host)
	select {
	case err := <-host.finished:
		if err == nil {
			t.Fatal("startup failure was not reported")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("failed process did not cancel handoff promptly")
	}
	if task := taskResult(t, s); task.Status != "error" {
		t.Fatalf("unexpected task: %+v", task)
	}
	if len(s.State().Sessions) != 0 {
		t.Fatal("failed virtual session remained registered")
	}
}
