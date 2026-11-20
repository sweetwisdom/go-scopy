package pty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
)

// StartCommand shares the existing bounded stream with a process on ordinary
// pipes. Device terminals follow escrcpy's device provider: adb shell -tt owns
// the remote PTY, so no local ConPTY (or visible console window) is needed.
// The caller configures the command's environment and platform launch flags.
func StartCommand(cmd *exec.Cmd, notify func()) (*Shell, error) {
	if cmd == nil || cmd.Path == "" {
		return nil, errors.New("terminal: missing program")
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, childOutput, err := os.Pipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = childOutput, childOutput
	if err := cmd.Start(); err != nil {
		input.Close()
		output.Close()
		childOutput.Close()
		return nil, fmt.Errorf("terminal: start: %w", err)
	}
	// The child has its own output handle. Keeping our copy open prevents EOF.
	childOutput.Close()
	var stopped atomic.Bool
	stop := func() { stopped.Store(true); _ = cmd.Process.Kill() }
	b := backend{
		io:     &pipeIO{input: input, output: output},
		resize: func(int, int) {}, // Like escrcpy's device provider, no local PTY.
		hangup: stop, kill: stop,
		wait: func() (int, error) {
			err := cmd.Wait()
			if stopped.Load() {
				return 0, nil
			}
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				return exit.ExitCode(), nil
			}
			return 0, err
		},
	}
	return startStream(b, notify), nil
}

type pipeIO struct {
	input  io.WriteCloser
	output *os.File
}

func (p *pipeIO) Read(b []byte) (int, error)  { return p.output.Read(b) }
func (p *pipeIO) Write(b []byte) (int, error) { return p.input.Write(b) }
func (p *pipeIO) Close() error                { _ = p.input.Close(); return p.output.Close() }
