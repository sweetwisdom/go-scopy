package desktop

import (
	"errors"
	"strings"

	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

func (u *UI) appExportSettings(f *flex.Frame) {
	u.card(f, func() {
		f.Text("应用导出").FontSize(16).Weight(ui.WeightSemibold)
		u.pathField(f, "上次选择的导出目录", "app-export-dir", &u.appExportDir, "尚未选择，导出时会自动记住")
		f.Row().Gap(10).Children(func() {
			if f.Button(u.id("save-app-export-settings"), "保存导出设置").Clicked() {
				settings := u.backend.State().Settings
				settings.AppExportDir = strings.TrimSpace(u.appExportDir.Text())
				if err := config.Validate(settings); err != nil {
					u.notice = err.Error()
				} else {
					u.backend.Save(settings)
					u.state.Settings.AppExportDir = settings.AppExportDir
					u.notice = "应用导出设置已提交保存"
				}
				u.wake.Invalidate()
			}
			dir := u.state.Settings.AppExportDir
			if f.Button(u.id("open-app-export-dir"), "打开导出目录").Outline().Disabled(dir == "").Clicked() {
				if err := app.OpenPath(dir); err != nil {
					u.notice = err.Error()
					u.wake.Invalidate()
				}
			}
		})
		u.small(f, "自动记住上次选择的位置，下次导出优先打开该目录；单个另存为和批量导出共用此设置。")
		u.small(f, "开始新的导出后自动收起旧结果；不会删除文件，也不会中断仍在传输的任务。")
	})
}

func (u *UI) rememberAppExportDirectory(dir string) {
	u.appExportDir.SetText(dir)
	settings := u.backend.State().Settings
	u.state.Settings.AppExportDir = dir
	if settings.AppExportDir == dir {
		return
	}
	settings.AppExportDir = dir
	u.backend.Save(settings)
}

func (u *UI) chooseAppExportDirectory() {
	dir, err := u.appExportPicker("选择应用导出目录", "", false)
	if errors.Is(err, app.ErrCanceled) || (err == nil && dir == "") {
		return
	}
	if err != nil {
		u.notice = "选择导出目录失败：" + err.Error()
	} else {
		u.rememberAppExportDirectory(dir)
		u.notice = "应用导出目录已提交保存"
	}
	u.wake.Invalidate()
}

func (u *UI) appExportPicker(title, filename string, single bool) (string, error) {
	return pickAppExportDestination(u.state.Settings.AppExportDir, title, filename, single, u.application != nil)
}

// Keep the regular Viva picker as the first-use and headless-driver path.
func basicAppExportPicker(title, filename string, single bool) (string, error) {
	if single {
		return app.SaveFile(app.SaveFileOptions{Title: title, SuggestedName: filename, DefaultExtension: "apk", Filters: []app.FileFilter{{Name: "Android 安装包", Pattern: "*.apk"}}})
	}
	return app.OpenDirectory(app.OpenDirectoryOptions{Title: title})
}
