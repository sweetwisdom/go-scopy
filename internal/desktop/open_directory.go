package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// openLocalDirectory opens a directory, unlike app.OpenPath which only accepts documents.
func openLocalDirectory(dir string) error {
	if dir == "" {
		return fmt.Errorf("请先设置保存目录")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("保存目录无效：%w", err)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("无法创建目录：%w", err)
	}
	if os.Getenv("VIVA_HEADLESS") == "1" {
		return nil
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("explorer.exe", dir)
	case "darwin":
		command = exec.Command("open", dir)
	default:
		command = exec.Command("xdg-open", dir)
	}
	if err = command.Start(); err != nil {
		return fmt.Errorf("无法打开目录：%w", err)
	}
	go func() { _ = command.Wait() }()
	return nil
}
