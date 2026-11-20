package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/windowhost"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"
)

type toolbarBackend struct {
	*fixture
	resolve func(string, error)
	display *domain.VirtualDisplay
}

func (f *toolbarBackend) ResolveForeground(serial string, done func(string, error)) {
	f.action, f.serial, f.resolve = "resolve", serial, done
}
func (f *toolbarBackend) StartVirtual(serial string, options domain.Options, display domain.VirtualDisplay, record bool) {
	f.action, f.serial, f.options, f.display, f.record = "virtual", serial, options, &display, record
}

func (f *toolbarBackend) StartHosted(string, string, domain.Options, *domain.VirtualDisplay, bool, domain.SessionHost) {
}
func (f *toolbarBackend) Key(serial, action string) { f.serial, f.action = serial, action }

func TestSessionToolbarTargetsItsDevice(t *testing.T) {
	f := &toolbarBackend{fixture: sample()}
	u := New(f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &sessionWindow{owner: u, serial: "192.168.1.28:37001", name: "Pixel 9 Pro", ready: true, collapsed: true, ctx: ctx, cancel: cancel, native: windowhost.New("test")}
	w.scope = w.tree.Feature("session")
	s, err := headless.Open(app.Config{Width: 236, Height: 48, Paint: w.paintTools, Tree: &w.tree}, nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	clean(t, s)
	if _, exists := s.Hit("桌面"); exists {
		t.Fatal("collapsed island exposes phone commands")
	}
	preview := os.Getenv("GOSCOPY_ISLAND_PREVIEW")
	if preview != "" {
		if err := os.MkdirAll(preview, 0755); err != nil {
			t.Fatal(err)
		}
		if err := s.Screenshot(filepath.Join(preview, "island-collapsed.png")); err != nil {
			t.Fatal(err)
		}
	}
	click(t, s, "展开控制面板")
	if w.collapsed {
		t.Fatal("island did not expand")
	}
	if err := s.Driver().Resize(344, 440); err != nil {
		t.Fatal(err)
	}
	clean(t, s)
	if preview != "" {
		if err := s.Screenshot(filepath.Join(preview, "island-expanded.png")); err != nil {
			t.Fatal(err)
		}
	}
	click(t, s, "桌面")
	if f.action != "home" || f.serial != w.serial {
		t.Fatal("toolbar targeted another device")
	}
	click(t, s, "任务")
	if f.action != "recents" {
		t.Fatal("recent apps action was not dispatched")
	}
	click(t, s, "截图")
	if f.action != "capture" || f.serial != w.serial {
		t.Fatal("screenshot targeted another device")
	}
	w.closing = true
	s.Driver().Resize(344, 440)
	if _, exists := s.Hit("桌面"); exists {
		t.Fatal("closing session accepts phone commands")
	}
}

func testIsland(t *testing.T, record bool) (*sessionWindow, *toolbarBackend, *headless.Session) {
	t.Helper()
	f := &toolbarBackend{fixture: sample()}
	u := New(f)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	w := &sessionWindow{
		owner: u, serial: "USB-DEMO-01", session: domain.SessionID("USB-DEMO-01", domain.SessionMirror),
		kind: domain.SessionMirror, name: "Pixel", ready: true, recording: record,
		ctx: ctx, cancel: cancel, native: windowhost.New("test"), options: u.state.Settings.Defaults,
	}
	w.scope = w.tree.Feature("session")
	u.sessionWindows = map[string]*sessionWindow{w.session: w}
	s, err := headless.Open(app.Config{Width: 344, Height: 440, Paint: w.paintTools, Tree: &w.tree}, nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	clean(t, s)
	return w, f, s
}

func TestIndependentAppResolvesThenLaunchesWithoutNavigatingEarly(t *testing.T) {
	w, f, s := testIsland(t, false)
	click(t, s, "独立窗口")
	if f.action != "resolve" || f.serial != w.serial || !w.resolving {
		t.Fatal("did not resolve this island's device")
	}
	w.detachCurrentApp()
	if f.display != nil {
		t.Fatal("started a virtual display before resolving the app")
	}
	// Workers only publish the result. UI launch must wait for onWake.
	f.resolve("com.example.current", nil)
	if f.display != nil {
		t.Fatal("worker performed a UI launch")
	}
	w.onWake()
	if w.resolving || f.display == nil || f.display.StartApp != "com.example.current" || !f.display.ReturnHome {
		t.Fatalf("wrong independent launch: %+v", f.display)
	}
	if f.display.Width != 1080 || f.display.Height != 2400 || f.display.DPI != 420 || f.options != w.options || f.record {
		t.Fatal("shortcut did not reuse virtual defaults and source options")
	}
	if w.closing || w.ctx.Err() != nil {
		t.Fatal("shortcut stopped the original mirror")
	}
}

func TestIndependentAppErrorsAndLateResultsDoNotLaunch(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "closed"}[late], func(t *testing.T) {
			w, f, _ := testIsland(t, false)
			w.detachCurrentApp()
			if late {
				w.requestClose()
				f.resolve("com.example.current", nil)
			} else {
				f.resolve("", errors.New("current app is launcher"))
			}
			w.onWake()
			if f.display != nil {
				t.Fatal("late/failed resolution launched an app")
			}
			if !late && w.owner.notice == "" {
				t.Fatal("missing failure message")
			}
		})
	}
}

func TestVirtualIslandCannotDetachMainDisplayApp(t *testing.T) {
	w, f, s := testIsland(t, false)
	w.kind = domain.SessionVirtual
	s.Driver().Resize(344, 440)
	clean(t, s)
	if _, exists := s.Hit("独立窗口"); exists {
		t.Fatal("virtual island exposes main-display app shortcut")
	}
	w.detachCurrentApp()
	if f.resolve != nil {
		t.Fatal("virtual island resolved the wrong display")
	}
}

func TestIslandRecordingUsesGracefulRestart(t *testing.T) {
	for _, record := range []bool{false, true} {
		label := "开始录屏"
		if record {
			label = "停止录屏"
		}
		t.Run(label, func(t *testing.T) {
			w, f, s := testIsland(t, record)
			click(t, s, label)
			if w.recording == record || !w.restart || !w.closing || w.ctx.Err() == nil {
				t.Fatal("recording toggle did not request a graceful restart")
			}
			if w.disposed || f.action != "" || w.owner.sessionWindows[w.session] != w {
				t.Fatal("old session was disposed/replaced before recorder finished")
			}
			w.toggleRecording()
			if w.recording == record {
				t.Fatal("closing session accepted a second toggle")
			}
		})
	}
}

func TestShutdownCancelsPendingIslandRestart(t *testing.T) {
	w, _, _ := testIsland(t, false)
	w.toggleRecording()
	w.owner.closeSessionWindows()
	if w.restart {
		t.Fatal("application shutdown left a restart scheduled")
	}
}

func TestFailedRecordingShutdownCancelsRestart(t *testing.T) {
	w, _, _ := testIsland(t, true)
	w.toggleRecording()
	w.Finished(errors.New("recording flush failed"))
	w.onWake()
	if w.restart || !w.disposed {
		t.Fatal("failed recording shutdown restarted a session")
	}
}

func TestIndependentWindowBecomesReadyAfterBackendHandoff(t *testing.T) {
	w, _, _ := testIsland(t, false)
	w.ready = false
	w.display = &domain.VirtualDisplay{StartApp: "com.example.app", ReturnHome: true}
	w.onWake()
	if w.ready || !w.display.ReturnHome {
		t.Fatal("session became ready before handoff")
	}
	w.Ready()
	if w.ready {
		t.Fatal("worker changed UI readiness directly")
	}
	w.onWake()
	if !w.ready || w.display.ReturnHome {
		t.Fatal("completed handoff did not enable controls and clear one-shot flag")
	}
}
