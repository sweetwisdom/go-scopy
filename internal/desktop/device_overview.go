package desktop

import (
	"fmt"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// The overview uses an elastic information column and a content-sized preview.
// Full diagnostics live in a modal instead of stretching the workspace.
func (u *UI) deviceOverview(f *flex.Frame, d domain.Device) {
	info, exists := u.state.PhoneInfos[d.Serial]
	if !exists {
		info.BatteryLevel, info.MemAvailableKB = -1, -1
		if d.State == "device" {
			u.backend.FetchPhoneInfo(d.Serial)
		}
	}
	leftWidth, rightWidth := overviewColumnWidths(u.overviewWidth)
	information := func() {
		u.overviewBasic(f, info, d)
		if leftWidth >= 340 {
			f.Row().FillWidth().Gap(14).Items(flex.Start).Children(func() {
				f.Column().Width(0).Grow(1).Children(func() { u.overviewBattery(f, info) })
				f.Column().Width(0).Grow(1).Children(func() { u.overviewResources(f, info) })
			})
		} else {
			u.overviewBattery(f, info)
			u.overviewResources(f, info)
		}
	}
	if u.overviewWidth < 540 {
		f.Column().FillWidth().Gap(14).Children(func() {
			information()
			f.Row().FillWidth().Justify(flex.Center).Children(func() {
				f.Column().Width(rightWidth).Children(func() { u.screenPreview(f, d, rightWidth) })
			})
		})
		return
	}
	f.Row().FillWidth().Gap(16).Items(flex.Start).Children(func() {
		f.Column().Width(0).Grow(1).Gap(14).Children(information)
		f.Column().Width(rightWidth).Shrink(0).Children(func() { u.screenPreview(f, d, rightWidth) })
	})
}

func (u *UI) overviewBasic(f *flex.Frame, i domain.PhoneInfo, device domain.Device) {
	pairs := [][4]string{
		{"系统版本", joinInfo(i.AndroidVersion, apiSuffix(i.APILevel)), "设备型号", i.Model},
		{"分辨率", effectiveInfo(i.PhysicalSize, i.OverrideSize), "屏幕密度", effectiveInfo(i.PhysicalDPI, i.OverrideDPI)},
		{"IP 地址", i.IPAddress, "无线网络", i.WiFiSSID},
		{"运行时间", uptimeText(i.UptimeSeconds), "序列号", i.SerialNumber},
	}
	gap := float32(12)
	if u.overviewHeight < 780 {
		gap = 8
	}
	f.Column().FillWidth().Gap(gap).Pad(16).Radius(12).Background(u.c.card).Border(1, u.c.border).Children(func() {
		f.Row().FillWidth().Height(26).Gap(8).Items(flex.Center).Children(func() {
			f.Icon(iconPhone).Size(15, 15).Color(u.c.muted)
			f.Text("设备信息").FontSize(14).Weight(ui.WeightSemibold).Grow(1)
			if f.Button(u.id("refresh-overview"), "刷新信息").Small().Ghost().Disabled(device.State != "device" || i.Loading).Clicked() {
				u.backend.FetchPhoneInfo(device.Serial)
			}
		})
		for _, row := range pairs {
			f.Row().FillWidth().Gap(14).Children(func() {
				for n := 0; n < 4; n += 2 {
					if u.overviewCompact && u.overviewHeight >= 780 {
						f.Column().Width(0).Grow(1).Gap(4).Children(func() {
							f.Text(row[n]).FontSize(10).Color(u.c.muted)
							f.Text(knownInfo(row[n+1])).FillWidth().Ellipsis().FontSize(12)
						})
					} else {
						f.Row().Width(0).Grow(1).Height(24).Gap(8).Items(flex.Center).Children(func() {
							labelWidth := float32(60)
							if u.overviewCompact {
								labelWidth = 48
							}
							f.Text(row[n]).Width(labelWidth).FontSize(10).Color(u.c.muted)
							f.Text(knownInfo(row[n+1])).Width(0).Grow(1).FontSize(12).Ellipsis()
						})
					}
				}
			})
		}
		f.Stack().FillWidth().Height(1).Background(u.c.border)
		f.Row().FillWidth().Gap(6).Items(flex.Center).Children(func() {
			f.Text("安全补丁 · " + knownInfo(i.SecurityPatch)).Width(0).Grow(1).Ellipsis().FontSize(10).Color(u.c.muted)
			if f.Button(u.id("overview-details"), "全部设备信息").Small().Ghost().Clicked() {
				u.overviewDetailsSerial = device.Serial
				u.view.ScrollState(u.id("overview-details-scroll")).ScrollTo(0)
				u.wake.Invalidate()
			}
		})
		status := ""
		switch {
		case i.Loading:
			status = "正在读取设备信息…"
		case device.State != "device":
			status = "设备未连接，显示上次读取的信息"
		case i.Error != "":
			status = "读取设备信息失败：" + i.Error
		case len(i.Warnings) > 0:
			status = fmt.Sprintf("%d 项信息未能读取，可在全部设备信息中查看", len(i.Warnings))
		}
		if status != "" {
			f.Paragraph(status, 11).FillWidth().FontSize(11).Color(u.c.muted)
		}
	})
}

func (u *UI) overviewTitle(f *flex.Frame, title string) {
	f.Text(title).FontSize(13).Weight(ui.WeightSemibold)
}

func (u *UI) overviewBattery(f *flex.Frame, i domain.PhoneInfo) {
	f.Column().FillWidth().Height(u.overviewVitalsHeight()).Gap(12).Pad(16).Radius(12).Background(u.c.card).Border(1, u.c.border).Children(func() {
		u.overviewTitle(f, "电池")
		f.Row().FillWidth().Gap(8).Items(flex.Center).Children(func() {
			f.Text(batteryText(i.BatteryLevel)).FontSize(30).Weight(ui.WeightSemibold)
			f.Text(knownInfo(i.BatteryStatus)).Width(0).Grow(1).Ellipsis().FontSize(11).Color(u.c.green)
		})
		f.Text(knownInfo(scaledInfo(i.BatteryTemperatureTenthsC, 10, "°C"))).FontSize(12).Color(u.c.muted)
		f.Text("健康 · " + knownInfo(i.BatteryHealth)).FillWidth().Ellipsis().FontSize(11).Color(u.c.muted)
	})
}

func (u *UI) overviewResources(f *flex.Frame, i domain.PhoneInfo) {
	f.Column().FillWidth().Height(u.overviewVitalsHeight()).Gap(6).Pad(16).Radius(12).Background(u.c.card).Border(1, u.c.border).Children(func() {
		u.overviewTitle(f, "存储与内存")
		f.Text("内存").FontSize(10).Color(u.c.muted)
		f.Text(memoryText(i.MemTotalKB, i.MemAvailableKB)).FillWidth().Ellipsis().FontSize(11)
		u.overviewMeter(f, memoryRatio(i.MemTotalKB, i.MemAvailableKB))
		f.Text("存储 /data").FontSize(10).Color(u.c.muted)
		f.Text(storageText(i.StorageTotalBytes, i.StorageUsedBytes)).FillWidth().Ellipsis().FontSize(11)
		u.overviewMeter(f, storageRatio(i.StorageTotalBytes, i.StorageUsedBytes))
	})
}

func (u *UI) overviewVitalsHeight() float32 {
	if u.overviewHeight < 780 {
		return 170
	}
	return 186
}

func (u *UI) overviewMeter(f *flex.Frame, ratio float32) {
	f.Row().FillWidth().Height(12).Gap(8).Items(flex.Center).Children(func() {
		f.Stack().Width(0).Grow(1).Height(5).Radius(3).Background(u.c.border).Children(func() {
			if ratio >= 0 {
				f.Stack().WidthPercent(min(ratio, 1) * 100).FillHeight().Radius(3).Background(u.c.blue)
			}
		})
		label := "未知"
		if ratio >= 0 {
			label = fmt.Sprintf("%d%%", int(min(ratio, 1)*100+0.5))
		}
		f.Text(label).FontSize(10).Color(u.c.muted)
	})
}

func effectiveInfo(physical, override string) string {
	if override != "" {
		return override
	}
	return physical
}

func (u *UI) overviewDetailsDialog(f *flex.Frame) {
	serial := u.overviewDetailsSerial
	if serial == "" {
		return
	}
	i, exists := u.state.PhoneInfos[serial]
	if !exists {
		i.BatteryLevel, i.MemAvailableKB = -1, -1
	}
	d := f.Dialog(flex.DialogSpec{Title: "全部设备信息", Description: u.dialogDevice(serial).Name(), Width: 640,
		Scope: u.id("overview-details-dialog"), DismissID: u.id("dismiss-overview-details"), CloseID: u.id("close-overview-details")})
	d.Body(func() {
		f.ScrollArea(u.id("overview-details-scroll")).FillWidth().Height(min(float32(480), max(u.overviewHeight-240, 160))).Gap(12).Children(func() {
			for _, row := range [][2]string{
				{"品牌", i.Brand}, {"型号", i.Model}, {"制造商", i.Manufacturer}, {"代号", i.DeviceCode},
				{"Android", joinInfo(i.AndroidVersion, apiSuffix(i.APILevel))}, {"安全补丁", i.SecurityPatch},
				{"CPU 架构", i.CPUABI}, {"硬件平台", i.Hardware}, {"IP 地址", i.IPAddress}, {"Wi-Fi", i.WiFiSSID},
				{"序列号", i.SerialNumber}, {"A/B 槽", i.Slot}, {"BL 锁", i.BootloaderState}, {"验证启动", i.VerifiedBoot},
				{"闪存", i.StorageType}, {"开机时长", uptimeText(i.UptimeSeconds)},
				{"电量", batteryText(i.BatteryLevel)}, {"电池状态", i.BatteryStatus}, {"健康状态", i.BatteryHealth},
				{"温度", scaledInfo(i.BatteryTemperatureTenthsC, 10, "°C")}, {"电压", scaledInfo(i.BatteryVoltageMV, 1000, " V")},
				{"剩余电荷", scaledInfo(i.BatteryChargeCounterUAH, 1000, " mAh")}, {"瞬时电流", scaledInfo(i.BatteryCurrentUA, 1000, " mA")},
				{"连接标识", serial}, {"产品名称", i.Product}, {"主板", i.Board}, {"ABI 列表", i.CPUABIs},
				{"物理分辨率", i.PhysicalSize}, {"覆盖分辨率", i.OverrideSize}, {"物理 DPI", i.PhysicalDPI}, {"覆盖 DPI", i.OverrideDPI},
				{"CPU 最小频率", scaledInfo(i.CPUFreqMinKHz, 1000, " MHz")}, {"CPU 最大频率", scaledInfo(i.CPUFreqMaxKHz, 1000, " MHz")},
				{"CPU 当前频率", scaledInfo(i.CPUFreqCurKHz, 1000, " MHz")}, {"构建 ID", i.BuildID}, {"构建版本", i.BuildDisplay},
				{"构建指纹", i.Fingerprint}, {"基带版本", i.Baseband}, {"Root 检测依据", i.RootStatus}, {"内核版本", i.KernelVersion},
			} {
				f.Row().FillWidth().Gap(16).Items(flex.Start).Children(func() {
					f.Text(row[0]).Width(96).FontSize(11).Color(u.c.muted)
					f.Paragraph(knownInfo(row[1]), 12).Width(0).Grow(1).FontSize(12)
				})
			}
			u.small(f, "剩余电荷不是设计容量；电流正负方向由设备驱动定义。Root 检测不执行 su 或任何提权操作。")
			if i.Error != "" {
				u.small(f, i.Error)
			}
			for _, warning := range i.Warnings {
				u.small(f, warning)
			}
		})
	})
	d.Footer(func() { f.Button(u.id("dismiss-overview-details"), "关闭详情").Outline() })
	if d.Dismissed() {
		u.overviewDetailsSerial = ""
		u.wake.Invalidate()
	}
}
