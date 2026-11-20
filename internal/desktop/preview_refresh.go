package desktop

import (
	"strconv"
	"strings"
	"time"

	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// All scheduling state is UI-thread-owned. Timer callbacks only invalidate;
// they never inspect the selected device or access the backend/UI state.
type previewRefreshState struct {
	serial          string
	waiting         bool
	interval        time.Duration
	next, scheduled time.Time
	timer           *time.Timer
}

func (r *previewRefreshState) stopTimer() {
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.scheduled = time.Time{}
}

// step returns one acquisition at entry and then waits an interval after each
// completion, including failures. An in-flight request never queues another.
func (r *previewRefreshState) step(now time.Time, serial string, interval time.Duration, loading bool) bool {
	if serial == "" {
		r.serial, r.waiting, r.next = "", false, time.Time{}
		return false
	}
	entry := r.serial != serial
	if entry {
		r.serial, r.waiting, r.next = serial, false, time.Time{}
	}
	if interval != r.interval {
		r.interval = interval
		r.next = now.Add(interval)
	}
	if loading {
		r.waiting, r.next = true, time.Time{}
		return false
	}
	if r.waiting {
		r.waiting, r.next = false, now.Add(interval)
		return false
	}
	if entry || (interval > 0 && !r.next.IsZero() && !now.Before(r.next)) {
		r.waiting, r.next = true, time.Time{}
		return true
	}
	return false
}

func (u *UI) syncPreviewRefresh(now time.Time) {
	backend, supported := u.backend.(domain.ScreenPreviewBackend)
	serial := ""
	if supported && !u.windowClosed && !u.minimized && !u.closing && !u.connecting && u.section == "devices" && u.deviceTab == "overview" {
		for _, d := range u.state.Devices {
			if d.Serial == u.selected && d.State == "device" {
				serial = d.Serial
				break
			}
		}
	}
	interval := time.Duration(u.state.Settings.ScreenPreviewIntervalSeconds) * time.Second
	loading := u.state.Preview.Serial == serial && u.state.Preview.Loading
	if u.previewRefresh.step(now, serial, interval, loading) {
		backend.FetchPreview(serial)
		// Ensure a completion is observed even by immediate/offline backends.
		u.wake.Invalidate()
	}
	r := &u.previewRefresh
	if serial == "" || interval <= 0 || r.next.IsZero() {
		r.stopTimer()
		return
	}
	if r.scheduled.Equal(r.next) {
		return
	}
	r.stopTimer()
	r.scheduled = r.next
	r.timer = time.AfterFunc(max(r.next.Sub(now), time.Millisecond), u.wake.Invalidate)
}

func (u *UI) previewRefreshSettings(f *flex.Frame) {
	u.card(f, func() {
		f.Text("设备屏幕预览").FontSize(16).Weight(ui.WeightSemibold)
		u.small(f, "连接设备、进入设备概况时立即刷新；仅在概况页定时刷新，最小化时暂停。")
		f.Row().FillWidth().Gap(12).Items(flex.Center).Children(func() {
			f.Text("自动刷新间隔（秒）").FontSize(12)
			f.Field(&u.previewInterval, u.id("preview-interval"), "自动刷新间隔（秒）", "5").Width(90).Height(34)
			if f.Button(u.id("save-preview-interval"), "保存刷新频率").Small().Outline().Clicked() {
				seconds, err := strconv.Atoi(strings.TrimSpace(u.previewInterval.Text()))
				settings := u.state.Settings
				settings.ScreenPreviewIntervalSeconds = seconds
				if err != nil {
					u.notice = "刷新间隔请输入整数秒"
				} else if err := config.Validate(settings); err != nil {
					u.notice = err.Error()
				} else {
					u.backend.Save(settings)
					u.notice = "刷新频率已提交保存"
				}
				u.wake.Invalidate()
			}
		})
		u.small(f, "默认 5 秒；可设置 1–300 秒，0 关闭定时刷新。截图只保留在内存中。")
	})
}
