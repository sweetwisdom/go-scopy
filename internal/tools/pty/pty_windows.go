//go:build windows

package pty

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// startBackend runs the program on a ConPTY: Windows' pseudo-console, which
// turns a console program's screen into the same escape-sequence stream a Unix
// pty carries, so the terminal view needs no Windows special case.
func startBackend(config Config) (backend, error) {
	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return backend{}, fmt.Errorf("pty: input pipe: %w", err)
	}
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		windows.CloseHandle(inRead)
		windows.CloseHandle(inWrite)
		return backend{}, fmt.Errorf("pty: output pipe: %w", err)
	}
	size := windows.Coord{X: int16(max(config.Cols, 1)), Y: int16(max(config.Rows, 1))}
	var console windows.Handle
	if err := windows.CreatePseudoConsole(size, inRead, outWrite, 0, &console); err != nil {
		for _, handle := range []windows.Handle{inRead, inWrite, outRead, outWrite} {
			windows.CloseHandle(handle)
		}
		return backend{}, fmt.Errorf("pty: pseudo console (needs Windows 10 1809 or newer): %w", err)
	}
	// The console holds its own references to its ends of the pipes.
	windows.CloseHandle(inRead)
	windows.CloseHandle(outWrite)

	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.ClosePseudoConsole(console)
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		return backend{}, err
	}
	fail := func(err error) (backend, error) {
		attributes.Delete()
		windows.ClosePseudoConsole(console)
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		return backend{}, err
	}
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&console)), unsafe.Sizeof(console)); err != nil {
		return fail(err)
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.StartupInfo.Cb = uint32(unsafe.Sizeof(startup))
	// A process started from a pipe inherits our redirected handles and talks
	// to those instead of the pseudo console, which then shows nothing.
	startup.StartupInfo.Flags |= windows.STARTF_USESTDHANDLES

	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{config.Path}, config.Args...)))
	if err != nil {
		return fail(err)
	}
	var directory *uint16
	if config.Dir != "" {
		if directory, err = windows.UTF16PtrFromString(config.Dir); err != nil {
			return fail(err)
		}
	}
	environment := os.Environ()
	if config.Env != nil {
		environment = config.Env
	}
	block, err := environmentBlock(environment)
	if err != nil {
		return fail(err)
	}
	var process windows.ProcessInformation
	if err := windows.CreateProcess(nil, commandLine, nil, nil, false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT,
		&block[0], directory, &startup.StartupInfo, &process); err != nil {
		return fail(fmt.Errorf("pty: start %s: %w", config.Path, err))
	}
	windows.CloseHandle(process.Thread)
	attributes.Delete()

	output := os.NewFile(uintptr(outRead), "conpty-out")
	input := os.NewFile(uintptr(inWrite), "conpty-in")
	var closeOnce sync.Once
	closeConsole := func() { closeOnce.Do(func() { windows.ClosePseudoConsole(console) }) }
	return backend{
		io: &consoleIO{output: output, input: input, closeConsole: closeConsole},
		resize: func(cols, rows int) {
			_ = windows.ResizePseudoConsole(console, windows.Coord{X: int16(cols), Y: int16(rows)})
		},
		wait: func() (int, error) {
			defer windows.CloseHandle(process.Process)
			if event, err := windows.WaitForSingleObject(process.Process, windows.INFINITE); err != nil || event != windows.WAIT_OBJECT_0 {
				return 0, errors.New("pty: waiting for the program failed")
			}
			// The console keeps its output pipe open after the program is gone;
			// closing it is what gives the reader its end of file. It can block
			// until the output is drained, so it runs beside the reader.
			go closeConsole()
			var code uint32
			if err := windows.GetExitCodeProcess(process.Process, &code); err != nil {
				return 0, err
			}
			if code == 1 || code == 0xC000013A { // terminated by Close, or by Ctrl+C
				return 0, nil
			}
			return int(code), nil
		},
		hangup: func() { _ = windows.TerminateProcess(process.Process, 1) },
		kill:   func() { _ = windows.TerminateProcess(process.Process, 1) },
	}, nil
}

// consoleIO is the two pipe ends of a pseudo console as one stream.
type consoleIO struct {
	output, input *os.File
	closeConsole  func()
}

func (stream *consoleIO) Read(p []byte) (int, error)  { return stream.output.Read(p) }
func (stream *consoleIO) Write(p []byte) (int, error) { return stream.input.Write(p) }

// Close ends the console first: it can wait for the output pipe to be drained,
// so it runs beside the closing of the pipes rather than before.
func (stream *consoleIO) Close() error {
	go stream.closeConsole()
	_ = stream.input.Close()
	return stream.output.Close()
}

// environmentBlock is the UTF-16, double-NUL-terminated block CreateProcess takes.
func environmentBlock(environment []string) ([]uint16, error) {
	var block []uint16
	for _, entry := range environment {
		if strings.IndexByte(entry, 0) >= 0 {
			return nil, errors.New("pty: environment entry contains NUL")
		}
		encoded, err := windows.UTF16FromString(entry)
		if err != nil {
			return nil, err
		}
		block = append(block, encoded...) // each entry already ends with its NUL
	}
	return append(block, 0), nil
}
