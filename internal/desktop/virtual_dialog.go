package desktop

import (
	"strconv"
	"strings"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// virtualDialog is the inline card that configures and launches a scrcpy
// virtual display: a separate window where one app runs on its own screen.
type virtualDialog struct {
	open               bool
	serial             string
	width, height, dpi flex.Field
	decorations        bool // default on: system bars are shown
	destroy            bool // default off: content moves to the main screen
	ime                int  // index into imePolicies
	app                int  // index into the app picker
	imeSelect          flex.SelectState
	appSelect          flex.SelectState
	appPackages        []string
}

// imePolicies map the dialog's dropdown to scrcpy's display-ime-policy.
var imePolicies = []struct{ label, value string }{
	{"使用默认策略", ""}, {"输入法显示在虚拟屏幕", "local"},
	{"输入法显示在备用屏幕", "fallback"}, {"隐藏输入法", "hide"},
}

// openVirtualDialog resets the dialog to sane defaults for one device.
func (u *UI) openVirtualDialog(serial string) {
	u.vd.open = true
	u.vd.serial = serial
	u.vd.width.SetText("1080")
	u.vd.height.SetText("2400")
	u.vd.dpi.SetText("420")
	u.vd.decorations = true
	u.vd.destroy = false
	u.vd.ime = 0
	u.vd.app = 0
	u.vd.appSelect = flex.SelectState{}
	u.vd.imeSelect = flex.SelectState{}
	// Reuse the device's shared app listing; only fetch if it has not been
	// loaded yet (for example, when the app tab has never been opened).
	if _, ok := u.state.Apps[serial]; !ok {
		u.backend.FetchApps(serial)
	}
	u.wake.Invalidate()
}

// virtualDisplayCard paints the dialog above the page content while open.
func (u *UI) virtualDisplayCard(f *flex.Frame) {
	if !u.vd.open {
		return
	}
	u.card(f, func() {
		f.Row().FillWidth().Items(flex.Center).Children(func() {
			f.Column().Grow(1).Gap(4).Children(func() {
				f.Text("配置虚拟显示").FontSize(18).Weight(ui.WeightSemibold)
				u.small(f, "每次启动创建一个独立窗口，可与其他虚拟屏幕同时运行。")
			})
			if f.Button(u.id("vd-close"), "✕").Ghost().Small().Clicked() {
				u.vd.open = false
				u.wake.Invalidate()
			}
		})

		f.Text("分辨率配置").FontSize(13).Weight(ui.WeightSemibold).MarginSides(0, 0, 10, 0)
		f.Row().FillWidth().Gap(14).Children(func() {
			f.Column().Grow(1).Gap(6).Children(func() {
				f.Text("宽度").FontSize(11).Color(u.c.muted)
				f.Field(&u.vd.width, u.id("vd-width"), "1080", "").Height(36)
			})
			f.Column().Grow(1).Gap(6).Children(func() {
				f.Text("高度").FontSize(11).Color(u.c.muted)
				f.Field(&u.vd.height, u.id("vd-height"), "2400", "").Height(36)
			})
			f.Column().Grow(1).Gap(6).Children(func() {
				f.Text("DPI").FontSize(11).Color(u.c.muted)
				f.Field(&u.vd.dpi, u.id("vd-dpi"), "420", "").Height(36)
			})
		})

		f.Row().FillWidth().Gap(12).Items(flex.Center).Children(func() {
			f.Column().Grow(1).Gap(3).Children(func() {
				f.Text("系统装饰").FontSize(13).Weight(ui.WeightMedium)
				u.small(f, "显示状态栏和导航栏（默认：开启）")
			})
			if f.Switch(u.id("vd-decorations"), u.vd.decorations).Clicked() {
				u.vd.decorations = !u.vd.decorations
				u.wake.Invalidate()
			}
		})
		f.Row().FillWidth().Gap(12).Items(flex.Center).Children(func() {
			f.Column().Grow(1).Gap(3).Children(func() {
				f.Text("关闭时销毁内容").FontSize(13).Weight(ui.WeightMedium)
				u.small(f, "关闭时销毁虚拟屏幕内容，否则移动到主屏幕（默认：否）")
			})
			if f.Switch(u.id("vd-destroy"), u.vd.destroy).Clicked() {
				u.vd.destroy = !u.vd.destroy
				u.wake.Invalidate()
			}
		})

		f.Column().FillWidth().Gap(6).Children(func() {
			f.Text("输入法策略").FontSize(11).Color(u.c.muted)
			labels := make([]string, len(imePolicies))
			for i, p := range imePolicies {
				labels[i] = p.label
			}
			if i, ok := f.Select(u.tree.Feature("vd-ime"), &u.vd.imeSelect, labels, u.vd.ime).Picked(); ok {
				u.vd.ime = i
				u.wake.Invalidate()
			}
		})

		// App picker: package names labelled by whatever meta has arrived.
		apps := u.state.Apps[u.vd.serial]
		u.vd.appPackages = u.vd.appPackages[:0]
		labels := []string{}
		for _, a := range apps.Apps {
			if a.System {
				continue
			}
			name := u.virtualAppLabel(u.vd.serial, a.Package)
			u.vd.appPackages = append(u.vd.appPackages, a.Package)
			labels = append(labels, name)
		}
		if len(labels) == 0 {
			labels = []string{"（没有用户应用）"}
		}
		f.Column().FillWidth().Gap(6).Children(func() {
			f.Text("启动应用").FontSize(11).Color(u.c.muted)
			if i, ok := f.Select(u.tree.Feature("vd-app"), &u.vd.appSelect, labels, u.vd.app).Picked(); ok {
				u.vd.app = i
				u.wake.Invalidate()
			}
			u.small(f, "选择要在虚拟屏幕中运行的应用（必选）")
		})

		f.Row().FillWidth().Gap(10).Children(func() {
			f.Stack().Grow(1)
			if f.Button(u.id("vd-reset"), "重置为默认值").Outline().Small().Clicked() {
				u.openVirtualDialog(u.vd.serial)
			}
			if f.Button(u.id("vd-start"), "启动").Small().Disabled(len(u.vd.appPackages) == 0).Clicked() {
				u.launchVirtualDialog()
			}
		})
	})
}

// launchVirtualDialog starts the virtual session beside its island window.
func (u *UI) launchVirtualDialog() {
	width, _ := strconv.Atoi(strings.TrimSpace(u.vd.width.Text()))
	height, _ := strconv.Atoi(strings.TrimSpace(u.vd.height.Text()))
	dpi, _ := strconv.Atoi(strings.TrimSpace(u.vd.dpi.Text()))
	display := domain.VirtualDisplay{
		Width: width, Height: height, DPI: dpi,
		SystemDecorations: u.vd.decorations,
		DestroyOnClose:    u.vd.destroy,
		IMEPolicy:         imePolicies[u.vd.ime].value,
	}
	if u.vd.app >= 0 && u.vd.app < len(u.vd.appPackages) {
		display.StartApp = u.vd.appPackages[u.vd.app]
	}
	if display.StartApp == "" {
		u.notice = "请选择要在虚拟屏幕中运行的应用"
		u.wake.Invalidate()
		return
	}
	u.vd.open = false
	options := u.state.Settings.OptionsFor(u.vd.serial)
	if !u.openSessionWindow(u.vd.serial, domain.SessionVirtual, &display, options, false) {
		u.backend.StartVirtual(u.vd.serial, options, display, false)
	}
}
