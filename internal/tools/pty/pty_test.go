package pty

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("GOSCOPY_PTY_HELPER") == "1" {
		fmt.Println("PTY_READY")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "exit" {
				os.Exit(0)
			}
			fmt.Println("PTY_ECHO:" + line)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestStartupFailure(t *testing.T) {
	for _, path := range []string{"", filepath.Join(t.TempDir(), "missing-program.exe")} {
		s, err := Start(Config{Path: path, Cols: 80, Rows: 24})
		if err == nil || s != nil {
			t.Fatalf("missing executable accepted: %q %v", path, err)
		}
	}
}

func TestInteractiveProcessResizeAndClose(t *testing.T) {
	t.Setenv("GOSCOPY_PTY_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Start(Config{Path: exe, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	var output []byte
	waitOutput := func(marker string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			output = s.Drain(output)
			if strings.Contains(string(output), marker) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("did not receive %q; output %q", marker, string(output))
	}
	waitOutput("PTY_READY")
	s.Resize(100, 30)
	if !s.Write([]byte("hello-pty\r")) {
		t.Fatal("input refused")
	}
	waitOutput("PTY_ECHO:hello-pty")
	if !s.Write([]byte("exit\r")) {
		t.Fatal("exit refused")
	}
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("terminal did not exit")
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s.Close()
	if s.Write([]byte("after-close")) {
		t.Fatal("closed terminal accepted input")
	}
}
