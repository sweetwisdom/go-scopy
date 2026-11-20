package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/tools"
)

// Opt-in: package reads and disposable local cache files only. No installation,
// uninstallation, TCP-mode changes or user app launch occurs here.
func TestAppCacheLiveDevice(t *testing.T) {
	serial, adb := os.Getenv("GOSCOPY_APP_ICONS_LIVE_SERIAL"), os.Getenv("GOSCOPY_ADB")
	if serial == "" || adb == "" {
		t.Skip("select opt-in live ADB and serial")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{dir: t.TempDir(), ctx: ctx, cancel: cancel, tool: tools.Tools{ADB: adb, Root: root}}
	defer func() { cancel(); s.wg.Wait() }()
	s.FetchApps(serial)
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		pending := len(s.inflight) > 0
		s.mu.Unlock()
		if !pending {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	listing := s.State().Apps[serial]
	if listing.Refreshing || listing.Error != "" || len(listing.Apps) == 0 {
		t.Fatalf("device read failed: %+v", listing)
	}
	key := s.appCacheDevice(ctx, serial)
	start := time.Now()
	cached, err := s.loadAppCache(key)
	elapsed := time.Since(start)
	if err != nil || len(cached) != len(listing.Apps) {
		t.Fatal("live cache round trip failed", err)
	}
	icons := 0
	for i, a := range cached {
		if a.Package != listing.Apps[i].Package || a.VersionCode != listing.Apps[i].VersionCode {
			t.Fatal("cached metadata changed")
		}
		if len(a.IconPNG) != 0 {
			t.Fatal("device snapshot retained global icon pixels")
		}
		if meta, err := s.loadGlobalAppMeta(appPackageKey(a.Package), a.Package); err == nil && len(meta.IconPNG) > 0 {
			icons++
		}
	}
	if icons == 0 {
		t.Fatal("no real icons cached")
	}
	t.Logf("actual device: %d apps, %d cached icons, disk read %s", len(cached), icons, elapsed)
	if report := os.Getenv("GOSCOPY_APP_CACHE_REPORT"); report != "" {
		data, _ := json.MarshalIndent(map[string]any{"apps": len(cached), "icons": icons, "diskReadMs": float64(elapsed.Microseconds()) / 1000, "readOnly": true}, "", "  ")
		if err := os.MkdirAll(filepath.Dir(report), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(report, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
