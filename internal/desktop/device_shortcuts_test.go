package desktop

import (
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"
)

func TestWirelessShortcutFollowsVirtualDisplayAndTargetsDevice(t *testing.T) {
	for _, width := range []int{1000, 1240, 1400} {
		f := sample()
		u := New(f)
		cfg := u.Config()
		cfg.Width = width
		s, err := headless.Open(cfg, nil, headless.Options{Scale: 1.5})
		if err != nil {
			t.Fatal(err)
		}
		virtual, ok := s.Hit("虚拟屏幕")
		if !ok {
			t.Fatal("virtual shortcut missing")
		}
		wireless, ok := s.Hit("切换无线")
		if !ok {
			t.Fatal("wireless shortcut missing")
		}
		if wireless.Bounds.X < virtual.Bounds.X+virtual.Bounds.Width || wireless.Bounds.Y != virtual.Bounds.Y {
			t.Fatal("wireless shortcut not immediately after virtual display")
		}
		click(t, s, "切换无线")
		if f.action != "tcp" || f.serial != "USB-DEMO-01" {
			t.Fatal("wireless targeted another device")
		}
		count := 0
		for _, hit := range s.Snapshot().Hits {
			if hit.Label == "切换无线" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected only header shortcut, got %d", count)
		}
		click(t, s, "Xiaomi 15")
		if _, ok := s.Hit("切换无线"); ok {
			t.Fatal("wireless device exposes USB conversion")
		}
		clean(t, s)
		s.Close()
	}
}

func TestVirtualAppsNeverAllocateIsland(t *testing.T) {
	w, f, _ := testIsland(t, false)
	// A nonnil application and HostedBackend must not make virtual apps hosted.
	w.owner.SetApplication(&app.Application{})
	for range 3 {
		if w.owner.openSessionWindow(w.serial, domain.SessionVirtual, &domain.VirtualDisplay{StartApp: "com.example"}, w.options, false) {
			t.Fatal("virtual session attempted to open an island")
		}
		w.launchIndependentApp("com.example")
		if f.action != "virtual" || f.display == nil || f.display.StartApp != "com.example" {
			t.Fatal("independent app did not use existing virtual runner")
		}
		if len(w.owner.sessionWindows) != 1 || w.owner.sessionWindows[w.session] != w {
			t.Fatal("independent app replaced/duplicated phone island")
		}
		if w.closing || w.ctx.Err() != nil {
			t.Fatal("independent launch stopped main mirror")
		}
	}
}
