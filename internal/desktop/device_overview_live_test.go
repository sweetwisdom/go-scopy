package desktop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools"
	"github.com/vivaui/viva/headless"
)

// Opt-in, read-only validation; never installs, launches apps or modifies a device.
func TestOverviewLiveReadOnly(t *testing.T) {
	serial := os.Getenv("GOSCOPY_PHONEINFO_LIVE_SERIAL")
	if serial == "" {
		t.Skip("set GOSCOPY_PHONEINFO_LIVE_SERIAL for read-only device validation")
	}
	adb := os.Getenv("GOSCOPY_ADB")
	if adb == "" {
		t.Fatal("GOSCOPY_ADB must explicitly select the test ADB binary")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	info, err := (tools.Tools{ADB: adb}).PhoneInfo(ctx, serial)
	if err != nil {
		t.Fatal(err)
	}
	info.UpdatedAt = time.Now()
	if info.Model == "" || info.AndroidVersion == "" || info.StorageTotalBytes <= 0 {
		t.Fatalf("core fields not resolved: %+v", info)
	}
	t.Logf("read-only collection %s: model=%s Android=%s API=%s storage=%d used=%d warnings=%d", time.Since(start), info.Model, info.AndroidVersion, info.APILevel, info.StorageTotalBytes, info.StorageUsedBytes, len(info.Warnings))
	f := sample()
	f.state.Devices = []domain.Device{{Serial: serial, Model: info.Model, State: "device", Connection: "Wi-Fi"}}
	f.state.PhoneInfos = map[string]domain.PhoneInfo{serial: info}
	u := New(f)
	cfg := u.Config()
	cfg.Width, cfg.Height = 1240, 1000
	s, err := headless.Open(cfg, nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	clean(t, s)
	if dir := os.Getenv("GOSCOPY_ARTIFACT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "device-info.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := s.Screenshot(filepath.Join(dir, "overview-live.png")); err != nil {
			t.Fatal(err)
		}
	}
}
