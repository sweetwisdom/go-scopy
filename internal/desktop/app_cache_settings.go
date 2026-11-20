package desktop

import (
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

func (u *UI) appCacheSettings(f *flex.Frame) {
	_, supported := u.backend.(domain.AppCacheBackend)
	u.card(f, func() {
		f.Text("应用缓存").FontSize(16).Weight(ui.WeightSemibold)
		u.small(f, "应用名称和图标按包名全局共享，USB、Wi-Fi 和不同手机直接复用；安装列表仍按设备读取。")
		u.small(f, "应用更新后图标仍是旧版本时，可清理缓存重新获取；不会卸载应用或删除导出文件。")
		if f.Button(u.id("clear-app-cache"), "清理应用缓存").Outline().Disabled(!supported).Clicked() {
			u.clearAppCache = true
			u.wake.Invalidate()
		}
	})
}

func (u *UI) appCacheDialog(f *flex.Frame) {
	if !u.clearAppCache {
		return
	}
	d := f.Dialog(flex.DialogSpec{Title: "清理应用缓存？", Description: "清理全部设备共享的应用名称和图标", Width: 480,
		Scope: u.id("clear-app-cache-dialog"), DismissID: u.id("cancel-clear-app-cache"), CloseID: u.id("close-clear-app-cache")})
	d.Body(func() {
		u.small(f, "只清理本机应用缓存，不会删除设备记录、投屏参数、脚本、APK 导出文件或手机数据。下次打开应用列表时重新加载。")
	})
	d.Footer(func() {
		f.Button(u.id("cancel-clear-app-cache"), "取消").Outline()
		if f.Button(u.id("confirm-clear-app-cache"), "确认清理缓存").Destructive().Clicked() {
			if backend, ok := u.backend.(domain.AppCacheBackend); ok {
				backend.ClearAppCache()
				u.notice = "缓存清理已提交，可在任务中心查看结果"
			}
			u.clearAppCache = false
			u.wake.Invalidate()
		}
	})
	if d.Dismissed() {
		u.clearAppCache = false
		u.wake.Invalidate()
	}
}
