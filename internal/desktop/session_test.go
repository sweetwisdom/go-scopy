package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/windowhost"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"
)

type toolbarBackend struct{ *fixture }

func (f *toolbarBackend) StartHosted(string, string, domain.Options, *domain.VirtualDisplay, bool, domain.SessionHost) {
}
func (f *toolbarBackend) Key(serial, action string) { f.serial, f.action = serial, action }

func TestSessionToolbarTargetsItsDevice(t *testing.T) {
	f := &toolbarBackend{sample()}
	u := New(f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &sessionWindow{owner: u, serial: "192.168.1.28:37001", name: "Pixel 9 Pro", ready: true, collapsed: true, ctx: ctx, cancel: cancel, native: windowhost.New("test")}
	w.scope = w.tree.Feature("session")
	s, err := headless.Open(app.Config{Width: 236, Height: 48, Paint: w.paintTools, Tree: &w.tree}, nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	clean(t, s)
	if _, exists := s.Hit("桌面"); exists {
		t.Fatal("collapsed island exposes phone commands")
	}
	preview := os.Getenv("GOSCOPY_ISLAND_PREVIEW")
	if preview != "" {
		if err := os.MkdirAll(preview, 0755); err != nil {
			t.Fatal(err)
		}
		if err := s.Screenshot(filepath.Join(preview, "island-collapsed.png")); err != nil {
			t.Fatal(err)
		}
	}
	click(t, s, "展开控制面板")
	if w.collapsed {
		t.Fatal("island did not expand")
	}
	if err := s.Driver().Resize(344, 368); err != nil {
		t.Fatal(err)
	}
	clean(t, s)
	if preview != "" {
		if err := s.Screenshot(filepath.Join(preview, "island-expanded.png")); err != nil {
			t.Fatal(err)
		}
	}
	click(t, s, "桌面")
	if f.action != "home" || f.serial != w.serial {
		t.Fatal("toolbar targeted another device")
	}
	click(t, s, "任务")
	if f.action != "recents" {
		t.Fatal("recent apps action was not dispatched")
	}
	click(t, s, "截图")
	if f.action != "capture" || f.serial != w.serial {
		t.Fatal("screenshot targeted another device")
	}
	w.closing = true
	s.Driver().Resize(344, 368)
	if _, exists := s.Hit("桌面"); exists {
		t.Fatal("closing session accepts phone commands")
	}
}
