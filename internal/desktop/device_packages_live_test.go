package desktop

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools"
	"github.com/vivaui/viva/headless"
)

// Opt-in integration check against a running Agent; does not launch or modify apps.
func TestPackageIconsLive(t *testing.T) {
	serial := os.Getenv("GOSCOPY_APP_ICONS_LIVE_SERIAL")
	if serial == "" {
		t.Skip("set GOSCOPY_APP_ICONS_LIVE_SERIAL to validate device icons")
	}
	adb := os.Getenv("GOSCOPY_ADB")
	if adb == "" {
		t.Fatal("GOSCOPY_ADB must select the test ADB binary")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	apps, err := (tools.Tools{ADB: adb}).ListApps(ctx, serial)
	if err != nil {
		t.Fatal(err)
	}
	icons := 0
	for _, a := range apps {
		if len(a.IconPNG) == 0 {
			continue
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(a.IconPNG))
		if err != nil {
			t.Fatalf("%s: %v", a.Package, err)
		}
		if cfg.Width <= 1 || cfg.Height <= 1 {
			t.Fatalf("collapsed icon %s: %dx%d", a.Package, cfg.Width, cfg.Height)
		}
		t.Logf("%s: %dx%d, %d PNG bytes", a.Package, cfg.Width, cfg.Height, len(a.IconPNG))
		icons++
	}
	if icons == 0 {
		t.Fatal("no Agent icons received; fallback package enumeration does not verify icons")
	}
	t.Logf("%d apps, %d valid icons", len(apps), icons)
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			f := sample()
			f.state.Settings.Theme = theme
			f.state.Devices = []domain.Device{{Serial: serial, Model: "Icon validation", State: "device", Connection: "Wi-Fi"}}
			f.state.Apps = map[string]domain.AppListing{serial: {Apps: apps}}
			u := New(f)
			u.deviceTab = "packages"
			cfg := u.Config()
			cfg.Width, cfg.Height = 1240, 1000
			s, err := headless.Open(cfg, nil, headless.Options{Scale: 1.5})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			clean(t, s)
			if dir := os.Getenv("GOSCOPY_ARTIFACT_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := s.Screenshot(filepath.Join(dir, "packages-icons-"+theme+".png")); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
