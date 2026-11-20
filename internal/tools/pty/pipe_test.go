package pty

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPipeStreamStartupFailure(t *testing.T) {
	for _, cmd := range []*exec.Cmd{nil, exec.Command(filepath.Join(t.TempDir(), "missing-program.exe"))} {
		stream, err := StartCommand(cmd, nil)
		if stream != nil || err == nil {
			t.Fatalf("invalid command accepted: %v %v", stream, err)
		}
	}
}
func TestPipeStreamUsesExistingBoundedLifecycle(t *testing.T) {
	t.Setenv("GOSCOPY_PTY_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := StartCommand(exec.Command(exe), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stream.Close)
	if !stream.Write([]byte("pipe-echo\nexit\n")) {
		t.Fatal("input refused")
	}
	select {
	case <-stream.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("pipe stream did not exit")
	}
	output := string(stream.Drain(nil))
	if !strings.Contains(output, "PTY_READY") || !strings.Contains(output, "PTY_ECHO:pipe-echo") {
		t.Fatalf("lost pipe output: %q", output)
	}
	if stream.Err() != nil {
		t.Fatal(stream.Err())
	}
	stream.Close()
	stream.Close()
	if stream.Write([]byte("closed")) {
		t.Fatal("closed pipe stream accepted input")
	}
}
