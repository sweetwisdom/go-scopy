package desktop

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools/pty"
	"github.com/vivaui/viva/headless"
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

type terminalStream struct {
	input, output []byte
	done          chan struct{}
	closed        int
	drains        int
}

func (s *terminalStream) Write(p []byte) bool { s.input = append(s.input, p...); return true }
func (*terminalStream) Resize(int, int)       {}
func (s *terminalStream) Drain(dst []byte) []byte {
	s.drains++
	dst = append(dst, s.output...)
	s.output = nil
	return dst
}
func (s *terminalStream) Done() <-chan struct{} { return s.done }
func (*terminalStream) Err() error              { return nil }
func (s *terminalStream) Close()                { s.closed++ }

func terminalUI(t *testing.T, stream domain.Terminal, err error) (*UI, *terminalFixture, *[]map[string]any) {
	t.Helper()
	f := &terminalFixture{fixture: sample(), stream: stream, err: err}
	u := New(f)
	u.deviceTab = "terminal"
	u.initTerminalWeb()
	messages := []map[string]any{}
	u.terminalWeb.post = func(raw string) {
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) != nil {
			t.Fatalf("invalid message: %s", raw)
		}
		messages = append(messages, m)
	}
	u.terminalPageMessage(`{"action":"ready"}`)
	t.Cleanup(u.closeTerminals)
	return u, f, &messages
}
func sendTerminal(u *UI, m terminalMessage) {
	data, _ := json.Marshal(m)
	u.terminalPageMessage(string(data))
}
func createTerminal(u *UI, mode string) {
	sendTerminal(u, terminalMessage{ID: 1, Action: "create", Serial: u.selected, Mode: mode})
}

func TestTerminalWebStartupErrorsAreVisibleAndNeverDrained(t *testing.T) {
	for _, mode := range []string{"shell", "logcat"} {
		t.Run(mode, func(t *testing.T) {
			u, f, messages := terminalUI(t, (*pty.Shell)(nil), errors.New("adb could not start"))
			createTerminal(u, mode)
			u.drainTerminals()
			if f.opens != 1 || len(u.terminals) != 0 {
				t.Fatal("failed/typed-nil session was retained")
			}
			last := (*messages)[len(*messages)-1]
			if last["error"] != "adb could not start" {
				t.Fatalf("error not sent to page: %#v", last)
			}
		})
	}
}
func TestTerminalWebInputIsScopedToDeviceAndActivePage(t *testing.T) {
	s := &terminalStream{done: make(chan struct{})}
	u, _, _ := terminalUI(t, s, nil)
	createTerminal(u, "shell")
	p := u.terminals[u.selected+"\x00shell"]
	send := func(text string) {
		sendTerminal(u, terminalMessage{ID: 2, Action: "write", Serial: p.serial, Session: p.id, Data: text})
	}
	send("printf hello\r")
	if string(s.input) != "printf hello\r" {
		t.Fatalf("input: %q", s.input)
	}
	before := len(s.input)
	for _, change := range []func(){
		func() { u.deviceTab = "files" },
		func() { u.deviceTab = "terminal"; u.selected = "another-device" },
		func() { u.selected = p.serial; u.vd.open = true },
		func() { u.vd.open = false; u.closing = true },
		func() { u.closing = false; u.connecting = true },
	} {
		change()
		send("must-not-send")
		if len(s.input) != before {
			t.Fatal("stale/hidden page sent device input")
		}
	}
	u.connecting = false
	sendTerminal(u, terminalMessage{ID: 3, Action: "write", Serial: p.serial, Session: "forged", Data: "no"})
	if len(s.input) != before {
		t.Fatal("forged session accepted")
	}
	close(s.done)
	u.drainTerminals()
	u.drainTerminals()
	if s.closed != 1 || p.stream != nil {
		t.Fatal("ended stream was not released exactly once")
	}
	send("after-exit")
	if len(s.input) != before {
		t.Fatal("ended stream accepted input")
	}
}
func TestTerminalWebBackpressureAndUTF8AreByteSafe(t *testing.T) {
	output := []byte(strings.Repeat("中文", 15000))
	s := &terminalStream{done: make(chan struct{}), output: append([]byte(nil), output...)}
	u, _, messages := terminalUI(t, s, nil)
	createTerminal(u, "shell")
	p := u.terminals[u.selected+"\x00shell"]
	if !p.waiting || s.drains != 1 {
		t.Fatal("first chunk was not limited")
	}
	u.drainTerminals()
	if s.drains != 1 {
		t.Fatal("drained while browser still busy")
	}
	var received []byte
	for p.waiting {
		last := (*messages)[len(*messages)-1]
		if last["type"] != "data" {
			t.Fatalf("missing output: %#v", last)
		}
		raw, err := base64.StdEncoding.DecodeString(last["data"].(string))
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > 32<<10 {
			t.Fatal("unbounded bridge chunk")
		}
		received = append(received, raw...)
		sendTerminal(u, terminalMessage{Action: "ack", Session: p.id})
	}
	if string(received) != string(output) {
		t.Fatal("UTF-8 changed across chunks")
	}
}
func TestTerminalWebReadOnlyLogcatReloadAndOffline(t *testing.T) {
	s := &terminalStream{done: make(chan struct{})}
	u, _, _ := terminalUI(t, s, nil)
	createTerminal(u, "logcat")
	p := u.terminals[u.selected+"\x00logcat"]
	sendTerminal(u, terminalMessage{ID: 2, Action: "write", Serial: u.selected, Session: p.id, Data: "no"})
	if len(s.input) != 0 {
		t.Fatal("Logcat accepted shell input")
	}
	u.terminalPageMessage(`{"action":"ready"}`)
	if s.closed != 1 || len(u.terminals) != 0 {
		t.Fatal("reload retained the old stream")
	}
	createTerminal(u, "shell")
	p = u.terminals[u.selected+"\x00shell"]
	u.state.Devices[0].State = "offline"
	u.drainTerminals()
	if p.stream != nil || !p.ended {
		t.Fatal("offline stream retained")
	}
}
func TestTerminalWebHeadlessNeverStartsDeviceProcesses(t *testing.T) {
	t.Setenv("VIVA_HEADLESS", "1")
	f := &terminalFixture{fixture: sample()}
	u := New(f)
	u.deviceTab = "terminal"
	session, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer u.closeTerminals()
	if f.opens != 0 || !session.HasText("设备终端 · WebView") {
		t.Fatal("headless invoked a device process or failed to show fallback")
	}
}

func TestTerminalWebOfflineClosesWithoutWaitingForBrowserACK(t *testing.T) {
	stream := &terminalStream{done: make(chan struct{}), output: []byte("pending log output")}
	u, _, _ := terminalUI(t, stream, nil)
	createTerminal(u, "logcat")
	panel := u.terminals[u.selected+"\x00logcat"]
	if !panel.waiting {
		t.Fatal("expected in-flight browser output")
	}
	u.state.Devices[0].State = "offline"
	u.drainTerminals()
	if panel.stream != nil || stream.closed != 1 {
		t.Fatal("offline cleanup waited for a browser ACK")
	}
}
func TestTerminalWebProgrammaticCloseEndsSessions(t *testing.T) {
	stream := &terminalStream{done: make(chan struct{})}
	u, _, _ := terminalUI(t, stream, nil)
	createTerminal(u, "shell")
	config := u.Config()
	config.OnClosed()
	config.OnClosed()
	if stream.closed != 1 || len(u.terminals) != 0 || !u.terminalWeb.closed {
		t.Fatal("programmatic close leaked a session")
	}
}
