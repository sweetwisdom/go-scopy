// Package desktop implements Go-Scopy's native Viva workspace.
package desktop

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-scopy/go-scopy/internal/brand"
	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

type colors struct{ rail, side, bg, card, border, selected, muted, blue, green, red ui.Color }

func palette(dark bool) colors {
	if dark {
		return colors{ui.RGB(15, 17, 21), ui.RGB(18, 20, 25), ui.RGB(11, 13, 17), ui.RGB(23, 26, 33), ui.RGB(39, 44, 54), ui.RGB(25, 47, 88), ui.RGB(145, 151, 164), ui.RGB(122, 173, 255), ui.RGB(55, 197, 118), ui.RGB(255, 112, 116)}
	}
	return colors{ui.RGB(253, 253, 255), ui.RGB(251, 252, 254), ui.RGB(249, 251, 254), ui.RGB(255, 255, 255), ui.RGB(231, 235, 241), ui.RGB(229, 239, 255), ui.RGB(117, 125, 139), ui.RGB(31, 80, 191), ui.RGB(34, 176, 94), ui.RGB(220, 48, 54)}
}

type UI struct {
	backend                             domain.Backend
	state                               domain.State
	tree                                ui.Tree
	scope                               ui.Scope
	ids                                 map[string]ui.ID
	view                                flex.View
	wake                                app.Invalidator
	theme                               ui.Theme
	c                                   colors
	section, selected, filter, category string
	connecting                          bool
	search, address, pairAddress, code  flex.Field
	size, fps, bitrate                  flex.Field
	adb, scrcpy, output                 flex.Field
	options                             domain.Options
	notice                              string
	pendingDialog, dialogSerial         string
	closing                             bool
	allowClose                          func()
	presentation                        map[string]devicePresentation
	stats                               string
	renameSerial                        string
	deviceName                          flex.Field
	deviceAddress, newDeviceName        flex.Field
	deleteSerial                        string
}

type devicePresentation struct {
	id                      ui.ID
	rowStatus, detailStatus string
}

func (u *UI) Wake() { u.wake.Invalidate() }

func New(backend domain.Backend) *UI {
	u := &UI{backend: backend, state: backend.State(), ids: map[string]ui.ID{}, section: "devices", filter: "all", category: "general"}
	u.scope = u.tree.Feature("workspace")
	for _, key := range strings.Fields("nav-devices nav-tasks nav-settings theme search filter-all filter-usb filter-wifi refresh add connect-address pair-address pair-code connect pair cancel size fps bitrate audio screen-off awake top start record stop capture install tcp disconnect save-options default-options settings-general settings-tools settings-about adb scrcpy output browse-adb browse-scrcpy browse-output save-settings open-output content-scroll device-scroll task-all task-running task-error close-confirm close-cancel") {
		u.id(key)
	}
	u.code.Secure = true
	u.adb.SetText(u.state.Settings.ADBPath)
	u.scrcpy.SetText(u.state.Settings.ScrcpyPath)
	u.output.SetText(u.state.Settings.OutputDir)
	u.setTheme(u.state.Settings.Theme)
	u.loadOptions()
	u.syncDevices()
	backend.SetNotify(u.wake.Invalidate)
	return u
}

func (u *UI) id(key string) ui.ID {
	if id, ok := u.ids[key]; ok {
		return id
	}
	id := u.tree.ID(u.scope, key)
	u.ids[key] = id
	return id
}

func (u *UI) setTheme(name string) {
	u.theme = ui.LightTheme()
	if name == "dark" {
		u.theme = ui.DarkTheme()
	}
	u.theme.Primary = ui.RGB(34, 83, 229)
	u.c = palette(name == "dark")
}

func (u *UI) syncDevices() {
	found := false
	u.presentation = make(map[string]devicePresentation, len(u.state.Devices))
	u.stats = fmt.Sprintf("%d 台设备  ·  %d 个投屏会话", len(u.state.Devices), len(u.state.Sessions))
	for i := range u.state.Devices {
		d := &u.state.Devices[i]
		d.Alias = u.state.Settings.DeviceNames[d.Serial]
	}
	for _, d := range u.state.Devices {
		u.presentation[d.Serial] = devicePresentation{u.id("device/" + d.Serial), stateLabel(d.State) + " · " + d.Connection, stateLabel(d.State) + "  /  " + d.Connection}
		if d.Serial == u.selected {
			found = true
		}
	}
	for _, t := range u.state.Tasks {
		if t.Path != "" {
			u.id(fmt.Sprintf("task/%d", t.ID))
		}
	}
	if !found {
		u.selected = ""
		if len(u.state.Devices) > 0 {
			u.selected = u.state.Devices[0].Serial
		}
		u.loadOptions()
	}
}

func (u *UI) loadOptions() {
	u.options = u.state.Settings.OptionsFor(u.selected)
	u.size.SetText(strconv.Itoa(u.options.MaxSize))
	u.fps.SetText(strconv.Itoa(u.options.FPS))
	u.bitrate.SetText(strconv.Itoa(u.options.Bitrate))
}

func (u *UI) readOptions() (domain.Options, error) {
	o := u.options
	var err error
	if o.MaxSize, err = strconv.Atoi(strings.TrimSpace(u.size.Text())); err != nil {
		return o, fmt.Errorf("分辨率上限请输入整数")
	}
	if o.FPS, err = strconv.Atoi(strings.TrimSpace(u.fps.Text())); err != nil {
		return o, fmt.Errorf("帧率请输入整数")
	}
	if o.Bitrate, err = strconv.Atoi(strings.TrimSpace(u.bitrate.Text())); err != nil {
		return o, fmt.Errorf("码率请输入整数")
	}
	return o, config.ValidateOptions(o)
}

func (u *UI) Config() app.Config {
	return app.Config{Title: "Go-Scopy", Icon: brand.WindowIcon(), Class: "GoScopy.Desktop", Width: 1240, Height: 820, MinWidth: 1000, MinHeight: 700,
		WindowMode: app.WindowCustomTitlebar, TitlebarHeight: 44, Tree: &u.tree, Invalidator: &u.wake,
		WindowControls: func() app.WindowControlsStyle {
			return app.WindowControlsStyle{Foreground: u.c.muted, Hover: u.c.selected, Close: ui.RGB(220, 48, 54), CloseForeground: ui.RGB(255, 255, 255), Minimize: "最小化", Maximize: "最大化", Restore: "还原", CloseName: "关闭"}
		},
		TextInput: flex.Fields(&u.search, &u.address, &u.pairAddress, &u.code, &u.size, &u.fps, &u.bitrate, &u.adb, &u.scrcpy, &u.output, &u.deviceName, &u.deviceAddress, &u.newDeviceName),
		OnScroll:  u.view.Wheel, Paint: u.paint, OnWake: u.onWake,
		OnCloseRequest: func(allow func()) {
			if len(u.state.Sessions) > 0 {
				u.closing = true
				u.allowClose = allow
				u.wake.Invalidate()
			} else {
				allow()
			}
		},
	}
}

func (u *UI) onWake() {
	u.state = u.backend.State()
	u.setTheme(u.state.Settings.Theme)
	u.syncDevices()
	if u.pendingDialog != "" {
		kind, serial := u.pendingDialog, u.dialogSerial
		u.pendingDialog = ""
		if kind == "output" {
			path, err := app.OpenDirectory(app.OpenDirectoryOptions{Title: "选择截图与录像保存目录"})
			if err != nil {
				u.notice = err.Error()
			} else if path != "" {
				u.output.SetText(path)
			}
			return
		}
		filter := app.FileFilter{Name: "可执行文件", Pattern: "*.exe"}
		if kind == "apk" {
			filter = app.FileFilter{Name: "Android 应用", Pattern: "*.apk"}
		}
		paths, err := app.OpenFiles(app.OpenFileOptions{Title: "选择文件", Filters: []app.FileFilter{filter}})
		if err != nil {
			u.notice = err.Error()
		} else if len(paths) > 0 {
			switch kind {
			case "apk":
				u.backend.Install(serial, paths[0])
			case "adb":
				u.adb.SetText(paths[0])
			case "scrcpy":
				u.scrcpy.SetText(paths[0])
			}
		}
	}
}

func (u *UI) dialog(kind string) {
	u.pendingDialog = kind
	u.dialogSerial = u.selected
	u.wake.Invalidate()
}

func (u *UI) paint(canvas *ui.Canvas, size ui.Size) {
	bounds := ui.Rect{Width: size.Width, Height: size.Height}
	canvas.Fill(bounds, u.c.bg)
	u.view.Draw(canvas, u.theme, bounds, func(f *flex.Frame) {
		f.Column().Fill().Children(func() {
			f.Row().FillWidth().Height(44).PadX(22).Gap(12).Items(flex.Center).Background(u.c.rail).Border(1, u.c.border).MovesWindow().Children(func() {
				f.Image(brand.Bitmap()).Size(25, 25)
				f.Text("Go-Scopy").FontSize(15).Weight(ui.WeightSemibold)
				f.Text("ANDROID WORKSPACE").FontSize(10).Color(u.c.muted).Grow(1)
				f.Text("本地连接").FontSize(11).Color(u.c.muted).MarginSides(0, 150, 0, 0)
			})
			f.Row().FillWidth().Height(max(size.Height-72, 0)).Children(func() { u.rail(f); u.sidebar(f); u.content(f) })
			f.Row().FillWidth().Height(28).PadX(18).Gap(8).Items(flex.Center).Background(u.c.rail).Border(1, u.c.border).Children(func() {
				color := u.c.green
				if u.state.ToolError != "" {
					color = u.c.red
				}
				f.Stack().Size(5, 5).Radius(3).Background(color)
				f.Text(u.state.Connection).FontSize(10).Color(u.c.muted).Grow(1).Ellipsis()
				f.Text(u.stats).FontSize(10).Color(u.c.muted)
			})
		})
	})
}

func (u *UI) rail(f *flex.Frame) {
	f.Column().Width(76).FillHeight().PadX(8).PadY(22).Gap(12).Background(u.c.rail).Border(1, u.c.border).Children(func() {
		f.Column().Height(42).Items(flex.Center).Children(func() { f.Image(brand.Bitmap()).Size(38, 38) })
		for _, nav := range []struct {
			key, label, control string
			icon                ui.Icon
		}{{"devices", "设备", "nav-devices", iconPhone}, {"tasks", "任务", "nav-tasks", iconTasks}} {
			if u.nav(f, nav.control, nav.label, nav.icon, u.section == nav.key) {
				u.section = nav.key
				u.filter = "all"
				u.connecting = false
				u.notice = ""
				u.wake.Invalidate()
			}
		}
		f.Stack().Grow(1)
		if u.nav(f, "theme", "外观", iconTheme, false) {
			s := u.state.Settings
			if s.Theme == "dark" {
				s.Theme = "light"
			} else {
				s.Theme = "dark"
			}
			u.setTheme(s.Theme)
			u.backend.Save(s)
		}
		if u.nav(f, "nav-settings", "设置", iconSettings, u.section == "settings") {
			u.section = "settings"
			u.connecting = false
			u.notice = ""
			u.wake.Invalidate()
		}
	})
}

func (u *UI) nav(f *flex.Frame, key, label string, icon ui.Icon, selected bool) bool {
	bg := ui.Color{}
	fg := u.c.muted
	if selected {
		bg = u.c.selected
		fg = u.c.blue
	}
	return f.Column().FillWidth().Height(62).Gap(5).Center().Radius(11).Background(bg).HoverBackground(u.c.selected).Hit(u.id(key), label).Children(func() { f.Icon(icon).Size(21, 21).Color(fg); f.Text(label).FontSize(11).Color(fg) }).Clicked()
}

func (u *UI) sidebar(f *flex.Frame) {
	f.Column().Width(270).FillHeight().Pad(20).Gap(18).Background(u.c.side).Border(1, u.c.border).Children(func() {
		title := "我的设备"
		if u.section == "tasks" {
			title = "任务中心"
		}
		if u.section == "settings" {
			title = "设置"
		}
		f.Row().FillWidth().Items(flex.Center).Children(func() {
			f.Text(title).FontSize(18).Weight(ui.WeightSemibold).Grow(1)
			if u.section == "devices" {
				f.Text(strconv.Itoa(len(u.state.Devices))).FontSize(12).Color(u.c.muted)
			}
		})
		if u.section == "settings" {
			for _, item := range []struct{ key, label, desc string }{{"general", "常规", "外观与保存位置"}, {"tools", "运行环境", "adb 与 scrcpy"}, {"about", "关于", "版本与开源项目"}} {
				bg := ui.Color{}
				if u.category == item.key {
					bg = u.c.selected
				}
				if f.Column().FillWidth().Pad(14).Gap(5).Radius(9).Background(bg).Hit(u.id("settings-"+item.key), item.label).Children(func() {
					f.Text(item.label).FontSize(14).Weight(ui.WeightMedium)
					f.Text(item.desc).FontSize(11).Color(u.c.muted)
				}).Clicked() {
					u.category = item.key
					u.notice = ""
					u.wake.Invalidate()
				}
			}
			f.Stack().Grow(1)
			u.small(f, "设置保存在本机")
			return
		}
		if u.section == "tasks" {
			for _, item := range []struct{ key, label string }{{"all", "全部任务"}, {"running", "进行中"}, {"error", "失败任务"}} {
				b := f.Button(u.id("task-"+item.key), item.label).FillWidth().Height(38).Ghost()
				if u.filter == item.key {
					b.Background(u.c.selected)
				}
				if b.Clicked() {
					u.filter = item.key
					u.wake.Invalidate()
				}
			}
			f.Stack().Grow(1)
			u.small(f, "保留最近 100 条已结束任务")
			return
		}
		f.Field(&u.search, u.id("search"), "搜索设备", "搜索名称或序列号").FillWidth().Height(36)
		f.Row().FillWidth().Gap(5).Children(func() {
			for _, item := range []struct{ key, label, control string }{{"all", "全部", "filter-all"}, {"usb", "USB", "filter-usb"}, {"wifi", "Wi-Fi", "filter-wifi"}} {
				b := f.Button(u.id(item.control), item.label).Grow(1).Small().Ghost()
				if u.filter == item.key {
					b.Background(u.c.selected)
				}
				if b.Clicked() {
					u.filter = item.key
					u.wake.Invalidate()
				}
			}
		})
		f.ScrollArea(u.id("device-scroll")).FillWidth().Grow(1).Gap(9).Children(func() {
			count := 0
			query := strings.ToLower(u.search.Text())
			for _, d := range u.state.Devices {
				if query != "" && !strings.Contains(strings.ToLower(d.Name()+" "+d.Model+" "+d.Serial), query) {
					continue
				}
				if u.filter == "usb" && d.Connection != "USB" || u.filter == "wifi" && d.Connection != "Wi-Fi" {
					continue
				}
				count++
				u.deviceRow(f, d)
			}
			if count == 0 {
				f.Column().FillWidth().PadY(24).Gap(8).Children(func() {
					u.small(f, "暂无匹配的设备")
					u.small(f, "使用 USB 数据线连接手机，")
					u.small(f, "或添加无线设备。")
				})
			}
		})
		if f.Button(u.id("add"), "＋  无线连接").FillWidth().Height(38).Outline().Clicked() {
			u.connecting = true
			u.notice = ""
			u.wake.Invalidate()
		}
		if f.Button(u.id("refresh"), "刷新设备").FillWidth().Height(30).Ghost().Clicked() {
			u.backend.Refresh()
		}
	})
}

func (u *UI) deviceRow(f *flex.Frame, d domain.Device) {
	present := u.presentation[d.Serial]
	bg := ui.Color{}
	if d.Serial == u.selected && !u.connecting {
		bg = u.c.selected
	}
	if f.Column().FillWidth().Pad(13).Gap(8).Radius(10).Background(bg).HoverBackground(u.c.selected).Hit(present.id, d.Name()).Children(func() {
		f.Row().FillWidth().Gap(10).Items(flex.Center).Children(func() {
			f.Icon(iconPhone).Size(20, 20).Color(u.c.blue)
			f.Text(d.Name()).FontSize(14).Weight(ui.WeightMedium).Grow(1).Ellipsis()
		})
		f.Text(d.Serial).FontSize(10).Color(u.c.muted).Ellipsis()
		f.Row().Gap(6).Items(flex.Center).Children(func() {
			u.dot(f, d.State == "device")
			f.Text(present.rowStatus).FontSize(10).Color(u.c.muted)
			if _, ok := u.state.Sessions[d.Serial]; ok {
				f.Text("投屏中").FontSize(10).Color(u.c.blue)
			}
		})
	}).Clicked() {
		u.selected = d.Serial
		u.connecting = false
		u.notice = ""
		u.loadOptions()
		u.wake.Invalidate()
	}
}

func stateLabel(state string) string {
	switch state {
	case "device":
		return "已连接"
	case "unauthorized":
		return "等待授权"
	case "offline":
		return "离线"
	default:
		return state
	}
}
func (u *UI) small(f *flex.Frame, text string) {
	f.Paragraph(text, 24).FillWidth().FontSize(12).Color(u.c.muted)
}
func (u *UI) dot(f *flex.Frame, online bool) {
	color := u.c.muted
	if online {
		color = u.c.green
	}
	f.Stack().Size(6, 6).Radius(3).Background(color)
}

func (u *UI) heading(f *flex.Frame, kicker, title, desc string) {
	f.Column().FillWidth().Gap(8).MarginSides(0, 0, 8, 0).Children(func() {
		f.Text(kicker).FontSize(11).Color(u.c.muted)
		f.Text(title).FillWidth().Ellipsis().FontSize(26).Weight(ui.WeightSemibold)
		u.small(f, desc)
	})
}

func (u *UI) card(f *flex.Frame, build func()) {
	f.Column().FillWidth().Pad(22).Gap(16).Background(u.c.card).Border(1, u.c.border).Radius(12).Children(build)
}

func (u *UI) content(f *flex.Frame) {
	f.ScrollArea(u.id("content-scroll")).Grow(1).FillHeight().Pad(30).Gap(20).Children(func() {
		if u.closing {
			u.card(f, func() {
				f.Text("结束投屏并退出？").FontSize(18).Weight(ui.WeightSemibold)
				u.small(f, "正在进行的投屏与录屏将停止，录像完成封装后退出。")
				f.Row().Gap(10).Children(func() {
					if f.Button(u.id("close-cancel"), "继续使用").Outline().Clicked() {
						u.closing = false
						u.wake.Invalidate()
					}
					if f.Button(u.id("close-confirm"), "结束并退出").Destructive().Clicked() {
						if u.allowClose != nil {
							u.allowClose()
						}
					}
				})
			})
			return
		}
		if u.notice != "" {
			f.Paragraph(u.notice, 12).FillWidth().Pad(12).Background(u.c.selected).Radius(8).FontSize(12)
		}
		if u.section == "settings" {
			u.settings(f)
			return
		}
		if u.section == "tasks" {
			u.tasks(f)
			return
		}
		if u.connecting {
			u.wireless(f)
			return
		}
		if u.state.ToolError != "" {
			u.card(f, func() {
				f.Text("运行环境需要配置").FontSize(16).Weight(ui.WeightSemibold).Color(u.c.red)
				u.small(f, u.state.ToolError)
				u.small(f, "打开左下角设置 → 运行环境，指定工具路径后重新启动。")
			})
		}
		var device domain.Device
		for _, d := range u.state.Devices {
			if d.Serial == u.selected {
				device = d
				break
			}
		}
		if device.Serial == "" {
			u.empty(f)
			return
		}
		u.device(f, device)
	})
}

func (u *UI) empty(f *flex.Frame) {
	u.heading(f, "工作台 / 设备", "让手机，连接桌面。", "连接 Android 设备，即可开始投屏、录屏与应用安装。")
	u.card(f, func() {
		f.Column().FillWidth().Height(180).Gap(18).Center().Children(func() {
			f.Icon(iconPhone).Size(54, 54).Color(u.c.blue)
			f.Text("等待你的第一台设备").FontSize(19).Weight(ui.WeightMedium)
			u.small(f, "手机画面将在独立的 scrcpy 窗口中打开")
		})
		f.Row().FillWidth().Gap(18).Children(func() {
			f.Column().Grow(1).Gap(8).Children(func() {
				f.Text("01  通过 USB 连接").FontSize(14).Weight(ui.WeightMedium)
				u.small(f, "开启开发者选项和 USB 调试，连接数据线后，在手机上允许本机调试。")
			})
			f.Column().Grow(1).Gap(8).Children(func() {
				f.Text("02  或使用无线调试").FontSize(14).Weight(ui.WeightMedium)
				u.small(f, "让电脑与手机处于同一局域网，通过配对码或 IP 地址建立连接。")
			})
		})
	})
	u.small(f, "提示：未授权设备会出现在左侧列表中；无需安装额外的手机应用。")
}

func (u *UI) device(f *flex.Frame, d domain.Device) {
	u.heading(f, "工作台 / 设备", d.Name(), "管理这台设备的投屏会话与常用操作。")
	active, hasSession := u.state.Sessions[d.Serial]
	ready := d.State == "device"
	u.card(f, func() {
		f.Row().FillWidth().Gap(18).Items(flex.Center).Children(func() {
			f.Stack().Size(60, 68).Radius(13).Background(u.c.selected).Center().Children(func() { f.Icon(iconPhone).Size(32, 32).Color(u.c.blue) })
			f.Column().Grow(1).Gap(8).Children(func() {
				f.Text(d.Name()).FillWidth().Ellipsis().FontSize(18).Weight(ui.WeightSemibold)
				if d.Alias != "" && d.Model != "" {
					f.Text(d.Model).FontSize(11).Color(u.c.muted)
				}
				f.Text(d.Serial).FontSize(12).Color(u.c.muted).Ellipsis()
				f.Row().Gap(7).Items(flex.Center).Children(func() {
					u.dot(f, ready)
					f.Text(u.presentation[d.Serial].detailStatus).FontSize(11).Color(u.c.muted)
				})
			})
			if f.Button(u.id("rename-device"), "重命名").Ghost().Small().Clicked() {
				u.renameSerial = d.Serial
				u.deviceName.SetText(d.Alias)
				u.deviceAddress.SetText(d.Serial)
				u.wake.Invalidate()
			}
			if hasSession {
				label := "投屏中"
				if active.Recording != "" {
					label = "正在录屏"
				}
				if active.Status == "starting" {
					label = "正在启动"
				}
				if active.Status == "stopping" {
					label = "正在停止"
				}
				f.Text(label).FontSize(12).Color(u.c.blue)
			}
		})
		if u.renameSerial == d.Serial {
			if d.Connection == "Wi-Fi" {
				f.Field(&u.deviceAddress, u.id("edit-device-address"), "连接地址", "IP:端口").FillWidth().Height(38)
			}
			f.Row().FillWidth().Gap(8).Items(flex.Center).Children(func() {
				f.Field(&u.deviceName, u.id("device-name"), "设备备注", "例如：我的小米手机").Grow(1).Height(38)
				if f.Button(u.id("save-device-name"), "保存名称").Small().Clicked() {
					u.saveDeviceRecord(d)
				}
				if f.Button(u.id("cancel-device-name"), "取消重命名").Ghost().Small().Clicked() {
					u.renameSerial = ""
					u.wake.Invalidate()
				}
			})
			u.small(f, "备注仅保存在本机，最多 40 个字符；留空可恢复原名称。")
			if d.Connection == "Wi-Fi" {
				u.small(f, "可修改上方连接地址。修改仅保存记录，已有连接需自行断开。")
			}
		}
		if !ready {
			if d.State == "unauthorized" {
				u.small(f, "请解锁手机并允许 USB 调试。如果授权弹窗未出现，请重新插拔数据线。")
			} else {
				u.small(f, "设备不可用。请检查数据线或无线网络，重新连接后再试。")
			}
		}
		f.Row().Gap(10).Children(func() {
			if f.Button(u.id("start"), "开始投屏").Height(38).Disabled(!ready || hasSession || u.state.ScrcpyVersion == "").Clicked() {
				u.start(false)
			}
			if f.Button(u.id("record"), "投屏并录屏").Height(38).Outline().Disabled(!ready || hasSession || u.state.ScrcpyVersion == "").Clicked() {
				u.start(true)
			}
			if f.Button(u.id("stop"), "停止会话").Height(38).Outline().Disabled(!hasSession || active.Status != "running").Clicked() {
				u.backend.Stop(u.selected)
			}
		})
		if active.Recording != "" {
			u.small(f, "录屏文件："+active.Recording)
		}
	})
	u.card(f, func() {
		f.Row().FillWidth().Items(flex.Center).Children(func() {
			f.Text("投屏参数").FontSize(16).Weight(ui.WeightSemibold).Grow(1)
			f.Text("下一次启动时生效").FontSize(11).Color(u.c.muted)
		})
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
		f.Row().Gap(10).Children(func() {
			if f.Button(u.id("save-options"), "保存此设备参数").Outline().Small().Clicked() {
				u.saveOptions(false)
			}
			if f.Button(u.id("default-options"), "设为全局默认").Ghost().Small().Clicked() {
				u.saveOptions(true)
			}
		})
	})
	u.card(f, func() {
		f.Text("常用操作").FontSize(16).Weight(ui.WeightSemibold)
		f.Row().FillWidth().Gap(10).Children(func() {
			if f.Button(u.id("capture"), "保存截图").Outline().Disabled(!ready).Clicked() {
				u.backend.Capture(u.selected)
			}
			if f.Button(u.id("install"), "安装 APK").Outline().Disabled(!ready).Clicked() {
				u.dialog("apk")
			}
			if d.Connection == "USB" {
				if f.Button(u.id("tcp"), "切换无线").Outline().Disabled(!ready).Clicked() {
					u.backend.EnableTCP(u.selected)
				}
			} else {
				if f.Button(u.id("reconnect-device"), "连接设备").Outline().Disabled(ready || hasSession).Clicked() {
					u.backend.Connect(d.Serial)
				}
				if f.Button(u.id("disconnect"), "断开连接").Outline().Disabled(hasSession).Clicked() {
					u.backend.Disconnect(u.selected)
				}
			}
		})
		if d.Connection == "Wi-Fi" {
			f.Row().Gap(10).Children(func() {
				if !u.state.Settings.SavedWireless[d.Serial] {
					if f.Button(u.id("remember-device"), "保存到设备列表").Ghost().Small().Clicked() {
						u.persistDevice(d.Serial, d.Serial, d.Alias, true)
					}
				} else if f.Button(u.id("forget-device"), "删除记录").Ghost().Small().Clicked() {
					u.deleteSerial = d.Serial
					u.wake.Invalidate()
				}
			})
			if u.deleteSerial == d.Serial {
				u.small(f, "删除本机保存的地址、备注及设备参数？已连接设备仍会由 ADB 自动发现，可先断开连接。")
				f.Row().Gap(10).Children(func() {
					if f.Button(u.id("confirm-forget-device"), "确认删除记录").Outline().Small().Clicked() {
						u.backend.Save(config.ForgetDevice(u.state.Settings, d.Serial))
						u.deleteSerial = ""
						u.wake.Invalidate()
					}
					if f.Button(u.id("cancel-forget-device"), "保留记录").Ghost().Small().Clicked() {
						u.deleteSerial = ""
						u.wake.Invalidate()
					}
				})
			}
		}
		u.small(f, "截图和录像保存在设置中的目录，执行结果可在任务中心查看。")
	})
	if len(u.state.Tasks) > 0 {
		t := u.state.Tasks[0]
		text := "最近任务：" + t.Title + " · " + taskLabel(t.Status)
		if t.Detail != "" {
			text += " · " + t.Detail
		}
		u.small(f, text)
	}
}

func (u *UI) saveDeviceRecord(d domain.Device) {
	if u.persistDevice(d.Serial, u.deviceAddress.Text(), u.deviceName.Text(), d.Connection == "Wi-Fi") {
		u.renameSerial = ""
	}
}

func (u *UI) persistDevice(previous, address, name string, wireless bool) bool {
	s, err := config.EditDevice(u.state.Settings, previous, address, name, wireless)
	if err != nil {
		u.notice = err.Error()
		u.wake.Invalidate()
		return false
	}
	if wireless {
		u.selected = strings.TrimSpace(address)
	}
	u.backend.Save(s)
	u.notice = "设备记录已提交保存，结果可在任务中心查看"
	u.wake.Invalidate()
	return true
}

func (u *UI) optionField(f *flex.Frame, label string, field *flex.Field, key string) {
	f.Column().Grow(1).Gap(8).Children(func() { u.small(f, label); f.Field(field, u.id(key), label, "").FillWidth().Height(35) })
}
func (u *UI) toggle(f *flex.Frame, key, label string, value *bool) {
	f.Row().Grow(1).Gap(8).Items(flex.Center).Children(func() {
		if f.Checkbox(u.id(key), *value).Label(label).Clicked() {
			*value = !*value
			u.wake.Invalidate()
		}
		f.Text(label).FontSize(13)
	})
}
func (u *UI) start(record bool) {
	o, err := u.readOptions()
	if err != nil {
		u.notice = err.Error()
		u.wake.Invalidate()
		return
	}
	u.notice = ""
	u.backend.Start(u.selected, o, record)
}
func (u *UI) saveOptions(global bool) {
	o, err := u.readOptions()
	if err != nil {
		u.notice = err.Error()
		u.wake.Invalidate()
		return
	}
	s := u.state.Settings
	if s.Devices == nil {
		s.Devices = map[string]domain.Options{}
	}
	if global {
		s.Defaults = o
	} else {
		s.Devices[u.selected] = o
	}
	u.backend.Save(s)
	u.notice = "参数已提交保存"
	u.wake.Invalidate()
}

func (u *UI) wireless(f *flex.Frame) {
	u.heading(f, "工作台 / 无线连接", "无线，也能随手连接。", "确保手机与电脑连接同一局域网，并开启手机的无线调试。")
	u.card(f, func() {
		f.Text("连接已配对设备").FontSize(17).Weight(ui.WeightSemibold)
		u.small(f, "输入无线调试主页面的 IP 地址与连接端口。旧版 TCP/IP 调试通常使用 5555。")
		f.Field(&u.newDeviceName, u.id("new-device-name"), "设备备注（可选）", "例如：我的小米手机").FillWidth().Height(38)
		f.Row().FillWidth().Gap(12).Children(func() {
			f.Field(&u.address, u.id("connect-address"), "连接地址", "192.168.1.20:5555").Grow(1).Height(38)
			if f.Button(u.id("connect"), "连接设备").Height(38).Clicked() {
				u.backend.Connect(u.address.Text())
				u.notice = "连接请求已提交，可在任务中心查看结果"
				u.wake.Invalidate()
			}
		})
		if f.Button(u.id("save-wireless-device"), "保存无线设备").Outline().Small().Clicked() {
			if u.persistDevice("", u.address.Text(), u.newDeviceName.Text(), true) {
				u.connecting = false
				u.newDeviceName.SetText("")
			}
		}
		u.small(f, "保存后离线仍保留在设备列表；选中设备即可连接、编辑或删除记录。")
	})
	u.card(f, func() {
		f.Text("首次连接：使用配对码").FontSize(17).Weight(ui.WeightSemibold)
		u.small(f, "Android 11 及以上：开发者选项 → 无线调试 → 使用配对码配对设备。")
		f.Field(&u.pairAddress, u.id("pair-address"), "配对地址", "手机配对弹窗中的 IP:端口").FillWidth().Height(38)
		f.Row().FillWidth().Gap(12).Children(func() {
			f.Field(&u.code, u.id("pair-code"), "配对码", "6 位配对码").Grow(1).Height(38)
			if f.Button(u.id("pair"), "配对设备").Height(38).Outline().Clicked() {
				u.backend.Pair(u.pairAddress.Text(), u.code.Text())
				u.code.SetText("")
				u.notice = "配对请求已提交，成功后请在上方输入连接地址"
				u.wake.Invalidate()
			}
		})
		u.small(f, "配对端口与连接端口通常不同。配对成功后，使用无线调试主页面上的连接端口。")
	})
	if f.Button(u.id("cancel"), "返回设备").Ghost().Clicked() {
		u.connecting = false
		u.notice = ""
		u.wake.Invalidate()
	}
}

func taskLabel(status string) string {
	switch status {
	case "running":
		return "进行中"
	case "error":
		return "失败"
	default:
		return "已完成"
	}
}
func (u *UI) tasks(f *flex.Frame) {
	u.heading(f, "工作台 / 任务", "每一步，都有记录。", "查看投屏、录屏、截图和安装任务的执行结果。")
	count := 0
	for _, t := range u.state.Tasks {
		if u.filter != "all" && u.filter != "usb" && u.filter != "wifi" && u.filter != t.Status {
			continue
		}
		count++
		u.card(f, func() {
			f.Row().FillWidth().Gap(10).Items(flex.Center).Children(func() {
				f.Text(t.Title).FontSize(15).Weight(ui.WeightMedium).Grow(1).Ellipsis()
				color := u.c.green
				if t.Status == "error" {
					color = u.c.red
				}
				if t.Status == "running" {
					color = u.c.blue
				}
				f.Text(taskLabel(t.Status)).FontSize(11).Color(color)
			})
			u.small(f, t.Started.Format("2006-01-02 15:04:05"))
			if t.Detail != "" {
				u.small(f, t.Detail)
			}
			if t.Path != "" {
				u.small(f, t.Path)
				if f.Button(u.id(fmt.Sprintf("task/%d", t.ID)), "打开所在文件夹").Small().Outline().Clicked() {
					if err := app.OpenPath(filepath.Dir(t.Path)); err != nil {
						u.notice = err.Error()
						u.wake.Invalidate()
					}
				}
			}
		})
	}
	if count == 0 {
		u.card(f, func() {
			f.Icon(iconTasks).Size(32, 32).Color(u.c.muted)
			f.Text("暂无任务").FontSize(17)
			u.small(f, "执行设备操作后，进度和结果将在这里显示。")
		})
	}
}

func (u *UI) settings(f *flex.Frame) {
	if u.category == "about" {
		u.heading(f, "设置 / 关于", "Go-Scopy", "用 Go 与原生 Viva 构建的 Android 桌面工作台。")
		u.card(f, func() {
			f.Text("Go-Scopy 0.1.1").FontSize(20).Weight(ui.WeightSemibold)
			u.small(f, "界面风格参考 GoIM；设备操作参考 escrcpy，投屏与控制由官方 scrcpy 提供。")
			u.small(f, "开源项目：GoIM (MIT)、escrcpy (Apache-2.0)、Viva (Apache-2.0)、scrcpy (Apache-2.0)。完整声明随发布包提供。")
			u.small(f, "首版支持 Windows x64。多设备可分别投屏，每台设备使用一个独立窗口。")
		})
		return
	}
	if u.category == "tools" {
		u.heading(f, "设置 / 运行环境", "连接能力，来自成熟工具。", "优先使用指定路径，其次随包工具，最后查找系统 PATH。")
		u.card(f, func() {
			u.pathField(f, "adb 可执行文件", "adb", &u.adb, "留空使用随包工具或 PATH")
			u.pathField(f, "scrcpy 可执行文件", "scrcpy", &u.scrcpy, "留空使用随包工具或 PATH")
			u.small(f, "工具路径保存后重新启动应用生效。")
			if f.Button(u.id("save-settings"), "保存设置").Clicked() {
				u.saveSettings()
			}
		})
		u.card(f, func() {
			f.Text("当前运行环境").FontSize(16).Weight(ui.WeightSemibold)
			for _, line := range []string{u.state.ADBVersion, u.state.ADBPath, u.state.ScrcpyVersion, u.state.ScrcpyPath, u.state.ToolError} {
				if line != "" {
					u.small(f, line)
				}
			}
		})
		return
	}
	u.heading(f, "设置 / 常规", "按你的习惯，准备就绪。", "外观与本地文件保存设置。")
	u.card(f, func() {
		f.Row().FillWidth().Items(flex.Center).Children(func() {
			f.Column().Grow(1).Gap(6).Children(func() {
				f.Text("界面外观").FontSize(15).Weight(ui.WeightMedium)
				u.small(f, "浅色与深色主题，即时切换")
			})
			label := "切换为深色"
			if u.state.Settings.Theme == "dark" {
				label = "切换为浅色"
			}
			if f.Button(u.id("settings-theme"), label).Outline().Clicked() {
				s := u.state.Settings
				if s.Theme == "dark" {
					s.Theme = "light"
				} else {
					s.Theme = "dark"
				}
				u.setTheme(s.Theme)
				u.backend.Save(s)
			}
		})
	})
	u.card(f, func() {
		f.Text("截图与录屏").FontSize(16).Weight(ui.WeightSemibold)
		u.pathField(f, "保存目录", "output", &u.output, "请选择保存目录")
		f.Row().Gap(10).Children(func() {
			if f.Button(u.id("save-settings"), "保存设置").Clicked() {
				u.saveSettings()
			}
			if f.Button(u.id("open-output"), "打开目录").Outline().Clicked() {
				if err := app.OpenPath(u.state.Settings.OutputDir); err != nil {
					u.notice = err.Error()
					u.wake.Invalidate()
				}
			}
		})
		u.small(f, "文件名包含时间与设备序列号。录屏开始前选择参数，停止会话后自动保存。")
	})
}

func (u *UI) pathField(f *flex.Frame, label, key string, field *flex.Field, placeholder string) {
	f.Column().FillWidth().Gap(8).Children(func() {
		u.small(f, label)
		f.Row().FillWidth().Gap(10).Children(func() {
			f.Field(field, u.id(key), label, placeholder).Grow(1).Height(36)
			if f.Button(u.id("browse-"+key), "选择").Outline().Height(36).Clicked() {
				u.dialog(key)
			}
		})
	})
}
func (u *UI) saveSettings() {
	s := u.state.Settings
	s.ADBPath = strings.TrimSpace(u.adb.Text())
	s.ScrcpyPath = strings.TrimSpace(u.scrcpy.Text())
	s.OutputDir = strings.TrimSpace(u.output.Text())
	if err := config.Validate(s); err != nil {
		u.notice = err.Error()
	} else {
		u.backend.Save(s)
		u.notice = "设置已提交保存，结果见任务中心"
	}
	u.wake.Invalidate()
}
