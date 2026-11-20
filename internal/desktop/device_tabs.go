package desktop

import (
	"fmt"
	"strconv"
	"strings"

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

// deviceOverview keeps acquisition state separate from optional device fields.
func (u *UI) deviceOverview(f *flex.Frame, d domain.Device) {
	info, exists := u.state.PhoneInfos[d.Serial]
	ready := d.State == "device"
	if !exists {
		info.BatteryLevel, info.MemAvailableKB = -1, -1
		if ready {
			u.backend.FetchPhoneInfo(d.Serial)
		}
	}
	f.Row().FillWidth().Height(36).Gap(10).Items(flex.Center).Children(func() {
		f.Icon(iconPhone).Size(18, 18).Color(u.c.blue)
		f.Text("设备概览").FontSize(15).Weight(ui.WeightSemibold)
		status := "尚未读取"
		switch {
		case info.Loading || (!exists && ready):
			status = "正在读取设备信息…"
		case !ready:
			status = "设备未连接"
		case info.Error != "":
			status = "读取失败，可重试"
		case len(info.Warnings) > 0:
			status = fmt.Sprintf("已读取 · %d 项提示", len(info.Warnings))
		case !info.UpdatedAt.IsZero():
			status = "更新于 " + info.UpdatedAt.Format("15:04:05")
		case exists:
			status = "已读取"
		}
		f.Text(status).FontSize(11).Color(u.c.muted).Grow(1).Ellipsis()
		if f.Button(u.id("refresh-overview"), "刷新").Small().Outline().Disabled(!ready || info.Loading).Clicked() {
			u.backend.FetchPhoneInfo(d.Serial)
		}
	})
	if info.Error != "" {
		u.small(f, "读取设备信息失败："+info.Error)
	} else if len(info.Warnings) > 0 {
		u.small(f, "部分信息未能读取，其余结果正常显示；展开详细信息可查看原因。")
	}
	if u.overviewCompact {
		u.overviewBasic(f, info)
		f.Row().FillWidth().Gap(14).Children(func() {
			f.Column().Width(0).Grow(1).Children(func() { u.overviewBattery(f, info) })
			f.Column().Width(0).Grow(1).Children(func() { u.overviewResources(f, info) })
		})
	} else {
		f.Row().FillWidth().Gap(16).Children(func() {
			f.Column().Width(0).Grow(1).Children(func() { u.overviewBasic(f, info) })
			f.Column().Width(280).Gap(14).Children(func() {
				u.overviewBattery(f, info)
				u.overviewResources(f, info)
			})
		})
	}
	u.overviewDetails(f, info, d.Serial)

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

// overviewBasic uses paired rows rather than large, sparse summary tiles.
func (u *UI) overviewBasic(f *flex.Frame, i domain.PhoneInfo) {
	pairs := [][4]string{
		{"品牌", i.Brand, "型号", i.Model},
		{"制造商", i.Manufacturer, "代号", i.DeviceCode},
		{"Android", joinInfo(i.AndroidVersion, apiSuffix(i.APILevel)), "安全补丁", i.SecurityPatch},
		{"CPU 架构", i.CPUABI, "硬件平台", i.Hardware},
		{"分辨率", displayInfo(i.PhysicalSize, i.OverrideSize), "DPI", displayInfo(i.PhysicalDPI, i.OverrideDPI)},
		{"IP 地址", i.IPAddress, "Wi-Fi", i.WiFiSSID},
		{"序列号", i.SerialNumber, "A/B 槽", i.Slot},
		{"BL 锁", i.BootloaderState, "验证启动", i.VerifiedBoot},
		{"闪存", i.StorageType, "开机时长", uptimeText(i.UptimeSeconds)},
	}
	f.Column().FillWidth().Gap(4).Pad(16).Radius(12).Background(u.c.card).Border(1, u.c.border).Children(func() {
		u.overviewTitle(f, "基本信息")
		for _, row := range pairs {
			f.Row().FillWidth().Gap(16).Children(func() {
				for n := 0; n < 4; n += 2 {
					f.Row().Width(0).Grow(1).Height(30).Gap(8).Items(flex.Center).Children(func() {
						f.Text(row[n]).Width(60).FontSize(11).Color(u.c.muted)
						f.Text(knownInfo(row[n+1])).Width(0).Grow(1).FontSize(12).Ellipsis()
					})
				}
			})
		}
		u.overviewLine(f, "Root 检测", i.RootStatus)
		u.overviewLine(f, "内核", i.KernelVersion)
	})
}

func (u *UI) overviewTitle(f *flex.Frame, title string) {
	f.Row().FillWidth().Height(30).Gap(8).Items(flex.Center).Children(func() {
		f.Icon(iconPhone).Size(15, 15).Color(u.c.blue)
		f.Text(title).FontSize(14).Weight(ui.WeightSemibold)
	})
}

func (u *UI) overviewLine(f *flex.Frame, label, value string) {
	f.Row().FillWidth().Height(28).Gap(8).Items(flex.Center).Children(func() {
		f.Text(label).Width(72).FontSize(11).Color(u.c.muted)
		f.Text(knownInfo(value)).Width(0).Grow(1).FontSize(12).Ellipsis()
	})
}

func (u *UI) overviewBattery(f *flex.Frame, i domain.PhoneInfo) {
	f.Column().FillWidth().Gap(3).Pad(16).Radius(12).Background(u.c.card).Border(1, u.c.border).Children(func() {
		u.overviewTitle(f, "电池信息")
		f.Row().FillWidth().Gap(10).Items(flex.Center).Children(func() {
			f.Text(batteryText(i.BatteryLevel)).FontSize(27).Weight(ui.WeightSemibold).Color(u.c.blue)
			f.Text(knownInfo(i.BatteryStatus)).FontSize(12).Color(u.c.muted)
		})
		u.overviewLine(f, "温度", scaledInfo(i.BatteryTemperatureTenthsC, 10, "°C"))
		u.overviewLine(f, "电压", scaledInfo(i.BatteryVoltageMV, 1000, " V"))
		u.overviewLine(f, "健康状态", i.BatteryHealth)
		u.overviewLine(f, "剩余电荷", scaledInfo(i.BatteryChargeCounterUAH, 1000, " mAh"))
		u.overviewLine(f, "瞬时电流", scaledInfo(i.BatteryCurrentUA, 1000, " mA"))
		u.small(f, "数值由设备报告；模拟器可能返回占位值。")
	})
}

func (u *UI) overviewResources(f *flex.Frame, i domain.PhoneInfo) {
	f.Column().FillWidth().Gap(8).Pad(16).Radius(12).Background(u.c.card).Border(1, u.c.border).Children(func() {
		u.overviewTitle(f, "存储与内存")
		f.Text("内存 · 已用 / 总量").FontSize(11).Color(u.c.muted)
		f.Text(memoryText(i.MemTotalKB, i.MemAvailableKB)).FillWidth().Ellipsis().FontSize(12)
		u.meter(f, memoryRatio(i.MemTotalKB, i.MemAvailableKB))
		f.Text("数据分区 /data · 已用 / 总量").FontSize(11).Color(u.c.muted)
		f.Text(storageText(i.StorageTotalBytes, i.StorageUsedBytes)).FillWidth().Ellipsis().FontSize(12)
		u.meter(f, storageRatio(i.StorageTotalBytes, i.StorageUsedBytes))
	})
}

func (u *UI) overviewDetails(f *flex.Frame, i domain.PhoneInfo, serial string) {
	f.Column().FillWidth().Gap(8).Pad(16).Radius(12).Background(u.c.card).Border(1, u.c.border).Children(func() {
		label := "展开详细信息"
		if u.overviewAdvanced {
			label = "收起详细信息"
		}
		if f.Button(u.id("overview-details"), label).Small().Ghost().Clicked() {
			u.overviewAdvanced = !u.overviewAdvanced
		}
		if !u.overviewAdvanced {
			return
		}
		for _, row := range [][2]string{
			{"连接标识", serial}, {"产品名称", i.Product}, {"主板", i.Board}, {"ABI 列表", i.CPUABIs},
			{"物理分辨率", i.PhysicalSize}, {"覆盖分辨率", i.OverrideSize},
			{"物理 DPI", i.PhysicalDPI}, {"覆盖 DPI", i.OverrideDPI},
			{"CPU 最小频率", scaledInfo(i.CPUFreqMinKHz, 1000, " MHz")},
			{"CPU 最大频率", scaledInfo(i.CPUFreqMaxKHz, 1000, " MHz")},
			{"CPU 当前频率", scaledInfo(i.CPUFreqCurKHz, 1000, " MHz")},
			{"构建 ID", i.BuildID}, {"构建版本", i.BuildDisplay}, {"构建指纹", i.Fingerprint},
			{"基带版本", i.Baseband}, {"Root 检测依据", i.RootStatus}, {"内核版本", i.KernelVersion},
		} {
			f.Text(row[0]).FontSize(11).Color(u.c.muted)
			f.Paragraph(knownInfo(row[1]), 12).FillWidth().FontSize(12)
		}
		u.small(f, "屏幕数值中的箭头表示物理值 → 覆盖值。剩余电荷不是设计容量；电流来自设备 sysfs，正负方向由设备驱动定义。Root 检测不执行 su 或任何提权操作。")
		for _, warning := range i.Warnings {
			u.small(f, warning)
		}
	})
}

func knownInfo(value string) string {
	if strings.TrimSpace(value) == "" {
		return "未知"
	}
	return value
}

func joinInfo(version, suffix string) string {
	if version == "" {
		return strings.TrimSpace(suffix)
	}
	return version + suffix
}

func displayInfo(physical, override string) string {
	if override == "" {
		return physical
	}
	return knownInfo(physical) + " → " + override
}

func scaledInfo(n *int64, divisor float64, unit string) string {
	if n == nil {
		return ""
	}
	return strconv.FormatFloat(float64(*n)/divisor, 'f', -1, 64) + unit
}

func uptimeText(seconds *int64) string {
	if seconds == nil {
		return ""
	}
	n := *seconds
	if n >= 86400 {
		return fmt.Sprintf("%d天 %d小时", n/86400, n%86400/3600)
	}
	return fmt.Sprintf("%d小时 %d分", n/3600, n%3600/60)
}

// meter is the thin progress bar under a usage line.
func (u *UI) meter(f *flex.Frame, ratio float32) {
	if ratio < 0 {
		f.Stack().FillWidth().Height(8).Radius(4).Background(u.c.border)
		f.Text("未能读取用量").FontSize(11).Color(u.c.muted)
		return
	}
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
	if totalKB <= 0 || availableKB < 0 || availableKB > totalKB {
		return "未知"
	}
	used := totalKB - availableKB
	return formatBytes(used*1024) + " / " + formatBytes(totalKB*1024)
}

func memoryRatio(totalKB, availableKB int64) float32 {
	if totalKB <= 0 || availableKB < 0 || availableKB > totalKB {
		return -1
	}
	return float32(totalKB-availableKB) / float32(totalKB)
}

func storageText(total, used int64) string {
	if total <= 0 || used < 0 || used > total {
		return "未知"
	}
	return formatBytes(used) + " / " + formatBytes(total)
}

func storageRatio(total, used int64) float32 {
	if total <= 0 || used < 0 || used > total {
		return -1
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
