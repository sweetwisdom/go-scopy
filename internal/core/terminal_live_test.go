package core

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools"
)

// Opt-in: open a real device shell and print only our marker; no files, apps or
// settings are changed, and no device output is logged.
func TestLiveDeviceTerminalRead(t *testing.T) {
	adb, serial := os.Getenv("GOSCOPY_LIVE_ADB"), os.Getenv("GOSCOPY_LIVE_SERIAL")
	if adb == "" || serial == "" {
		t.Skip("set GOSCOPY_LIVE_ADB and GOSCOPY_LIVE_SERIAL for read-only terminal verification")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &Service{ctx: ctx, cancel: cancel, tool: tools.Tools{ADB: adb}, state: domain.State{Devices: []domain.Device{{Serial: serial, State: "device"}}}}
	stream, err := s.OpenShell(serial, 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stream.Close)
	stream.Resize(100, 30)
	// The pipe-based provider needs a line feed (not a local ConPTY CR).
	if !stream.Write([]byte("printf 'GOSCOPY_%s\\n' SHELL_OK; exit\r\n")) {
		t.Fatal("shell refused input")
	}
	deadline := time.Now().Add(12 * time.Second)
	var output []byte
	for time.Now().Before(deadline) {
		output = stream.Drain(output)
		if strings.Contains(string(output), "GOSCOPY_SHELL_OK") {
			return
		}
		select {
		case <-stream.Done():
			output = stream.Drain(output)
			if strings.Contains(string(output), "GOSCOPY_SHELL_OK") {
				return
			}
			t.Fatalf("device shell exited before marker: %v", stream.Err())
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("read-only device shell did not return its marker")
}
