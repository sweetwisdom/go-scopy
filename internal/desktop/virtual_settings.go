package desktop

import (
	"fmt"
	"sort"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

func virtualPresetName(p domain.VirtualPreferences) string {
	switch p.Preset {
	case "landscape":
		return "手机横屏"
	case "tablet":
		return "平板横屏"
	}
	return "手机竖屏"
}

func (u *UI) virtualSettings(f *flex.Frame) {
	u.heading(f, "设置 / 虚拟屏幕", "记住每台设备的使用习惯。", "启动时自动保存显示参数和所选应用；关闭窗口默认结束虚拟屏任务。")
	u.card(f, func() {
		f.Text("新设备默认参数").FontSize(16).Weight(ui.WeightSemibold)
		prefs := u.state.Settings.VirtualDefaults
		if prefs.Width <= 0 {
			prefs = domain.DefaultVirtualPreferences()
		}
		u.small(f, fmt.Sprintf("%s · %d×%d / %d DPI", virtualPresetName(prefs), prefs.Width, prefs.Height, prefs.DPI))
		if f.Button(u.id("virtual-defaults-edit"), "编辑默认参数").Outline().Clicked() {
			u.openVirtualPreferences("", true, true)
		}
	})
	serials := map[string]bool{}
	for serial := range u.state.Settings.VirtualDevices {
		serials[serial] = true
	}
	for _, d := range u.state.Devices {
		serials[d.Serial] = true
	}
	keys := make([]string, 0, len(serials))
	for serial := range serials {
		keys = append(keys, serial)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		u.small(f, "连接设备后可设置独立参数。")
	}
	for _, serial := range keys {
		u.card(f, func() {
			d := u.dialogDevice(serial)
			prefs := u.state.Settings.VirtualOptionsFor(serial)
			f.Text(d.Name()).FontSize(15).Weight(ui.WeightMedium)
			u.small(f, serial)
			u.small(f, fmt.Sprintf("%s · %d×%d / %d DPI", virtualPresetName(prefs), prefs.Width, prefs.Height, prefs.DPI))
			closeLabel := "关闭后结束应用任务"
			if !prefs.DestroyOnClose {
				closeLabel = "关闭后移回手机主屏"
			}
			u.small(f, closeLabel)
			label := "尚未选择应用"
			if prefs.LastApp != "" {
				label = "上次应用：" + u.virtualAppLabel(serial, prefs.LastApp)
			}
			u.small(f, label)
			f.Row().Gap(10).Children(func() {
				if f.Button(u.id("virtual-device-edit-"+serial), "编辑设备配置").Outline().Clicked() {
					u.openVirtualPreferences(serial, true, false)
				}
				_, saved := u.state.Settings.VirtualDevices[serial]
				if f.Button(u.id("virtual-device-reset-"+serial), "使用默认参数").Ghost().Disabled(!saved).Clicked() {
					settings := u.state.Settings
					profiles := make(map[string]domain.VirtualPreferences, len(settings.VirtualDevices))
					for key, value := range settings.VirtualDevices {
						if key != serial {
							profiles[key] = value
						}
					}
					settings.VirtualDevices = profiles
					u.state.Settings = settings
					u.backend.Save(settings)
					u.wake.Invalidate()
				}
			})
		})
	}
}
