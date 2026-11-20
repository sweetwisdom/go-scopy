package core

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools"
)

// Opt-in, read-only: no file creation, transfer, deletion or device settings.
func TestLiveDeviceFileReads(t *testing.T) {
	adb, serial := os.Getenv("GOSCOPY_LIVE_ADB"), os.Getenv("GOSCOPY_LIVE_SERIAL")
	if adb == "" || serial == "" {
		t.Skip("set GOSCOPY_LIVE_ADB and GOSCOPY_LIVE_SERIAL to enable read-only device verification")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{ctx: ctx, cancel: cancel, tool: tools.Tools{ADB: adb}, state: domain.State{Devices: []domain.Device{{Serial: serial, State: "device"}}}}
	t.Cleanup(cancel)
	entries, err := s.BrowseFiles(ctx, serial, "/sdcard")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("read-only device directory: %d entries (names not logged)", len(entries))
	data, err := s.ReadFilePreview(ctx, serial, "/proc/version", 4096)
	if err != nil || !strings.Contains(string(data), "Linux version") {
		t.Fatalf("bounded raw device preview failed: %v", err)
	}
}
