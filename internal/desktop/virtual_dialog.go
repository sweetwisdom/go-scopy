package desktop

import (
	"strconv"
	"strings"

	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// virtualDialog is the modal that configures and launches a scrcpy
// virtual display: a separate window where one app runs on its own screen.
type virtualDialog struct {
	keyboard                     string
	open                         bool
	settingsOnly, globalDefaults bool
	lastApp                      flex.Field
	serial                       string
	width, height, dpi           flex.Field
	preset                       virtualScreenPreset
	decorations                  bool // default on: system bars are shown
	destroy                      bool // default on: finish virtual-display tasks
	ime                          int  // index into imePolicies
	imeSelect                    flex.SelectState
	selectedApp                  string
	search                       flex.Field
	filter                       string
	rows                         []appMetaRow
	listBox                      flex.Box
}

// imePolicies map the dialog's dropdown to scrcpy's display-ime-policy.
var imePolicies = []struct{ label, value string }{
	{"使用默认策略", ""}, {"输入法显示在虚拟屏幕", "local"},
	{"输入法显示在备用屏幕", "fallback"}, {"隐藏输入法", "hide"},
}

// Opening restores the device's last successful launch settings.
func (u *UI) openVirtualDialog(serial string) { u.openVirtualPreferences(serial, false, false) }

func (u *UI) openVirtualPreferences(serial string, settingsOnly, global bool) {
	u.vd.open = true
	u.vd.serial = serial
	u.vd.settingsOnly = settingsOnly
	u.vd.globalDefaults = global
	prefs := u.state.Settings.VirtualOptionsFor(serial)
	if global {
		prefs = u.state.Settings.VirtualDefaults
		if prefs.Width <= 0 {
			prefs = domain.DefaultVirtualPreferences()
		}
	}
	u.applyVirtualPreferences(prefs)
	u.vd.search.SetText("")
	u.vd.filter = "user"
	u.notice = ""
	u.vd.imeSelect = flex.SelectState{}
	if !global && u.dialogDevice(serial).State == "device" {
		if _, ok := u.state.Apps[serial]; !ok {
			u.backend.FetchApps(serial)
		}
	}
	u.wake.Invalidate()
}

func (u *UI) applyVirtualPreferences(p domain.VirtualPreferences) {
	u.vd.width.SetText(strconv.Itoa(p.Width))
	u.vd.height.SetText(strconv.Itoa(p.Height))
	u.vd.dpi.SetText(strconv.Itoa(p.DPI))
	u.vd.preset = screenPhonePortrait
	switch p.Preset {
	case "landscape":
		u.vd.preset = screenPhoneLandscape
	case "tablet":
		u.vd.preset = screenTabletLandscape
	}
	u.vd.keyboard = p.Keyboard
	u.vd.decorations = p.SystemDecorations
	u.vd.destroy = p.DestroyOnClose
	u.vd.ime = 0
	for i, policy := range imePolicies {
		if policy.value == p.IMEPolicy {
			u.vd.ime = i
		}
	}
	u.vd.selectedApp = p.LastApp
	u.vd.lastApp.SetText(p.LastApp)
}

// virtualDisplayCard declares a root-level modal, never an inline workspace card.
func (u *UI) virtualDisplayCard(f *flex.Frame) {
	if !u.vd.open {
		return
	}
	device := u.dialogDevice(u.vd.serial)
	title, description, width := "虚拟屏幕", device.Name()+" · 为应用创建独立窗口", float32(860)
	if u.vd.settingsOnly {
		title = "虚拟屏幕设置"
	}
	if u.vd.globalDefaults {
		description = "新设备使用的默认参数"
		width = 540
	}
	d := f.Dialog(flex.DialogSpec{Title: title, Description: description, Width: width,
		Scope: u.id("virtual-display-dialog"), DismissID: u.id("vd-cancel"), CloseID: u.id("vd-close")})
	d.Body(func() {
		f.Column().FillWidth().Gap(10).Children(func() {
			f.Row().FillWidth().Gap(22).Items(flex.Start).Children(func() {
				options := f.ScrollArea(u.id("vd-options-scroll")).Width(246).Height(max(280, min(440, u.overviewHeight-260))).Gap(12)
				if u.vd.globalDefaults {
					options = options.FillWidth()
				}
				options.Children(func() { u.virtualDisplayOptions(f) })
				if !u.vd.globalDefaults {
					f.Column().Width(0).Grow(1).Gap(10).Children(func() {
						if u.vd.settingsOnly {
							f.Text("上次启动应用（包名）").FontSize(12)
							f.Field(&u.vd.lastApp, u.id("vd-last-app"), "应用包名", "留空则启动时选择").FillWidth().Height(36)
						}
						u.virtualAppPicker(f, device)
					})
				}
			})
			if u.notice != "" {
				u.small(f, u.notice)
			}
		})
	})
	d.Footer(func() {
		f.Button(u.id("vd-cancel"), "取消").Outline()
		if f.Button(u.id("vd-reset"), "恢复默认参数").Outline().Clicked() {
			app := u.vd.selectedApp
			p := u.state.Settings.VirtualDefaults
			if u.vd.globalDefaults || p.Width <= 0 {
				p = domain.DefaultVirtualPreferences()
			}
			u.applyVirtualPreferences(p)
			if !u.vd.globalDefaults {
				u.vd.selectedApp = app
				u.vd.lastApp.SetText(app)
			}
			u.wake.Invalidate()
		}
		if u.vd.settingsOnly {
			if f.Button(u.id("vd-save-settings"), "保存虚拟屏设置").Clicked() {
				if prefs, ok := u.readVirtualPreferences(); ok {
					u.rememberVirtualPreferences(u.vd.serial, prefs, u.vd.globalDefaults)
					u.vd.open = false
					u.notice = "虚拟屏设置已提交保存"
					u.wake.Invalidate()
				}
			}
		} else if f.Button(u.id("vd-start"), "启动虚拟屏幕").Disabled(device.State != "device" || !u.virtualAppInstalled()).Clicked() {
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
	f.Row().FillWidth().Gap(8).Children(func() {
		for _, option := range []struct {
			label  string
			preset virtualScreenPreset
		}{
			{"竖屏", screenPhonePortrait}, {"横屏", screenPhoneLandscape}, {"平板横屏", screenTabletLandscape},
		} {
			b := f.Button(u.id("vd-orientation-"+option.label), option.label).Small()
			if u.vd.preset != option.preset {
				b = b.Outline()
			}
			if b.Clicked() && u.vd.preset != option.preset {
				u.selectVirtualPreset(option.preset)
			}
		}
	})
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
	if u.vd.preset == screenTabletLandscape {
		f.Wrap("平板横屏 · 大屏布局取决于应用支持。").FillWidth().FontSize(11).Color(u.c.muted)
	}
	f.Column().FillWidth().Gap(6).Children(func() {
		f.Text("键盘输入").FontSize(11).Color(u.c.muted)
		f.Row().FillWidth().Gap(8).Children(func() {
			for _, item := range []struct{ label, mode string }{{"兼容模式", ""}, {"外接键盘", "uhid"}} {
				button := f.Button(u.id("vd-keyboard-"+item.mode), item.label).Small()
				active := u.vd.keyboard == item.mode || (item.mode == "" && u.vd.keyboard == "sdk")
				if !active {
					button = button.Outline()
				}
				if button.Clicked() {
					u.vd.keyboard = item.mode
					u.wake.Invalidate()
				}
			}
		})
		f.Wrap("外接键盘：电脑切英文，手机输入法切中文选词。电脑中文可复制后 Ctrl+V。").FillWidth().FontSize(11).Color(u.c.muted)
		ready := !u.vd.globalDefaults && u.dialogDevice(u.vd.serial).State == "device"
		if ready {
			f.Row().FillWidth().Gap(6).Children(func() {
				if f.Button(u.id("vd-input-settings"), "输入法设置").Small().Ghost().Clicked() {
					u.backend.QuickAction(u.vd.serial, "input-method")
				}
				if f.Button(u.id("vd-keyboard-settings"), "外接键盘设置").Small().Ghost().Clicked() {
					u.backend.QuickAction(u.vd.serial, "physical-keyboard")
				}
			})
		}
	})
	for _, option := range []struct {
		label, key, description string
		value                   *bool
	}{
		{"系统装饰", "vd-decorations", "显示状态栏和导航栏", &u.vd.decorations},
		{"关闭时结束应用任务", "vd-destroy", "默认结束虚拟屏任务；关闭此选项会移回手机主屏", &u.vd.destroy},
	} {
		f.Column().FillWidth().Gap(5).Children(func() {
			f.Row().FillWidth().Items(flex.Center).Children(func() {
				f.Text(option.label).Width(0).Grow(1).FontSize(12)
				if f.Switch(u.id(option.key), *option.value).Clicked() {
					*option.value = !*option.value
					u.wake.Invalidate()
				}
			})
			f.Wrap(option.description).FillWidth().FontSize(11).Color(u.c.muted)
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

}

func (u *UI) virtualAppPicker(f *flex.Frame, device domain.Device) {
	listing := u.state.Apps[device.Serial]
	if u.vd.selectedApp != "" && !u.virtualAppInstalled() && !listing.Refreshing && len(listing.Apps) > 0 {
		u.small(f, "上次应用当前不可用，请重新选择。")
	}
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
				u.vd.lastApp.SetText(a.Package)
				u.wake.Invalidate()
			}
			meta := u.appMeta(device.Serial, a.Package)
			if device.State == "device" && !a.System && !meta.Resolved && meta.Error == "" {
				u.vd.rows = append(u.vd.rows, appMetaRow{box: row, serial: device.Serial, app: a})
			}
		}
	})
}

// launchVirtualDialog starts an independent app window without an island.
func (u *UI) launchVirtualDialog() {
	prefs, ok := u.readVirtualPreferences()
	if !ok {
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
		Width: prefs.Width, Height: prefs.Height, DPI: prefs.DPI,
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
	u.rememberVirtualPreferences(u.vd.serial, prefs, false)
	u.vd.open = false
	options := u.state.Settings.OptionsFor(u.vd.serial)
	options.Keyboard = prefs.Keyboard
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

// The tablet preset exposes an 800dp shortest side, allowing Android apps to
// select their large-screen resources without changing the physical display.
type virtualScreenPreset uint8

const (
	screenPhonePortrait virtualScreenPreset = iota
	screenPhoneLandscape
	screenTabletLandscape
)

func virtualDisplayDefaults(preset virtualScreenPreset) domain.VirtualDisplay {
	display := domain.VirtualDisplay{Width: 1080, Height: 2400, DPI: 420, SystemDecorations: true, DestroyOnClose: true}
	switch preset {
	case screenPhoneLandscape:
		display.Width, display.Height = 2400, 1080
	case screenTabletLandscape:
		display.Width, display.Height, display.DPI = 1920, 1200, 240
	}
	return display
}

func (u *UI) selectVirtualPreset(preset virtualScreenPreset) {
	defaults := virtualDisplayDefaults(preset)
	if preset == screenTabletLandscape || u.vd.preset == screenTabletLandscape {
		u.vd.width.SetText(strconv.Itoa(defaults.Width))
		u.vd.height.SetText(strconv.Itoa(defaults.Height))
		u.vd.dpi.SetText(strconv.Itoa(defaults.DPI))
	} else {
		width, widthErr := strconv.Atoi(strings.TrimSpace(u.vd.width.Text()))
		height, heightErr := strconv.Atoi(strings.TrimSpace(u.vd.height.Text()))
		if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
			width, height = defaults.Width, defaults.Height
		}
		if (preset == screenPhonePortrait && width > height) || (preset == screenPhoneLandscape && width < height) {
			width, height = height, width
		}
		u.vd.width.SetText(strconv.Itoa(width))
		u.vd.height.SetText(strconv.Itoa(height))
	}
	u.vd.preset = preset
	if preset == screenTabletLandscape {
		u.vd.keyboard = "uhid"
		for i, policy := range imePolicies {
			if policy.value == "local" {
				u.vd.ime = i
			}
		}
	}
	u.wake.Invalidate()
}

func (u *UI) readVirtualPreferences() (domain.VirtualPreferences, bool) {
	p := domain.VirtualPreferences{Keyboard: u.vd.keyboard, Preset: virtualPresetKey(u.vd.preset), SystemDecorations: u.vd.decorations, DestroyOnClose: u.vd.destroy, LastApp: u.vd.selectedApp}
	var err error
	if p.Width, err = strconv.Atoi(strings.TrimSpace(u.vd.width.Text())); err != nil {
		u.notice = "虚拟屏宽度必须是正整数"
		u.wake.Invalidate()
		return p, false
	}
	if p.Height, err = strconv.Atoi(strings.TrimSpace(u.vd.height.Text())); err != nil {
		u.notice = "虚拟屏高度必须是正整数"
		u.wake.Invalidate()
		return p, false
	}
	if p.DPI, err = strconv.Atoi(strings.TrimSpace(u.vd.dpi.Text())); err != nil {
		u.notice = "DPI 必须是非负整数"
		u.wake.Invalidate()
		return p, false
	}
	if u.vd.ime >= 0 && u.vd.ime < len(imePolicies) {
		p.IMEPolicy = imePolicies[u.vd.ime].value
	}
	if u.vd.settingsOnly {
		p.LastApp = strings.TrimSpace(u.vd.lastApp.Text())
	}
	if u.vd.globalDefaults {
		p.LastApp = ""
	}
	if err = config.ValidateVirtualPreferences(p); err != nil {
		u.notice = err.Error()
		u.wake.Invalidate()
		return p, false
	}
	return p, true
}

func (u *UI) rememberVirtualPreferences(serial string, p domain.VirtualPreferences, global bool) {
	settings := u.state.Settings
	if global {
		p.LastApp = ""
		settings.VirtualDefaults = p
	} else {
		profiles := make(map[string]domain.VirtualPreferences, len(settings.VirtualDevices)+1)
		for key, value := range settings.VirtualDevices {
			profiles[key] = value
		}
		profiles[serial] = p
		settings.VirtualDevices = profiles
	}
	u.state.Settings = settings
	u.backend.Save(settings)
}

func virtualPresetKey(preset virtualScreenPreset) string {
	switch preset {
	case screenPhoneLandscape:
		return "landscape"
	case screenTabletLandscape:
		return "tablet"
	}
	return "portrait"
}
