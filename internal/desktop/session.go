package desktop

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/windowhost"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

var nextSessionWindow atomic.Uint64

type sessionWindow struct {
	owner                         *UI
	serial, name                  string
	session                       string // unique session key of this window
	kind                          domain.SessionKind
	display                       *domain.VirtualDisplay
	options                       domain.Options
	window                        *app.Window
	native                        *windowhost.Host
	wake                          app.Invalidator
	canvas                        ui.Canvas
	tree                          ui.Tree
	scope                         ui.Scope
	ctx                           context.Context
	cancel                        context.CancelFunc
	mu                            sync.Mutex
	attached, finished            bool
	ready, ended, closing         bool
	disposed, collapsed, dragging bool
	recording, restart, screenOff bool
	resolving, foregroundDone     bool
	foregroundPackage             string
	foregroundErr, finishErr      error
	islandMessage                 string
}

func (u *UI) SetApplication(application *app.Application) { u.application = application }

func (u *UI) startWindow(options domain.Options, record bool) bool {
	return u.openSessionWindow(u.selected, domain.SessionMirror, nil, options, record)
}

// openSessionWindow opens the island control window and the session it
// controls: the main screen mirror, or one virtual display.
func (u *UI) openSessionWindow(serial string, kind domain.SessionKind, display *domain.VirtualDisplay, options domain.Options, record bool) bool {
	backend, ok := u.backend.(domain.HostedBackend)
	if !ok || u.application == nil || !windowhost.Supported || os.Getenv("VIVA_HEADLESS") == "1" {
		return false
	}
	session := domain.SessionID(serial, kind)
	if kind == domain.SessionVirtual {
		session = domain.NewVirtualSessionID(serial)
	}
	if existing := u.sessionWindows[session]; existing != nil {
		if !existing.ended {
			existing.window.Show()
			existing.native.Focus()
			return true
		}
		existing.destroy()
	}
	name := serial
	for _, d := range u.state.Devices {
		if d.Serial == serial {
			name = d.Name()
			break
		}
	}
	prefix := "Go-Scopy Island"
	if kind == domain.SessionVirtual {
		prefix = "Go-Scopy Virtual"
		if display != nil {
			name = u.virtualAppLabel(serial, display.StartApp) + " · " + name
		}
	}
	title := fmt.Sprintf("%s · %s · #%d", prefix, name, nextSessionWindow.Add(1))
	ctx, cancel := context.WithCancel(context.Background())
	s := &sessionWindow{owner: u, serial: serial, name: name, session: session, kind: kind,
		display: display, options: options, ctx: ctx, cancel: cancel, collapsed: true, recording: record}
	s.scope = s.tree.Feature("session")
	s.native = windowhost.New(title)
	if err := s.openDock(title); err != nil {
		cancel()
		u.notice = "灵动岛控制窗无法打开：" + err.Error()
		u.wake.Invalidate()
		return true
	}
	if u.sessionWindows == nil {
		u.sessionWindows = map[string]*sessionWindow{}
	}
	u.sessionWindows[session] = s
	backend.StartHosted(serial, session, options, display, record, s)
	return true
}

func (s *sessionWindow) Done() <-chan struct{} { return s.ctx.Done() }
func (s *sessionWindow) Attach(ctx context.Context, pid int) error {
	return s.native.Attach(ctx, pid)
}
func (s *sessionWindow) Ready() {
	s.mu.Lock()
	s.attached = true
	s.mu.Unlock()
	s.wake.Invalidate()
}
func (s *sessionWindow) Finished(err error) {
	s.mu.Lock()
	s.finished, s.finishErr = true, err
	s.mu.Unlock()
	s.wake.Invalidate()
}
func (s *sessionWindow) onWake() {
	if s.disposed {
		return
	}
	s.mu.Lock()
	attached, finished, finishErr := s.attached, s.finished, s.finishErr
	resolved, pkg, foregroundErr := s.foregroundDone, s.foregroundPackage, s.foregroundErr
	s.foregroundDone = false
	s.mu.Unlock()
	if finished {
		if finishErr != nil {
			s.restart = false
		}
		s.ended, s.ready = true, false
		s.destroy()
		return
	}
	if resolved && !s.closing {
		s.resolving = false
		if foregroundErr != nil {
			s.islandMessage = "操作失败，详情见主窗口任务中心"
			s.owner.notice = foregroundErr.Error()
			s.owner.wake.Invalidate()
		} else {
			s.islandMessage = ""
			s.launchIndependentApp(pkg)
		}
	}
	if attached && !s.ready {
		// ReturnHome is a one-shot shortcut, not a recording/keyboard restart option.
		if s.display != nil {
			s.display.ReturnHome = false
		}
		s.ready = true
		s.native.Focus()
	}
	s.native.Dock(s.collapsed)
}

// Keep the island alive until scrcpy has flushed any recording.
func (s *sessionWindow) requestClose() {
	if s.disposed {
		return
	}
	if s.ended {
		s.destroy()
		return
	}
	s.closing = true
	s.cancel()
	s.wake.Invalidate()
}
func (s *sessionWindow) dispose() {
	if s.disposed {
		return
	}
	s.disposed = true
	s.cancel()
	s.native.Release()
	if s.owner.sessionWindows[s.session] == s {
		delete(s.owner.sessionWindows, s.session)
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
	// A keyboard/recording change reopens the session with the new setting once
	// the old scrcpy has exited and any recording is safely flushed.
	if s.restart {
		s.restart = false
		s.owner.openSessionWindow(s.serial, s.kind, s.display, s.options, s.recording)
	}
}
func (u *UI) closeSessionWindows() {
	for _, s := range u.sessionWindows {
		s.restart = false
		s.requestClose()
	}
}
