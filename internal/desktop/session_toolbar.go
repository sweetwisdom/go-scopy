package desktop

import (
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

func (s *sessionWindow) openDock(title string) error {
	window, err := s.owner.application.OpenWindow(app.Config{
		Title: title, Width: 236, Height: 48, WindowMode: app.WindowFrameless,
		Tree: &s.tree, Invalidator: &s.wake, Paint: s.paintTools,
		// This nonzero value rounds to transparent in the composition surface.
		ClearColor:     app.Color{Alpha: 0.000001},
		WindowBackdrop: func() app.BackdropStyle { return app.BackdropStyle{Kind: app.BackdropNone} },
		OnWake:         s.onWake,
		OnPointer:      s.islandPointer,
		OnFrameChange:  func(ui.Rect) { s.native.RefreshDock() },
		OnCloseRequest: func(func()) { s.requestClose() },
		OnClosed:       s.dispose,
	}, &s.canvas)
	if err != nil {
		return err
	}
	s.window = window
	s.native.Dock(s.collapsed)
	return nil
}

func (s *sessionWindow) islandPointer(e ui.PointerEvent) ui.EventResult {
	if s.disposed {
		return ui.EventResult{}
	}
	if e.Kind == ui.PointerDown && e.Button == ui.PointerButtonPrimary && e.Target == s.tree.ID(s.scope, "drag-island") {
		s.dragging = true
		s.native.BeginDrag()
		return ui.EventResult{Handled: true}
	}
	if s.dragging {
		switch e.Kind {
		case ui.PointerMove:
			s.native.MoveDrag()
		case ui.PointerUp:
			s.native.EndDrag()
			s.dragging = false
		}
		return ui.EventResult{Handled: true}
	}
	return ui.EventResult{}
}

func (s *sessionWindow) paintTools(canvas *ui.Canvas, size ui.Size) {
	white, muted := ui.RGB(242, 244, 248), ui.RGB(140, 147, 160)
	bg, border := ui.RGB(12, 13, 16), ui.RGB(44, 46, 53)
	radius := float32(28)
	if s.collapsed {
		radius = size.Height / 2
	}
	// Leave transparent room for the GPU's antialiased outer edge instead of
	// placing its coverage fringe on the native window's clipping boundary.
	const inset = float32(1)
	canvas.RoundedBorder(ui.Rect{X: inset, Y: inset, Width: size.Width - 2*inset, Height: size.Height - 2*inset}, radius-inset, 1, border, bg)

	// Dedicated grip keeps window dragging separate from clicks on controls.
	grip := ui.Rect{X: 10, Y: 8, Width: 34, Height: 32}
	canvas.HitWithOptions(s.tree.ID(s.scope, "drag-island"), grip, "拖动灵动岛", ui.HitOptions{Role: ui.RoleButton, Cursor: ui.PointerCursorHand, Hint: "拖动后靠近屏幕边缘松手吸附"})
	for i := 0; i < 2; i++ {
		canvas.Rounded(ui.Rect{X: 20, Y: float32(21 + i*5), Width: 13, Height: 2}, 1, muted)
	}

	dot := ui.RGB(98, 206, 155)
	status := "已连接"
	if !s.ready {
		dot, status = ui.RGB(233, 184, 91), "连接中"
	}
	if s.recording && s.ready {
		dot, status = ui.RGB(248, 101, 114), "录屏中"
	}
	if s.closing {
		dot, status = muted, "正在保存并结束…"
	}
	canvas.Rounded(ui.Rect{X: 49, Y: 21, Width: 6, Height: 6}, 3, dot)
	name := s.name
	if name == "" {
		name = s.serial
	}
	canvas.Label(name, ui.Rect{X: 65, Y: 5, Width: size.Width - 111, Height: 38}, ui.TextStyle{Size: 13, Weight: ui.WeightSemibold, Color: white})
	toggle := "⌃"
	toggleLabel := "收起控制面板"
	if s.collapsed {
		toggle, toggleLabel = "⌄", "展开控制面板"
	}
	toggleBounds := ui.Rect{X: 44, Y: 0, Width: size.Width - 44, Height: 48}
	id := s.tree.ID(s.scope, "toggle-tools")
	canvas.HitWithOptions(id, toggleBounds, toggleLabel, ui.HitOptions{Role: ui.RoleButton, Cursor: ui.PointerCursorHand, Expandable: true, Expanded: !s.collapsed})
	canvas.LabelCenter(toggle, ui.Rect{X: size.Width - 40, Y: 5, Width: 28, Height: 36}, ui.TextStyle{Size: 17, Color: muted})
	if canvas.Clicked(id) {
		s.collapsed = !s.collapsed
		s.wake.Invalidate()
	}
	if s.collapsed {
		return
	}

	canvas.Label(status, ui.Rect{X: 20, Y: 49, Width: 170, Height: 24}, ui.TextStyle{Size: 11, Color: dot})
	canvas.Label("SCRCPY", ui.Rect{X: size.Width - 75, Y: 49, Width: 60, Height: 24}, ui.TextStyle{Size: 10, Weight: ui.WeightSemibold, Color: muted})
	for i, action := range []struct{ key, label, symbol string }{
		{"back", "返回", "‹"}, {"home", "桌面", "○"}, {"recents", "任务", "▢"},
		{"menu", "菜单", "≡"}, {"volume-down", "音量−", "−"}, {"volume-up", "音量＋", "+"},
		{"capture", "截图", "▣"}, {"sleep", "熄屏", "◐"}, {"wake", "唤醒", "☼"},
	} {
		bounds := ui.Rect{X: 16 + float32(i%3)*106, Y: 82 + float32(i/3)*50, Width: 100, Height: 44}
		if s.islandButton(canvas, action.key, action.label, action.symbol, bounds, !s.ready || s.closing, false) {
			if action.key == "capture" {
				s.owner.backend.Capture(s.serial)
			} else {
				s.owner.backend.(domain.HostedBackend).Key(s.serial, action.key)
			}
			s.native.Focus()
		}
	}
	canvas.Label("拖动左侧短线 · 靠近屏幕边缘吸附", ui.Rect{X: 20, Y: 238, Width: 234, Height: 24}, ui.TextStyle{Size: 10, Color: muted})
	if s.islandButton(canvas, "close-session", "结束", "", ui.Rect{X: size.Width - 74, Y: 235, Width: 58, Height: 28}, s.closing, true) {
		s.requestClose()
	}
}

func (s *sessionWindow) islandButton(canvas *ui.Canvas, key, label, symbol string, bounds ui.Rect, disabled, danger bool) bool {
	id := s.tree.ID(s.scope, key)
	fill, color := ui.RGB(28, 30, 36), ui.RGB(222, 226, 234)
	if danger {
		fill, color = ui.RGB(53, 27, 32), ui.RGB(249, 145, 155)
	}
	if canvas.Hovered() == id || canvas.FocusVisible() == id {
		fill = ui.RGB(48, 52, 62)
	}
	if canvas.Pressed() == id {
		fill = ui.RGB(59, 65, 78)
	}
	if disabled {
		color = ui.RGB(83, 88, 101)
	}
	canvas.Rounded(bounds, 12, fill)
	canvas.HitWithOptions(id, bounds, label, ui.HitOptions{Role: ui.RoleButton, Cursor: ui.PointerCursorHand, Disabled: disabled})
	if symbol != "" {
		canvas.LabelCenter(symbol, ui.Rect{X: bounds.X + 9, Y: bounds.Y + 3, Width: 24, Height: bounds.Height - 6}, ui.TextStyle{Size: 21, Color: color})
		canvas.LabelCenter(label, ui.Rect{X: bounds.X + 35, Y: bounds.Y, Width: bounds.Width - 42, Height: bounds.Height}, ui.TextStyle{Size: 12, Color: color})
	} else {
		canvas.LabelCenter(label, bounds, ui.TextStyle{Size: 11, Color: color})
	}
	return !disabled && canvas.Clicked(id)
}
