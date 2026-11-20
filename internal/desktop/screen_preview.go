package desktop

import (
	"bytes"
	"math"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

type screenPreviewState struct {
	data          []byte
	bitmap        ui.Bitmap
	valid         bool
	frame, screen flex.Box
}

func overviewColumnWidths(width float32) (left, right float32) {
	right = min(float32(240), max(float32(196), width*0.27))
	if width < 540 {
		return width, min(width, right)
	}
	return max(width-right-16, 1), right
}

func (u *UI) screenPreview(f *flex.Frame, device domain.Device, columnWidth float32) {
	backend, supported := u.backend.(domain.ScreenPreviewBackend)
	preview := u.state.Preview
	if preview.Serial != device.Serial {
		preview = domain.ScreenPreview{}
		u.preview = screenPreviewState{}
	}
	if !bytes.Equal(u.preview.data, preview.PNG) {
		u.preview.data = preview.PNG
		// Retain enough pixels for the high-DPI phone preview.
		u.preview.bitmap, u.preview.valid = pngBitmap("screen-preview:"+device.Serial, preview.PNG, 1280, 8192, 32<<20)
		if u.preview.valid {
			u.preview.bitmap = roundedPreviewBitmap(u.preview.bitmap)
		}
	}
	screenWidth := min(max(columnWidth-44, 1), 184)
	aspect := float32(20.0 / 9.0)
	if u.preview.valid {
		aspect = float32(u.preview.bitmap.Height) / float32(u.preview.bitmap.Width)
	}
	screenHeight := screenWidth * aspect
	// Reserve space for the workspace header, tabs, preview header and bezel.
	maxHeight := min(float32(410), max(u.overviewHeight-390, 160))
	if screenHeight > maxHeight {
		screenWidth *= maxHeight / screenHeight
		screenHeight = maxHeight
	}
	bezel := ui.RGB(25, 29, 37)
	rim := ui.RGB(78, 86, 102)
	f.Column().FillWidth().Pad(16).Gap(14).Radius(12).Background(u.c.side).Border(1, u.c.border).Children(func() {
		f.Row().FillWidth().Gap(8).Items(flex.Center).Children(func() {
			f.Text("屏幕截图").FontSize(12).Weight(ui.WeightSemibold).Grow(1)
			if u.overviewIconButton(f, "capture", iconCamera, "保存截图", device.State != "device") {
				u.backend.Capture(device.Serial)
			}
			if u.overviewIconButton(f, "refresh-screen-preview", mottyRotateCw, "刷新截图", !supported || device.State != "device" || preview.Loading) {
				backend.FetchPreview(device.Serial)
			}
		})
		f.Column().FillWidth().Items(flex.Center).Children(func() {
			u.preview.frame = f.Column().Size(screenWidth+8, screenHeight+8).Pad(4).Radius(23).Background(bezel).Border(1, rim).Shrink(0).Children(func() {
				if u.preview.valid {
					u.preview.screen = f.Image(u.preview.bitmap).Size(screenWidth, screenHeight).Shrink(0)
				} else {
					u.preview.screen = f.Column().Size(screenWidth, screenHeight).Radius(19).Center().Gap(14).Background(u.c.bg).Children(func() {
						f.Icon(iconPhone).Size(42, 42).Color(u.c.muted)
						if device.State != "device" {
							f.Text("设备未连接").FontSize(12).Color(u.c.muted)
						} else {
							f.Text("点击刷新截图").FontSize(12).Color(u.c.muted)
						}
					})
				}
			})
		})
		if preview.Loading {
			u.small(f, "正在获取截图…")
		}
		if preview.Error != "" {
			u.small(f, "刷新失败："+preview.Error)
		}
	})
}

// Viva's container clip is rectangular. Mask only the preview bitmap's corner
// alpha once per decoded screenshot; the source PNG used for saving is intact.
func roundedPreviewBitmap(bitmap ui.Bitmap) ui.Bitmap {
	bitmap.Pix = append([]byte(nil), bitmap.Pix...)
	bitmap.Version ^= 0x726f756e
	radius := float64(min(bitmap.Width, bitmap.Height)) * 0.1
	for y := 0; y < int(math.Ceil(radius)); y++ {
		for x := 0; x < int(math.Ceil(radius)); x++ {
			coverage := min(max(radius+0.5-math.Hypot(radius-float64(x)-0.5, radius-float64(y)-0.5), 0), 1)
			if coverage >= 1 {
				continue
			}
			for _, point := range [][2]int{{x, y}, {bitmap.Width - 1 - x, y}, {x, bitmap.Height - 1 - y}, {bitmap.Width - 1 - x, bitmap.Height - 1 - y}} {
				i := point[1]*bitmap.Stride + point[0]*4
				for c := 0; c < 4; c++ {
					bitmap.Pix[i+c] = byte(float64(bitmap.Pix[i+c]) * coverage)
				}
			}
		}
	}
	return bitmap
}

// Explicit hit disabling is required for custom Viva controls.
func (u *UI) overviewIconButton(f *flex.Frame, key string, icon ui.Icon, label string, disabled bool) bool {
	id := u.id(key)
	button := f.Column().Size(26, 26).Shrink(0).Center().Radius(6).Background(u.c.card).HoverBackground(u.c.selected).
		HitWith(id, label, ui.HitOptions{Role: ui.RoleButton, Cursor: ui.PointerCursorHand, Disabled: disabled})
	button.Children(func() { f.Icon(icon).Size(14, 14).Color(u.c.muted) })
	u.overviewTools = append(u.overviewTools, deviceConnectionState{box: button, id: id, label: label})
	return !disabled && button.Clicked()
}
