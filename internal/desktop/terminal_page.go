package desktop

import (
	"math"
	"unicode/utf8"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/term"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// termFont is the terminal's fixed-pitch size; cell metrics come from the
// canvas so the grid stays on whole pixels at any DPI.
const termFont = float32(13)

// terminalPanel is one shell or logcat stream of one device, rendered by the
// terminal emulator inside the device page's terminal tab.
type terminalPanel struct {
	key, serial, mode string
	stream            domain.Terminal
	term              *term.Terminal
	cols, rows        int
	exit              string
	buffer            []byte
}

// terminalAnsi is the classic xterm palette the emulator's indexes resolve to.
var terminalAnsi = [16][3]uint8{
	{12, 13, 16}, {205, 75, 75}, {87, 187, 126}, {222, 190, 91},
	{96, 145, 230}, {192, 116, 214}, {84, 192, 207}, {208, 212, 220},
	{90, 96, 108}, {235, 110, 110}, {118, 214, 154}, {238, 212, 122},
	{132, 176, 255}, {216, 148, 236}, {118, 218, 231}, {242, 244, 248},
}

var termDefaultFG = ui.RGB(222, 226, 234)
var termDefaultBG = ui.RGB(12, 13, 16)
var termCursorBG = ui.RGB(222, 226, 234)

// ensureTerminal opens the device's shell or logcat stream once per tab view.
// The stream is kept after the tab closes so output history survives a peek
// at the app list; the panel is torn down with the window.
func (u *UI) ensureTerminal(mode string) *terminalPanel {
	if u.terminals == nil {
		u.terminals = map[string]*terminalPanel{}
	}
	key := u.selected + "\x00" + mode
	if p := u.terminals[key]; p != nil {
		return p
	}
	if u.selected == "" {
		return nil
	}
	p := &terminalPanel{key: key, serial: u.selected, mode: mode, cols: 80, rows: 24}
	p.term = term.New(80, 24, 2000)
	notify := u.wake.Invalidate
	var err error
	if mode == "logcat" {
		p.stream, err = u.backend.OpenLogcat(p.serial, 80, 24, notify)
	} else {
		p.stream, err = u.backend.OpenShell(p.serial, 80, 24, notify)
	}
	if err != nil {
		p.exit = "无法打开终端：" + err.Error()
	}
	u.terminals[key] = p
	return p
}

// drainTerminals moves arrived output into the emulators. It runs on the UI
// thread because the emulator is not thread-safe.
func (u *UI) drainTerminals() {
	for _, p := range u.terminals {
		if p.stream == nil {
			continue
		}
		p.buffer = p.stream.Drain(p.buffer[:0])
		if len(p.buffer) > 0 {
			_, _ = p.term.Write(p.buffer)
		}
		select {
		case <-p.stream.Done():
			if p.exit == "" {
				p.exit = "会话已结束"
				if err := p.stream.Err(); err != nil {
					p.exit = "会话已结束：" + err.Error()
				}
				p.stream.Close()
			}
		default:
		}
	}
}

// closeTerminals ends every stream; the window owns the panels' lifetime.
func (u *UI) closeTerminals() {
	for _, p := range u.terminals {
		if p.stream != nil {
			p.stream.Close()
		}
	}
	u.terminals = nil
}

// terminalKey encodes one key event for the focused stream and returns true
// when the terminal consumed it.
func (u *UI) terminalKey(event ui.KeyEvent) bool {
	if u.termFocus == "" {
		return false
	}
	p := u.terminals[u.termFocus]
	if p == nil || p.stream == nil || p.exit != "" {
		return false
	}
	data := term.EncodeKeyEvent(event, p.term.KeyboardFlags(), p.term.Modes().AppCursorKeys)
	if len(data) == 0 {
		return false
	}
	return p.stream.Write(data)
}

// terminalWheel scrolls the focused terminal's scrollback.
func (u *UI) terminalWheel(event ui.ScrollEvent) bool {
	if u.termFocus == "" {
		return false
	}
	p := u.terminals[u.termFocus]
	if p == nil {
		return false
	}
	lines := int(math.Round(float64(event.DeltaY) / 24))
	if lines == 0 {
		if event.DeltaY > 0 {
			lines = 1
		} else if event.DeltaY < 0 {
			lines = -1
		}
	}
	return p.term.Scroll(lines)
}

// paintTerminal draws one terminal pane over the reserved rect. Clicking the
// pane focuses it so typing goes to the device.
func (u *UI) paintTerminal(canvas *ui.Canvas, rect ui.Rect, mode string) {
	key := u.selected + "\x00" + mode
	p := u.terminals[key]
	if p == nil {
		return
	}
	canvas.Rounded(rect, 12, termDefaultBG)
	focused := u.termFocus == key
	border := ui.RGB(39, 44, 54)
	if focused {
		border = u.c.blue
	}
	canvas.RoundedBorder(rect, 12, 1, border, ui.Color{})

	inner := ui.Rect{X: rect.X + 10, Y: rect.Y + 10, Width: rect.Width - 20, Height: rect.Height - 20}
	cellW, cellH := canvas.MonoCellSize(termFont, ui.MonoSystem)
	if cellW <= 0 || cellH <= 0 {
		return
	}
	cols, rows := int(inner.Width/cellW), int(inner.Height/cellH)
	if cols < 2 || rows < 2 {
		return
	}
	if cols != p.cols || rows != p.rows {
		p.cols, p.rows = cols, rows
		p.term.Resize(cols, rows)
		if p.stream != nil {
			p.stream.Resize(cols, rows)
		}
	}

	cursor := p.term.Cursor()
	base := ui.TextStyle{Size: termFont, Mono: true, Color: termDefaultFG}
	for row := 0; row < rows; row++ {
		line := p.term.Row(row)
		y := inner.Y + float32(row)*cellH
		bounds := ui.Rect{X: inner.X, Y: y, Width: float32(cols) * cellW, Height: cellH}
		u.paintTerminalRow(canvas, line, bounds, cellW, cols, cursor, row, base)
	}
}

// paintTerminalRow paints one emulator row: background spans first, then the
// colored text runs in one call, then the cursor is part of both passes.
func (u *UI) paintTerminalRow(canvas *ui.Canvas, line term.Row, bounds ui.Rect, cellW float32, cols int, cursor term.Cursor, row int, base ui.TextStyle) {
	cells := line.Cells
	count := min(len(cells), cols)
	cursorCol := -1
	if row == cursor.Y && cursor.Visible && cursor.X < cols {
		cursorCol = cursor.X
		if cursorCol >= 0 && cursorCol < count && cells[cursorCol].Attr&term.WideTail != 0 {
			cursorCol--
		}
	}

	// Background pass: shared-background stretches become one fill each.
	spanFrom := -1
	spanColor := ui.Color{}
	closeSpan := func(to int) {
		if spanFrom >= 0 && spanFrom < to {
			canvas.Fill(ui.Rect{X: bounds.X + float32(spanFrom)*cellW, Y: bounds.Y, Width: float32(to-spanFrom) * cellW, Height: bounds.Height}, spanColor)
		}
		spanFrom = -1
	}
	for x := 0; x < count; x++ {
		if cells[x].Attr&term.WideTail != 0 {
			continue
		}
		bg, hasBG := u.termColor(cells[x].BG, ui.Color{})
		if x == cursorCol {
			bg, hasBG = termCursorBG, true
		}
		if !hasBG {
			closeSpan(x)
			continue
		}
		if spanFrom < 0 {
			spanFrom, spanColor = x, bg
			continue
		}
		if spanColor != bg {
			closeSpan(x)
			spanFrom, spanColor = x, bg
		}
	}
	closeSpan(count)

	// Text pass: one run per stretch of equal ink. Blanks are swallowed into
	// the plain, contiguous run before them and trimmed from its end.
	type termInk struct {
		fg                        ui.Color
		weight                    float32
		italic, underline, strike bool
		plain                     bool
	}
	var (
		runs            []ui.TextRun
		text            []byte
		runStart, runAt = -1, 0
		runCells        int
		runInk          termInk
	)
	flush := func() {
		if runStart >= 0 {
			runs = append(runs, ui.TextRun{
				Column: int32(runStart), Bytes: int32(runAt), Cells: int32(runCells),
				Color: runInk.fg, Weight: runInk.weight, Italic: runInk.italic,
				Underline: runInk.underline, Strike: runInk.strike,
			})
			runStart, runAt, runCells = -1, 0, 0
		}
	}
	for x := 0; x < count; x++ {
		cell := cells[x]
		if cell.Attr&term.WideTail != 0 {
			continue
		}
		width := 1
		if cell.Attr&term.WideHead != 0 {
			width = 2
		}
		character := cell.R
		if character < 0x20 || (character >= 0x7f && character < 0xa0) {
			character = 0
		}
		decorated := cell.Attr&(term.Underline|term.Strike) != 0
		blank := (character == 0 || character == ' ' || cell.Attr&term.Hidden != 0) && !decorated && x != cursorCol

		if blank {
			if runStart >= 0 && runInk.plain && runStart+runCells == x {
				text = append(text, ' ')
				runAt++
				runCells++
			} else {
				flush()
			}
			continue
		}
		if character == 0 {
			character = ' '
		}

		ink := termInk{fg: termDefaultFG, plain: character < 0x7f}
		if cell.Attr&term.Inverse != 0 {
			ink.fg, _ = u.termColor(cell.BG, termDefaultBG)
		} else {
			ink.fg, _ = u.termColor(cell.FG, termDefaultFG)
		}
		if cell.Attr&term.Dim != 0 {
			ink.fg.Alpha *= 0.6
		}
		if cell.Attr&term.Bold != 0 {
			ink.weight = ui.WeightBold
		}
		ink.italic = cell.Attr&term.Italic != 0
		ink.underline = cell.Attr&term.Underline != 0
		ink.strike = cell.Attr&term.Strike != 0
		if x == cursorCol {
			ink.fg = termDefaultBG
		}

		if runStart >= 0 && runInk == ink && ink.plain && runStart+runCells == x {
			text = utf8.AppendRune(text, character)
			runAt += utf8.RuneLen(character)
			runCells += width
		} else {
			flush()
			runStart, runInk = x, ink
			text = utf8.AppendRune(text, character)
			runAt += utf8.RuneLen(character)
			runCells = width
		}
	}
	// Trailing swallowed blanks are not worth drawing.
	for runStart >= 0 && runAt > 0 && text[len(text)-1] == ' ' {
		text = text[:len(text)-1]
		runAt--
		runCells--
		if runCells == 0 {
			runStart = -1
		}
	}
	flush()
	canvas.LabelRuns(string(text), bounds, base, cellW, runs)
}

// termColor resolves a terminal color against the palette; ok is false for
// the terminal's default color, which the caller substitutes.
func (u *UI) termColor(color term.Color, fallback ui.Color) (ui.Color, bool) {
	switch color.Kind() {
	case term.PaletteColor:
		r, g, b := term.Resolve(color.Index(), &terminalAnsi)
		return ui.RGB(r, g, b), true
	case term.TrueColor:
		r, g, b := color.Components()
		return ui.RGB(r, g, b), true
	}
	return fallback, false
}

// terminalStatusLine is the small header shown above the terminal pane.
func terminalStatusLine(p *terminalPanel) string {
	if p == nil {
		return ""
	}
	if p.exit != "" {
		return p.exit
	}
	if p.mode == "logcat" {
		return "已连接 · logcat 实时输出，滚动可回看"
	}
	return "已连接 · 点击终端后开始输入，滚动可回看历史"
}

func (u *UI) deviceTerminalPage(f *flex.Frame, d domain.Device) {
	if u.termMode != "logcat" {
		u.termMode = "shell"
	}
	f.Row().FillWidth().Gap(8).Items(flex.Center).Children(func() {
		for _, item := range []struct{ mode, label string }{{"shell", "Shell"}, {"logcat", "Logcat"}} {
			if f.Button(u.id("term-"+item.mode), item.label).Small().Outline().Clicked() {
				u.termMode = item.mode
				u.ensureTerminal(item.mode)
				u.wake.Invalidate()
			}
		}
	})
	p := u.ensureTerminal(u.termMode)
	if p != nil {
		u.termFocus = p.key
	}
	f.Text(terminalStatusLine(p)).FontSize(11).Color(u.c.muted)
	slot := f.Slot().FillWidth().Height(500).HitWith(u.id("terminal-pane"), "设备终端", ui.HitOptions{Role: ui.RoleTerminal, Autofocus: true})
	if slot.Clicked() {
		u.termFocus = d.Serial + "\x00" + u.termMode
	}
	u.terminalSlot = slot
	u.terminalSlotSet = true
}
