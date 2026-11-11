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
	recording                     bool
}

func (u *UI) SetApplication(application *app.Application) { u.application = application }

func (u *UI) startWindow(options domain.Options, record bool) bool {
	backend, ok := u.backend.(domain.HostedBackend)
	if !ok || u.application == nil || !windowhost.Supported || os.Getenv("VIVA_HEADLESS") == "1" {
		return false
	}
	if existing := u.sessionWindows[u.selected]; existing != nil {
		if !existing.ended {
			existing.window.Show()
			existing.native.Focus()
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
	title := fmt.Sprintf("Go-Scopy Island · %s · #%d", name, nextSessionWindow.Add(1))
	ctx, cancel := context.WithCancel(context.Background())
	s := &sessionWindow{owner: u, serial: u.selected, name: name, ctx: ctx, cancel: cancel, collapsed: true, recording: record}
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
func (s *sessionWindow) Finished(_ error) {
	s.mu.Lock()
	s.finished = true
	s.mu.Unlock()
	s.wake.Invalidate()
}
func (s *sessionWindow) onWake() {
	if s.disposed {
		return
	}
	s.mu.Lock()
	attached, finished := s.attached, s.finished
	s.mu.Unlock()
	if finished {
		s.ended, s.ready = true, false
		s.destroy()
		return
	}
	if attached && !s.ready {
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
func (u *UI) closeSessionWindows() {
	for _, s := range u.sessionWindows {
		s.requestClose()
	}
}
