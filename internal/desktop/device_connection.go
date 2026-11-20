package desktop

import (
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

type deviceConnectionState struct {
	box   flex.Box
	id    ui.ID
	label string
}

// ADB disconnect is meaningful only for wireless transports, not USB devices.
func (u *UI) deviceConnectionButton(f *flex.Frame, d domain.Device) {
	if d.Connection != "Wi-Fi" {
		return
	}
	ready := d.State == "device"
	_, hasSession := u.state.Sessions[domain.SessionID(d.Serial, domain.SessionMirror)]
	glyph, color, label := iconConnect, u.c.muted, "连接设备"
	if ready {
		glyph, color, label = iconDisconnect, u.c.green, "断开连接"
	}
	disabled := hasSession || d.State == "unauthorized"
	if disabled {
		color = u.c.muted
	}
	id := u.id("device-connection-toggle")
	button := f.Column().Size(30, 30).Shrink(0).Center().Radius(6).HoverBackground(u.c.selected).
		HitWith(id, label, ui.HitOptions{Role: ui.RoleButton, Cursor: ui.PointerCursorHand, Disabled: disabled})
	button.Children(func() { f.Icon(glyph).Size(18, 18).Color(color) })
	hint := label
	if hasSession {
		hint = "请先停止投屏会话，再" + label
	}
	if d.State == "unauthorized" {
		hint = "请先在手机上允许 USB 调试"
	}
	u.deviceConnection = deviceConnectionState{box: button, id: id, label: hint}
	if !disabled && button.Clicked() {
		if ready {
			u.backend.Disconnect(d.Serial)
		} else {
			u.backend.Connect(d.Serial)
		}
	}
}

func (u *UI) paintDeviceConnectionTooltip(canvas *ui.Canvas, bounds ui.Rect) {
	for _, tool := range u.overviewTools {
		u.paintDeviceTooltip(canvas, bounds, tool)
	}
	c := u.deviceConnection
	u.paintDeviceTooltip(canvas, bounds, c)
}

func (u *UI) paintDeviceTooltip(canvas *ui.Canvas, bounds ui.Rect, c deviceConnectionState) {
	if c.label == "" || u.deviceDialogOpen() {
		return
	}
	if canvas.Hovered() == c.id || canvas.FocusVisible() == c.id {
		components.BeginTooltip(canvas, u.theme, bounds, c.box.Rect(), components.TooltipOptions{Layer: 10, Side: components.TooltipBottom}).Paint(c.label)
	}
}
