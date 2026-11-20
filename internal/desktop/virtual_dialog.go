package desktop

import (
	"strconv"
	"strings"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// virtualDialog is the modal that configures and launches a scrcpy
// virtual display: a separate window where one app runs on its own screen.
type virtualDialog struct {
	open               bool
	serial             string
	width, height, dpi flex.Field
	decorations        bool // default on: system bars are shown
	destroy            bool // default off: content moves to the main screen
	ime                int  // index into imePolicies
	imeSelect          flex.SelectState
	selectedApp        string
	search             flex.Field
	filter             string
	rows               []appMetaRow
	listBox            flex.Box
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
	u.vd.selectedApp = ""
	u.vd.search.SetText("")
	u.vd.filter = "user"
	u.notice = ""
	u.vd.imeSelect = flex.SelectState{}
	// Reuse the device's shared app listing; only fetch if it has not been
	// loaded yet (for example, when the app tab has never been opened).
	if _, ok := u.state.Apps[serial]; !ok {
		u.backend.FetchApps(serial)
	}
	u.wake.Invalidate()
}

// virtualDisplayCard declares a root-level modal, never an inline workspace card.
func (u *UI) virtualDisplayCard(f *flex.Frame) {
	if !u.vd.open {
		return
	}
	device := u.dialogDevice(u.vd.serial)
	d := f.Dialog(flex.DialogSpec{Title: "虚拟屏幕", Description: device.Name() + " · 为应用创建独立窗口", Width: 860,
		Scope: u.id("virtual-display-dialog"), DismissID: u.id("vd-cancel"), CloseID: u.id("vd-close")})
	d.Body(func() {
		f.Column().FillWidth().Gap(10).Children(func() {
			f.Row().FillWidth().Gap(22).Items(flex.Start).Children(func() {
				f.Column().Width(246).Gap(16).Children(func() { u.virtualDisplayOptions(f) })
				f.Column().Width(0).Grow(1).Gap(10).Children(func() { u.virtualAppPicker(f, device) })
			})
			if u.notice != "" {
				u.small(f, u.notice)
			}
		})
	})
	d.Footer(func() {
		f.Button(u.id("vd-cancel"), "取消").Outline()
		if f.Button(u.id("vd-reset"), "重置为默认值").Outline().Clicked() {
			u.openVirtualDialog(u.vd.serial)
		}
		if f.Button(u.id("vd-start"), "启动虚拟屏幕").Disabled(device.State != "device" || !u.virtualAppInstalled()).Clicked() {
			u.launchVirtualDialog()
		}
	})
	if d.Dismissed() {
		u.vd.open = false
		u.wake.Invalidate()
	}
}

func (u *UI) virtualDisplayOptions(f *flex.Frame) {
	f.Text("显示参数").FontSize(14).Weight(ui.WeightSemibold)
	f.Row().FillWidth().Gap(10).Children(func() {
		for _, field := range []struct {
			label, key string
			value      *flex.Field
		}{{"宽度", "vd-width", &u.vd.width}, {"高度", "vd-height", &u.vd.height}, {"DPI", "vd-dpi", &u.vd.dpi}} {
			f.Column().Width(0).Grow(1).Gap(6).Children(func() {
				f.Text(field.label).FontSize(11).Color(u.c.muted)
				f.Field(field.value, u.id(field.key), field.label, "").FillWidth().Height(36)
			})
		}
	})
	for _, option := range []struct {
		label, key, description string
		value                   *bool
	}{
		{"系统装饰", "vd-decorations", "显示状态栏和导航栏", &u.vd.decorations},
		{"关闭时销毁内容", "vd-destroy", "关闭后销毁，否则移回主屏幕", &u.vd.destroy},
	} {
		f.Column().FillWidth().Gap(5).Children(func() {
			f.Row().FillWidth().Items(flex.Center).Children(func() {
				f.Text(option.label).Width(0).Grow(1).FontSize(12)
				if f.Switch(u.id(option.key), *option.value).Clicked() {
					*option.value = !*option.value
					u.wake.Invalidate()
				}
			})
			u.small(f, option.description)
		})
	}
	f.Column().FillWidth().Gap(6).Children(func() {
		f.Text("输入法策略").FontSize(11).Color(u.c.muted)
		labels := make([]string, len(imePolicies))
		for i, p := range imePolicies {
			labels[i] = p.label
		}
		if i, ok := f.Select(u.tree.Feature("vd-ime"), &u.vd.imeSelect, labels, u.vd.ime).FillWidth().Picked(); ok {
			u.vd.ime = i
			u.wake.Invalidate()
		}
	})
	u.small(f, "可同时运行多个虚拟屏幕，不影响普通投屏。")
}

func (u *UI) virtualAppPicker(f *flex.Frame, device domain.Device) {
	listing := u.state.Apps[device.Serial]
	f.Row().FillWidth().Gap(8).Items(flex.Center).Children(func() {
		f.Text("启动应用").FontSize(14).Weight(ui.WeightSemibold).Grow(1)
		if f.Button(u.id("vd-refresh-apps"), "刷新应用").Small().Outline().Disabled(device.State != "device" || listing.Refreshing).Clicked() {
			u.backend.FetchApps(device.Serial)
		}
	})
	f.Field(&u.vd.search, u.id("vd-app-search"), "搜索应用名称或包名", "搜索应用名称或包名").FillWidth().Height(36)
	f.Row().FillWidth().Gap(6).Children(func() {
		for _, kind := range []struct{ key, label string }{{"all", "全部"}, {"user", "用户"}, {"system", "系统"}} {
			button := f.Button(u.id("vd-filter-"+kind.key), kind.label).Small()
			if u.vd.filter != kind.key {
				button = button.Ghost()
			}
			if button.Clicked() {
				u.vd.filter = kind.key
				u.wake.Invalidate()
			}
		}
	})
	apps := u.filterAppList(device.Serial, listing.Apps, u.vd.search.Text(), u.vd.filter)
	u.vd.listBox = f.ScrollArea(u.id("vd-app-list")).FillWidth().Height(270).Gap(4).Children(func() {
		if listing.Error != "" {
			u.small(f, "读取失败："+listing.Error)
		}
		if len(apps) == 0 {
			message := "没有匹配的应用"
			if listing.Refreshing {
				message = "正在读取应用列表…"
			}
			u.small(f, message)
		}
		for _, a := range apps {
			selected := a.Package == u.vd.selectedApp
			bg := ui.Color{}
			if selected {
				bg = u.c.selected
			}
			row := f.Row().FillWidth().Height(54).PadX(8).Gap(10).Items(flex.Center).Radius(8).Background(bg).HoverBackground(u.c.selected).
				Hit(u.id("vd-app-"+a.Package), "选择应用 · "+u.virtualAppLabel(device.Serial, a.Package)+" · "+a.Package).Children(func() {
				u.appIdentity(f, device.Serial, a)
				if selected {
					f.Text("已选").FontSize(11).Color(u.c.blue).Shrink(0)
				}
			})
			if row.Clicked() {
				u.vd.selectedApp = a.Package
				u.wake.Invalidate()
			}
			meta := u.appMeta(device.Serial, a.Package)
			if device.State == "device" && !meta.Resolved && meta.Error == "" {
				u.vd.rows = append(u.vd.rows, appMetaRow{box: row, serial: device.Serial, app: a})
			}
		}
	})
}

// launchVirtualDialog starts an independent app window without an island.
func (u *UI) launchVirtualDialog() {
	width, widthErr := strconv.Atoi(strings.TrimSpace(u.vd.width.Text()))
	height, heightErr := strconv.Atoi(strings.TrimSpace(u.vd.height.Text()))
	dpi, dpiErr := strconv.Atoi(strings.TrimSpace(u.vd.dpi.Text()))
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		u.notice = "虚拟屏幕宽度和高度必须是正整数"
		u.wake.Invalidate()
		return
	}
	if dpiErr != nil || dpi < 0 {
		u.notice = "DPI 必须是非负整数，0 表示跟随设备"
		u.wake.Invalidate()
		return
	}
	if u.dialogDevice(u.vd.serial).State != "device" {
		u.notice = "设备未连接，请重新连接后启动"
		u.wake.Invalidate()
		return
	}
	if u.vd.ime < 0 || u.vd.ime >= len(imePolicies) {
		u.vd.ime = 0
	}
	display := domain.VirtualDisplay{
		Width: width, Height: height, DPI: dpi,
		SystemDecorations: u.vd.decorations,
		DestroyOnClose:    u.vd.destroy,
		IMEPolicy:         imePolicies[u.vd.ime].value,
	}
	display.StartApp = u.vd.selectedApp
	if !u.virtualAppInstalled() {
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

// Keep selection by package when search, sorting or asynchronous metadata changes.
func (u *UI) virtualAppInstalled() bool {
	if u.vd.selectedApp == "" {
		return false
	}
	for _, a := range u.state.Apps[u.vd.serial].Apps {
		if a.Package == u.vd.selectedApp {
			return true
		}
	}
	return false
}
