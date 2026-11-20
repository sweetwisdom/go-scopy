package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

func appCacheTestPNG(t *testing.T, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.SetNRGBA(0, 0, c)
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestAppCacheRoundTripIsolationAndRefresh(t *testing.T) {
	s := &Service{dir: t.TempDir()}
	icon := appCacheTestPNG(t, color.NRGBA{R: 255, A: 255})
	apps := []domain.AppInfo{{Package: "com.example.app", Label: "应用", VersionCode: "1", CanLaunch: true, IconPNG: icon},
		{Package: "com.example.other", IconPNG: icon, System: true}}
	key := appCacheHash([]byte("phone-one"))
	if err := s.saveAppCache(key, apps); err != nil {
		t.Fatal(err)
	}
	dir := s.appCacheDir(key)
	files, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	if len(files) != 1 {
		t.Fatalf("identical icons not deduplicated: %v", files)
	}
	before, _ := os.Stat(files[0])
	got, err := s.loadAppCache(key)
	if err != nil || len(got) != 2 || got[0].Label != "应用" || got[0].VersionCode != "1" || !got[0].CanLaunch || !got[1].System || !bytes.Equal(got[0].IconPNG, icon) {
		t.Fatalf("cache round trip failed: %+v %v", got, err)
	}
	if err := s.saveAppCache(key, apps); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(files[0])
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("unchanged icon was rewritten")
	}
	if _, err := s.loadAppCache(appCacheHash([]byte("phone-two"))); err == nil {
		t.Fatal("different phones shared cache")
	}
	apps[0].VersionCode = "2"
	apps[0].IconPNG = appCacheTestPNG(t, color.NRGBA{B: 255, A: 255})
	if err := s.saveAppCache(key, apps[:1]); err != nil {
		t.Fatal(err)
	}
	got, err = s.loadAppCache(key)
	if err != nil || len(got) != 1 || got[0].VersionCode != "2" || !bytes.Equal(got[0].IconPNG, apps[0].IconPNG) {
		t.Fatalf("updated/removed app not reflected: %+v %v", got, err)
	}
}

func TestAppCacheDamagedIconsAndIndexAreDisposable(t *testing.T) {
	s := &Service{dir: t.TempDir()}
	key := appCacheHash([]byte("phone"))
	icon := appCacheTestPNG(t, color.NRGBA{G: 255, A: 255})
	apps := []domain.AppInfo{{Package: "com.example.app", Label: "Example", IconPNG: icon}}
	if err := s.saveAppCache(key, apps); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.appCacheDir(key), appCacheHash(icon)+".png")
	if err := os.WriteFile(path, []byte("bad PNG"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := s.loadAppCache(key)
	if err != nil || len(got) != 1 || len(got[0].IconPNG) != 0 {
		t.Fatal("bad icon should leave a usable label/list", err)
	}
	if err := s.saveAppCache(key, apps); err != nil {
		t.Fatal(err)
	}
	got, err = s.loadAppCache(key)
	if err != nil || !bytes.Equal(got[0].IconPNG, icon) {
		t.Fatal("refresh failed to repair damaged file", err)
	}
	indexPath := filepath.Join(s.appCacheDir(key), "apps.json")
	malicious := appCacheIndex{Version: 1, Apps: []cachedApp{{AppInfo: domain.AppInfo{Package: "com.test", IconPNG: icon}, Icon: "../outside"}}}
	data, _ := json.Marshal(malicious)
	if err := os.WriteFile(indexPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err = s.loadAppCache(key)
	if err != nil || len(got[0].IconPNG) != 0 {
		t.Fatal("inline/traversal icon accepted", err)
	}
	for _, data := range [][]byte{[]byte("{broken"), []byte(`{"Version":2}`), []byte(`{"Version":1,"Apps":[{"Package":""}]}`)} {
		if err := os.WriteFile(indexPath, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.loadAppCache(key); err == nil {
			t.Fatal("invalid index accepted")
		}
	}
}

func runAppsHelper(args []string, mode string) {
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "getprop ro.serialno"):
		fmt.Println("cache-phone")
	case strings.Contains(joined, "pm list packages -f"):
		time.Sleep(300 * time.Millisecond)
		if mode == "fail" {
			fmt.Fprintln(os.Stderr, "device offline")
			os.Exit(1)
		}
		fmt.Println("package:/data/app/new/base.apk=com.example.new")
	case strings.Contains(joined, "pm list packages -3"):
		fmt.Println("package:com.example.new")
	default:
		fmt.Fprintln(os.Stderr, "Agent unavailable in helper")
		os.Exit(1)
	}
	os.Exit(0)
}

func TestFetchAppsPublishesDiskCacheBeforeRefreshAndKeepsOnFailure(t *testing.T) {
	for _, mode := range []string{"success", "fail"} {
		t.Run(mode, func(t *testing.T) {
			s := testService(t)
			t.Setenv("GOSCOPY_APPS_HELPER", mode)
			key := appCacheHash([]byte("hardware:cache-phone"))
			icon := appCacheTestPNG(t, color.NRGBA{R: 255, A: 255})
			if err := s.saveAppCache(key, []domain.AppInfo{{Package: "com.cached", Label: "Cached", IconPNG: icon}}); err != nil {
				t.Fatal(err)
			}
			s.FetchApps("test-serial")
			deadline := time.Now().Add(5 * time.Second)
			sawCache := false
			var listing domain.AppListing
			for time.Now().Before(deadline) {
				listing = s.State().Apps["test-serial"]
				if listing.Refreshing && listing.Cached && len(listing.Apps) == 1 && listing.Apps[0].Package == "com.cached" {
					sawCache = true
				}
				s.mu.Lock()
				pending := s.inflight["apps:test-serial"]
				s.mu.Unlock()
				if !pending {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !sawCache {
				t.Fatal("cache not visible before slow device refresh")
			}
			if listing.Refreshing {
				t.Fatal("refresh never completed")
			}
			if mode == "fail" {
				if listing.Error == "" || !listing.Cached || listing.Apps[0].Package != "com.cached" {
					t.Fatalf("lost cache after failure: %+v", listing)
				}
			} else {
				if listing.Error != "" || listing.Cached || len(listing.Apps) != 1 || listing.Apps[0].Package != "com.example.new" {
					t.Fatalf("live refresh not authoritative: %+v", listing)
				}
				if _, ok := s.State().AppMetas["test-serial\x00com.cached"]; ok {
					t.Fatal("stale metadata survived refresh")
				}
			}
		})
	}
}

func TestAppCacheIdentitySharesUSBAndWireless(t *testing.T) {
	s := testService(t)
	t.Setenv("GOSCOPY_APPS_HELPER", "success")
	first := s.appCacheDevice(context.Background(), "usb-serial")
	second := s.appCacheDevice(context.Background(), "192.168.1.2:5555")
	if first != second || first != appCacheHash([]byte("hardware:cache-phone")) {
		t.Fatal("USB/wireless do not share physical-device cache")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s.appCacheDevice(ctx, "phone-one") == s.appCacheDevice(ctx, "phone-two") {
		t.Fatal("unknown hardware identity merged phones")
	}
}
