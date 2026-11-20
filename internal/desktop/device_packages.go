package desktop

import (
	"bytes"
	"hash/fnv"
	"image/png"
	"strconv"
	"strings"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// appsState is the app-packages tab's own view state.
type appsState struct {
	search          flex.Field
	filter          string // "all" | "user" | "system"
	selected        map[string]bool
	confirm         string          // package pending uninstall confirmation
	menuFor         string          // package whose row actions are open
	exportPackages  []string        // Snapshot for the pending native destination picker.
	exportDismissed map[uint64]bool // Result cards hidden for this application session.
}

// devicePackages is the app list: search, filter, install, export, backup and
// uninstall installed packages. Labels and icons arrive lazily per row.
func (u *UI) devicePackages(f *flex.Frame, d domain.Device) {
	ready := d.State == "device"
	if u.apps.filter == "" {
		u.apps.filter = "all"
	}
	if u.apps.selected == nil {
		u.apps.selected = map[string]bool{}
	}
	listing := u.state.Apps[d.Serial]
	apps := u.filterApps(d.Serial, listing.Apps)
	users, systems := 0, 0
	for _, a := range listing.Apps {
		if a.System {
			systems++
		} else {
			users++
		}
	}

	// Explicit groups and gaps keep text badges and buttons from touching at
	// non-integer Windows DPI scales; margins on text are not a row spacer.
	f.Row().FillWidth().Height(40).Gap(12).Items(flex.Center).Children(func() {
		f.Row().Grow(1).Gap(10).Items(flex.Center).Children(func() {
			f.Row().Gap(6).Items(flex.Center).Children(func() {
				f.Icon(iconPhone).Size(18, 18).Color(u.c.blue)
				f.Text("应用包").FontSize(15).Weight(ui.WeightSemibold)
			})
			f.Row().Gap(6).Items(flex.Center).Children(func() {
				for _, option := range []struct{ key, label string }{
					{"all", strconv.Itoa(len(listing.Apps)) + " 个应用"},
					{"system", "系统 " + strconv.Itoa(systems)},
					{"user", "用户 " + strconv.Itoa(users)},
				} {
					button := f.Button(u.id("apps-kind-"+option.key), option.label).Small()
					if u.apps.filter != option.key {
						button = button.Ghost()
					}
					if button.Clicked() {
						u.apps.filter = option.key
						u.wake.Invalidate()
					}
				}
			})
		})
		if f.Button(u.id("apps-install"), "安装 APK").Small().Disabled(!ready).Clicked() {
			u.pendingDialog = "apk"
			u.dialogSerial = d.Serial
			u.wake.Invalidate()
		}
		if f.Button(u.id("apps-refresh"), "刷新").Small().Outline().Disabled(!ready).Clicked() {
			u.backend.FetchApps(d.Serial)
		}
	})

	// Action bar: search, filter, bulk actions and the selection count.
	f.Row().FillWidth().Height(40).Gap(10).Items(flex.Center).Children(func() {
		f.Field(&u.apps.search, u.id("apps-search"), "按名称或包名搜索..", "").Grow(1).Height(34)
		if f.Button(u.id("apps-filter"), filterLabel(u.apps.filter)).Small().Outline().Clicked() {
			switch u.apps.filter {
			case "all":
				u.apps.filter = "user"
			case "user":
				u.apps.filter = "system"
			default:
				u.apps.filter = "all"
			}
			u.wake.Invalidate()
		}
		count := len(u.apps.selected)
		if f.Button(u.id("apps-export"), "导出").Small().Outline().Disabled(count == 0 || !ready).Clicked() {
			packages := make([]string, 0, count)
			// Include selections hidden by a filter, in stable listing order.
			for _, a := range listing.Apps {
				if u.apps.selected[a.Package] {
					packages = append(packages, a.Package)
				}
			}
			u.requestAppExport(d.Serial, packages)
		}
		if f.Button(u.id("apps-backup"), "备份").Small().Outline().Disabled(count == 0 || !ready).Clicked() {
			for pkg := range u.apps.selected {
				u.backend.AppBackup(d.Serial, pkg, "")
			}
			u.apps.selected = map[string]bool{}
			u.wake.Invalidate()
		}
		if f.Button(u.id("apps-uninstall"), "卸载").Small().Destructive().Disabled(count == 0 || !ready).Clicked() {
			u.apps.confirm = "selected"
			u.wake.Invalidate()
		}
		if count > 0 {
			f.Text("已选 " + strconv.Itoa(count) + " 项").FontSize(11).Color(u.c.muted)
		}
	})

	u.appExportStatus(f, d.Serial)

	if u.apps.confirm != "" {
		u.card(f, func() {
			if u.apps.confirm == "selected" {
				u.small(f, "确认卸载选中的 "+strconv.Itoa(len(u.apps.selected))+" 个应用？系统应用将移除其更新。")
			} else {
				u.small(f, "确认卸载“"+u.apps.confirm+"”？此操作不可恢复。")
			}
			f.Row().Gap(10).Children(func() {
				if f.Button(u.id("apps-uninstall-yes"), "确认卸载").Destructive().Small().Disabled(!ready).Clicked() {
					if u.apps.confirm == "selected" {
						for pkg := range u.apps.selected {
							u.backend.AppUninstall(d.Serial, pkg, u.appIsSystem(listing.Apps, pkg))
						}
						u.apps.selected = map[string]bool{}
					} else {
						u.backend.AppUninstall(d.Serial, u.apps.confirm, u.appIsSystem(listing.Apps, u.apps.confirm))
					}
					u.apps.confirm = ""
					u.wake.Invalidate()
				}
				if f.Button(u.id("apps-uninstall-no"), "保留").Ghost().Small().Clicked() {
					u.apps.confirm = ""
					u.wake.Invalidate()
				}
			})
		})
	}

	u.card(f, func() {
		if listing.Error != "" {
			u.small(f, "读取应用列表失败："+listing.Error)
			return
		}
		if len(apps) == 0 {
			u.small(f, "没有匹配的应用。")
			return
		}
		f.ScrollArea(u.id("apps-list-scroll")).FillWidth().Height(480).Children(func() {
			f.Grid(360).FillWidth().Gap(8).Children(func() {
				for _, a := range apps {
					u.appRow(f, d, a)
				}
			})
		})
	})
}

func (u *UI) appRow(f *flex.Frame, d domain.Device, a domain.AppInfo) {
	ready := d.State == "device"
	meta := u.state.AppMetas[d.Serial+"\x00"+a.Package]
	label := meta.Label
	if label == "" {
		label = a.Label
	}
	if label == "" {
		label = a.Package
	}
	checked := u.apps.selected[a.Package]
	f.Row().FillWidth().Height(52).Gap(10).PadX(10).Items(flex.Center).HoverBackground(u.c.selected).Children(func() {
		if f.Checkbox(u.id("app-check-"+a.Package), checked).Clicked() {
			if checked {
				delete(u.apps.selected, a.Package)
			} else {
				u.apps.selected[a.Package] = true
			}
			u.wake.Invalidate()
		}
		icon := meta.IconPNG
		if len(icon) == 0 {
			icon = a.IconPNG
		}
		if bmp, ok := appIconBitmap(a.Package, icon); ok {
			f.Image(bmp).Size(34, 34).Radius(8)
		} else {
			f.Stack().Size(34, 34).Radius(8).Background(u.c.selected).Center().Children(func() {
				f.Text(strings.ToUpper(firstChar(label))).FontSize(14).Weight(ui.WeightSemibold).Color(u.c.blue)
			})
		}
		f.Column().Grow(1).Gap(2).Children(func() {
			f.Row().Gap(6).Items(flex.Center).Children(func() {
				f.Text(label).FontSize(13).Weight(ui.WeightMedium).Ellipsis()
				if a.System {
					f.Text("系统").FontSize(9).Shrink(0).Color(u.c.muted).PadX(6).PadY(2).Radius(6).Background(u.c.selected)
				}
			})
			f.Text(a.Package).FontSize(11).Color(u.c.muted).Ellipsis()
		})
		f.Text(formatBytes(a.SizeBytes)).FontSize(12).Color(u.c.muted).Width(86)
		version := a.VersionName
		if version == "" {
			version = a.VersionCode
		}
		f.Text(version).FontSize(12).Color(u.c.muted).Width(80).Ellipsis()
		if f.Button(u.id("app-menu-"+a.Package), "⋮").Ghost().Small().Clicked() {
			if u.apps.menuFor == a.Package {
				u.apps.menuFor = ""
			} else {
				u.apps.menuFor = a.Package
			}
			u.wake.Invalidate()
		}
	})
	if u.apps.menuFor != a.Package {
		return
	}
	u.card(f, func() {
		f.Row().FillWidth().Gap(10).Children(func() {
			if f.Button(u.id("app-export-one"), "导出安装包").Small().Outline().Disabled(!ready).Clicked() {
				u.requestAppExport(d.Serial, []string{a.Package})
				u.apps.menuFor = ""
				u.wake.Invalidate()
			}
			if f.Button(u.id("app-backup-one"), "备份应用").Small().Outline().Disabled(!ready).Clicked() {
				u.backend.AppBackup(d.Serial, a.Package, "")
				u.apps.menuFor = ""
				u.wake.Invalidate()
			}
			if f.Button(u.id("app-uninstall-one"), "卸载").Small().Destructive().Disabled(!ready).Clicked() {
				u.apps.confirm = a.Package
				u.apps.menuFor = ""
				u.wake.Invalidate()
			}
		})
	})
	// Ask for this row's label and icon once it is on screen.
	if meta.Label == "" && len(meta.IconPNG) == 0 && a.Label == "" && len(a.IconPNG) == 0 && meta.Error == "" {
		u.backend.FetchAppMeta(d.Serial, a.APKPath, a.Package)
	}
}

func (u *UI) appIsSystem(apps []domain.AppInfo, pkg string) bool {
	for _, a := range apps {
		if a.Package == pkg {
			return a.System
		}
	}
	return false
}

// filterApps applies the search box (package name or fetched label) and the
// user/system filter.
func (u *UI) filterApps(serial string, apps []domain.AppInfo) []domain.AppInfo {
	query := strings.ToLower(strings.TrimSpace(u.apps.search.Text()))
	var found []domain.AppInfo
	for _, a := range apps {
		if u.apps.filter == "user" && a.System {
			continue
		}
		if u.apps.filter == "system" && !a.System {
			continue
		}
		if query != "" {
			label := u.state.AppMetas[serial+"\x00"+a.Package].Label
			if label == "" {
				label = a.Label
			}
			if !strings.Contains(strings.ToLower(a.Package), query) &&
				(label == "" || !strings.Contains(strings.ToLower(label), query)) {
				continue
			}
		}
		found = append(found, a)
	}
	return found
}

func filterLabel(filter string) string {
	switch filter {
	case "user":
		return "筛选：用户"
	case "system":
		return "筛选：系统"
	}
	return "筛选"
}

func firstChar(s string) string {
	for _, r := range s {
		return string(r)
	}
	return "?"
}

// appIconBitmap converts one app icon's PNG bytes to a canvas bitmap. The ID
// identifies the package; Version changes with the PNG, including across devices.
func appIconBitmap(pkg string, data []byte) (ui.Bitmap, bool) {
	if len(data) == 0 {
		return ui.Bitmap{}, false
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return ui.Bitmap{}, false
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w == 0 || h == 0 {
		return ui.Bitmap{}, false
	}
	pix := make([]byte, w*h*4)
	i := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			// RGBA() already returns premultiplied channels; do not multiply again.
			pix[i] = byte(b >> 8)
			pix[i+1] = byte(g >> 8)
			pix[i+2] = byte(r >> 8)
			pix[i+3] = byte(a >> 8)
			i += 4
		}
	}
	hsh := fnv.New64a()
	hsh.Write([]byte(pkg))
	content := fnv.New32a()
	content.Write(data)
	return ui.Bitmap{ID: hsh.Sum64() | 1, Version: content.Sum32(), Width: w, Height: h, Stride: w * 4, Pix: pix}, true
}
