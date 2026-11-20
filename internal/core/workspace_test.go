package core

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/brand"
	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/go-scopy/go-scopy/internal/domain"
)

func runWorkspaceHelper(args []string, mode string) {
	joined := strings.Join(args, " ")
	if log := os.Getenv("GOSCOPY_WORKSPACE_LOG"); log != "" {
		file, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err == nil {
			fmt.Fprintln(file, joined)
			file.Close()
		}
	}
	serial := "phone-a"
	for i, arg := range args {
		if arg == "-s" && i+1 < len(args) {
			serial = args[i+1]
		}
	}
	switch {
	case strings.Contains(joined, "pm path "):
		fmt.Printf("package:/data/app/%s/base.apk\npackage:/data/app/%s/split_config.zh.apk\n", serial, serial)
	case strings.Contains(joined, "sha256sum "):
		if mode == "slow-identity" {
			time.Sleep(400 * time.Millisecond)
		}
		split := "b"
		if mode == "different-split" && serial == "phone-b" {
			split = "c"
		}
		fmt.Printf("35\nzh-CN\n%s  /data/app/%s/base.apk\n%s  /data/app/%s/split_config.zh.apk\n", strings.Repeat("a", 64), serial, strings.Repeat(split, 64), serial)
	case strings.Contains(joined, "getprop ro.serialno"):
		fmt.Println(serial)
	case strings.Contains(joined, "pm list packages -f"):
		if mode == "old-pm" && strings.Contains(joined, "--show-versioncode") {
			fmt.Println("Error: Unknown option: --show-versioncode")
			break
		}
		fmt.Printf("package:/data/app/%s/base.apk=com.example versionCode:42\n", serial)
	case strings.Contains(joined, "pm list packages -3"):
		fmt.Println("package:com.example")
	case strings.Contains(joined, "screencap"):
		if mode == "slow-preview" {
			time.Sleep(400 * time.Millisecond)
		}
		if mode == "invalid-preview" {
			fmt.Print("not a PNG")
			break
		}
		_ = png.Encode(os.Stdout, image.NewRGBA(image.Rect(0, 0, 32, 64)))
	case strings.HasSuffix(joined, "mdns services"):
		fmt.Println("List of discovered mdns services\nadb-other _adb-tls-connect._tcp. 192.168.1.8:40000\nadb-remembered _adb-tls-connect._tcp. 192.168.1.9:40001")
	case strings.Contains(joined, " connect "):
		if mode == "slow-connect" {
			time.Sleep(500 * time.Millisecond)
		}
		fmt.Println("connected to " + args[len(args)-1])
	case strings.Contains(joined, "get-state"):
		fmt.Println("device")
	case strings.Contains(joined, " disconnect "):
		fmt.Println("disconnected")
	default:
		fmt.Fprintln(os.Stderr, "unsupported helper command")
		os.Exit(1)
	}
	os.Exit(0)
}

func workspaceService(t *testing.T, mode string) (*Service, string) {
	t.Helper()
	s := testService(t)
	t.Setenv("GOSCOPY_WORKSPACE_HELPER", mode)
	log := filepath.Join(t.TempDir(), "commands.log")
	t.Setenv("GOSCOPY_WORKSPACE_LOG", log)
	return s, log
}

func waitWorkspace(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("workspace operation timed out")
}

func TestGlobalAppMetadataSharesPackageAcrossPhonesWithoutADB(t *testing.T) {
	s, log := workspaceService(t, "normal")
	icon := appCacheTestPNG(t, color.NRGBA{R: 200, A: 255})
	meta := domain.AppMeta{Package: "com.example", Label: "Example", IconPNG: icon}
	if err := s.saveGlobalAppMeta(appPackageKey(meta.Package), meta); err != nil {
		t.Fatal(err)
	}
	for _, serial := range []string{"usb-phone", "192.168.1.8:5555", "another-phone"} {
		got := s.resolveAppMeta(s.ctx, serial, "/different/base.apk", meta.Package)
		if !got.Resolved || got.Label != meta.Label || !bytes.Equal(got.IconPNG, icon) {
			t.Fatalf("global package cache miss: %+v", got)
		}
	}
	commands, _ := os.ReadFile(log)
	if len(commands) != 0 {
		t.Fatalf("package cache hit used ADB: %s", commands)
	}
	state := s.State()
	if state.SharedAppMetas[meta.Package].Label != meta.Label {
		t.Fatal("disk hit not published globally")
	}
	delete(state.SharedAppMetas, meta.Package)
	if s.State().SharedAppMetas[meta.Package].Label == "" {
		t.Fatal("shared snapshot map aliases backend")
	}
}

func TestGlobalAppMetadataDeduplicatesAndRejectsDamagedFiles(t *testing.T) {
	s, _ := workspaceService(t, "normal")
	icon := appCacheTestPNG(t, color.NRGBA{G: 100, A: 255})
	for _, key := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		if err := s.saveGlobalAppMeta(key, domain.AppMeta{Package: "com.example", Label: "Example", IconPNG: icon}); err != nil {
			t.Fatal(err)
		}
	}
	icons, _ := filepath.Glob(filepath.Join(s.globalAppsDir(), "icons", "*.png"))
	if len(icons) != 1 {
		t.Fatal("PNG content not deduplicated", icons)
	}
	if _, err := s.loadGlobalAppMeta(strings.Repeat("a", 64), "com.other"); err == nil {
		t.Fatal("metadata accepted another package")
	}
	if err := os.WriteFile(icons[0], []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.loadGlobalAppMeta(strings.Repeat("a", 64), "com.example"); err == nil {
		t.Fatal("damaged PNG accepted")
	}
	if _, err := s.loadGlobalAppMeta("../outside", "com.example"); err == nil {
		t.Fatal("cache traversal accepted")
	}
}

func TestFastAppPackagesNeverStartAgentOrFetchIcons(t *testing.T) {
	s, log := workspaceService(t, "normal")
	apps, err := s.tool.ListAppPackages(s.ctx, "phone-a")
	if err != nil || len(apps) != 1 || apps[0].VersionCode != "42" || apps[0].System || apps[0].APKPath != "/data/app/phone-a/base.apk" || len(apps[0].IconPNG) != 0 {
		t.Fatalf("fast listing: %+v %v", apps, err)
	}
	commands, _ := os.ReadFile(log)
	for _, forbidden := range []string{"forward", "push", "unzip", "sha256sum"} {
		if strings.Contains(string(commands), forbidden) {
			t.Fatalf("fast list did metadata work: %s", commands)
		}
	}
}

func TestScreenPreviewIsMemoryOnlyAndClearFencesLateResults(t *testing.T) {
	s, _ := workspaceService(t, "normal")
	s.FetchPreview("test-serial")
	waitWorkspace(t, func() bool { preview := s.State().Preview; return !preview.Loading && len(preview.PNG) > 0 })
	if s.State().Preview.UpdatedAt.IsZero() {
		t.Fatal("missing capture time")
	}
	var files []string
	_ = filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if len(files) > 0 {
		t.Fatal("preview created disk files", files)
	}
	if _, err := os.Stat(s.state.Settings.OutputDir); !os.IsNotExist(err) {
		t.Fatal("preview touched user capture directory")
	}
	t.Setenv("GOSCOPY_WORKSPACE_HELPER", "invalid-preview")
	before := s.State().Preview
	s.FetchPreview("test-serial")
	waitWorkspace(t, func() bool { return !s.State().Preview.Loading })
	if preview := s.State().Preview; preview.Error == "" || !bytes.Equal(preview.PNG, before.PNG) || !preview.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("failed refresh lost last capture or timestamp")
	}
	t.Setenv("GOSCOPY_WORKSPACE_HELPER", "slow-preview")
	s.FetchPreview("test-serial")
	s.ClearPreview("test-serial")
	time.Sleep(500 * time.Millisecond)
	if preview := s.State().Preview; preview.Serial != "" || len(preview.PNG) != 0 || preview.Loading {
		t.Fatal("late result resurrected cleared preview")
	}
}

func TestSessionIdentityUsesBrandOrAppAndCleansOnlyOwnFiles(t *testing.T) {
	s, _ := workspaceService(t, "normal")
	first, cleanup, err := s.sessionIdentity(s.ctx, "phone-a", nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(first.IconDir, "scrcpy.png"))
	if !bytes.Equal(data, brand.PNG()) {
		t.Fatal("mirror did not use requested brand asset")
	}
	icon := appCacheTestPNG(t, color.NRGBA{B: 255, A: 255})
	s.state.AppMetas = map[string]domain.AppMeta{"phone-a\x00com.example": {Package: "com.example", Label: "应用名称", IconPNG: icon}}
	second, cleanupSecond, err := s.sessionIdentity(s.ctx, "phone-a", &domain.VirtualDisplay{StartApp: "com.example"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupSecond()
	if second.Title != "应用名称" || second.IconDir == first.IconDir {
		t.Fatal("incorrect app/window identity", second)
	}
	data, _ = os.ReadFile(filepath.Join(second.IconDir, "scrcpy.png"))
	if !bytes.Equal(data, icon) {
		t.Fatal("app window icon not selected")
	}
	cleanup()
	if _, err := os.Stat(first.IconDir); !os.IsNotExist(err) {
		t.Fatal("session assets not removed")
	}
	if _, err := os.Stat(second.IconDir); err != nil {
		t.Fatal("cleanup removed another session", err)
	}
}

func TestWirelessRecoveryPreservesObservedDevicesNotAllSavedAddresses(t *testing.T) {
	s, log := workspaceService(t, "normal")
	s.state.Settings.SavedWireless = map[string]bool{"192.168.1.20:5555": true}
	device := domain.Device{Serial: "192.168.1.9:5555", Model: "Remembered", Connection: "Wi-Fi", State: "device"}
	s.acceptDevices([]domain.Device{device})
	s.connection("监听中断", true)
	if len(s.State().Devices) != 2 {
		t.Fatal("previously active unsaved wireless device disappeared")
	}
	s.recoverWireless(s.ctx)
	if strings.Contains(s.State().Connection, "未恢复") {
		t.Fatal("wireless recovery failed")
	}
	commands, _ := os.ReadFile(log)
	if !strings.Contains(string(commands), "connect 192.168.1.9:5555") || strings.Contains(string(commands), "connect 192.168.1.20:5555") {
		t.Fatal("recovery ignored active intent or connected every saved address", string(commands))
	}
	s.setWirelessIntent(device.Serial, false)
	// A stale tracker notification must not re-enable a user disconnect.
	s.acceptDevices([]domain.Device{device})
	s.connection("监听中断", true)
	if len(s.wirelessCandidates()) != 0 {
		t.Fatal("manual disconnect was re-enabled by tracker")
	}
	before, _ := os.ReadFile(log)
	s.recoverWireless(s.ctx)
	after, _ := os.ReadFile(log)
	if !bytes.Equal(before, after) {
		t.Fatal("manual disconnect triggered recovery")
	}
}

func TestDisconnectWinsAgainstInFlightRecovery(t *testing.T) {
	s, log := workspaceService(t, "slow-connect")
	serial := "192.168.1.9:5555"
	s.acceptDevices([]domain.Device{{Serial: serial, State: "device", Connection: "Wi-Fi"}})
	s.connection("service interrupted", true)
	done := make(chan struct{})
	go func() { defer close(done); s.recoverWireless(s.ctx) }()
	waitWorkspace(t, func() bool { data, _ := os.ReadFile(log); return strings.Contains(string(data), "connect "+serial) })
	s.Disconnect(serial)
	if result := taskResult(t, s); result.Status != "done" {
		t.Fatal("disconnect failed", result)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("recovery did not stop")
	}
	data, _ := os.ReadFile(log)
	if strings.Count(string(data), "connect "+serial) != 2 || !strings.HasSuffix(strings.TrimSpace(string(data)), "disconnect "+serial) {
		t.Fatal("recovery reconnected after explicit disconnect", string(data))
	}
}

func TestWirelessMDNSRecoveryOnlyResolvesRememberedService(t *testing.T) {
	s, _ := workspaceService(t, "normal")
	address, err := s.tool.DiscoverWireless(context.Background(), "adb-remembered._adb-tls-connect._tcp")
	if err != nil || address != "192.168.1.9:40001" {
		t.Fatal(address, err)
	}
	if _, err := s.tool.DiscoverWireless(context.Background(), "unknown._adb-tls-connect._tcp"); err == nil {
		t.Fatal("resolved unrelated device")
	}
}

func TestForgetSavedOfflineRecordDoesNotDisconnect(t *testing.T) {
	s, log := workspaceService(t, "normal")
	serial := "192.168.1.9:5555"
	s.state.Settings.SavedWireless = map[string]bool{serial: true}
	s.acceptDevices([]domain.Device{{Serial: serial, State: "device", Connection: "Wi-Fi"}})
	s.connection("service interrupted", true)
	s.Save(config.ForgetDevice(s.State().Settings, serial))
	if result := taskResult(t, s); result.Status != "done" {
		t.Fatal(result)
	}
	if len(s.State().Devices) != 0 {
		t.Fatal("forgotten offline record retained")
	}
	data, _ := os.ReadFile(log)
	if strings.Contains(string(data), "disconnect") {
		t.Fatal("forget disconnected ADB")
	}
}

func TestFastAppPackagesFallbackForLegacyPM(t *testing.T) {
	s, _ := workspaceService(t, "old-pm")
	apps, err := s.tool.ListAppPackages(s.ctx, "phone-a")
	if err != nil || len(apps) != 1 || apps[0].Package != "com.example" {
		t.Fatalf("legacy package manager fallback failed: %+v %v", apps, err)
	}
}

func TestGlobalAppAgentPublishesAllIconsAndClearFencesOldWork(t *testing.T) {
	s, _ := workspaceService(t, "normal")
	icon := appCacheTestPNG(t, color.NRGBA{B: 210, A: 255})
	const serial = "usb-phone"
	s.mu.Lock()
	s.setAppListing(serial, domain.AppListing{Apps: []domain.AppInfo{{Package: "com.example", APKPath: "/usb/base.apk", VersionCode: "42"}}})
	generation := s.appListingGeneration[serial]
	s.setAppListing("wifi-phone", domain.AppListing{Apps: []domain.AppInfo{{Package: "com.example", APKPath: "/wifi/base.apk", VersionCode: "99"}}})
	s.mu.Unlock()
	epoch := s.appCacheEpoch.Load()
	s.publishAgentApps(serial, generation, epoch, map[string]domain.AppInfo{"com.example": {Package: "com.example", Label: "Example", IconPNG: icon, VersionCode: "42"}})
	state := s.State()
	if !bytes.Equal(state.SharedAppMetas["com.example"].IconPNG, icon) {
		t.Fatal("Agent icons not globally published")
	}
	if state.Apps["wifi-phone"].Apps[0].VersionCode != "99" || state.Apps["wifi-phone"].Apps[0].APKPath != "/wifi/base.apk" {
		t.Fatal("global icon changed another installation")
	}
	legacyKey := strings.Repeat("a", 64)
	if err := s.saveAppCache(legacyKey, []domain.AppInfo{{Package: "com.example", Label: "Old", IconPNG: icon}}); err != nil {
		t.Fatal(err)
	}
	protected := filepath.Join(s.dir, "settings.json")
	if err := os.WriteFile(protected, []byte("user settings"), 0600); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(s.globalAppsDir(), "icons", "user-not-cache.png")
	if err := os.WriteFile(userFile, icon, 0600); err != nil {
		t.Fatal(err)
	}
	count, err := s.clearAppCache()
	if err != nil || count < 4 {
		t.Fatalf("cache clear failed: count=%d err=%v", count, err)
	}
	state = s.State()
	if len(state.SharedAppMetas) != 0 || len(state.AppMetas) != 0 || state.AppCacheGeneration != epoch+1 {
		t.Fatal("clear retained global memory")
	}
	if len(state.Apps[serial].Apps) != 1 || state.Apps[serial].Apps[0].APKPath != "/usb/base.apk" {
		t.Fatal("clear removed installed list")
	}
	for _, path := range []string{protected, userFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("clear removed unrelated file", path, err)
		}
	}
	late := domain.AppMeta{Package: "com.example", Label: "Stale", IconPNG: icon, Resolved: true}
	s.publishSharedAppMetas(epoch, []domain.AppMeta{late})
	if err := s.saveAppSnapshot(epoch, legacyKey, []domain.AppInfo{{Package: "com.example", Label: "Stale"}}); err != nil {
		t.Fatal(err)
	}
	if len(s.State().SharedAppMetas) != 0 {
		t.Fatal("pre-clear read resurrected global memory")
	}
	if _, err := s.loadGlobalAppMeta(appPackageKey(late.Package), late.Package); err == nil {
		t.Fatal("pre-clear read rewrote disk cache")
	}
	if _, err := s.loadAppCache(legacyKey); err == nil {
		t.Fatal("pre-clear listing rewrote disk snapshot")
	}
}
