// Package pty runs a program on a pseudo-terminal: a ConPTY on Windows, a pty
// elsewhere. Go-Scopy uses it for interactive adb sessions, so keyboard input
// and terminal size behave exactly as they do in a system terminal. Adapted
// from motty's localshell package (Apache-2.0).
package pty

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	// maxPendingOutput is how much output may wait for the reader before the
	// program is slowed through the pty.
	maxPendingOutput = 1 << 20
	// maxPendingInput is how much typed or pasted input may wait to be written.
	maxPendingInput = 4 << 20
	// readDrainGrace gives the reader time to collect the last output after the
	// program ends.
	readDrainGrace = 500 * time.Millisecond
)

// Config describes the program to run on the terminal.
type Config struct {
	// Cols and Rows are the terminal size in cells.
	Cols, Rows int
	// Path is the executable; Args are its arguments.
	Path string
	Args []string
	// Dir is the working directory. Empty keeps the caller's.
	Dir string
	// Env replaces the child's environment entirely. Nil inherits ours.
	Env []string
	// Notify is called after output arrives and when the program ends, from any
	// goroutine; it must not block.
	Notify func()
}

// backend is one platform's pseudo-terminal and the process on it.
type backend struct {
	// io is the terminal's master side: read what the program prints, write
	// what the user types.
	io io.ReadWriteCloser
	// resize tells the program the terminal size in cells.
	resize func(cols, rows int)
	// wait blocks until the program has ended and returns its exit status.
	wait func() (status int, err error)
	// hangup asks the program to end; kill makes sure it does.
	hangup, kill func()
}

// ExitError is why a program ended with a nonzero status.
type ExitError struct{ Code int }

func (err *ExitError) Error() string   { return fmt.Sprintf("process exited with status %d", err.Code) }
func (err *ExitError) ExitStatus() int { return err.Code }

// Shell is a program running on a pseudo-terminal.
type Shell struct {
	b      backend
	notify func()
	done   chan struct{}
	// readDone is closed when the reader has seen the end of the terminal.
	readDone chan struct{}

	mu       sync.Mutex
	space    *sync.Cond
	data     []byte
	finished bool
	exitErr  error

	input   chan []byte
	pending int
	closed  bool
}

// Start launches the program. It returns once the process is running.
func Start(config Config) (*Shell, error) {
	if config.Path == "" {
		return nil, errors.New("pty: missing program")
	}
	b, err := startBackend(config)
	if err != nil {
		return nil, err
	}
	return startStream(b, config.Notify), nil
}

// startStream shares bounded I/O and lifecycle handling between PTYs and pipes.
func startStream(b backend, notify func()) *Shell {
	shell := &Shell{b: b, notify: notify, done: make(chan struct{}), readDone: make(chan struct{}), input: make(chan []byte, 256)}
	shell.space = sync.NewCond(&shell.mu)
	go shell.readLoop()
	go shell.writeLoop()
	go shell.wait()
	return shell
}

func (shell *Shell) wake() {
	if shell.notify != nil {
		shell.notify()
	}
}

// readLoop moves the program's output into the buffer. It blocks while the
// buffer is full, which stops reading the pty and so slows the program.
func (shell *Shell) readLoop() {
	defer close(shell.readDone)
	buffer := make([]byte, 32<<10)
	for {
		n, err := shell.b.io.Read(buffer)
		if n > 0 {
			shell.mu.Lock()
			for len(shell.data) >= maxPendingOutput && !shell.finished {
				shell.space.Wait()
			}
			first := len(shell.data) == 0
			shell.data = append(shell.data, buffer[:n]...)
			shell.mu.Unlock()
			if first {
				shell.wake()
			}
		}
		if err != nil {
			return
		}
	}
}

func (shell *Shell) writeLoop() {
	for data := range shell.input {
		shell.mu.Lock()
		shell.pending -= len(data)
		shell.mu.Unlock()
		if _, err := shell.b.io.Write(data); err != nil {
			return
		}
	}
}

func (shell *Shell) wait() {
	status, err := shell.b.wait()
	if err == nil && status != 0 {
		err = &ExitError{Code: status}
	}
	select {
	case <-shell.readDone:
	case <-time.After(readDrainGrace):
	}
	shell.mu.Lock()
	shell.exitErr = err
	shell.finished = true
	shell.mu.Unlock()
	shell.space.Broadcast()
	close(shell.done)
	shell.wake()
}

// Drain appends the output received since the last call to dst. It never blocks.
func (shell *Shell) Drain(dst []byte) []byte {
	shell.mu.Lock()
	dst = append(dst, shell.data...)
	if cap(shell.data) > 4*maxPendingOutput {
		shell.data = nil
	} else {
		shell.data = shell.data[:0]
	}
	shell.mu.Unlock()
	shell.space.Broadcast()
	return dst
}

// Write queues input for the program. It reports false when the program has
// ended or the backlog is full; it never blocks the caller.
func (shell *Shell) Write(p []byte) bool {
	if len(p) == 0 {
		return true
	}
	shell.mu.Lock()
	defer shell.mu.Unlock()
	if shell.closed || shell.finished || shell.pending+len(p) > maxPendingInput {
		return false
	}
	select {
	case shell.input <- append([]byte(nil), p...):
		shell.pending += len(p)
		return true
	default:
		return false
	}
}

// Resize tells the program the terminal size in cells.
func (shell *Shell) Resize(cols, rows int) {
	if cols < 1 || rows < 1 {
		return
	}
	shell.b.resize(cols, rows)
}

// Done is closed once the program has ended and its output is complete.
func (shell *Shell) Done() <-chan struct{} { return shell.done }

// Err is why the program ended: nil for a clean exit or a hangup, an *ExitError
// for a nonzero status. Only meaningful after Done is closed.
func (shell *Shell) Err() error {
	shell.mu.Lock()
	defer shell.mu.Unlock()
	return shell.exitErr
}

// Close asks the program to end and releases the terminal once it has.
func (shell *Shell) Close() {
	shell.mu.Lock()
	if shell.closed {
		shell.mu.Unlock()
		return
	}
	shell.closed = true
	shell.mu.Unlock()
	shell.b.hangup()
	select {
	case <-shell.done:
	case <-time.After(2 * time.Second):
		shell.b.kill()
		<-shell.done
	}
	_ = shell.b.io.Close()
	close(shell.input)
}
