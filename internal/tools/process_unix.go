//go:build !windows

package tools

import (
	"os"
	"os/exec"
)

func configureProcess(c *exec.Cmd) {}
func requestStop(p *os.Process)    { _ = p.Signal(os.Interrupt) }
