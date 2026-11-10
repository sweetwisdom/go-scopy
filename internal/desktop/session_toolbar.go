package desktop

import (
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

func (s *sessionWindow) openDock(title string) error {
	s.dockTitle = title + " · controls"
	window, err := s.owner.application.OpenWindow(app.Config{
		Title: s.dockTitle, Width: 76, Height: 512, WindowMode: app.WindowFrameless,
		Tree: &s.tree, Invalidator: &s.dockWake, Paint: s.paintTools,
		// A nonzero value bypasses Viva's default opaque clear color; it rounds
		// to zero alpha in the BGRA8 composition surface.
		ClearColor:     app.Color{Alpha: 0.000001},
		WindowBackdrop: func() app.BackdropStyle { return app.BackdropStyle{Kind: app.BackdropNone} },
		OnWake: func() {
			if !s.disposed {
				s.native.Dock(s.dockTitle, s.collapsed)
			}
		},
		OnCloseRequest: func(func()) { s.requestClose() },
		OnClosed:       func() { s.dock = nil },
	}, &s.dockCanvas)
	if err != nil {
		return err
	}
	s.dock = window
	s.native.Dock(s.dockTitle, s.collapsed)
	return nil
}

func (s *sessionWindow) paintTools(canvas *ui.Canvas, size ui.Size) {
	c := s.owner.c
	s.view.Draw(canvas, s.owner.theme, ui.Rect{Width: size.Width, Height: size.Height}, func(f *flex.Frame) {
		f.Column().Fill().Pad(6).Gap(6).Children(func() {
			label := "收起 ›"
			if s.collapsed {
				label = "‹"
			}
			if f.Button(s.tree.ID(s.scope, "toggle-tools"), label).FillWidth().Height(40).Outline().Small().Background(c.card).Clicked() {
				s.collapsed = !s.collapsed
				s.dockWake.Invalidate()
			}
			if s.collapsed {
				return
			}
			for _, action := range []struct{ key, label string }{
				{"back", "返回"}, {"home", "桌面"}, {"recents", "任务"}, {"menu", "菜单"},
				{"volume-up", "音量＋"}, {"volume-down", "音量−"}, {"capture", "截图"}, {"sleep", "熄屏"}, {"wake", "唤醒"},
			} {
				if f.Button(s.tree.ID(s.scope, action.key), action.label).FillWidth().Height(40).Outline().Small().Background(c.card).Disabled(!s.ready || s.closing).Clicked() {
					if action.key == "capture" {
						s.owner.backend.Capture(s.serial)
					} else {
						s.owner.backend.(domain.HostedBackend).Key(s.serial, action.key)
					}
					s.native.Focus()
				}
			}
			if f.Button(s.tree.ID(s.scope, "close-session"), "关闭").FillWidth().Height(40).Outline().Small().Background(c.card).Disabled(s.closing).Clicked() {
				s.requestClose()
			}
		})
	})
}
