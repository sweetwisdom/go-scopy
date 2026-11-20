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
	{"terminal", "设备终端"},
}

// device hosts the management tabs of one device.
func (u *UI) device(f *flex.Frame, d domain.Device) {
	u.deviceHeading(f, d)
	u.deviceVirtualSessions(f, d.Serial)
	u.deviceTabs(f, d)
	switch u.deviceTab {
	case "files":
		u.deviceFiles(f, d)
	case "packages":
		u.devicePackages(f, d)
	case "terminal":
		u.deviceTerminalPage(f, d)
	default:
		u.deviceOverview(f, d)
	}
}

// deviceHeading separates device identity from session actions to keep the
// workspace header calm and easy to scan.
func (u *UI) deviceHeading(f *flex.Frame, d domain.Device) {
	info := u.state.PhoneInfos[d.Serial]
	identity := func() {
		f.Row().Width(0).Grow(1).Gap(12).Items(flex.Center).Children(func() {
			f.Column().Size(42, 48).Shrink(0).Center().Radius(10).Background(u.c.selected).Children(func() {
				f.Icon(iconPhone).Size(23, 23).Color(u.c.blue)
			})
			f.Column().Width(0).Grow(1).Gap(6).Children(func() {
				nameWidth := max(u.overviewWidth-210, 1)
				if u.overviewWidth >= 800 {
					nameWidth = max(u.overviewWidth-550, 1)
				}
				f.Row().FillWidth().Gap(8).Items(flex.Center).Children(func() {
					f.Text(d.Name()).MaxWidth(nameWidth).Ellipsis().FontSize(24).Weight(ui.WeightSemibold)
					u.deviceConnectionButton(f, d)
					status, color := "在线", u.c.green
					if d.State != "device" {
						status, color = "离线", u.c.muted
					}
					if d.State == "unauthorized" {
						status = "未授权"
					}
					f.Text(status + " · " + d.Connection).FontSize(11).Color(color)
				})
				subtitle := strings.TrimSpace(info.Brand + " " + info.Model)
				if subtitle == "" {
					subtitle = d.Model
				}
				if info.AndroidVersion != "" {
					subtitle += " · Android " + info.AndroidVersion
				}
				f.Text(knownInfo(subtitle)).FillWidth().Ellipsis().FontSize(11).Color(u.c.muted)
			})
		})
	}
	actions := func() {
		ready := d.State == "device"
		f.Row().Gap(8).Items(flex.Center).Children(func() {
			if f.Button(u.id("device-mirror"), "投屏").Small().Disabled(!ready).Clicked() {
				u.start(false)
			}
			if f.Button(u.id("open-virtual"), "虚拟屏幕").Small().Outline().Disabled(!ready).Clicked() {
				u.openVirtualDialog(d.Serial)
			}
			if f.Button(u.id("open-projection-options"), "投屏参数").Small().Outline().Clicked() {
				u.optionsSerial = d.Serial
				u.loadOptions()
				u.wake.Invalidate()
			}
			if d.Connection == "USB" {
				if f.Button(u.id("tcp"), "切换无线").Small().Outline().Disabled(!ready).Clicked() {
					u.backend.EnableTCP(d.Serial)
				}
			}
		})
	}
	f.Column().FillWidth().Gap(14).Children(func() {
		if u.overviewHeight >= 780 {
			f.Text("我的设备 / " + d.Name()).FillWidth().Ellipsis().FontSize(11).Color(u.c.muted)
		}
		if u.overviewWidth >= 800 {
			f.Row().FillWidth().Gap(20).Items(flex.Center).Children(func() { identity(); actions() })
		} else {
			f.Row().FillWidth().Children(identity)
			actions()
		}
	})
}

func (u *UI) deviceTabs(f *flex.Frame, d domain.Device) {
	f.Row().FillWidth().Height(40).Gap(4).Items(flex.Center).MarginSides(0, 0, 4, 0).Children(func() {
		for _, tab := range devicePageKeys {
			if u.deviceTabButton(f, tab.key, tab.label) {
				if tab.key == "overview" {
					u.previewRefresh.serial = ""
				}
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
