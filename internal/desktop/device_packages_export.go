package desktop

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

func (u *UI) requestAppExport(serial string, packages []string) {
	if len(packages) == 0 || u.pendingDialog != "" {
		return
	}
	u.apps.exportPackages = append([]string(nil), packages...)
	u.pendingDialog, u.dialogSerial = "app-export", serial
	u.wake.Invalidate()
}

func (u *UI) openAppExportDialog(serial string) {
	packages := u.apps.exportPackages
	u.apps.exportPackages = nil
	if len(packages) == 0 {
		return
	}
	var destination string
	var err error
	single := len(packages) == 1
	if single {
		info := domain.AppInfo{Package: packages[0]}
		for _, a := range u.state.Apps[serial].Apps {
			if a.Package == packages[0] {
				info = a
				break
			}
		}
		filename := domain.AppExportFilename(info, u.state.AppMetas[serial+"\x00"+packages[0]])
		destination, err = u.appExportPicker("导出应用安装包", filename, true)
	} else {
		destination, err = u.appExportPicker(fmt.Sprintf("选择 %d 个应用的导出目录", len(packages)), "", false)
	}
	if errors.Is(err, app.ErrCanceled) || (err == nil && destination == "") {
		return
	}
	if err != nil {
		u.notice = "选择导出位置失败：" + err.Error()
		return
	}
	if serial != u.selected {
		u.notice = "设备已切换，未导出应用"
		return
	}
	online := false
	for _, d := range u.state.Devices {
		if d.Serial == serial && d.State == "device" {
			online = true
			break
		}
	}
	if !online {
		u.notice = "设备已离线，未导出应用"
		return
	}
	dir := destination
	if single {
		dir = filepath.Dir(destination)
	}
	u.rememberAppExportDirectory(dir)
	if u.apps.exportDismissed == nil {
		u.apps.exportDismissed = map[uint64]bool{}
	}
	// Earlier transfers stay visible until they finish; only their result cards are hidden.
	for _, task := range u.backend.State().Tasks {
		if task.Export.Total > 0 && task.Export.Serial == serial {
			u.apps.exportDismissed[task.ID] = true
		}
	}
	u.backend.ExportApps(serial, packages, destination, single)
	u.apps.selected = map[string]bool{}
	u.notice = "已开始导出，可在应用包页面查看进度"
	u.wake.Invalidate()
}

func (u *UI) appExportStatus(f *flex.Frame, serial string) {
	shown := 0
	for _, task := range u.state.Tasks {
		p := task.Export
		if p.Total == 0 || p.Serial != serial || (task.Status != "running" && u.apps.exportDismissed[task.ID]) {
			continue
		}
		if task.Status != "running" && shown >= 3 {
			continue
		}
		shown++
		u.card(f, func() {
			f.Row().FillWidth().Gap(10).Items(flex.Center).Children(func() {
				f.Text("安装包导出 · " + taskLabel(task.Status)).FontSize(13).Weight(ui.WeightMedium).Grow(1)
				f.Text(fmt.Sprintf("%d/%d 已处理", p.Completed, p.Total)).FontSize(11).Color(u.c.muted)
				if task.Status != "running" {
					if f.Button(u.id(fmt.Sprintf("app-export-dismiss-%d", task.ID)), "✕").Ghost().Small().Clicked() {
						if u.apps.exportDismissed == nil {
							u.apps.exportDismissed = map[uint64]bool{}
						}
						u.apps.exportDismissed[task.ID] = true
						u.wake.Invalidate()
					}
				}
			})
			if task.Status == "running" {
				if p.Current != "" {
					u.small(f, p.Current)
				}
				if p.TotalBytes > 0 {
					ratio := min(float32(p.Bytes)/float32(p.TotalBytes), 1)
					f.Progress(ratio).FillWidth().Height(6)
					u.small(f, fmt.Sprintf("传输 %.0f%% · %s / %s", ratio*100, formatBytes(p.Bytes), formatBytes(p.TotalBytes)))
				} else if p.Phase == "准备安装包" {
					u.small(f, "正在读取安装包路径与大小…")
				} else if p.Current != "" {
					f.Progress(0).Indeterminate().FillWidth().Height(6)
					u.small(f, "正在传输 · "+formatBytes(p.Bytes)+"（总大小未知）")
				} else {
					u.small(f, "正在准备导出…")
				}
			} else {
				u.small(f, task.Detail)
				if task.Path != "" {
					u.small(f, task.Path)
					if f.Button(u.id(fmt.Sprintf("app-export-open-%d", task.ID)), "打开保存目录").Small().Outline().Clicked() {
						if err := app.OpenPath(filepath.Dir(task.Path)); err != nil {
							u.notice = err.Error()
							u.wake.Invalidate()
						}
					}
				}
			}
		})
	}
}
