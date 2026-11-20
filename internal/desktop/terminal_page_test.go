package desktop

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools/pty"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"
	"github.com/vivaui/viva/ui"
)

type terminalFixture struct {
	*fixture
	stream domain.Terminal
	err    error
	opens  int
}

func (f *terminalFixture) OpenShell(string, int, int, func()) (domain.Terminal, error) {
	f.opens++
	return f.stream, f.err
}
func (f *terminalFixture) OpenLogcat(s string, c, r int, n func()) (domain.Terminal, error) {
	return f.OpenShell(s, c, r, n)
}

func TestFailedTerminalDoesNotDrainResizeOrAcceptInput(t *testing.T) {
	for _, mode := range []string{"shell", "logcat"} {
		t.Run(mode, func(t *testing.T) {
			// The historical backend returned this typed-nil interface on errors.
			f := &terminalFixture{fixture: sample(), stream: (*pty.Shell)(nil), err: errors.New("adb could not start")}
			u := New(f)
			u.deviceTab, u.termMode = "terminal", mode
			cfg := u.Config()
			s, err := headless.Open(cfg, nil, headless.Options{Scale: 1})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.Close)
			p := u.ensureTerminal(mode)
			if p.stream != nil || !strings.Contains(p.exit, "adb could not start") {
				t.Fatalf("failed stream retained: %+v", p)
			}
			u.drainTerminals()
			if u.terminalKey(ui.KeyEvent{Target: u.id("terminal-pane"), Key: ui.KeyEnter}) {
				t.Fatal("failed terminal accepted input")
			}
			if cfg.TextInput(app.TextInputContext{Terminal: true, ID: u.id("terminal-pane")}) != nil {
				t.Fatal("failed terminal has a text handler")
			}
			if !s.HasText("无法打开终端：adb could not start") {
				t.Fatal("startup error not visible")
			}
			click(t, s, "重新连接")
			if f.opens != 2 {
				t.Fatalf("unexpected reconnect count: %d", f.opens)
			}
			u.closeTerminals()
		})
	}
}

type terminalStream struct {
	input   []byte
	done    chan struct{}
	closed  int
	resizes int
}

func (s *terminalStream) Write(p []byte) bool     { s.input = append(s.input, p...); return true }
func (s *terminalStream) Resize(int, int)         { s.resizes++ }
func (s *terminalStream) Drain(dst []byte) []byte { return dst }
func (s *terminalStream) Done() <-chan struct{}   { return s.done }
func (*terminalStream) Err() error                { return nil }
func (s *terminalStream) Close()                  { s.closed++ }

func TestTerminalTextAndKeysDoNotLeakAcrossTabsOrDevices(t *testing.T) {
	stream := &terminalStream{done: make(chan struct{})}
	f := &terminalFixture{fixture: sample(), stream: stream}
	u := New(f)
	u.deviceTab, u.termMode = "terminal", "shell"
	cfg := u.Config()
	s, err := headless.Open(cfg, nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	t.Cleanup(u.closeTerminals)
	// Route through Viva's real headless input driver, not just direct writes.
	if err := s.Type("hello"); err != nil {
		t.Fatal(err)
	}
	if err := s.Keys("enter"); err != nil {
		t.Fatal(err)
	}
	if string(stream.input) != "hello\r" {
		t.Fatalf("terminal input: %q", stream.input)
	}
	input := cfg.TextInput(app.TextInputContext{Terminal: true, ID: u.id("terminal-pane")})
	if input == nil {
		t.Fatal("live terminal text handler missing")
	}
	oldHandler := input.OnEvent
	before := len(stream.input)
	for _, change := range []func(){
		func() { u.deviceTab = "files" },
		func() { u.deviceTab = "terminal"; u.selected = "another-device" },
		func() { u.selected = f.state.Devices[0].Serial; u.vd.open = true },
	} {
		change()
		if u.terminalKey(ui.KeyEvent{Target: u.id("terminal-pane"), Key: ui.KeyEnter}) {
			t.Fatal("hidden/stale terminal consumed a key")
		}
		_, _ = oldHandler(app.TextEvent{Kind: app.TextInsert, Text: "must-not-send"})
		if len(stream.input) != before {
			t.Fatal("stale text handler sent input")
		}
	}
	u.vd.open = false
	u.selected = f.state.Devices[0].Serial
	if u.terminalKey(ui.KeyEvent{Target: u.id("some-other-field"), Key: ui.KeyEnter}) {
		t.Fatal("terminal intercepted another control")
	}
	close(stream.done)
	u.drainTerminals()
	u.drainTerminals()
	if stream.closed != 1 || u.ensureTerminal("shell").stream != nil {
		t.Fatal("finished terminal not released exactly once")
	}
	_, _ = oldHandler(app.TextEvent{Kind: app.TextPaste, Text: "after-exit"})
	if len(stream.input) != before {
		t.Fatal("ended terminal accepted paste")
	}
}
