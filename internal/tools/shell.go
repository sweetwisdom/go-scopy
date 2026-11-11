package tools

import (
	"os"
	"path/filepath"

	"github.com/go-scopy/go-scopy/internal/tools/pty"
)

// DeviceShell opens an interactive adb shell on a pseudo-terminal, so line
// editing, colors and full-screen programs behave as they do in a terminal.
// The caller drains output and closes the session; sizing works because the
// adb client resizes the device pty when the pseudo-terminal changes.
func (t Tools) DeviceShell(serial string, cols, rows int, notify func()) (*pty.Shell, error) {
	if t.ADB == "" {
		return nil, errUnsupported("adb 未就绪")
	}
	return pty.Start(pty.Config{
		Cols: cols, Rows: rows, Path: t.ADB,
		Args:   []string{"-P", "5037", "-s", serial, "shell", "-t"},
		Env:    localADBEnvironment(t.ADB),
		Notify: notify,
	})
}

// DeviceLogcat streams the device log through the same pseudo-terminal path,
// so the terminal view renders it without a second code path.
func (t Tools) DeviceLogcat(serial string, cols, rows int, notify func()) (*pty.Shell, error) {
	if t.ADB == "" {
		return nil, errUnsupported("adb 未就绪")
	}
	return pty.Start(pty.Config{
		Cols: cols, Rows: rows, Path: t.ADB,
		Args:   []string{"-P", "5037", "-s", serial, "logcat", "-v", "threadtime"},
		Env:    localADBEnvironment(t.ADB),
		Notify: notify,
	})
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
