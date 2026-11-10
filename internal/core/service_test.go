package core

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools"
)

// The helper is a real subprocess: tests exercise binary I/O, exit handling and
// argument boundaries without connecting to a user's phone or adb server.
func TestMain(m *testing.M) {
	if os.Getenv("GOSCOPY_TOOL_HELPER") == "1" {
		a := os.Args[1:]
		joined := strings.Join(a, " ")
		switch {
		case strings.Contains(joined, "shell ip route"):
			fmt.Println("10.155.160.0/24 dev rndis0 proto kernel scope link src 10.155.160.129")
			fmt.Println("10.163.225.60/30 dev rmnet_data2 proto kernel scope link src 10.163.225.62")
			if os.Getenv("GOSCOPY_WIFI_FALLBACK") != "1" {
				fmt.Println("10.168.2.0/24 dev wlan0 proto kernel scope link src 10.168.2.20")
			}
			os.Exit(0)
		case strings.Contains(joined, "addr show dev wlan0"):
			fmt.Println("17: wlan0 inet 10.168.2.20/24 brd 10.168.2.255 scope global wlan0")
			os.Exit(0)
		case strings.Contains(joined, "get-state"):
			if os.Getenv("GOSCOPY_WIFI_OFFLINE") == "1" {
				fmt.Println("offline")
			} else {
				fmt.Println("device")
			}
			os.Exit(0)
		case strings.Contains(joined, " connect ") && !strings.Contains(joined, "bad.local"):
			fmt.Println("connected to " + a[len(a)-1])
			os.Exit(0)
		case strings.Contains(joined, "connect bad.local"):
			fmt.Println("failed to connect to bad.local")
			os.Exit(0)
		case strings.Contains(joined, " pair "):
			fmt.Fprintln(os.Stderr, "pair failed")
			os.Exit(1)
		case strings.Contains(joined, "screencap"):
			png.Encode(os.Stdout, image.NewRGBA(image.Rect(0, 0, 2, 2)))
			os.Exit(0)
		case strings.Contains(joined, "install"):
			if len(a) < 2 || !strings.HasSuffix(a[len(a)-1], "中文 空格.apk") {
				fmt.Fprintln(os.Stderr, "path argument was split")
				os.Exit(1)
			}
			fmt.Println("Success")
			os.Exit(0)
		case strings.Contains(joined, "--serial"):
			fmt.Fprintln(os.Stderr, "ERROR: deliberate scrcpy startup failure")
			os.Exit(1)
		default:
			fmt.Println("connected")
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func testService(t *testing.T) *Service {
	t.Helper()
	t.Setenv("GOSCOPY_TOOL_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{dir: t.TempDir(), ctx: ctx, cancel: cancel, tool: tools.Tools{ADB: exe, Scrcpy: exe}, processes: map[string]*tools.Process{}, state: domain.State{Settings: config.Default(), Sessions: map[string]domain.Session{}, Devices: []domain.Device{{Serial: "test-serial", State: "device", Connection: "USB"}}}}
	s.state.Settings.OutputDir = filepath.Join(t.TempDir(), "图片 录像")
	t.Cleanup(s.Close)
	return s
}

func taskResult(t *testing.T, s *Service) domain.Task {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state := s.State()
		if len(state.Tasks) > 0 && state.Tasks[0].Status != "running" {
			return state.Tasks[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("task did not finish")
	return domain.Task{}
}

func TestConnectDetectsZeroExitFailureAndPairHidesSecret(t *testing.T) {
	s := testService(t)
	s.Connect("bad.local:5555")
	if task := taskResult(t, s); task.Status != "error" {
		t.Fatalf("false success: %+v", task)
	}
	s.Pair("127.0.0.1:37001", "123456")
	task := taskResult(t, s)
	if task.Status != "error" || strings.Contains(task.Detail, "123456") {
		t.Fatalf("pair result: %+v", task)
	}
}

func TestEnableTCPUsesWiFiInsteadOfUSBTethering(t *testing.T) {
	for _, fallback := range []string{"0", "1"} {
		t.Run("fallback="+fallback, func(t *testing.T) {
			s := testService(t)
			t.Setenv("GOSCOPY_WIFI_FALLBACK", fallback)
			s.EnableTCP("test-serial")
			task := taskResult(t, s)
			if task.Status != "done" || !strings.Contains(task.Detail, "10.168.2.20:5555") {
				t.Fatalf("incorrect wireless endpoint: %+v", task)
			}
		})
	}
}

func TestConnectRejectsOfflineTransportAfterSuccessfulConnect(t *testing.T) {
	s := testService(t)
	t.Setenv("GOSCOPY_WIFI_OFFLINE", "1")
	s.Connect("10.168.2.20:5555")
	task := taskResult(t, s)
	if task.Status != "error" || !strings.Contains(task.Detail, "offline") {
		t.Fatalf("false wireless success: %+v", task)
	}
}

func TestScreenshotAndAPKWithUnicodePaths(t *testing.T) {
	s := testService(t)
	s.Capture("test-serial")
	task := taskResult(t, s)
	if task.Status != "done" {
		t.Fatal(task.Detail)
	}
	file, err := os.Open(task.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = png.Decode(file); err != nil {
		t.Fatal(err)
	}
	apk := filepath.Join(t.TempDir(), "中文 空格.apk")
	os.WriteFile(apk, []byte("fixture"), 0600)
	s.Install("test-serial", apk)
	if task := taskResult(t, s); task.Status != "done" {
		t.Fatal(task.Detail)
	}
}

func TestStartupFailureReleasesDeviceSession(t *testing.T) {
	s := testService(t)
	s.Start("test-serial", s.state.Settings.Defaults, false)
	task := taskResult(t, s)
	if task.Status != "error" || !strings.Contains(task.Detail, "deliberate scrcpy startup failure") {
		t.Fatal(task)
	}
	if len(s.State().Sessions) != 0 {
		t.Fatal("failed session was retained")
	}
}

type startupHost struct{ finished chan error }

func (*startupHost) Attach(ctx context.Context, _ int) error { <-ctx.Done(); return ctx.Err() }
func (*startupHost) Done() <-chan struct{}                   { return nil }
func (h *startupHost) Finished(err error)                    { h.finished <- err }

func TestHostedStartupFailureReleasesContainer(t *testing.T) {
	s := testService(t)
	h := &startupHost{finished: make(chan error, 1)}
	s.StartHosted("test-serial", s.State().Settings.Defaults, false, h)
	task := taskResult(t, s)
	if task.Status != "error" || !strings.Contains(task.Detail, "deliberate scrcpy startup failure") {
		t.Fatal(task)
	}
	select {
	case err := <-h.finished:
		if err == nil {
			t.Fatal("container did not receive startup error")
		}
	case <-time.After(time.Second):
		t.Fatal("container left waiting after process exit")
	}
	if len(s.State().Sessions) != 0 {
		t.Fatal("failed hosted session was retained")
	}
}

func TestUnauthorizedDeviceRejectsOperation(t *testing.T) {
	s := testService(t)
	s.state.Devices[0].State = "unauthorized"
	s.Capture("test-serial")
	task := taskResult(t, s)
	if task.Status != "error" || !strings.Contains(task.Detail, "允许 USB 调试") {
		t.Fatal(task)
	}
}

func TestSnapshotsAreIndependentAndConfigPersists(t *testing.T) {
	s := testService(t)
	first := s.State()
	first.Devices[0].State = "offline"
	first.Sessions["mutated"] = domain.Session{}
	first.Settings.Devices["mutated"] = first.Settings.Defaults
	if s.State().Devices[0].State != "device" || len(s.State().Sessions) > 0 || len(s.State().Settings.Devices) > 0 {
		t.Fatal("snapshot leaked mutable state")
	}
	next := s.State().Settings
	next.Theme = "dark"
	s.Save(next)
	if task := taskResult(t, s); task.Status != "done" {
		t.Fatal(task)
	}
	saved, err := config.Load(s.dir)
	if err != nil || saved.Theme != "dark" {
		t.Fatalf("settings not persisted: %v", err)
	}
}

func TestSavedWirelessDevicesRemainOfflineWithoutDuplicateTransports(t *testing.T) {
	s := testService(t)
	s.state.Settings.SavedWireless = map[string]bool{"10.168.2.20:5555": true, "10.168.2.21:5555": true}
	s.state.Settings.DeviceNames = map[string]string{"10.168.2.20:5555": "手机"}
	s.state.Devices = append(s.state.Devices, domain.Device{Serial: "10.168.2.21:5555", State: "device", Connection: "Wi-Fi"})
	state := s.State()
	if len(state.Devices) != 3 {
		t.Fatalf("saved device missing or duplicated: %+v", state.Devices)
	}
	for _, d := range state.Devices {
		if d.Serial == "10.168.2.20:5555" && (d.State != "offline" || d.Connection != "Wi-Fi") {
			t.Fatal("saved device incorrectly marked online")
		}
	}
	state.Settings.DeviceNames["10.168.2.20:5555"] = "changed"
	delete(state.Settings.SavedWireless, "10.168.2.21:5555")
	if s.State().Settings.DeviceNames["10.168.2.20:5555"] != "手机" || !s.State().Settings.SavedWireless["10.168.2.21:5555"] {
		t.Fatal("record snapshot mutated backend")
	}
	s.Save(config.ForgetDevice(s.State().Settings, "10.168.2.20:5555"))
	if task := taskResult(t, s); task.Status != "done" || len(s.State().Devices) != 2 {
		t.Fatalf("forget failed: %+v", task)
	}
}
