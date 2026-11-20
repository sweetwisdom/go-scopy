package desktop

import (
	"fmt"
	"strconv"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// devicePageKeys are the device workspace's tabs, in display order.
var devicePageKeys = []struct{ key, label string }{
	{"overview", "设备概况"}, {"files", "文件管理"}, {"packages", "应用包"},
	{"terminal", "设备终端"}, {"scripts", "Python 脚本"}, {"settings", "设备设置"},
}

// device hosts the management tabs of one device.
func (u *UI) device(f *flex.Frame, d domain.Device) {
	u.deviceHeading(f, d)
	u.deviceVirtualSessions(f, d.Serial)
	u.deviceTabs(f, d)
	switch u.deviceTab {
	case "overview":
		u.deviceOverview(f, d)
	case "files":
		u.deviceFiles(f, d)
	case "packages":
		u.devicePackages(f, d)
	case "terminal":
		u.deviceTerminalPage(f, d)
	case "scripts":
		u.deviceScripts(f, d)
	default:
		u.deviceSettings(f, d)
	}
}

// deviceHeading separates device identity from session actions to keep the
// workspace header calm and easy to scan.
func (u *UI) deviceHeading(f *flex.Frame, d domain.Device) {
	f.Column().FillWidth().Gap(8).MarginSides(0, 0, 14, 0).Children(func() {
		f.Text("工作台 / 设备").FontSize(11).Color(u.c.muted)
		f.Text(d.Name()).FillWidth().Ellipsis().FontSize(26).Weight(ui.WeightSemibold)
		u.small(f, "管理这台设备的投屏会话与常用操作。")
		ready := d.State == "device"
		f.Row().FillWidth().Gap(8).Items(flex.Center).Children(func() {
			if f.Button(u.id("device-mirror"), "投屏").Small().Disabled(!ready).Clicked() {
				u.start(false)
			}
			if f.Button(u.id("open-virtual"), "虚拟屏幕").Small().Outline().Disabled(!ready).Clicked() {
				u.openVirtualDialog(d.Serial)
			}
		})
	})
}

func (u *UI) deviceTabs(f *flex.Frame, d domain.Device) {
	f.Row().FillWidth().Height(42).Gap(4).Items(flex.Center).MarginSides(0, 0, 14, 0).Children(func() {
		for _, tab := range devicePageKeys {
			if u.deviceTabButton(f, tab.key, tab.label) {
				u.deviceTab = tab.key
				switch tab.key {
				case "overview":
					if _, ok := u.state.PhoneInfos[d.Serial]; !ok {
						u.backend.FetchPhoneInfo(d.Serial)
					}
				case "packages":
					if _, ok := u.state.Apps[d.Serial]; !ok {
						u.backend.FetchApps(d.Serial)
					}
				case "files":
					if u.files.dir == "" {
						u.files.dir = "/sdcard"
					}
					if _, ok := u.state.Files[d.Serial+"\x00"+u.files.dir]; !ok {
						u.backend.FetchFiles(d.Serial, u.files.dir)
					}
				}
				u.wake.Invalidate()
			}
		}
		f.Stack().Grow(1)
	})
}

func (u *UI) deviceTabButton(f *flex.Frame, key, label string) bool {
	active := u.deviceTab == key
	fg := u.c.muted
	underline := ui.Color{}
	if active {
		fg = u.c.blue
		underline = u.c.blue
	}
	b := f.Column().Height(38).PadX(12).Gap(6).Center().HoverBackground(u.c.selected).
		Hit(u.id("tab-"+key), label).Children(func() {
		f.Text(label).FontSize(13).Weight(ui.WeightSemibold).Color(fg)
		f.Stack().FillWidth().Height(2).Radius(1).Background(underline)
	})
	return b.Clicked()
}

// deviceOverview is the info-and-shortcuts tab: device, network, battery,
// memory and storage cards plus the ADB quick actions.
func (u *UI) deviceOverview(f *flex.Frame, d domain.Device) {
	info := u.state.PhoneInfos[d.Serial]
	f.Row().FillWidth().Items(flex.Center).Children(func() {
		f.Icon(iconPhone).Size(18, 18).Color(u.c.blue)
		f.Text("设备概览").FontSize(15).Weight(ui.WeightSemibold).Grow(1).MarginSides(8, 0, 0, 0)
		if f.Button(u.id("refresh-overview"), "刷新").Small().Clicked() {
			u.backend.FetchPhoneInfo(d.Serial)
		}
	})
	if info.Error != "" {
		u.card(f, func() { u.small(f, "读取设备信息失败："+info.Error) })
		return
	}
	if info.Model == "" && info.Manufacturer == "" {
		u.small(f, "正在读取设备信息…")
		u.backend.FetchPhoneInfo(d.Serial)
	}
	f.Row().FillWidth().Gap(16).Children(func() {
		u.overviewCard(f, "设备信息", iconPhone, [][2]string{
			{"制造商", info.Manufacturer}, {"型号", info.Model},
			{"Android 版本", info.AndroidVersion + apiSuffix(info.APILevel)},
		}, true)
		u.overviewCard(f, "网络信息", iconPhone, [][2]string{
			{"WiFi", info.WiFiSSID}, {"IP 地址", info.IPAddress},
		}, true)
	})
	f.Row().FillWidth().Gap(16).Children(func() {
		u.overviewCard(f, "电池信息", iconPhone, [][2]string{
			{"电量", batteryText(info.BatteryLevel)}, {"状态", info.BatteryStatus},
		}, true)
		mem := [][2]string{{"内存", memoryText(info.MemTotalKB, info.MemAvailableKB)}}
		u.overviewMeterCard(f, "内存使用", mem, memoryRatio(info.MemTotalKB, info.MemAvailableKB))
	})
	u.overviewMeterCard(f, "存储使用", [][2]string{
		{"存储", storageText(info.StorageTotalBytes, info.StorageUsedBytes)},
	}, storageRatio(info.StorageTotalBytes, info.StorageUsedBytes))

	u.card(f, func() {
		f.Text("快捷操作").FontSize(16).Weight(ui.WeightSemibold)
		u.small(f, "通过 ADB 展开/收起状态栏、打开常用系统设置；部分机型对状态栏命令支持可能不同。")
		ready := d.State == "device"
		f.Row().FillWidth().Gap(10).Items(flex.Center).Children(func() {
			f.Text("状态栏").FontSize(12).Color(u.c.muted).Grow(1)
			for _, a := range []struct{ action, label string }{
				{"expand-notifications", "展开通知"}, {"expand-settings", "快捷设置"}, {"collapse", "收起"},
			} {
				if f.Button(u.id("qa-"+a.action), a.label).Small().Outline().Disabled(!ready).Clicked() {
					u.backend.QuickAction(d.Serial, a.action)
				}
			}
		})
		f.Row().FillWidth().Gap(10).Items(flex.Center).Children(func() {
			f.Text("常用设置").FontSize(12).Color(u.c.muted).Grow(1)
			for _, a := range []struct{ action, label string }{
				{"settings", "设备设置"}, {"wifi", "WLAN"}, {"developer", "开发者选项"},
			} {
				if f.Button(u.id("qa-"+a.action), a.label).Small().Outline().Disabled(!ready).Clicked() {
					u.backend.QuickAction(d.Serial, a.action)
				}
			}
		})
	})
}

func (u *UI) overviewCard(f *flex.Frame, title string, icon ui.Icon, rows [][2]string, half bool) {
	card := f.Column().Gap(14).Pad(18).Radius(12).Background(u.c.card).Border(1, u.c.border)
	if half {
		card = card.Grow(1)
	}
	card.Children(func() {
		f.Row().Gap(8).Items(flex.Center).Children(func() {
			f.Icon(icon).Size(16, 16).Color(u.c.blue)
			f.Text(title).FontSize(14).Weight(ui.WeightSemibold)
		})
		for _, row := range rows {
			f.Row().FillWidth().Items(flex.Center).Children(func() {
				f.Text(row[0]).FontSize(12).Color(u.c.muted).Grow(1)
				f.Text(row[1]).FontSize(13).Ellipsis()
			})
		}
	})
}

func (u *UI) overviewMeterCard(f *flex.Frame, title string, rows [][2]string, ratio float32) {
	u.card(f, func() {
		f.Row().Gap(8).Items(flex.Center).Children(func() {
			f.Icon(iconPhone).Size(16, 16).Color(u.c.blue)
			f.Text(title).FontSize(14).Weight(ui.WeightSemibold)
		})
		for _, row := range rows {
			f.Row().FillWidth().Items(flex.Center).Children(func() {
				f.Text(row[0]).FontSize(12).Color(u.c.muted).Grow(1)
				f.Text(row[1]).FontSize(13)
			})
		}
		u.meter(f, ratio)
	})
}

// meter is the thin progress bar under a usage line.
func (u *UI) meter(f *flex.Frame, ratio float32) {
	ratio = max(min(ratio, 1), 0)
	f.Stack().FillWidth().Height(8).Radius(4).Background(u.c.border).Children(func() {
		f.Stack().WidthPercent(ratio * 100).FillHeight().Radius(4).Background(u.c.blue)
	})
	f.Row().FillWidth().Children(func() {
		f.Stack().Grow(1)
		f.Text(strconv.Itoa(int(ratio*100+0.5)) + "%").FontSize(11).Color(u.c.muted)
	})
}

func apiSuffix(api string) string {
	if api == "" {
		return ""
	}
	return " (API " + api + ")"
}

func batteryText(level int) string {
	if level < 0 {
		return "未知"
	}
	return strconv.Itoa(level) + "%"
}

// memoryText shows "7.93 GB / 14.84 GB" style usage of RAM.
func memoryText(totalKB, availableKB int64) string {
	if totalKB <= 0 {
		return "未知"
	}
	used := totalKB - availableKB
	return formatBytes(used*1024) + " / " + formatBytes(totalKB*1024)
}

func memoryRatio(totalKB, availableKB int64) float32 {
	if totalKB <= 0 {
		return 0
	}
	return float32(totalKB-availableKB) / float32(totalKB)
}

func storageText(total, used int64) string {
	if total <= 0 {
		return "未知"
	}
	return formatBytes(used) + " / " + formatBytes(total)
}

func storageRatio(total, used int64) float32 {
	if total <= 0 {
		return 0
	}
	return float32(used) / float32(total)
}

// formatBytes keeps two decimals and picks the unit a user reads: an app is
// megabytes, storage is gigabytes.
func formatBytes(n int64) string {
	const kb, mb, gb = 1 << 10, 1 << 20, 1 << 30
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.2f MB", float64(n)/mb)
	default:
		return fmt.Sprintf("%.2f KB", float64(n)/kb)
	}
}
