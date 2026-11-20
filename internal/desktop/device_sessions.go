package desktop

import (
	"sort"
	"strconv"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/flex"
)

func (u *UI) virtualAppLabel(serial, pkg string) string {
	if label := u.appMeta(serial, pkg).Label; label != "" {
		return label
	}
	for _, a := range u.state.Apps[serial].Apps {
		if a.Package == pkg && a.Label != "" {
			return a.Label
		}
	}
	return pkg
}

// Stable ordering keeps each stop button attached to the same session while
// asynchronous launches finish. Never reconstruct a virtual ID from a device.
func (u *UI) virtualSessions(serial string) []domain.Session {
	var sessions []domain.Session
	for _, s := range u.state.Sessions {
		if s.Serial == serial && s.Kind == domain.SessionVirtual {
			sessions = append(sessions, s)
		}
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Started.Equal(sessions[j].Started) {
			return sessions[i].ID < sessions[j].ID
		}
		return sessions[i].Started.Before(sessions[j].Started)
	})
	return sessions
}

func (u *UI) stopVirtualSession(id string) {
	if window := u.sessionWindows[id]; window != nil {
		window.requestClose()
	} else {
		u.backend.Stop(id)
	}
}

func (u *UI) deviceVirtualSessions(f *flex.Frame, serial string) {
	sessions := u.virtualSessions(serial)
	if len(sessions) == 0 {
		return
	}
	f.Column().FillWidth().Gap(6).MarginSides(0, 0, 14, 0).Children(func() {
		for i, s := range sessions {
			f.Row().FillWidth().Gap(8).Items(flex.Center).Children(func() {
				label := u.virtualAppLabel(serial, s.StartApp)
				if label == "" {
					label = "虚拟屏幕"
				}
				f.Text(label).FontSize(12).Grow(1).Ellipsis()
				status := "运行中"
				switch s.Status {
				case "starting":
					status = "启动中"
				case "stopping":
					status = "结束中"
				}
				// The ordinal distinguishes repeated launches of the same package.
				u.small(f, "#"+strconv.Itoa(i+1)+" · "+status)
				if window := u.sessionWindows[s.ID]; window != nil {
					if f.Button(u.id("vd-focus-"+s.ID), "显示").Small().Ghost().Disabled(window.closing).Clicked() {
						window.window.Show()
						window.native.Focus()
					}
				}
				if f.Button(u.id("vd-stop-"+s.ID), "停止").Small().Outline().Disabled(s.Status != "running").Clicked() {
					u.stopVirtualSession(s.ID)
				}
			})
		}
	})
}
