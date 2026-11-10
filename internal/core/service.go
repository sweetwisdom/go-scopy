// Package core owns device workflows, process lifetimes and persistent settings.
package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools"
)

type Service struct {
	mu        sync.Mutex
	configMu  sync.Mutex
	state     domain.State
	tool      tools.Tools
	dir       string
	ctx       context.Context
	cancel    context.CancelFunc
	notify    func()
	processes map[string]*tools.Process
	closed    bool
	wg        sync.WaitGroup
	next      uint64
}

func New(dir, root string) (*Service, error) {
	settings, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{dir: dir, ctx: ctx, cancel: cancel, processes: map[string]*tools.Process{}}
	s.state = domain.State{Settings: settings, Sessions: map[string]domain.Session{}, Connection: "正在检查运行环境…"}
	adb, ae := tools.Resolve(settings.ADBPath, "adb", root)
	scrcpy, se := tools.Resolve(settings.ScrcpyPath, "scrcpy", root)
	s.tool = tools.Tools{ADB: adb, Scrcpy: scrcpy}
	s.state.ADBPath = adb
	s.state.ScrcpyPath = scrcpy
	var problems []string
	for _, e := range []error{ae, se} {
		if e != nil {
			problems = append(problems, e.Error())
		}
	}
	s.state.ToolError = strings.Join(problems, "；")
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.monitor() }()
	return s, nil
}

func cloneSettings(v domain.Settings) domain.Settings {
	v.Devices = cloneMap(v.Devices)
	v.DeviceNames = cloneMap(v.DeviceNames)
	v.SavedWireless = cloneMap(v.SavedWireless)
	return v
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	n := make(map[K]V, len(m))
	for k, v := range m {
		n[k] = v
	}
	return n
}

func (s *Service) State() domain.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.state
	v.Devices = append([]domain.Device(nil), v.Devices...)
	v.Tasks = append([]domain.Task(nil), v.Tasks...)
	v.Sessions = cloneMap(v.Sessions)
	v.Settings = cloneSettings(v.Settings)
	seen := make(map[string]bool, len(v.Devices))
	for _, d := range v.Devices {
		seen[d.Serial] = true
	}
	for address, saved := range v.Settings.SavedWireless {
		if saved && !seen[address] {
			v.Devices = append(v.Devices, domain.Device{Serial: address, State: "offline", Connection: "Wi-Fi"})
		}
	}
	sort.Slice(v.Devices, func(i, j int) bool { return v.Devices[i].Serial < v.Devices[j].Serial })
	return v
}

func (s *Service) SetNotify(fn func()) { s.mu.Lock(); s.notify = fn; s.mu.Unlock(); s.wake() }
func (s *Service) wake() {
	s.mu.Lock()
	fn := s.notify
	closed := s.closed
	s.mu.Unlock()
	if fn != nil && !closed {
		fn()
	}
}

func (s *Service) connection(message string, offline bool) {
	s.mu.Lock()
	changed := s.state.Connection != message
	s.state.Connection = message
	if offline && len(s.state.Devices) > 0 {
		s.state.Devices = nil
		changed = true
	}
	s.mu.Unlock()
	if changed {
		s.wake()
	}
}

func (s *Service) monitor() {
	if s.tool.Scrcpy != "" {
		ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
		out, err := tools.Command(ctx, s.tool.Scrcpy, "--version").CombinedOutput()
		cancel()
		s.mu.Lock()
		if err == nil {
			s.state.ScrcpyVersion = firstLine(string(out))
		} else {
			s.state.ToolError += " scrcpy 无法运行：" + err.Error()
		}
		s.mu.Unlock()
		s.wake()
	}
	if s.tool.ADB == "" {
		s.connection("adb 未就绪，请配置工具路径", true)
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
	out, err := s.tool.Run(ctx, "version")
	cancel()
	s.mu.Lock()
	if err == nil {
		s.state.ADBVersion = firstLine(string(out))
	}
	s.mu.Unlock()
	s.wake()
	delay := time.Second
	for s.ctx.Err() == nil {
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		_, err := s.tool.Run(ctx, "start-server")
		cancel()
		if err == nil {
			err = tools.Track(s.ctx, func(devices []domain.Device) {
				s.mu.Lock()
				changed := !reflect.DeepEqual(s.state.Devices, devices) || s.state.Connection != "设备监听已连接"
				old := s.state.Devices
				s.state.Devices = devices
				s.state.Connection = "设备监听已连接"
				var disconnected []*tools.Process
				for _, before := range old {
					online := false
					for _, after := range devices {
						if after.Serial == before.Serial && after.State == "device" {
							online = true
						}
					}
					if !online {
						if p := s.processes[before.Serial]; p != nil {
							disconnected = append(disconnected, p)
						}
					}
				}
				s.mu.Unlock()
				if changed {
					s.wake()
				}
				for _, p := range disconnected {
					p.RequestStop()
				}
			})
		}
		if s.ctx.Err() != nil {
			return
		}
		s.connection("设备监听中断，将重试："+err.Error(), true)
		timer := time.NewTimer(delay)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, 15*time.Second)
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

func (s *Service) submit(title string, work func(context.Context) (string, string, error)) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.next++
	id := s.next
	s.state.Tasks = append([]domain.Task{{ID: id, Title: title, Status: "running", Started: time.Now()}}, s.state.Tasks...)
	if len(s.state.Tasks) > 100 {
		for i := len(s.state.Tasks) - 1; i >= 100; i-- {
			if s.state.Tasks[i].Status != "running" {
				s.state.Tasks = append(s.state.Tasks[:i], s.state.Tasks[i+1:]...)
			}
		}
	}
	s.wg.Add(1)
	s.mu.Unlock()
	s.wake()
	go func() {
		defer s.wg.Done()
		detail, path, err := work(s.ctx)
		s.mu.Lock()
		for i := range s.state.Tasks {
			if s.state.Tasks[i].ID == id {
				task := &s.state.Tasks[i]
				task.Status = "done"
				task.Detail = detail
				task.Path = path
				if err != nil {
					task.Status = "error"
					task.Detail = err.Error()
				}
				break
			}
		}
		s.mu.Unlock()
		s.wake()
	}()
}

func (s *Service) Refresh() {
	s.submit("刷新设备", func(ctx context.Context) (string, string, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		out, err := s.tool.Run(ctx, "devices", "-l")
		if err == nil {
			s.mu.Lock()
			s.state.Devices = tools.ParseDevices(string(out))
			s.mu.Unlock()
		}
		return "设备列表已更新", "", err
	})
}

func (s *Service) Connect(address string) {
	address = strings.TrimSpace(address)
	s.submit("连接无线设备", func(ctx context.Context) (string, string, error) {
		if err := tools.ValidateAddress(address); err != nil {
			return "", "", err
		}
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		out, err := s.tool.Connect(ctx, address)
		return strings.TrimSpace(string(out)), "", err
	})
}

func (s *Service) Pair(address, code string) {
	address = strings.TrimSpace(address)
	code = strings.TrimSpace(code)
	s.submit("配对无线设备", func(ctx context.Context) (string, string, error) {
		if err := tools.ValidateAddress(address); err != nil {
			return "", "", err
		}
		if len(code) != 6 || strings.IndexFunc(code, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return "", "", errors.New("请输入手机上显示的 6 位配对码")
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		out, err := s.tool.Run(ctx, "pair", address, code)
		if err != nil {
			return "", "", errors.New("配对失败，请检查配对地址和配对码，并保持手机配对页面打开")
		}
		return strings.TrimSpace(string(out)) + "；请使用无线调试页面的连接端口进行连接", "", nil
	})
}

func (s *Service) Disconnect(serial string) {
	s.submit("断开无线设备", func(ctx context.Context) (string, string, error) {
		if err := tools.ValidateAddress(serial); err != nil {
			return "", "", errors.New("仅支持断开以 IP:端口连接的无线设备")
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		out, err := s.tool.Run(ctx, "disconnect", serial)
		return string(out), "", err
	})
}

func (s *Service) online(serial string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.state.Devices {
		if d.Serial == serial {
			if d.State == "device" {
				return nil
			}
			if d.State == "unauthorized" {
				return errors.New("请在手机上允许 USB 调试，再重试")
			}
			return errors.New("设备已离线，请重新连接")
		}
	}
	return errors.New("设备已断开，请重新连接")
}

func (s *Service) EnableTCP(serial string) {
	s.submit("切换无线调试", func(ctx context.Context) (string, string, error) {
		if err := s.online(serial); err != nil {
			return "", "", err
		}
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		host, err := s.tool.WiFiAddress(ctx, serial)
		if err != nil {
			return "", "", err
		}
		if _, err = s.tool.Device(ctx, serial, "tcpip", "5555"); err != nil {
			return "", "", err
		}
		address := net.JoinHostPort(host, "5555")
		var result []byte
		for i := 0; i < 3; i++ {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", "", ctx.Err()
			case <-timer.C:
			}
			result, err = s.tool.Connect(ctx, address)
			if err == nil {
				break
			}
		}
		return strings.TrimSpace(string(result)), "", err
	})
}

func (s *Service) output(serial, ext string) (string, error) {
	s.mu.Lock()
	dir := s.state.Settings.OutputDir
	s.mu.Unlock()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '_'
	}, serial)
	f, err := os.CreateTemp(dir, time.Now().Format("20060102-150405")+"-"+safe+"-*."+ext)
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err = f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func (s *Service) Start(serial string, options domain.Options, record bool) {
	s.submit("投屏 · "+serial, func(ctx context.Context) (detail, path string, err error) {
		if err = s.online(serial); err != nil {
			return
		}
		s.mu.Lock()
		if _, exists := s.state.Sessions[serial]; exists {
			s.mu.Unlock()
			return "", "", errors.New("此设备已有投屏会话")
		}
		s.state.Sessions[serial] = domain.Session{Serial: serial, Status: "starting", Started: time.Now()}
		s.mu.Unlock()
		s.wake()
		defer func() {
			s.mu.Lock()
			delete(s.state.Sessions, serial)
			delete(s.processes, serial)
			s.mu.Unlock()
			s.wake()
		}()
		if record {
			path, err = s.output(serial, "mp4")
			if err != nil {
				return
			}
		}
		p, err := s.tool.Start(serial, options, path)
		if err != nil {
			if path != "" {
				os.Remove(path)
			}
			return "", "", err
		}
		s.mu.Lock()
		closed := s.closed
		s.processes[serial] = p
		s.state.Sessions[serial] = domain.Session{Serial: serial, Status: "running", Recording: path, Started: time.Now()}
		s.mu.Unlock()
		s.wake()
		if closed {
			stopProcess(p)
		}
		select {
		case <-p.Done:
		case <-ctx.Done():
			stopProcess(p)
			<-p.Done
		}
		if p.Err != nil {
			return "", path, fmt.Errorf("投屏已结束：%s (%w)", strings.TrimSpace(p.Output.String()), p.Err)
		}
		if path != "" {
			info, e := os.Stat(path)
			if e != nil || info.Size() == 0 {
				return "", path, errors.New("录屏未生成有效文件，请查看设备连接与 scrcpy 参数")
			}
			return "录屏已保存", path, nil
		}
		return "投屏已结束", "", nil
	})
}

func stopProcess(p *tools.Process) bool {
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	p.RequestStop()
	for {
		select {
		case <-p.Done:
			return false
		case <-ticker.C:
			p.RequestStop()
		case <-deadline.C:
			_ = p.Cmd.Process.Kill()
			<-p.Done
			return true
		}
	}
}

func (s *Service) Stop(serial string) {
	s.submit("停止投屏 · "+serial, func(ctx context.Context) (string, string, error) {
		s.mu.Lock()
		p := s.processes[serial]
		if p != nil {
			v := s.state.Sessions[serial]
			v.Status = "stopping"
			s.state.Sessions[serial] = v
		}
		s.mu.Unlock()
		s.wake()
		if p == nil {
			return "", "", errors.New("投屏尚未启动或已经结束")
		}
		if stopProcess(p) {
			return "", "", errors.New("投屏未响应，已强制结束；录像可能不完整")
		}
		return "投屏已停止", "", nil
	})
}

func (s *Service) Capture(serial string) {
	s.submit("截图 · "+serial, func(ctx context.Context) (string, string, error) {
		if err := s.online(serial); err != nil {
			return "", "", err
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		data, err := s.tool.Device(ctx, serial, "exec-out", "screencap", "-p")
		if err != nil {
			return "", "", err
		}
		if _, err = png.DecodeConfig(bytes.NewReader(data)); err != nil {
			return "", "", fmt.Errorf("设备未返回有效 PNG：%w", err)
		}
		path, err := s.output(serial, "png")
		if err != nil {
			return "", "", err
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			os.Remove(path)
			return "", "", err
		}
		return "截图已保存", path, nil
	})
}

func (s *Service) Install(serial, apk string) {
	s.submit("安装 APK · "+serial, func(ctx context.Context) (string, string, error) {
		if err := s.online(serial); err != nil {
			return "", "", err
		}
		if !strings.EqualFold(filepath.Ext(apk), ".apk") {
			return "", "", errors.New("请选择 .apk 文件")
		}
		info, err := os.Stat(apk)
		if err != nil {
			return "", "", err
		}
		if info.IsDir() {
			return "", "", errors.New("请选择 APK 文件")
		}
		ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		out, err := s.tool.Device(ctx, serial, "install", "-r", apk)
		return strings.TrimSpace(string(out)), "", err
	})
}

func (s *Service) Save(settings domain.Settings) {
	settings = cloneSettings(settings)
	s.submit("保存设置", func(ctx context.Context) (string, string, error) {
		s.configMu.Lock()
		defer s.configMu.Unlock()
		if err := config.Save(s.dir, settings); err != nil {
			return "", "", err
		}
		s.mu.Lock()
		restart := s.state.Settings.ADBPath != settings.ADBPath || s.state.Settings.ScrcpyPath != settings.ScrcpyPath
		s.state.Settings = settings
		s.mu.Unlock()
		if restart {
			return "设置已保存；工具路径在重新启动后生效", "", nil
		}
		return "设置已保存", "", nil
	})
}

func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	// Session workers observe cancellation and close their own scrcpy windows.
	s.wg.Wait()
}
