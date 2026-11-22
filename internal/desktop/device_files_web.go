package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/fileview"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
)

type fileMessage struct {
	ID        uint64   `json:"id"`
	Action    string   `json:"action"`
	Serial    string   `json:"serial"`
	Path      string   `json:"path"`
	Name      string   `json:"name"`
	Target    string   `json:"target"`
	Names     []string `json:"names"`
	Confirmed bool     `json:"confirmed"`
}

type fileResult struct {
	message    fileMessage
	generation uint64
	entries    []domain.FileEntry
	spec       fileview.Spec
	data       []byte
	err        error
}

type fileTask struct {
	serial, dir string
	refresh     bool
	status      string
}

type fileWebState struct {
	host                                                 app.WebHost
	page                                                 *app.WebView
	resources                                            fileview.Resources
	hostReady, requested, ready, slotSet, closed, listed bool
	err, serial, dir, contextSent                        string
	slot                                                 flex.Box
	entries                                              []domain.FileEntry
	ctx                                                  context.Context
	cancel, cancelList, cancelPreview                    context.CancelFunc
	results                                              chan fileResult
	browseVersion, previewVersion                        uint64
	dialog                                               *fileMessage
	tasks                                                map[uint64]fileTask
	loadTimer                                            *time.Timer
	loadDeadline                                         time.Time
}

func (u *UI) initFileWeb() {
	if u.fileWeb.ctx != nil {
		return
	}
	w := &u.fileWeb
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.results = make(chan fileResult, 8)
	w.tasks = map[uint64]fileTask{}
	w.host.OnReady = func() { w.hostReady = true; u.wake.Invalidate() }
}

func (u *UI) deviceFiles(f *flex.Frame, d domain.Device) {
	w := &u.fileWeb
	_, supported := u.backend.(fileview.Backend)
	if !supported || w.err != "" || os.Getenv("VIVA_HEADLESS") == "1" {
		if w.err != "" {
			u.small(f, "Web 文件工作台不可用，已回退原生视图："+w.err)
		}
		u.deviceFilesNative(f, d)
		return
	}
	if !w.requested {
		w.requested = true
		u.wake.Invalidate()
	}
	if w.page == nil {
		f.Column().FillWidth().Height(240).Center().Children(func() { f.Text("正在打开文件工作台…").Color(u.c.muted) })
		return
	}
	w.slot = f.Slot().FillWidth().Grow(1)
	w.slotSet = true
	// Selection can change during the native frame; deliver only on the UI thread.
	u.sendFileContext()
}

func (u *UI) filePageWake() {
	w := &u.fileWeb
	if w.closed || w.ctx == nil {
		return
	}
	if w.requested && w.hostReady && w.page == nil && w.err == "" {
		page, err := w.host.Open(app.WebViewConfig{
			URL:       "goscfiles://workspace/index.html",
			Scheme:    &app.WebScheme{Name: "goscfiles", Serve: w.resources.Serve},
			OnMessage: u.filePageMessage,
		})
		if err != nil {
			w.err = err.Error()
		} else {
			w.page = page
			w.loadDeadline = time.Now().Add(12 * time.Second)
			w.loadTimer = time.AfterFunc(12*time.Second, u.wake.Invalidate)
		}
	}
	if w.page != nil && !w.ready && !w.loadDeadline.IsZero() && time.Now().After(w.loadDeadline) {
		w.page.Close()
		w.page = nil
		w.err = "Web 页面加载超时，请检查系统 WebView 运行时"
		w.resources.Clear()
	}
	for {
		select {
		case result := <-w.results:
			u.filePageResult(result)
		default:
			u.sendFileContext()
			u.sendFileTasks()
			return
		}
	}
}

func (u *UI) postFileMessage(value any) {
	if u.fileWeb.page == nil || !u.fileWeb.ready || u.fileWeb.closed {
		return
	}
	data, err := json.Marshal(value)
	if err == nil {
		u.fileWeb.page.PostMessage(string(data))
	}
}

func (u *UI) fileReply(m fileMessage, data any, err error) {
	text := ""
	if err != nil {
		text = err.Error()
	}
	u.postFileMessage(struct {
		Type  string `json:"type"`
		ID    uint64 `json:"id"`
		Data  any    `json:"data,omitempty"`
		Error string `json:"error,omitempty"`
	}{"response", m.ID, data, text})
}

func (u *UI) sendFileContext() {
	w := &u.fileWeb
	if !w.ready {
		return
	}
	online, name := false, u.selected
	for _, d := range u.state.Devices {
		if d.Serial == u.selected {
			online, name = d.State == "device", d.Name()
			break
		}
	}
	payload := struct {
		Type, Serial, Name, Theme string
		Online                    bool
	}{"context", u.selected, name, u.state.Settings.Theme, online}
	// Keep the wire keys explicit; the snapshot is independent of backend maps.
	data, _ := json.Marshal(map[string]any{"type": payload.Type, "serial": payload.Serial, "name": payload.Name, "theme": payload.Theme, "online": payload.Online, "active": u.section == "devices" && u.deviceTab == "files" && !u.connecting && !u.closing})
	if string(data) == w.contextSent {
		return
	}
	if w.serial != u.selected || !online || u.section != "devices" || u.deviceTab != "files" {
		u.cancelFileReads()
		w.entries = nil
		if w.serial != u.selected {
			w.dir = "/sdcard"
			w.serial = u.selected
		}
	}
	w.contextSent = string(data)
	u.postFileMessage(json.RawMessage(data))
}

func (u *UI) cancelFileReads() {
	w := &u.fileWeb
	if w.cancelList != nil {
		w.cancelList()
		w.cancelList = nil
	}
	if w.cancelPreview != nil {
		w.cancelPreview()
		w.cancelPreview = nil
	}
	w.listed = false
	w.browseVersion++
	w.previewVersion++
	w.resources.Clear()
}

func (u *UI) filePageMessage(raw string) {
	if len(raw) > 64<<10 || u.fileWeb.closed {
		return
	}
	var m fileMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return
	}
	w := &u.fileWeb
	if m.Action == "ready" {
		w.ready = true
		if w.loadTimer != nil {
			w.loadTimer.Stop()
			w.loadTimer = nil
		}
		w.contextSent = ""
		u.sendFileContext()
		return
	}
	if m.ID == 0 || m.Serial == "" || m.Serial != u.selected || u.section != "devices" || u.deviceTab != "files" || u.connecting || u.closing {
		u.fileReply(m, nil, errors.New("设备或页面已切换，请重新打开文件管理"))
		return
	}
	if m.Action == "cancelPreview" {
		if w.cancelPreview != nil {
			w.cancelPreview()
			w.cancelPreview = nil
		}
		w.previewVersion++
		w.resources.Clear()
		u.fileReply(m, map[string]bool{"cancelled": true}, nil)
		return
	}
	backend, ok := u.backend.(fileview.Backend)
	if !ok {
		u.fileReply(m, nil, errors.New("后端不支持文件工作台"))
		return
	}
	online := false
	for _, d := range u.state.Devices {
		if d.Serial == m.Serial && d.State == "device" {
			online = true
			break
		}
	}
	if !online {
		u.fileReply(m, nil, errors.New("设备未连接或尚未授权"))
		return
	}
	dir, err := fileview.CleanPath(m.Path)
	if err != nil {
		u.fileReply(m, nil, err)
		return
	}
	if m.Action == "list" {
		u.cancelFileReads()
		w.dir = dir
		w.entries = nil
		m.Path = dir
		ctx, cancel := context.WithCancel(w.ctx)
		w.cancelList = cancel
		generation := w.browseVersion
		u.fileWorker(ctx, func() fileResult {
			entries, err := backend.BrowseFiles(ctx, m.Serial, dir)
			return fileResult{message: m, generation: generation, entries: entries, err: err}
		})
		return
	}
	if dir != w.dir || !w.listed {
		u.fileReply(m, nil, errors.New("目录已切换，请刷新后重试"))
		return
	}
	find := func(name string) (domain.FileEntry, bool) {
		if !fileview.ValidName(name) {
			return domain.FileEntry{}, false
		}
		for _, entry := range w.entries {
			if entry.Name == name {
				return entry, true
			}
		}
		return domain.FileEntry{}, false
	}
	switch m.Action {
	case "preview":
		entry, found := find(m.Name)
		if !found || entry.IsDir {
			u.fileReply(m, nil, errors.New("请选择当前目录中的文件"))
			return
		}
		spec, err := fileview.PreviewSpec(entry.Name)
		if err != nil {
			u.fileReply(m, nil, err)
			return
		}
		if entry.Size > spec.Limit {
			u.fileReply(m, nil, fmt.Errorf("文件超过预览上限（%d MB），请下载后打开", spec.Limit>>20))
			return
		}
		if w.cancelPreview != nil {
			w.cancelPreview()
		}
		w.previewVersion++
		w.resources.Clear()
		ctx, cancel := context.WithCancel(w.ctx)
		w.cancelPreview = cancel
		generation := w.previewVersion
		u.fileWorker(ctx, func() fileResult {
			data, err := backend.ReadFilePreview(ctx, m.Serial, path.Join(dir, entry.Name), spec.Limit)
			return fileResult{message: m, generation: generation, spec: spec, data: data, err: err}
		})
	case "mkdir":
		if !fileview.ValidName(m.Name) {
			u.fileReply(m, nil, errors.New("文件夹名称无效"))
			return
		}
		if _, found := find(m.Name); found {
			u.fileReply(m, nil, errors.New("当前目录已有同名项目"))
			return
		}
		u.enqueueFileTask(m, fileview.Operation{Action: "mkdir", Path: path.Join(dir, m.Name)}, true)
	case "rename":
		if _, found := find(m.Name); !found || !fileview.ValidName(m.Target) {
			u.fileReply(m, nil, errors.New("请选择当前目录中的项目并输入有效名称"))
			return
		}
		if _, found := find(m.Target); found {
			u.fileReply(m, nil, errors.New("当前目录已有同名项目"))
			return
		}
		u.enqueueFileTask(m, fileview.Operation{Action: "rename", Path: path.Join(dir, m.Name), Target: path.Join(dir, m.Target)}, true)
	case "delete", "download":
		if len(m.Names) == 0 || len(m.Names) > 200 {
			u.fileReply(m, nil, errors.New("请一次选择 1–200 个项目"))
			return
		}
		for _, name := range m.Names {
			if _, found := find(name); !found {
				u.fileReply(m, nil, errors.New("选择中包含无效项目，请刷新后重试"))
				return
			}
		}
		if m.Action == "delete" {
			if !m.Confirmed {
				u.fileReply(m, nil, errors.New("删除操作需要确认"))
				return
			}
			for _, name := range m.Names {
				entry, _ := find(name)
				u.enqueueFileTaskNoReply(m.Serial, dir, fileview.Operation{Action: "delete", Path: path.Join(dir, name), Recursive: entry.IsDir}, true)
			}
			u.fileReply(m, map[string]bool{"queued": true}, nil)
		} else {
			u.queueFileDialog(m)
		}
	case "upload", "uploadFolder":
		if !m.Confirmed {
			u.fileReply(m, nil, errors.New("上传前请确认目标目录及同名文件覆盖提示"))
			return
		}
		u.queueFileDialog(m)
	default:
		u.fileReply(m, nil, errors.New("不支持的文件操作"))
	}
}

func (u *UI) fileWorker(ctx context.Context, work func() fileResult) {
	results, lifetime, wake := u.fileWeb.results, u.fileWeb.ctx, u.wake.Invalidate
	go func() {
		result := work()
		if ctx.Err() != nil {
			result.data = nil
			result.entries = nil
			result.err = ctx.Err()
		}
		if lifetime.Err() != nil {
			return
		}
		// A cancelled request still receives a response unless the window closed.
		select {
		case results <- result:
			wake()
		case <-lifetime.Done():
		}
	}()
}

func (u *UI) filePageResult(result fileResult) {
	w := &u.fileWeb
	m := result.message
	if m.Serial != w.serial || m.Path != w.dir || (m.Action == "list" && result.generation != w.browseVersion) || (m.Action == "preview" && result.generation != w.previewVersion) {
		u.fileReply(m, nil, errors.New("请求已取消"))
		return
	}
	if result.err != nil {
		u.fileReply(m, nil, result.err)
		return
	}
	if m.Action == "list" {
		w.entries = result.entries
		w.listed = true
		u.fileReply(m, map[string]any{"path": w.dir, "entries": result.entries}, nil)
	} else {
		url, err := w.resources.Put(result.spec, result.data)
		u.fileReply(m, map[string]any{"url": url, "kind": result.spec.Kind, "mime": result.spec.MIME}, err)
	}
}

func (u *UI) enqueueFileTask(m fileMessage, op fileview.Operation, refresh bool) {
	id := u.enqueueFileTaskNoReply(m.Serial, m.Path, op, refresh)
	if id == 0 {
		u.fileReply(m, nil, errors.New("应用正在关闭，未启动文件操作"))
		return
	}
	u.fileReply(m, map[string]any{"taskID": id, "queued": true}, nil)
}

func (u *UI) enqueueFileTaskNoReply(serial, dir string, op fileview.Operation, refresh bool) uint64 {
	backend, ok := u.backend.(fileview.Backend)
	if !ok {
		return 0
	}
	id := backend.FileTask(serial, op)
	if id != 0 {
		u.fileWeb.tasks[id] = fileTask{serial: serial, dir: dir, refresh: refresh}
		u.wake.Invalidate()
	}
	return id
}

func (u *UI) sendFileTasks() {
	w := &u.fileWeb
	for _, task := range u.state.Tasks {
		tracked, ok := w.tasks[task.ID]
		if !ok || tracked.status == task.Status {
			continue
		}
		u.postFileMessage(map[string]any{"type": "task", "serial": tracked.serial, "path": tracked.dir, "refresh": tracked.refresh, "task": task})
		if task.Status != "running" {
			delete(w.tasks, task.ID)
		} else {
			tracked.status = task.Status
			w.tasks[task.ID] = tracked
		}
	}
}

func (u *UI) queueFileDialog(m fileMessage) {
	if u.fileWeb.dialog != nil || u.pendingDialog != "" {
		u.fileReply(m, nil, errors.New("已有文件选择窗口，请先完成或取消"))
		return
	}
	copy := m
	u.fileWeb.dialog = &copy
	u.pendingDialog = "file-workspace"
	u.wake.Invalidate()
}

func (u *UI) openFileWebDialog() {
	m := u.fileWeb.dialog
	u.fileWeb.dialog = nil
	if m == nil {
		return
	}
	// Capture the request's device and directory before the OS picker pumps events.
	if m.Serial != u.selected || m.Path != u.fileWeb.dir {
		u.fileReply(*m, nil, errors.New("设备或目录已切换"))
		return
	}
	if m.Action == "upload" || m.Action == "uploadFolder" {
		var paths []string
		var err error
		if m.Action == "uploadFolder" {
			var folder string
			folder, err = app.OpenDirectory(app.OpenDirectoryOptions{Title: "选择上传文件夹 · " + m.Path})
			if folder != "" {
				paths = []string{folder}
			}
		} else {
			paths, err = app.OpenFiles(app.OpenFileOptions{Title: "选择上传文件（可多选） · " + m.Path, AllowMultiple: true})
		}
		if err != nil {
			u.fileReply(*m, nil, err)
			return
		}
		if len(paths) == 0 {
			u.fileReply(*m, map[string]bool{"cancelled": true}, nil)
			return
		}
		if m.Serial != u.selected || m.Path != u.fileWeb.dir || u.fileWeb.closed {
			u.fileReply(*m, nil, errors.New("设备或目录已切换，未上传文件"))
			return
		}
		for _, local := range paths {
			u.enqueueFileTaskNoReply(m.Serial, m.Path, fileview.Operation{Action: "upload", Path: path.Join(m.Path, filepath.Base(local)), Target: local}, true)
		}
	} else {
		dir, err := app.OpenDirectory(app.OpenDirectoryOptions{Title: "选择文件下载目录"})
		if err != nil {
			u.fileReply(*m, nil, err)
			return
		}
		if dir == "" {
			u.fileReply(*m, map[string]bool{"cancelled": true}, nil)
			return
		}
		if m.Serial != u.selected || m.Path != u.fileWeb.dir || u.fileWeb.closed {
			u.fileReply(*m, nil, errors.New("设备或目录已切换，未下载文件"))
			return
		}
		for _, name := range m.Names {
			u.enqueueFileTaskNoReply(m.Serial, m.Path, fileview.Operation{Action: "download", Path: path.Join(m.Path, name), Target: dir}, false)
		}
	}
	u.fileReply(*m, map[string]bool{"queued": true}, nil)
}

func (u *UI) closeFileWeb() {
	w := &u.fileWeb
	if w.closed {
		return
	}
	w.closed = true
	if w.loadTimer != nil {
		w.loadTimer.Stop()
		w.loadTimer = nil
	}
	u.cancelFileReads()
	if w.cancel != nil {
		w.cancel()
	}
	if w.page != nil {
		w.page.Close()
		w.page = nil
	}
	// Do not leave up to eight completed previews retained after close.
	for w.results != nil {
		select {
		case <-w.results:
		default:
			return
		}
	}
}
