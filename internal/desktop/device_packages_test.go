package desktop

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/headless"
	"github.com/vivaui/viva/ui"
)

func TestPackageToolbarSpacingAtWindowSizes(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, width := range []int{1000, 1240, 1600} {
			t.Run(fmt.Sprintf("%s-%d", theme, width), func(t *testing.T) {
				f := sample()
				f.state.Settings.Theme = theme
				var apps []domain.AppInfo
				for i := range 12 {
					apps = append(apps, domain.AppInfo{Package: fmt.Sprintf("com.example.app%d", i), Label: fmt.Sprintf("应用 %d", i+1), System: i < 9, VersionName: "1.0", SizeBytes: 1024})
				}
				f.state.Apps = map[string]domain.AppListing{"USB-DEMO-01": {Apps: apps}}
				u := New(f)
				u.deviceTab = "packages"
				cfg := u.Config()
				cfg.Width, cfg.Height = width, 820
				s, err := headless.Open(cfg, nil, headless.Options{Scale: 1.5})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(s.Close)
				bounds := func(value string, size float32) ui.Rect {
					t.Helper()
					for _, text := range s.Snapshot().Texts {
						if text.Value == value && text.Style.Size == size {
							return text.Bounds
						}
					}
					t.Fatalf("missing header text %q", value)
					return ui.Rect{}
				}
				title := bounds("应用包", 15)
				count, ok := s.Hit("12 个应用")
				if !ok {
					t.Fatal("all filter not clickable")
				}
				system, ok := s.Hit("系统 9")
				if !ok {
					t.Fatal("system filter not clickable")
				}
				user, ok := s.Hit("用户 3")
				if !ok {
					t.Fatal("user filter not clickable")
				}
				if count.Bounds.X < title.X+title.Width+8 || system.Bounds.X < count.Bounds.X+count.Bounds.Width+5 || user.Bounds.X < system.Bounds.X+system.Bounds.Width+5 {
					t.Fatal("overlapping title/filter buttons")
				}
				install, ok := s.Hit("安装 APK")
				if !ok {
					t.Fatal("install action not reachable")
				}
				refresh, ok := s.Hit("刷新")
				if !ok {
					t.Fatal("refresh action not reachable")
				}
				if refresh.Bounds.X < install.Bounds.X+install.Bounds.Width+8 {
					t.Fatalf("buttons touch: %+v %+v", install.Bounds, refresh.Bounds)
				}
				if user.Bounds.X+user.Bounds.Width+8 > install.Bounds.X {
					t.Fatal("metadata overlaps actions")
				}
				center := func(r ui.Rect) float64 { return float64(r.Y + r.Height/2) }
				if math.Abs(center(title)-center(install.Bounds)) > 1 {
					t.Fatal("toolbar actions are not centered with title")
				}
				clean(t, s)
				if dir := os.Getenv("GOSCOPY_ARTIFACT_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					if err := s.Screenshot(filepath.Join(dir, fmt.Sprintf("packages-%s-%d.png", theme, width))); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestAppIconBitmapPixelsAndRefresh(t *testing.T) {
	encode := func(c color.NRGBA) []byte {
		t.Helper()
		img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		img.SetNRGBA(0, 0, c)
		var data bytes.Buffer
		if err := png.Encode(&data, img); err != nil {
			t.Fatal(err)
		}
		return data.Bytes()
	}
	data := encode(color.NRGBA{R: 200, G: 100, B: 50, A: 128})
	bmp, ok := appIconBitmap("com.example.app", data)
	if !ok || bmp.Width != 1 || bmp.Height != 1 || bmp.Stride != 4 {
		t.Fatal("invalid bitmap", bmp)
	}
	want := []byte{25, 50, 100, 128}
	if !bytes.Equal(bmp.Pix, want) {
		t.Fatalf("BGRA pixels %v, want %v; alpha must be applied once", bmp.Pix, want)
	}
	same, _ := appIconBitmap("com.example.app", data)
	changed, _ := appIconBitmap("com.example.app", encode(color.NRGBA{R: 20, A: 255}))
	if same.ID != bmp.ID || same.Version != bmp.Version || changed.ID != bmp.ID || changed.Version == bmp.Version {
		t.Fatal("bitmap cache must reuse identical content and invalidate changed PNGs")
	}
	for _, bad := range [][]byte{nil, []byte("not PNG")} {
		if _, ok := appIconBitmap("com.example.app", bad); ok {
			t.Fatal("invalid icon must use placeholder")
		}
	}
}

func TestPackageKindFiltersAreClickable(t *testing.T) {
	f := sample()
	f.state.Apps = map[string]domain.AppListing{"USB-DEMO-01": {Apps: []domain.AppInfo{
		{Package: "s.pkg", Label: "System app", System: true},
		{Package: "u.pkg", Label: "User app"},
	}}}
	u := New(f)
	u.deviceTab = "packages"
	s, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertRows := func(wantSystem, wantUser bool) {
		t.Helper()
		found := map[string]bool{}
		for _, text := range s.Snapshot().Texts {
			found[text.Value] = true
		}
		if found["s.pkg"] != wantSystem || found["u.pkg"] != wantUser {
			t.Fatalf("wrong visible rows: system=%v, user=%v", found["s.pkg"], found["u.pkg"])
		}
	}
	assertRows(true, true)
	click(t, s, "系统 1")
	if u.apps.filter != "system" {
		t.Fatal("system click ignored")
	}
	assertRows(true, false)
	click(t, s, "用户 1")
	if u.apps.filter != "user" {
		t.Fatal("user click ignored")
	}
	assertRows(false, true)
	click(t, s, "2 个应用")
	if u.apps.filter != "all" {
		t.Fatal("all click ignored")
	}
	assertRows(true, true)
	clean(t, s)
}

func TestPackageSearchCombinesKindAndAgentLabel(t *testing.T) {
	u := New(sample())
	apps := []domain.AppInfo{
		{Package: "com.example.system", Label: "Camera", System: true},
		{Package: "com.example.user", Label: "Camera Plus"},
	}
	u.apps.search.SetText("camera")
	u.apps.filter = "user"
	got := u.filterApps("USB-DEMO-01", apps)
	if len(got) != 1 || got[0].Package != "com.example.user" {
		t.Fatalf("search must use Agent labels and respect kind: %+v", got)
	}
}
