package core

import (
	"bytes"
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
)

func waitAppReads(t *testing.T, s *Service) {
	t.Helper()
	waitWorkspace(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.inflight) == 0
	})
}

func TestWarmAppListNeverStartsIconAcquisitionAcrossRestartAndTransports(t *testing.T) {
	s, commandLog := workspaceService(t, "normal")
	icon := appCacheTestPNG(t, color.NRGBA{R: 90, G: 120, A: 255})
	// A disk-only entry models reopening the app; the second read models another
	// transport with a different APK installation path and no device snapshot.
	meta := domain.AppMeta{Package: "com.example", Label: "Cached Example", IconPNG: icon}
	if err := s.saveGlobalAppMeta(appPackageKey(meta.Package), meta); err != nil {
		t.Fatal(err)
	}
	for _, serial := range []string{"phone-a", "phone-b", "phone-a"} {
		s.FetchApps(serial)
		waitAppReads(t, s)
		state := s.State()
		if listing := state.Apps[serial]; listing.Refreshing || len(listing.Apps) != 1 || listing.Apps[0].APKPath != "/data/app/"+serial+"/base.apk" {
			t.Fatalf("installation list not refreshed for %s: %+v", serial, listing)
		}
		if got := state.SharedAppMetas[meta.Package]; !got.Resolved || got.Label != meta.Label || !bytes.Equal(got.IconPNG, icon) {
			t.Fatal("cached icon not restored with list")
		}
		s.FetchAppMeta(serial, "/data/app/"+serial+"/base.apk", meta.Package)
		waitAppReads(t, s)
	}
	commands, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"forward", "push", "unzip", "pm path", "pull", "app_process"} {
		if strings.Contains(string(commands), forbidden) {
			t.Fatalf("warm cache caused device icon read %q: %s", forbidden, commands)
		}
	}
}

func TestMissingAppIconDoesNotInvokeLegacyAPKParserOrPoisonGlobalCache(t *testing.T) {
	s, commandLog := workspaceService(t, "normal")
	pkg := "com.example"
	// Legacy label-only entries must not count as successfully loaded icons.
	dir := filepath.Join(s.globalAppsDir(), "metadata")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(globalAppMeta{Version: 2, Package: pkg, Label: "Partial"})
	if err := os.WriteFile(filepath.Join(dir, appPackageKey(pkg)+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.loadGlobalAppMeta(appPackageKey(pkg), pkg); err == nil {
		t.Fatal("label-only cache marked complete")
	}
	meta := s.resolveAppMeta(s.ctx, "phone-a", "/data/app/base.apk", pkg)
	if meta.Error == "" || len(meta.IconPNG) != 0 {
		t.Fatalf("Agent failure hidden: %+v", meta)
	}
	if _, ok := s.State().SharedAppMetas[pkg]; ok {
		t.Fatal("failed icon poisoned shared cache")
	}
	commands, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"unzip", "pull", "pm path"} {
		if strings.Contains(string(commands), forbidden) {
			t.Fatal("unsafe APK extraction path invoked", string(commands))
		}
	}
}

func TestAgentRefreshKeepsExistingGlobalIconAndPublishesWithoutOpenList(t *testing.T) {
	s, _ := workspaceService(t, "normal")
	old := appCacheTestPNG(t, color.NRGBA{R: 200, A: 255})
	updated := appCacheTestPNG(t, color.NRGBA{B: 200, A: 255})
	epoch := s.appCacheEpoch.Load()
	s.publishSharedAppMetas(epoch, []domain.AppMeta{{Package: "com.cached", Label: "Cached", IconPNG: old}})
	s.publishAgentApps("phone-b", 1, epoch, map[string]domain.AppInfo{
		"com.cached":  {Package: "com.cached", Label: "Different phone", IconPNG: updated},
		"com.new":     {Package: "com.new", Label: "New", IconPNG: updated},
		"com.partial": {Package: "com.partial", Label: "No icon"},
	})
	state := s.State()
	if !bytes.Equal(state.SharedAppMetas["com.cached"].IconPNG, old) || state.SharedAppMetas["com.cached"].Label != "Cached" {
		t.Fatal("package-only cache unexpectedly invalidated")
	}
	if !bytes.Equal(state.SharedAppMetas["com.new"].IconPNG, updated) {
		t.Fatal("closed listing prevented global publication")
	}
	if _, ok := state.SharedAppMetas["com.partial"]; ok {
		t.Fatal("partial app marked globally complete")
	}
	if disk, err := s.loadGlobalAppMeta(appPackageKey("com.new"), "com.new"); err != nil || !bytes.Equal(disk.IconPNG, updated) {
		t.Fatal("Agent icon not persisted", err)
	}
}
