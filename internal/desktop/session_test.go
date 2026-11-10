package desktop

import (
	"context"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/windowhost"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"
)

type toolbarBackend struct{ *fixture }

func (f *toolbarBackend) StartHosted(string, domain.Options, bool, domain.SessionHost) {}
func (f *toolbarBackend) Key(serial, action string)                                    { f.serial, f.action = serial, action }

func TestSessionToolbarTargetsItsDevice(t *testing.T) {
	f := &toolbarBackend{sample()}
	u := New(f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &sessionWindow{owner: u, serial: "192.168.1.28:37001", ready: true, ctx: ctx, cancel: cancel, native: windowhost.New("test", sessionToolbarHeight, false)}
	w.scope = w.tree.Feature("session")
	s, err := headless.Open(app.Config{Width: 76, Height: 512, Paint: w.paintTools, Tree: &w.tree}, nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	clean(t, s)
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
	s.Driver().Resize(76, 512)
	if _, exists := s.Hit("桌面"); exists {
		t.Fatal("closing session accepts phone commands")
	}
}
