package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/terminalview"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
)

// Sessions outlive tab switches; xterm.js, not the native painter, owns the VT
// screen and keyboard. At most one bounded output chunk per session is in
// flight until xterm acknowledges that it has consumed it (including Logcat).
type terminalPanel struct {
	id, serial, mode string
	stream           domain.Terminal
	buffer           []byte
	waiting, ended   bool
}

type terminalMessage struct {
	ID      uint64 `json:"id"`
	Action  string `json:"action"`
	Serial  string `json:"serial"`
	Mode    string `json:"mode"`
	Session string `json:"session"`
	Data    string `json:"data"`
	Kind    string `json:"kind"`
}
type terminalResult struct {
	message terminalMessage
	values  []string
	err     error
}
type terminalWebState struct {
	page                              *app.WebView
	requested, ready, closed, slotSet bool
	slot                              flex.Box
	err, contextSent                  string
	next                              uint64
	ctx                               context.Context
	cancel                            context.CancelFunc
	results                           chan terminalResult
	timer                             *time.Timer
	deadline                          time.Time
	// Test sink exercises the exact wire protocol without starting a WebView.
	post func(string)
}

func (u *UI) initTerminalWeb() {
	w := &u.terminalWeb
	if w.ctx != nil {
		return
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.results = make(chan terminalResult, 8)
	u.terminals = map[string]*terminalPanel{}
}

func (u *UI) terminalActive() bool {
	return u.section == "devices" && u.deviceTab == "terminal" && !u.connecting && !u.closing && !u.vd.open && !u.terminalWeb.closed
}
func (u *UI) terminalOnline(serial string) bool {
	for _, d := range u.state.Devices {
		if d.Serial == serial {
			return d.State == "device"
		}
	}
	return false
}

func (u *UI) deviceTerminalPage(f *flex.Frame, d domain.Device) {
	w := &u.terminalWeb
	if os.Getenv("VIVA_HEADLESS") == "1" {
		f.Text("设备终端 · WebView").FontSize(16)
		u.small(f, "xterm.js 交互终端 / CodeMirror 命令补全 / 常用 ADB 命令。无头模式不启动设备会话。")
		return
	}
	if w.err != "" {
		f.Text("设备终端暂不可用").FontSize(16)
		u.small(f, w.err)
		u.small(f, "请检查系统 WebView 运行时；不会回退到原生终端或关闭应用。")
		if f.Button(u.id("terminal-web-retry"), "重试 WebView").Outline().Clicked() {
			w.err = ""
			w.requested = true
			u.wake.Invalidate()
		}
		return
	}
	if !w.requested {
		w.requested = true
		u.wake.Invalidate()
	}
	if w.page == nil {
		f.Text("正在打开设备终端…").Color(u.c.muted)
		return
	}
	w.slot = f.Slot().FillWidth().Grow(1)
	w.slotSet = true
	u.sendTerminalContext()
}

func (u *UI) terminalPageWake() {
	w := &u.terminalWeb
	if w.closed || w.ctx == nil {
		return
	}
	if w.requested && u.fileWeb.hostReady && w.page == nil && w.err == "" {
		page, err := u.fileWeb.host.Open(app.WebViewConfig{
			URL:       "goscterm://workspace/index.html",
			Scheme:    &app.WebScheme{Name: "goscterm", Serve: terminalview.Serve},
			OnMessage: u.terminalPageMessage,
		})
		if err != nil {
			w.err = err.Error()
		} else {
			w.page = page
			w.deadline = time.Now().Add(12 * time.Second)
			w.timer = time.AfterFunc(12*time.Second, u.wake.Invalidate)
		}
	}
	if w.page != nil && !w.ready && !w.deadline.IsZero() && time.Now().After(w.deadline) {
		w.page.Close()
		w.page = nil
		w.err = "Web 页面加载超时，请检查系统 WebView 运行时"
	}
	for {
		select {
		case result := <-w.results:
			if result.message.Serial != u.selected || !u.terminalActive() {
				result.err = errors.New("设备或页面已切换")
				result.values = nil
			}
			u.terminalReply(result.message, result.values, result.err)
		default:
			u.sendTerminalContext()
			u.drainTerminals()
			return
		}
	}
}

func (u *UI) postTerminal(value any) {
	w := &u.terminalWeb
	if !w.ready || w.closed {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	if w.post != nil {
		w.post(string(data))
	} else if w.page != nil {
		w.page.PostMessage(string(data))
	}
}
func (u *UI) terminalReply(m terminalMessage, data any, err error) {
	text := ""
	if err != nil {
		text = err.Error()
	}
	u.postTerminal(map[string]any{"type": "response", "id": m.ID, "data": data, "error": text})
}
func (u *UI) sendTerminalContext() {
	w := &u.terminalWeb
	if !w.ready {
		return
	}
	name := u.selected
	for _, d := range u.state.Devices {
		if d.Serial == u.selected {
			name = d.Name()
			break
		}
	}
	data, _ := json.Marshal(map[string]any{"type": "context", "serial": u.selected, "name": name, "online": u.terminalOnline(u.selected), "active": u.terminalActive(), "theme": u.state.Settings.Theme, "platform": runtime.GOOS})
	if string(data) != w.contextSent {
		w.contextSent = string(data)
		u.postTerminal(json.RawMessage(data))
	}
}

func (u *UI) terminalPageMessage(raw string) {
	w := &u.terminalWeb
	if len(raw) > 96<<10 || w.closed || w.ctx == nil {
		return
	}
	var m terminalMessage
	if json.Unmarshal([]byte(raw), &m) != nil {
		return
	}
	if m.Action == "ready" {
		// A navigation/reload must not leave old processes or unacknowledged data.
		for key, p := range u.terminals {
			u.endTerminal(p)
			delete(u.terminals, key)
		}
		w.ready = true
		w.contextSent = ""
		if w.timer != nil {
			w.timer.Stop()
			w.timer = nil
		}
		u.sendTerminalContext()
		return
	}
	if !w.ready {
		return
	}
	p := u.findTerminal(m.Session)
	// ACK and teardown remain valid for hidden sessions, but never send input.
	if m.Action == "ack" && p != nil {
		p.waiting = false
		u.drainTerminals()
		return
	}
	if m.Action == "destroy" && p != nil {
		u.endTerminal(p)
		delete(u.terminals, p.serial+"\x00"+p.mode)
		u.terminalReply(m, nil, nil)
		return
	}
	if !u.terminalActive() || m.Serial == "" || m.Serial != u.selected || !u.terminalOnline(m.Serial) {
		u.terminalReply(m, nil, errors.New("设备已离线或页面已切换"))
		return
	}
	switch m.Action {
	case "create":
		if m.Mode != "shell" && m.Mode != "logcat" {
			u.terminalReply(m, nil, errors.New("无效的终端类型"))
			return
		}
		key := m.Serial + "\x00" + m.Mode
		if existing := u.terminals[key]; existing != nil {
			u.terminalReply(m, map[string]string{"sessionId": existing.id}, nil)
			return
		}
		open := u.backend.OpenShell
		if m.Mode == "logcat" {
			open = u.backend.OpenLogcat
		}
		stream, err := open(m.Serial, 80, 24, u.wake.Invalidate)
		if err != nil {
			u.terminalReply(m, nil, err)
			return
		}
		if stream == nil {
			u.terminalReply(m, nil, errors.New("后端未返回会话"))
			return
		}
		w.next++
		panel := &terminalPanel{id: fmt.Sprintf("terminal-%d", w.next), serial: m.Serial, mode: m.Mode, stream: stream}
		u.terminals[key] = panel
		u.terminalReply(m, map[string]string{"sessionId": panel.id}, nil)
		u.drainTerminals()
	case "write":
		if p == nil || p.serial != m.Serial || p.stream == nil || p.ended {
			u.terminalReply(m, nil, errors.New("会话已结束，请重新连接"))
			return
		}
		if p.mode != "shell" {
			u.terminalReply(m, nil, errors.New("Logcat 仅支持查看"))
			return
		}
		if len(m.Data) > 64<<10 || !p.stream.Write([]byte(m.Data)) {
			u.terminalReply(m, nil, errors.New("终端输入队列已满或会话已结束"))
			return
		}
		if m.ID != 0 {
			u.terminalReply(m, nil, nil)
		}
	case "complete":
		backend, ok := u.backend.(terminalview.CompletionBackend)
		if !ok {
			u.terminalReply(m, []string{}, nil)
			return
		}
		if m.Kind != "packages" && m.Kind != "properties" && m.Kind != "services" {
			u.terminalReply(m, nil, errors.New("无效的补全类型"))
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(w.ctx, 4*time.Second)
			defer cancel()
			values, err := backend.TerminalCandidates(ctx, m.Serial, m.Kind)
			select {
			case w.results <- terminalResult{m, values, err}:
				u.wake.Invalidate()
			case <-w.ctx.Done():
			}
		}()
	default:
		u.terminalReply(m, nil, errors.New("不支持的终端操作"))
	}
}

func (u *UI) findTerminal(id string) *terminalPanel {
	if id == "" {
		return nil
	}
	for _, p := range u.terminals {
		if p.id == id {
			return p
		}
	}
	return nil
}
func (u *UI) endTerminal(p *terminalPanel) {
	if p.stream != nil {
		p.stream.Close()
		p.stream = nil
	}
	p.buffer = nil
	p.waiting = false
	p.ended = true
}
func (u *UI) drainTerminals() {
	for _, p := range u.terminals {
		if p.stream == nil {
			continue
		}
		if !u.terminalOnline(p.serial) {
			u.endTerminal(p)
			u.postTerminal(map[string]any{"type": "exit", "session": p.id, "error": "设备已离线", "code": -1})
			continue
		}
		if p.waiting {
			continue
		}
		if len(p.buffer) == 0 {
			p.buffer = p.stream.Drain(p.buffer[:0])
		}
		if len(p.buffer) > 0 {
			n := min(len(p.buffer), 32<<10)
			u.postTerminal(map[string]any{"type": "data", "session": p.id, "data": p.buffer[:n]})
			p.buffer = p.buffer[n:]
			p.waiting = true
			continue
		}
		select {
		case <-p.stream.Done():
			text := ""
			code := 0
			if err := p.stream.Err(); err != nil {
				text = err.Error()
				code = 1
			}
			u.endTerminal(p)
			u.postTerminal(map[string]any{"type": "exit", "session": p.id, "error": text, "code": code})
		default:
		}
	}
}
func (u *UI) closeTerminals() {
	w := &u.terminalWeb
	if w.closed {
		return
	}
	w.closed = true
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	if w.cancel != nil {
		w.cancel()
	}
	for _, p := range u.terminals {
		u.endTerminal(p)
	}
	u.terminals = nil
	if w.page != nil {
		w.page.Close()
		w.page = nil
	}
}
