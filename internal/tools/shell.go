package tools

import (
	"context"
	"os"
	"path/filepath"

	"github.com/go-scopy/go-scopy/internal/tools/pty"
)

// DeviceShell follows escrcpy's device provider: a remote Android PTY with
// ordinary local pipes. There is no Windows ConPTY in this path.
func (t Tools) DeviceShell(serial string, cols, rows int, notify func()) (*pty.Shell, error) {
	return t.deviceTerminal(serial, []string{"shell", "-tt"}, notify)
}

// DeviceLogcat uses the same bounded pipe stream as the device shell.
func (t Tools) DeviceLogcat(serial string, cols, rows int, notify func()) (*pty.Shell, error) {
	return t.deviceTerminal(serial, []string{"logcat", "-v", "threadtime"}, notify)
}

func (t Tools) deviceTerminal(serial string, args []string, notify func()) (*pty.Shell, error) {
	if t.ADB == "" {
		return nil, errUnsupported("adb 未就绪")
	}
	if serial == "" {
		return nil, errUnsupported("请先选择设备")
	}
	cmd := Command(context.Background(), t.ADB, append([]string{"-P", "5037", "-s", serial}, args...)...)
	cmd.Env = localADBEnvironment(t.ADB)
	return pty.StartCommand(cmd, notify)
}

// tempFilePath reserves a scratch file path beside the system temp directory.
func tempFilePath(name string) (string, error) {
	dir, err := os.MkdirTemp("", "go-scopy-")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func removeQuiet(path string) {
	_ = os.RemoveAll(path)
	if dir := filepath.Dir(path); filepath.Base(dir) != "" {
		// The file lived in its own temp directory; drop it too.
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			entries, _ := os.ReadDir(dir)
			if len(entries) == 0 {
				_ = os.Remove(dir)
			}
		}
	}
}
