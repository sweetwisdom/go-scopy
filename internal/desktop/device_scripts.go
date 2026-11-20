package desktop

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

type scriptPanel struct {
	path, python, timeout flex.Field
	loaded, trusted       bool
	selected              string
}

func scriptStatus(status string) string {
	switch status {
	case "running":
		return "运行中"
	case "stopping":
		return "正在停止"
	case "success":
		return "已完成"
	case "failed":
		return "失败"
	case "timeout":
		return "已超时"
	case "cancelled":
		return "已取消"
	case "interrupted":
		return "中断"
	default:
		return status
	}
}

func (u *UI) deviceScripts(f *flex.Frame, d domain.Device) {
	backend, ok := u.backend.(domain.ScriptBackend)
	if !ok {
		u.small(f, "当前后端未启用 Python 脚本运行器。")
		return
	}
	state := backend.ScriptState()
	p := &u.scripts
	if !p.loaded {
		p.loaded = true
		p.path.SetText(state.Last.Path)
		p.python.SetText(state.Last.Python)
		seconds := state.Last.TimeoutSeconds
		if seconds == 0 {
			seconds = 600
		}
		p.timeout.SetText(strconv.Itoa(seconds))
	}
	busy := false
	for _, r := range state.Runs {
		if r.Request.Serial == d.Serial && r.Active() {
			busy = true
		}
	}
	u.card(f, func() {
		f.Row().FillWidth().Gap(10).Items(flex.Center).Children(func() {
			f.Text("PYTHON / 自动化").FontSize(16).Weight(ui.WeightSemibold).Grow(1)
			f.Text("uiautomator2 · 独立进程").FontSize(11).Color(u.c.muted)
		})
		u.small(f, "脚本操作当前设备；投屏可保持开启。请避免同时手动操作或运行其他 UI 自动化服务。")
		u.pathField(f, "Python 脚本", "script-file", &p.path, "选择 .py 文件")
		u.pathField(f, "Python 解释器（可选）", "script-python", &p.python, "留空使用 PATH 中的默认 Python")
		f.Row().FillWidth().Gap(12).Items(flex.Center).Children(func() {
			f.Field(&p.timeout, u.id("script-timeout"), "执行超时（秒）", "600").Width(160).Height(36)
			f.Text("目标：" + d.Serial).FontSize(12).Color(u.c.muted).Grow(1).Ellipsis()
		})
		u.toggle(f, "script-trusted", "我信任此脚本，允许其访问本机文件、网络和所选设备", &p.trusted)
		f.Row().Gap(10).Children(func() {
			if f.Button(u.id("script-run"), "运行脚本").Disabled(d.State != "device" || busy || !p.trusted).Clicked() {
				seconds, err := strconv.Atoi(strings.TrimSpace(p.timeout.Text()))
				if err != nil || seconds < 1 || seconds > 86400 {
					u.notice = "超时应为 1–86400 秒"
				} else {
					id, e := backend.RunScript(domain.ScriptRequest{Path: p.path.Text(), Python: p.python.Text(), Serial: d.Serial, TimeoutSeconds: seconds})
					if e != nil {
						u.notice = e.Error()
					} else {
						p.selected = id
						u.notice = "脚本已启动"
					}
				}
				u.wake.Invalidate()
			}
			if busy {
				f.Text("当前设备已有脚本运行中").FontSize(12).Color(u.c.blue)
			}
		})
		u.small(f, "首次使用：python scripts/prepare-python.py；运行时不自动下载或安装依赖。脚本不是沙箱。")
		if state.Error != "" {
			f.Text(state.Error).FontSize(12).Color(u.c.red)
		}
	})
	var selected *domain.ScriptRun
	count := 0
	u.card(f, func() {
		f.Text("执行记录").FontSize(15).Weight(ui.WeightSemibold)
		for i := range state.Runs {
			r := &state.Runs[i]
			if r.Request.Serial != d.Serial {
				continue
			}
			count++
			if selected == nil {
				selected = r
			}
			if p.selected == r.ID {
				selected = r
			}
			f.Row().FillWidth().Gap(8).Items(flex.Center).Children(func() {
				label := r.Started.Format("01-02 15:04:05") + "  " + filepath.Base(r.Request.Path) + " · " + scriptStatus(r.Status)
				b := f.Button(u.id("script-history/"+r.ID), label).Ghost().Grow(1)
				if p.selected == r.ID {
					b.Background(u.c.selected)
				}
				if b.Clicked() {
					p.selected = r.ID
					u.wake.Invalidate()
				}
				if r.Active() && f.Button(u.id("script-stop/"+r.ID), "停止").Small().Outline().Disabled(r.Status == "stopping").Clicked() {
					backend.StopScript(r.ID)
				}
			})
		}
		if count == 0 {
			u.small(f, "还没有执行记录。选择可信的 Python 脚本后运行。")
		}
	})
	if selected != nil {
		r := *selected
		u.card(f, func() {
			f.Row().FillWidth().Items(flex.Center).Children(func() {
				f.Text("实时输出 / " + scriptStatus(r.Status)).FontSize(15).Weight(ui.WeightSemibold).Grow(1)
				if f.Button(u.id("script-open-output"), "打开日志与产物").Small().Outline().Clicked() {
					if e := app.OpenPath(r.OutputDir); e != nil {
						u.notice = e.Error()
					}
				}
			})
			u.small(f, fmt.Sprintf("退出码 %d · %s", r.ExitCode, r.Request.Path))
			if r.Error != "" {
				f.Text(r.Error).FontSize(12).Color(u.c.red)
			}
			// Show a bounded tail so an untrusted script cannot exhaust the UI.
			text := r.Log
			if len(text) > 12000 {
				text = strings.ToValidUTF8(text[len(text)-12000:], "�")
			}
			lines := strings.Split(text, "\n")
			if len(lines) > 100 {
				lines = lines[len(lines)-100:]
			}
			f.Column().FillWidth().Pad(12).Gap(3).Radius(8).Background(u.c.bg).Children(func() {
				for _, line := range lines {
					f.Text(line).FontSize(12).Color(u.c.muted).FillWidth().Ellipsis()
				}
			})
			u.small(f, "停止/超时终止桌面脚本进程；已发给设备的操作不保证撤销。完整日志最多 8 MiB。")
		})
	}
}
