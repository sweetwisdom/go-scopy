package desktop

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/headless"
)

func workspaceClick(t *testing.T, s *headless.Session, label string) {
	t.Helper()
	click(t, s, label)
	// Let presence enter/exit frames complete before opening a different dialog.
	if err := s.Wait(400 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := s.Rest(); err != nil {
		t.Fatal(err)
	}
}

func workspaceScreenshot(t *testing.T, s *headless.Session, name string) {
	t.Helper()
	if err := s.Rest(); err != nil {
		t.Fatal(err)
	}
	clean(t, s)
	if dir := os.Getenv("GOSCOPY_ARTIFACT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := s.Screenshot(filepath.Join(dir, name+".png")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkspaceDialogsLayoutAndSaveTargets(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, scale := range []float64{1, 1.5} {
			t.Run(fmt.Sprintf("%s-%.1f", theme, scale), func(t *testing.T) {
				f := sample()
				f.state.Settings.Theme = theme
				u := New(f)
				s, err := headless.Open(u.Config(), nil, headless.Options{Scale: scale})
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				for _, size := range [][2]int{{1000, 700}, {1240, 820}} {
					if err := s.Resize(size[0], size[1]); err != nil {
						t.Fatal(err)
					}
					workspaceClick(t, s, "投屏参数")
					clean(t, s)
					workspaceScreenshot(t, s, fmt.Sprintf("projection-%s-%.1f-%d", theme, scale, size[0]))
					u.fps.SetText("45")
					workspaceClick(t, s, "取消")
					if u.optionsSerial != "" || u.fps.Text() != fmt.Sprint(f.state.Settings.OptionsFor(u.selected).FPS) {
						t.Fatal("cancel retained projection draft")
					}
					// Editing another row must not depend on the currently selected device.
					workspaceClick(t, s, "编辑设备 · Xiaomi 15")
					if u.renameSerial != "192.168.1.28:37001" || u.selected != "USB-DEMO-01" {
						t.Fatal("row editor targeted current device")
					}
					clean(t, s)
					workspaceScreenshot(t, s, fmt.Sprintf("editor-%s-%.1f-%d", theme, scale, size[0]))
					workspaceClick(t, s, "取消")
					workspaceClick(t, s, "删除记录 · Xiaomi 15")
					clean(t, s)
					workspaceScreenshot(t, s, fmt.Sprintf("delete-%s-%.1f-%d", theme, scale, size[0]))
					workspaceClick(t, s, "保留记录")
				}
				workspaceClick(t, s, "投屏参数")
				f.action = ""
				u.fps.SetText("0")
				workspaceClick(t, s, "保存此设备参数")
				if f.action != "" || u.optionsSerial == "" || u.notice == "" {
					t.Fatal("invalid parameters were saved or dialog closed")
				}
				u.fps.SetText("45")
				workspaceClick(t, s, "保存此设备参数")
				if u.optionsSerial != "" || f.state.Settings.OptionsFor("USB-DEMO-01").FPS != 45 {
					t.Fatal("device parameters not saved")
				}
				workspaceClick(t, s, "Xiaomi 15")
				workspaceClick(t, s, "投屏参数")
				u.fps.SetText("30")
				workspaceClick(t, s, "设为全局默认")
				if f.state.Settings.Defaults.FPS != 30 || f.state.Settings.OptionsFor("USB-DEMO-01").FPS != 45 {
					t.Fatal("global save overwrote device override")
				}
			})
		}
	}
}

type workspaceFixture struct {
	*fixture
	requested                    map[string]bool
	previewSerial, clearedSerial string
}

func (f *workspaceFixture) FetchAppMeta(serial, path, pkg string) {
	f.requested[serial+"\x00"+pkg] = true
}
func (f *workspaceFixture) FetchPreview(serial string) { f.previewSerial = serial }
func (f *workspaceFixture) ClearPreview(serial string) {
	f.clearedSerial = serial
	f.state.Preview = domain.ScreenPreview{}
}

func TestWorkspacePreviewRefreshAndDeviceSwitch(t *testing.T) {
	f := &workspaceFixture{fixture: sample()}
	serial := f.state.Devices[0].Serial
	f.state.Preview = domain.ScreenPreview{Serial: serial, PNG: bitmapTestPNG(t, color.NRGBA{R: 80, G: 170, B: 210, A: 255}, 64), UpdatedAt: time.Now()}
	u := New(f)
	s, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !u.preview.valid {
		t.Fatal("preview PNG not decoded")
	}
	clean(t, s)
	workspaceScreenshot(t, s, "preview-light-1.5")
	workspaceClick(t, s, "刷新截图")
	if f.previewSerial != serial {
		t.Fatal("refresh targeted another device")
	}
	workspaceClick(t, s, "Xiaomi 15")
	if f.clearedSerial != serial || u.preview.valid || len(u.preview.data) != 0 {
		t.Fatal("device switch retained old preview")
	}
}

func TestWorkspaceOnlyVisibleAppsRequestMetadataWithoutMenus(t *testing.T) {
	f := &workspaceFixture{fixture: sample(), requested: map[string]bool{}}
	serial := f.state.Devices[0].Serial
	var apps []domain.AppInfo
	for i := 0; i < 200; i++ {
		apps = append(apps, domain.AppInfo{Package: fmt.Sprintf("com.example.app%03d", i), APKPath: fmt.Sprintf("/data/app/%d/base.apk", i)})
	}
	f.state.Apps = map[string]domain.AppListing{serial: {Apps: apps}}
	u := New(f)
	s, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	workspaceClick(t, s, "应用包")
	if len(f.requested) == 0 || len(f.requested) >= len(apps) || u.apps.menuFor != "" {
		t.Fatalf("not lazily loading visible apps: requested=%d menu=%s", len(f.requested), u.apps.menuFor)
	}
	clean(t, s)
}
