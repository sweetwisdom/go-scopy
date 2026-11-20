package desktop

import (
	"fmt"
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
				title, count, summary := bounds("应用包", 15), bounds("12 个应用", 11), bounds("系统 9 · 用户 3", 11)
				if count.X < title.X+title.Width+8 || summary.X < count.X+count.Width+8 {
					t.Fatalf("overlapping title/badges: %+v %+v %+v", title, count, summary)
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
				if summary.X+summary.Width+8 > install.Bounds.X {
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
