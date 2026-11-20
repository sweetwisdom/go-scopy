package desktop

import (
	"path"
	"strconv"
	"strings"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// filesState is the file manager tab's own view state.
type filesState struct {
	dir      string
	search   flex.Field
	name     flex.Field
	renaming string // entry being renamed
	confirm  string // entry pending delete confirmation
	creating bool   // the new-folder input is open
	menuFor  string // entry whose row actions are open
}

// deviceFiles is the file manager tab: browse, transfer and edit one device
// directory at a time.
func (u *UI) deviceFilesNative(f *flex.Frame, d domain.Device) {
	ready := d.State == "device"
	if u.files.dir == "" {
		u.files.dir = "/sdcard"
	}
	listing := u.state.Files[d.Serial+"\x00"+u.files.dir]
	entries := u.filterFiles(listing.Entries)

	// Compact toolbar modeled on Motty's file pane: actions stay in one
	// predictable row rather than competing with the file list.
	f.Row().FillWidth().Height(32).Gap(6).Items(flex.Center).Children(func() {
		f.Text("文件").FontSize(16).Weight(ui.WeightSemibold).Grow(1)
		if u.fileIconButton(f, "files-upload", mottyUpload, "上传文件", !ready) {
			u.pendingDialog = "upload"
			u.dialogSerial = d.Serial
			u.wake.Invalidate()
		}
		if u.fileIconButton(f, "files-mkdir", mottyFolderPlus, "新建文件夹", !ready) {
			u.files.creating = true
			u.files.name.SetText("")
			u.wake.Invalidate()
		}
		if u.fileIconButton(f, "files-refresh", mottyRotateCw, "刷新", !ready) {
			u.backend.FetchFiles(d.Serial, u.files.dir)
		}
	})

	// Breadcrumb and path filter mirror the compact address bar in Motty.
	f.Row().FillWidth().Height(32).Gap(6).Items(flex.Center).Children(func() {
		if u.fileIconButton(f, "files-up", mottyArrowUp, "返回上级", parentDir(u.files.dir) == u.files.dir) {
			u.files.dir = parentDir(u.files.dir)
			u.files.menuFor, u.files.renaming, u.files.confirm = "", "", ""
			u.backend.FetchFiles(d.Serial, u.files.dir)
			u.wake.Invalidate()
		}
		crumbs := strings.Split(strings.Trim(u.files.dir, "/"), "/")
		current := "/"
		for i, crumb := range crumbs {
			if crumb == "" {
				continue
			}
			current = joinDevice(current, crumb)
			if i > 0 {
				f.Text("›").FontSize(12).Color(u.c.muted)
			}
			pathAtCrumb := current
			box := f.Text(crumb).FontSize(12).Color(u.c.muted).Hit(u.id("files-crumb-"+strconv.Itoa(i)), "打开 "+pathAtCrumb)
			if pathAtCrumb == u.files.dir {
				box.Color(u.c.blue)
			}
			if box.Clicked() && pathAtCrumb != u.files.dir {
				u.files.dir = pathAtCrumb
				u.files.menuFor, u.files.renaming, u.files.confirm = "", "", ""
				u.backend.FetchFiles(d.Serial, pathAtCrumb)
				u.wake.Invalidate()
			}
		}
		f.Stack().Grow(1)
		f.Field(&u.files.search, u.id("files-search"), "筛选当前目录", "").Width(210).Height(30)
	})

	if u.files.creating {
		f.Column().FillWidth().Pad(8).MarginSides(0, 0, 4, 0).Gap(6).Background(u.c.card).Border(1, u.c.border).Radius(8).Children(func() {
			f.Row().FillWidth().Gap(8).Items(flex.Center).Children(func() {
				f.Field(&u.files.name, u.id("files-new-name"), "文件夹名称", "例如：backup").Grow(1).Height(32)
				if f.Button(u.id("files-create"), "创建").Small().Disabled(!ready).Clicked() {
					name := strings.TrimSpace(u.files.name.Text())
					if name != "" {
						u.backend.FileMkdir(d.Serial, joinDevice(u.files.dir, name))
					}
					u.files.creating = false
					u.wake.Invalidate()
				}
				if f.Button(u.id("files-create-cancel"), "取消").Ghost().Small().Clicked() {
					u.files.creating = false
					u.wake.Invalidate()
				}
			})
		})
	}

	// A tight, flat list keeps the file rows readable and dense like the
	// reference pane; the parent page card's generous spacing is avoided here.
	f.Column().FillWidth().Height(520).Gap(0).Pad(8).Background(u.c.card).Border(1, u.c.border).Radius(10).Children(func() {
		f.Row().FillWidth().Height(28).Gap(8).PadX(8).Items(flex.Center).Background(u.c.selected).Radius(5).Children(func() {
			f.Text("名称").FontSize(11).Color(u.c.muted).Grow(1)
			f.Text("大小").FontSize(11).Color(u.c.muted).Width(82).TextEnd()
			f.Text("修改时间").FontSize(11).Color(u.c.muted).Width(128).TextEnd()
			f.Stack().Width(30)
		})
		f.ScrollArea(u.id("files-list-scroll")).FillWidth().Height(480).Children(func() {
			if listing.Error != "" {
				f.Text("读取失败："+listing.Error).FontSize(12).Color(u.c.red).MarginSides(8, 0, 0, 0)
				return
			}
			if len(entries) == 0 {
				f.Text("此目录为空").FontSize(12).Color(u.c.muted).MarginSides(8, 0, 0, 0)
				return
			}
			for _, e := range entries {
				u.fileRow(f, d, e)
			}
		})
	})
}

func (u *UI) fileRow(f *flex.Frame, d domain.Device, e domain.FileEntry) {
	full := joinDevice(u.files.dir, e.Name)
	rowID := u.id("file/" + full)
	f.Row().FillWidth().Height(34).Gap(8).PadX(8).Items(flex.Center).HoverBackground(u.c.selected).Children(func() {
		if u.files.renaming == e.Name {
			f.Field(&u.files.name, u.id("file-rename-input"), "新名称", e.Name).Grow(1).Height(32)
			if f.Button(u.id("file-rename-save"), "保存").Small().Disabled(d.State != "device").Clicked() {
				name := strings.TrimSpace(u.files.name.Text())
				if name != "" && name != e.Name {
					u.backend.FileRename(d.Serial, full, joinDevice(u.files.dir, name))
				}
				u.files.renaming = ""
				u.wake.Invalidate()
			}
			if f.Button(u.id("file-rename-cancel"), "取消").Ghost().Small().Clicked() {
				u.files.renaming = ""
				u.wake.Invalidate()
			}
			return
		}
		label := e.Name
		glyph, color := fileEntryIcon(e, u.c.muted, u.c.blue)
		f.Icon(glyph).Size(18, 18).Color(color)
		text := f.Text(label).FontSize(12).Grow(1).Ellipsis().Hit(rowID, label)
		if e.IsDir && text.Clicked() {
			u.files.dir = full
			u.files.menuFor, u.files.renaming, u.files.confirm = "", "", ""
			u.backend.FetchFiles(d.Serial, full)
			u.wake.Invalidate()
		}
		size := formatBytes(e.Size)
		if e.IsDir {
			size = "-"
		}
		f.Text(size).FontSize(11).Color(u.c.muted).Width(82).TextEnd()
		f.Text(e.ModTime).FontSize(11).Color(u.c.muted).Width(128).TextEnd()
		if f.Button(u.id("file-menu-"+full), "⋯").Ghost().Small().Clicked() {
			if u.files.menuFor == e.Name {
				u.files.menuFor = ""
			} else {
				u.files.menuFor = e.Name
				u.files.confirm = ""
				u.files.renaming = ""
			}
			u.wake.Invalidate()
		}
	})
	if u.files.menuFor != e.Name {
		return
	}
	u.card(f, func() {
		if u.files.confirm == e.Name {
			u.small(f, "确认删除“"+e.Name+"”？此操作不可恢复。")
			f.Row().Gap(10).Children(func() {
				if f.Button(u.id("file-delete-yes"), "确认删除").Destructive().Small().Disabled(d.State != "device").Clicked() {
					u.backend.FileRemove(d.Serial, full, e.IsDir)
					u.files.confirm, u.files.menuFor = "", ""
					u.wake.Invalidate()
				}
				if f.Button(u.id("file-delete-no"), "保留").Ghost().Small().Clicked() {
					u.files.confirm = ""
					u.wake.Invalidate()
				}
			})
			return
		}
		f.Row().FillWidth().Gap(10).Children(func() {
			if f.Button(u.id("file-download"), "下载").Small().Outline().Disabled(d.State != "device").Clicked() {
				u.backend.FileDownload(d.Serial, full, "")
				u.files.menuFor = ""
				u.wake.Invalidate()
			}
			if f.Button(u.id("file-rename"), "重命名").Small().Outline().Disabled(d.State != "device").Clicked() {
				u.files.renaming = e.Name
				u.files.name.SetText(e.Name)
				u.files.menuFor = ""
				u.wake.Invalidate()
			}
			if f.Button(u.id("file-delete"), "删除").Small().Outline().Disabled(d.State != "device").Clicked() {
				u.files.confirm = e.Name
				u.wake.Invalidate()
			}
		})
	})
}

// filterFiles applies the search box to the current directory's listing.
func (u *UI) filterFiles(entries []domain.FileEntry) []domain.FileEntry {
	query := strings.ToLower(strings.TrimSpace(u.files.search.Text()))
	if query == "" {
		return entries
	}
	var found []domain.FileEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name), query) {
			found = append(found, e)
		}
	}
	return found
}

// joinDevice joins with forward slashes whatever the host platform is.
func joinDevice(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return path.Join(dir, name)
}

// parentDir returns the parent of an Android device path.
func parentDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}

// fileIconButton draws the original Motty-style glyph in a compact, accessible
// hit target while retaining the existing ADB action handlers.
func (u *UI) fileIconButton(f *flex.Frame, key string, glyph ui.Icon, label string, disabled bool) bool {
	button := f.Column().Size(30, 30).Center().Radius(6).HoverBackground(u.c.selected).
		HitWith(u.id(key), label, ui.HitOptions{Role: ui.RoleButton}).Disabled(disabled)
	button.Children(func() {
		color := u.c.muted
		if !disabled {
			color = u.c.blue
		}
		f.Icon(glyph).Size(16, 16).Color(color)
	})
	return button.Clicked()
}

// fileEntryIcon reuses the Material Icon Theme glyphs shown in Motty's SFTP
// list: yellow folders, configuration gears, shell-script terminal, and file.
func fileEntryIcon(entry domain.FileEntry, muted, accent ui.Color) (ui.Icon, ui.Color) {
	if entry.IsDir {
		return mottyFolderSolid, accent
	}
	name := strings.ToLower(entry.Name)
	switch {
	case strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".conf") || strings.HasSuffix(name, ".cfg") || strings.HasSuffix(name, ".ini"):
		return mottyMitSettings, accent
	case strings.HasSuffix(name, ".sh"), strings.HasSuffix(name, ".bash"), strings.HasSuffix(name, ".zsh"), strings.HasSuffix(name, ".fish"):
		return mottyMitConsole, accent
	default:
		return mottyFile, muted
	}
}
