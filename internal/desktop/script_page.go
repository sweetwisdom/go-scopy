package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/scriptstore"
	"github.com/go-scopy/go-scopy/internal/scriptview"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
)

type scriptMessage struct {
	ID      uint64 `json:"id"`
	Action  string `json:"action"`
	Path    string `json:"path"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Version string `json:"version"`
	Serial  string `json:"serial"`
	Python  string `json:"python"`
	RunID   string `json:"runId"`
	Timeout int    `json:"timeout"`
	Trusted bool   `json:"trusted"`
	Dirty   bool   `json:"dirty"`
}
type scriptWebState struct {
	page                                     *app.WebView
	requested, ready, closed, slotSet, dirty bool
	slot                                     flex.Box
	err, contextSent                         string
	deadline                                 time.Time
	timer                                    *time.Timer
	pollTimer                                *time.Timer
	pollDeadline                             time.Time
	dialog                                   *scriptMessage
	post                                     func(string)
	openPath                                 func(string) error
}

func (u *UI) scriptActive() bool {
	return u.section == "scripts" && !u.connecting && !u.closing && !u.scriptWeb.closed
}
func (u *UI) scriptWorkspace(f *flex.Frame) {
	w := &u.scriptWeb
	if os.Getenv("VIVA_HEADLESS") == "1" {
		f.Text("Python 脚本工作台 · CodeMirror 6").FontSize(18)
		u.small(f, "本地 .py 文件 / Python 补全 / 选择在线设备执行。无头模式不启动脚本。")
		return
	}
	if w.err != "" {
		f.Text("脚本工作台暂不可用").FontSize(16)
		u.small(f, w.err)
		if f.Button(u.id("script-web-retry"), "重试 WebView").Clicked() {
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
		f.Text("正在打开脚本工作台…").Color(u.c.muted)
		return
	}
	w.slot = f.Slot().FillWidth().Grow(1)
	w.slotSet = true
	u.sendScriptContext()
}
func (u *UI) scriptPageWake() {
	w := &u.scriptWeb
	if w.closed {
		return
	}
	if w.requested && u.fileWeb.hostReady && w.page == nil && w.err == "" {
		page, err := u.fileWeb.host.Open(app.WebViewConfig{URL: "goscscripts://workspace/index.html", Scheme: &app.WebScheme{Name: "goscscripts", Serve: scriptview.Serve}, OnMessage: u.scriptPageMessage})
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
	if w.dialog != nil {
		m := *w.dialog
		w.dialog = nil
		if !u.scriptActive() {
			u.scriptReply(m, nil, errors.New("页面已切换"))
			return
		}
		filter := app.FileFilter{Name: "Python 脚本", Pattern: "*.py"}
		if m.Action == "python" {
			filter = app.FileFilter{Name: "Python 解释器", Pattern: "*"}
		}
		paths, err := app.OpenFiles(app.OpenFileOptions{Title: "选择" + filter.Name, Filters: []app.FileFilter{filter}})
		var data any
		if err == nil && len(paths) > 0 {
			if m.Action == "python" {
				data = paths[0]
			} else if b, ok := u.backend.(scriptview.FileBackend); ok {
				data, err = b.OpenScriptFile(paths[0])
			} else {
				err = errors.New("后端不支持本地脚本")
			}
		}
		u.scriptReply(m, data, err)
	}
	u.sendScriptContext()
}
func (u *UI) postScript(value any) {
	w := &u.scriptWeb
	if !w.ready || w.closed {
		return
	}
	b, err := json.Marshal(value)
	if err != nil {
		return
	}
	if w.post != nil {
		w.post(string(b))
	} else if w.page != nil {
		w.page.PostMessage(string(b))
	}
}
func (u *UI) scriptReply(m scriptMessage, data any, err error) {
	text := ""
	if err != nil {
		text = err.Error()
	}
	u.postScript(map[string]any{"type": "response", "id": m.ID, "data": data, "error": text})
}
func (u *UI) sendScriptContext() {
	w := &u.scriptWeb
	if !w.ready || w.closed {
		return
	}
	devices := []map[string]string{}
	for _, d := range u.state.Devices {
		if d.State == "device" {
			devices = append(devices, map[string]string{"serial": d.Serial, "name": d.Name()})
		}
	}
	state := domain.ScriptState{}
	if b, ok := u.backend.(domain.ScriptBackend); ok {
		state = b.ScriptState()
	} else {
		state.Error = "后端未启用脚本运行器"
	}
	// The existing runner coalesces notifications. Poll only visible active runs
	// so a final short burst before a long sleep still reaches the output pane.
	running := false
	for _, r := range state.Runs {
		if r.Active() {
			running = true
			break
		}
	}
	if running && u.scriptActive() {
		if w.pollTimer == nil || !time.Now().Before(w.pollDeadline) {
			if w.pollTimer != nil {
				w.pollTimer.Stop()
			}
			w.pollDeadline = time.Now().Add(150 * time.Millisecond)
			w.pollTimer = time.AfterFunc(150*time.Millisecond, u.wake.Invalidate)
		}
	} else if w.pollTimer != nil {
		w.pollTimer.Stop()
		w.pollTimer = nil
	}
	runs := []any{}
	for _, r := range state.Runs {
		runs = append(runs, map[string]any{"id": r.ID, "request": r.Request, "status": r.Status, "started": r.Started, "finished": r.Finished, "exitCode": r.ExitCode, "error": r.Error, "outputDir": r.OutputDir, "log": r.Log})
	}
	data, _ := json.Marshal(map[string]any{"type": "context", "theme": u.state.Settings.Theme, "devices": devices, "selected": u.selected, "active": u.scriptActive(), "runs": runs, "last": state.Last, "error": state.Error})
	if string(data) != w.contextSent {
		w.contextSent = string(data)
		u.postScript(json.RawMessage(data))
	}
}
func (u *UI) scriptPageMessage(raw string) {
	w := &u.scriptWeb
	// JSON escaping may expand a bounded 1 MiB document up to sixfold.
	if w.closed || !w.requested || len(raw) > 7*scriptstore.MaxSize {
		return
	}
	var m scriptMessage
	if json.Unmarshal([]byte(raw), &m) != nil {
		return
	}
	if m.Action == "ready" {
		w.ready = true
		w.contextSent = ""
		if w.timer != nil {
			w.timer.Stop()
			w.timer = nil
		}
		u.sendScriptContext()
		return
	}
	if !w.ready {
		return
	}
	if m.Action == "dirty" {
		w.dirty = m.Dirty
		return
	}
	if !u.scriptActive() || m.ID == 0 {
		u.scriptReply(m, nil, errors.New("脚本页面已关闭或切换"))
		return
	}
	files, ok := u.backend.(scriptview.FileBackend)
	if !ok {
		u.scriptReply(m, nil, errors.New("后端不支持本地脚本"))
		return
	}
	var data any
	var err error
	switch m.Action {
	case "list":
		data, err = files.ScriptFiles()
	case "read":
		data, err = files.ReadScriptFile(m.Path)
	case "new":
		data, err = files.NewScriptFile(m.Name, m.Content)
	case "save":
		data, err = files.SaveScriptFile(m.Path, m.Content, m.Version)
	case "open", "python":
		if w.dialog != nil {
			err = errors.New("文件选择器已打开")
		} else {
			w.dialog = &m
			u.wake.Invalidate()
			return
		}
	case "guide":
		var path string
		path, err = files.ScriptGuide()
		if err == nil {
			open := app.OpenPath
			if w.openPath != nil {
				open = w.openPath
			}
			err = open(path)
			if err != nil {
				err = fmt.Errorf("无法打开开发指南，请设置系统 .md 默认打开应用（%s）：%w", path, err)
			}
		}
	case "folder":
		var list scriptstore.Listing
		list, err = files.ScriptFiles()
		if err == nil {
			err = app.OpenPath(list.Directory)
		}
	case "run":
		runner, supported := u.backend.(domain.ScriptBackend)
		if !supported {
			err = errors.New("后端未启用脚本运行器")
			break
		}
		if !m.Trusted {
			err = errors.New("请先确认信任此脚本；脚本以本机用户权限运行，并非沙箱")
			break
		}
		if !u.terminalOnline(m.Serial) {
			err = errors.New("请选择当前已连接并授权的设备")
			break
		}
		var doc scriptstore.Document
		doc, err = files.ReadScriptFile(m.Path)
		if err == nil && doc.Version != m.Version {
			err = errors.New("脚本已发生变化，请保存后重新执行")
		}
		if err == nil {
			data, err = runner.RunScript(domain.ScriptRequest{Path: doc.Path, Serial: m.Serial, Python: m.Python, TimeoutSeconds: m.Timeout})
		}
	case "stop", "output":
		runner, supported := u.backend.(domain.ScriptBackend)
		if !supported {
			err = errors.New("后端未启用脚本运行器")
			break
		}
		found := false
		for _, r := range runner.ScriptState().Runs {
			if r.ID == m.RunID {
				found = true
				if m.Action == "stop" {
					runner.StopScript(r.ID)
				} else {
					err = app.OpenPath(r.OutputDir)
				}
				break
			}
		}
		if !found {
			err = errors.New("执行记录不存在")
		}
	default:
		err = errors.New("不支持的脚本操作")
	}
	u.scriptReply(m, data, err)
	u.sendScriptContext()
}
func (u *UI) closeScriptWeb() {
	w := &u.scriptWeb
	if w.closed {
		return
	}
	w.closed = true
	if w.pollTimer != nil {
		w.pollTimer.Stop()
		w.pollTimer = nil
	}
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	if w.page != nil {
		w.page.Close()
		w.page = nil
	}
}
