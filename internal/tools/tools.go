// Package tools adapts official adb and scrcpy binaries without a shell.
package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/go-scopy/go-scopy/internal/domain"
)

type Tools struct{ ADB, Scrcpy string }

func Resolve(custom, name, root string) (string, error) {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if custom != "" {
		p, err := filepath.Abs(custom)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(p)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", fmt.Errorf("请选择可执行文件：%s", p)
		}
		return p, nil
	}
	for _, p := range []string{filepath.Join(root, "tools", "scrcpy", name), filepath.Join(root, "tools", name)} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("未找到 %s，请在设置中指定路径或使用完整便携包", name)
	}
	return filepath.Abs(p)
}

func Command(ctx context.Context, path string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, path, args...)
	c.Dir = filepath.Dir(path)
	configureProcess(c)
	return c
}

func (t Tools) Run(ctx context.Context, args ...string) ([]byte, error) {
	if t.ADB == "" {
		return nil, errors.New("adb 未就绪")
	}
	c := Command(ctx, t.ADB, append([]string{"-P", "5037"}, args...)...)
	c.Env = localADBEnvironment(t.ADB)
	var stderr limitedBuffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("adb：%s (%w)", strings.TrimSpace(stderr.String()), err)
	}
	// adb connect/pair may return exit code 0 even on failure.
	text := strings.TrimSpace(string(out))
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "failed") || strings.HasPrefix(lower, "cannot") || strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "unable") || strings.Contains(text, "Failure [") {
		return nil, errors.New(text)
	}
	return out, nil
}

func (t Tools) Device(ctx context.Context, serial string, args ...string) ([]byte, error) {
	if serial == "" {
		return nil, errors.New("请先选择设备")
	}
	return t.Run(ctx, append([]string{"-s", serial}, args...)...)
}

func ValidateAddress(address string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil || host == "" {
		return errors.New("请输入 IP:端口，例如 192.168.1.20:5555")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return errors.New("端口应为 1–65535")
	}
	if strings.ContainsAny(host, " \t\n\r") {
		return errors.New("设备地址包含无效字符")
	}
	return nil
}

func ScrcpyArgs(serial string, o domain.Options, recording string) ([]string, error) {
	if serial == "" {
		return nil, errors.New("请先选择设备")
	}
	if err := config.ValidateOptions(o); err != nil {
		return nil, err
	}
	a := []string{"--serial", serial, "--window-title", "Go-Scopy · " + serial, "--max-fps", strconv.Itoa(o.FPS), "--video-bit-rate", strconv.Itoa(o.Bitrate) + "M"}
	if o.MaxSize > 0 {
		a = append(a, "--max-size", strconv.Itoa(o.MaxSize))
	}
	if !o.Audio {
		a = append(a, "--no-audio")
	}
	if o.ScreenOff {
		a = append(a, "--turn-screen-off")
	}
	if o.StayAwake {
		a = append(a, "--stay-awake")
	}
	if o.AlwaysOnTop {
		a = append(a, "--always-on-top")
	}
	if recording != "" {
		a = append(a, "--record", recording)
	}
	return a, nil
}

// TailBuffer keeps diagnostic output bounded for long-running sessions.
type limitedBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	b.data = append(b.data, p...)
	if len(b.data) > 16384 {
		b.data = bytes.Clone(b.data[len(b.data)-16384:])
	}
	return n, nil
}
func (b *limitedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }

type Process struct {
	Cmd    *exec.Cmd
	Done   chan struct{}
	Output limitedBuffer
	Err    error
}

func (t Tools) Start(serial string, o domain.Options, recording string) (*Process, error) {
	return t.start(serial, o, recording, false)
}

func (t Tools) StartHosted(serial string, o domain.Options, recording string) (*Process, error) {
	return t.start(serial, o, recording, true)
}

func (t Tools) start(serial string, o domain.Options, recording string, hosted bool) (*Process, error) {
	if t.Scrcpy == "" {
		return nil, errors.New("scrcpy 未就绪")
	}
	a, err := ScrcpyArgs(serial, o, recording)
	if err != nil {
		return nil, err
	}
	if hosted {
		a = append(a, "--window-borderless", "--no-window-aspect-ratio-lock", "--window-x=-32000", "--window-y=-32000")
	}
	p := &Process{Cmd: Command(context.Background(), t.Scrcpy, a...), Done: make(chan struct{})}
	p.Cmd.Stdout = &p.Output
	p.Cmd.Stderr = &p.Output
	// Pin scrcpy to the same adb executable and local server used by the app.
	p.Cmd.Env = localADBEnvironment(t.ADB)
	if server := filepath.Join(filepath.Dir(t.Scrcpy), "scrcpy-server"); fileExists(server) {
		p.Cmd.Env = replaceEnv(p.Cmd.Env, "SCRCPY_SERVER_PATH", server)
	}
	if err = p.Cmd.Start(); err != nil {
		return nil, err
	}
	go func() { p.Err = p.Cmd.Wait(); close(p.Done) }()
	return p, nil
}

func (p *Process) RequestStop() {
	select {
	case <-p.Done:
		return
	default:
		requestStop(p.Cmd.Process)
	}
}

func fileExists(path string) bool { info, err := os.Stat(path); return err == nil && !info.IsDir() }

func replaceEnv(env []string, key, value string) []string {
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		existing, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(existing, key) {
			result = append(result, entry)
		}
	}
	return append(result, key+"="+value)
}

func localADBEnvironment(adb string) []string {
	result := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "ADB", "ADB_SERVER_SOCKET", "ANDROID_ADB_SERVER_ADDRESS", "ANDROID_ADB_SERVER_PORT":
			continue
		}
		result = append(result, entry)
	}
	return append(result, "ADB="+adb, "ANDROID_ADB_SERVER_PORT=5037")
}
