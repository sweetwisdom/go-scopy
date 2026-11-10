package desktop

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/go-scopy/go-scopy/internal/brand"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/windowhost"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

const sessionToolbarHeight = 0

var nextSessionWindow atomic.Uint64

type sessionWindow struct {
	owner                 *UI
	serial                string
	window                *app.Window
	native                *windowhost.Host
	wake                  app.Invalidator
	canvas                ui.Canvas
	tree                  ui.Tree
	scope                 ui.Scope
	view                  flex.View
	ctx                   context.Context
	cancel                context.CancelFunc
	mu                    sync.Mutex
	attached, finished    bool
	result                error
	ready, ended, closing bool
	status                string
	disposed              bool
	dock                  *app.Window
	dockCanvas            ui.Canvas
	dockWake              app.Invalidator
	dockTitle             string
	collapsed             bool
}

func (u *UI) SetApplication(application *app.Application) {
	u.application = application
}

func (u *UI) startWindow(options domain.Options, record bool) bool {
	backend, ok := u.backend.(domain.HostedBackend)
	if !ok || u.application == nil || !windowhost.Supported || os.Getenv("VIVA_HEADLESS") == "1" {
		return false
	}
	if existing := u.sessionWindows[u.selected]; existing != nil {
		if !existing.ended {
			existing.window.Show()
			return true
		}
		existing.destroy()
	}
	name := u.selected
	for _, d := range u.state.Devices {
		if d.Serial == u.selected {
			name = d.Name()
			break
		}
	}
	title := fmt.Sprintf("Go-Scopy · %s · #%d", name, nextSessionWindow.Add(1))
	ctx, cancel := context.WithCancel(context.Background())
	s := &sessionWindow{owner: u, serial: u.selected, ctx: ctx, cancel: cancel, status: "正在启动投屏…"}
	s.scope = s.tree.Feature("session")
	s.native = windowhost.New(title, sessionToolbarHeight, options.AlwaysOnTop)
	config := app.Config{Title: title, Icon: brand.WindowIcon(), Width: 400, Height: 820, MinWidth: 120, MinHeight: 180,
		Tree: &s.tree, Invalidator: &s.wake, Paint: s.paint, ClearColor: app.Color{Red: 0.025, Green: 0.03, Blue: 0.04, Alpha: 1},
		OnResize: func(ui.Size) { s.native.Resize() }, OnFrameChange: func(ui.Rect) { s.native.Resize() },
		OnMaximize:     func(bool) { s.native.Resize() },
		OnWake:         s.onWake,
		OnCloseRequest: func(func()) { s.requestClose() },
		OnClosed:       s.dispose,
	}
	window, err := u.application.OpenWindow(config, &s.canvas)
	if err != nil {
		cancel()
		u.notice = "投屏窗口无法打开：" + err.Error()
		u.wake.Invalidate()
		return true
	}
	s.window = window
	if err := s.openDock(title); err != nil {
		s.destroy()
		u.notice = "悬浮工具栏无法打开：" + err.Error()
		u.wake.Invalidate()
		return true
	}
	if u.sessionWindows == nil {
		u.sessionWindows = map[string]*sessionWindow{}
	}
	u.sessionWindows[s.serial] = s
	backend.StartHosted(s.serial, options, record, s)
	return true
}

func (s *sessionWindow) Done() <-chan struct{} { return s.ctx.Done() }
func (s *sessionWindow) Attach(ctx context.Context, pid int) error {
	if err := s.native.Attach(ctx, pid); err != nil {
		return err
	}
	s.mu.Lock()
	s.attached = true
	s.mu.Unlock()
	s.wake.Invalidate()
	return nil
}
func (s *sessionWindow) Finished(err error) {
	s.mu.Lock()
	s.finished, s.result = true, err
	s.mu.Unlock()
	s.wake.Invalidate()
}

func (s *sessionWindow) onWake() {
	if s.disposed {
		return
	}
	defer s.dockWake.Invalidate()
	s.mu.Lock()
	attached, finished, err := s.attached, s.finished, s.result
	s.mu.Unlock()
	if attached && !s.ready && !finished {
		s.ready = true
		s.status = "已连接 · 鼠标键盘可直接控制手机"
		s.native.Sync()
		s.native.Focus()
	}
	if finished {
		s.ended, s.ready = true, false
		s.status = "会话已结束，录屏结果可在任务中查看"
		if err != nil {
			s.status = err.Error()
		}
		s.destroy()
	}
}

// Closing keeps the native container alive until scrcpy has flushed recording.
func (s *sessionWindow) requestClose() {
	if s.disposed {
		return
	}
	if s.ended {
		s.destroy()
		return
	}
	s.closing, s.status = true, "正在结束会话并保存录像…"
	s.cancel()
	s.wake.Invalidate()
	s.dockWake.Invalidate()
}

// Dispose is idempotent and runs on the window thread, including native closes.
func (s *sessionWindow) dispose() {
	if s.disposed {
		return
	}
	s.disposed = true
	s.cancel()
	if s.dock != nil {
		dock := s.dock
		s.dock = nil
		dock.Close()
	}
	s.native.Release()
	if s.owner.sessionWindows[s.serial] == s {
		delete(s.owner.sessionWindows, s.serial)
	}
}

func (s *sessionWindow) destroy() {
	if s.disposed {
		return
	}
	s.dispose()
	if s.window != nil {
		s.window.Close()
	}
}

func (s *sessionWindow) paint(canvas *ui.Canvas, size ui.Size) {
	canvas.Fill(ui.Rect{Width: size.Width, Height: size.Height}, ui.RGB(8, 10, 14))
	if !s.ready {
		label := "正在连接手机画面…"
		if s.ended {
			label = "投屏已结束，详情见任务中心"
		}
		canvas.LabelCenter(label, ui.Rect{Y: sessionToolbarHeight, Width: size.Width, Height: max(size.Height-sessionToolbarHeight, 0)}, ui.TextStyle{Size: 14, Color: ui.RGB(153, 163, 180)})
	}
}

func (u *UI) closeSessionWindows() {
	for _, s := range u.sessionWindows {
		s.requestClose()
	}
}
