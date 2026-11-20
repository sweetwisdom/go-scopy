package desktop

import (
	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
)

func (u *UI) deviceDialogOpen() bool {
	return u.renameSerial != "" || u.deleteSerial != "" || u.optionsSerial != "" || u.clearAppCache || u.vd.open || u.overviewDetailsSerial != ""
}

func (u *UI) openDeviceEditor(d domain.Device) {
	u.renameSerial = d.Serial
	u.deviceName.SetText(d.Alias)
	u.deviceAddress.SetText(d.Serial)
	u.notice = ""
	u.wake.Invalidate()
}

func (u *UI) dialogDevice(serial string) domain.Device {
	for _, d := range u.state.Devices {
		if d.Serial == serial {
			return d
		}
	}
	connection := "USB"
	if u.state.Settings.SavedWireless[serial] {
		connection = "Wi-Fi"
	}
	return domain.Device{Serial: serial, Alias: u.state.Settings.DeviceNames[serial], Connection: connection, State: "offline"}
}

func (u *UI) deviceDialogs(f *flex.Frame) {
	u.overviewDetailsDialog(f)
	u.appCacheDialog(f)
	u.virtualDisplayCard(f)
	if u.renameSerial != "" {
		device := u.dialogDevice(u.renameSerial)
		d := f.Dialog(flex.DialogSpec{Title: "编辑设备", Description: device.Name(), Width: 480,
			Scope: u.id("edit-device-dialog"), DismissID: u.id("cancel-device-name"), CloseID: u.id("close-device-editor")})
		d.Body(func() {
			f.Column().FillWidth().Gap(12).Children(func() {
				u.small(f, device.Serial)
				if device.Connection == "Wi-Fi" {
					f.Text("连接地址").FontSize(12)
					f.Field(&u.deviceAddress, u.id("edit-device-address"), "连接地址", "IP:端口").FillWidth().Height(38)
				}
				f.Text("设备备注").FontSize(12)
				f.Field(&u.deviceName, u.id("device-name"), "设备备注", "例如：我的手机").FillWidth().Height(38)
				u.small(f, "备注仅保存在本机，最多 40 个字符；留空恢复原名称。")
				if device.Connection == "Wi-Fi" {
					u.small(f, "修改地址只保存本机记录，不会断开或迁移已有连接。")
				}
				if u.notice != "" {
					u.small(f, u.notice)
				}
			})
		})
		d.Footer(func() {
			f.Button(u.id("cancel-device-name"), "取消").Outline()
			if f.Button(u.id("save-device-name"), "保存设备").Clicked() {
				u.saveDeviceRecord(device)
			}
		})
		if d.Dismissed() {
			u.renameSerial = ""
			u.notice = ""
			u.wake.Invalidate()
		}
	}
	if u.deleteSerial != "" {
		device := u.dialogDevice(u.deleteSerial)
		d := f.Dialog(flex.DialogSpec{Title: "删除设备记录？", Description: device.Name(), Width: 480,
			Scope: u.id("delete-device-dialog"), DismissID: u.id("cancel-forget-device"), CloseID: u.id("close-forget-device")})
		d.Body(func() {
			f.Column().FillWidth().Gap(12).Children(func() {
				u.small(f, device.Serial)
				u.small(f, "仅删除本机保存的无线地址、设备备注及投屏参数，不会断开连接或删除手机数据。")
				u.small(f, "在线设备仍会由 ADB 自动发现，删除记录后可能继续显示在列表中。")
			})
		})
		d.Footer(func() {
			f.Button(u.id("cancel-forget-device"), "保留记录").Outline()
			if f.Button(u.id("confirm-forget-device"), "确认删除记录").Destructive().Clicked() {
				u.backend.Save(config.ForgetDevice(u.state.Settings, device.Serial))
				u.deleteSerial = ""
				u.wake.Invalidate()
			}
		})
		if d.Dismissed() {
			u.deleteSerial = ""
			u.wake.Invalidate()
		}
	}
	if u.optionsSerial != "" {
		d := f.Dialog(flex.DialogSpec{Title: "投屏参数", Description: u.dialogDevice(u.optionsSerial).Name() + " · 下一次启动时生效", Width: 560,
			Scope: u.id("projection-options-dialog"), DismissID: u.id("cancel-projection-options"), CloseID: u.id("close-projection-options")})
		d.Body(func() {
			f.Column().FillWidth().Gap(18).Children(func() {
				f.Row().FillWidth().Gap(14).Children(func() {
					u.optionField(f, "分辨率上限 (px)", &u.size, "size")
					u.optionField(f, "帧率 (fps)", &u.fps, "fps")
					u.optionField(f, "码率 (Mbps)", &u.bitrate, "bitrate")
				})
				f.Row().FillWidth().Gap(24).Children(func() {
					u.toggle(f, "audio", "传输音频", &u.options.Audio)
					u.toggle(f, "screen-off", "投屏时熄屏", &u.options.ScreenOff)
				})
				f.Row().FillWidth().Gap(24).Children(func() {
					u.toggle(f, "awake", "保持唤醒", &u.options.StayAwake)
					u.toggle(f, "top", "窗口置顶", &u.options.AlwaysOnTop)
				})
				if u.notice != "" {
					u.small(f, u.notice)
				}
			})
		})
		d.Footer(func() {
			f.Button(u.id("cancel-projection-options"), "取消").Outline()
			if f.Button(u.id("default-options"), "设为全局默认").Outline().Clicked() {
				u.saveOptions(true)
			}
			if f.Button(u.id("save-options"), "保存此设备参数").Clicked() {
				u.saveOptions(false)
			}
		})
		if d.Dismissed() {
			u.optionsSerial = ""
			u.loadOptions()
			u.notice = ""
			u.wake.Invalidate()
		}
	}
}
