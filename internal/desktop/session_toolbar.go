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
	_, canResolve := s.owner.backend.(domain.ForegroundBackend)
	independentLabel := "独立窗口"
	if s.resolving {
		independentLabel = "正在识别…"
	}
	if s.islandButton(canvas, "independent-app", independentLabel, "", ui.Rect{X: 16, Y: 238, Width: 150, Height: 38}, !s.ready || s.closing || s.resolving || s.kind == domain.SessionVirtual || !canResolve, false) {
		s.detachCurrentApp()
	}
	recordLabel := "开始录屏"
	if s.recording {
		recordLabel = "停止录屏"
	}
	if s.islandButton(canvas, "toggle-recording", recordLabel, "", ui.Rect{X: 174, Y: 238, Width: 154, Height: 38}, !s.ready || s.closing || s.resolving, s.recording) {
		s.toggleRecording()
	}
	message := "切换录屏将重启投屏，虚拟屏应用可能重启"
	if s.islandMessage != "" {
		message = s.islandMessage
	}
	canvas.Label(message, ui.Rect{X: 20, Y: 280, Width: size.Width - 40, Height: 20}, ui.TextStyle{Size: 10, Color: muted})
	// The two session toggles: keyboard injection mode and privacy screen.
	keyboardLabel, keyboardState := "模拟实体键盘", "SDK 注入模式"
	if s.options.Keyboard == domain.KeyboardUHID {
		keyboardState = "UHID 实体键盘"
	}
	if s.islandToggle(canvas, "keyboard-mode", keyboardLabel, keyboardState, ui.Rect{X: 16, Y: 304, Width: size.Width - 32, Height: 38}, s.closing) {
		s.toggleKeyboard()
	}
	privacyLabel, privacyState := "隐私模式", "屏幕开启"
	if s.screenOff {
		privacyState = "屏幕已关闭"
	}
	if s.islandToggle(canvas, "privacy-mode", privacyLabel, privacyState, ui.Rect{X: 16, Y: 348, Width: size.Width - 32, Height: 38}, s.closing || !s.ready) {
		s.togglePrivacy()
	}

	canvas.Label("拖动左侧短线 · 靠近屏幕边缘吸附", ui.Rect{X: 20, Y: 400, Width: 234, Height: 24}, ui.TextStyle{Size: 10, Color: muted})
	if s.islandButton(canvas, "close-session", "结束", "", ui.Rect{X: size.Width - 74, Y: 397, Width: 58, Height: 28}, s.closing, true) {
		s.requestClose()
	}
}

// islandToggle is a wide two-part button: the feature name on the left and
// its current state on the right.
func (s *sessionWindow) islandToggle(canvas *ui.Canvas, key, label, state string, bounds ui.Rect, disabled bool) bool {
	id := s.tree.ID(s.scope, key)
	fill, color := ui.RGB(28, 30, 36), ui.RGB(222, 226, 234)
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
	canvas.Label(label, ui.Rect{X: bounds.X + 14, Y: bounds.Y, Width: bounds.Width / 2, Height: bounds.Height}, ui.TextStyle{Size: 12, Color: color})
	canvas.Label(state, ui.Rect{X: bounds.X + bounds.Width/2, Y: bounds.Y, Width: bounds.Width/2 - 14, Height: bounds.Height}, ui.TextStyle{Size: 11, Color: ui.RGB(140, 147, 160)})
	return !disabled && canvas.Clicked(id)
}

// toggleKeyboard flips the scrcpy keyboard mode and restarts this session:
// the mode is a scrcpy startup parameter, and any recording is flushed by the
// normal shutdown path before the new session opens.
func (s *sessionWindow) toggleKeyboard() {
	if s.options.Keyboard == domain.KeyboardUHID {
		s.options.Keyboard = domain.KeyboardSDK
	} else {
		s.options.Keyboard = domain.KeyboardUHID
	}
	settings := s.owner.state.Settings
	if settings.Devices == nil {
		settings.Devices = map[string]domain.Options{}
	}
	saved := settings.OptionsFor(s.serial)
	saved.Keyboard = s.options.Keyboard
	settings.Devices[s.serial] = saved
	s.owner.backend.Save(settings)
	s.restart = true
	s.requestClose()
}

// togglePrivacy turns the phone's screen off or on while mirroring keeps
// running, by driving scrcpy's own screen shortcut on its window.
func (s *sessionWindow) togglePrivacy() {
	s.screenOff = !s.screenOff
	s.native.ScreenOff(s.screenOff)
	s.wake.Invalidate()
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

// detachCurrentApp never performs ADB I/O on the UI thread. Results are consumed
// by onWake, and late completions cannot reopen a closed session.
func (s *sessionWindow) detachCurrentApp() {
	backend, ok := s.owner.backend.(domain.ForegroundBackend)
	if !ok || !s.ready || s.closing || s.resolving || s.kind == domain.SessionVirtual {
		return
	}
	s.resolving = true
	s.islandMessage = "正在获取主屏前台应用…"
	backend.ResolveForeground(s.serial, func(pkg string, err error) {
		s.mu.Lock()
		s.foregroundPackage, s.foregroundErr, s.foregroundDone = pkg, err, true
		s.mu.Unlock()
		s.wake.Invalidate()
	})
	s.wake.Invalidate()
}

func (s *sessionWindow) launchIndependentApp(pkg string) {
	if pkg == "" {
		return
	}
	display := domain.VirtualDisplay{
		Width: 1080, Height: 2400, DPI: 420, SystemDecorations: true,
		StartApp: pkg, ReturnHome: true,
	}
	if !s.owner.openSessionWindow(s.serial, domain.SessionVirtual, &display, s.options, false) {
		s.owner.backend.StartVirtual(s.serial, s.options, display, false)
	}
}

// Recording is a scrcpy startup option. Use the existing graceful restart path
// so stopping first flushes the old MP4, then resumes a non-recording session.
func (s *sessionWindow) toggleRecording() {
	if !s.ready || s.closing || s.resolving {
		return
	}
	s.recording = !s.recording
	s.restart = true
	s.requestClose()
}
