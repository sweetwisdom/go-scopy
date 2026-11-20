//go:build windows

package automation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A Job Object owns only this script's local process tree. Closing it also
// reaps children left behind by a normally exited script. The shared adb server
// must be started by the application/CLI before launching scripts.
func runProcess(ctx context.Context, cmd *exec.Cmd) (int, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return -1, err
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return -1, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return windows.TerminateJobObject(job, 1)
	}
	if err = cmd.Start(); err != nil {
		return -1, err
	}
	handle, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if e == nil {
		e = windows.AssignProcessToJobObject(job, handle)
		windows.CloseHandle(handle)
	}
	if e != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return -1, fmt.Errorf("无法接管脚本进程树：%w", e)
	}
	// Cancellation may have arrived between Start and assigning the process.
	if ctx.Err() != nil {
		_ = windows.TerminateJobObject(job, 1)
	}
	err = cmd.Wait()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return code, err
}
