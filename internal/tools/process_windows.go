//go:build windows

package tools

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

func configureProcess(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} }

// WM_CLOSE lets SDL finish recording and flush its output before exiting.
var (
	user32              = syscall.NewLazyDLL("user32.dll")
	enumWindows         = user32.NewProc("EnumWindows")
	windowPID           = user32.NewProc("GetWindowThreadProcessId")
	postMessage         = user32.NewProc("PostMessageW")
	closeWindowCallback = syscall.NewCallback(func(hwnd, target uintptr) uintptr {
		var pid uint32
		windowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == uint32(target) {
			postMessage.Call(hwnd, 0x0010, 0, 0)
		}
		return 1
	})
)

func requestStop(p *os.Process) { enumWindows.Call(closeWindowCallback, uintptr(p.Pid)) }
