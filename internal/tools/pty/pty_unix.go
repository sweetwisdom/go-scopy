//go:build !windows

package pty

import (
	"errors"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

func startBackend(config Config) (backend, error) {
	cmd := exec.Command(config.Path, config.Args...)
	cmd.Dir = config.Dir
	if config.Env != nil {
		cmd.Env = config.Env
	}
	size := &pty.Winsize{Cols: uint16(max(config.Cols, 1)), Rows: uint16(max(config.Rows, 1))}
	master, err := pty.StartWithSize(cmd, size)
	if err != nil {
		return backend{}, err
	}
	return backend{
		io: master,
		resize: func(cols, rows int) {
			_ = pty.Setsize(master, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
		},
		wait: func() (int, error) {
			err := cmd.Wait()
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				if code := exit.ExitCode(); code > 0 {
					return code, nil
				}
				return 0, nil // ended by our own hangup or a signal
			}
			return 0, err
		},
		hangup: func() { _ = cmd.Process.Signal(syscall.SIGHUP) },
		kill:   func() { _ = cmd.Process.Kill() },
	}, nil
}
