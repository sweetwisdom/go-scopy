package desktop

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/headless"
	"github.com/vivaui/viva/ui"
)

// A synthetic portrait screen: all tests stay local and never capture a phone.
func portraitPreviewPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 360, 800))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 32, G: 66, B: 112, A: 255}), image.Point{}, draw.Src)
	for i := 0; i < 4; i++ {
		rect := image.Rect(24, 110+i*132, 336, 218+i*132)
		draw.Draw(img, rect, image.NewUniform(color.RGBA{R: uint8(70 + i*18), G: uint8(114 + i*18), B: 185, A: 255}), image.Point{}, draw.Src)
	}
	draw.Draw(img, image.Rect(30, 30, 120, 36), image.White, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(138, 778, 222, 783), image.White, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func paintedText(t *testing.T, s *headless.Session, value string) ui.Text {
	t.Helper()
	for _, text := range s.Snapshot().Texts {
		if text.Value == value {
			return text
		}
	}
	t.Fatalf("missing text %q", value)
	return ui.Text{}
}

func TestRefinedOverviewPositionsAndPhoneFrame(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, width := range []int{1000, 1240, 1600} {
			t.Run(fmt.Sprintf("%s-%d", theme, width), func(t *testing.T) {
				f := &workspaceFixture{fixture: sample()}
				f.state.Settings.Theme = theme
				serial := f.state.Devices[0].Serial
				f.state.PhoneInfos = map[string]domain.PhoneInfo{serial: overviewSample()}
				f.state.Preview = domain.ScreenPreview{Serial: serial, PNG: portraitPreviewPNG(t), UpdatedAt: time.Now()}
				u := New(f)
				cfg := u.Config()
				cfg.Width, cfg.Height = width, 820
				if width == 1000 {
					cfg.Height = 700
				}
				s, err := headless.Open(cfg, nil, headless.Options{Scale: 1.5})
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				clean(t, s)
				basic := paintedText(t, s, "设备信息").Bounds
				battery := paintedText(t, s, "电池").Bounds
				storage := paintedText(t, s, "存储与内存").Bounds
				preview := paintedText(t, s, "屏幕截图").Bounds
				if preview.X <= basic.X || battery.Y <= basic.Y || storage.Y <= basic.Y || battery.X >= preview.X || storage.X >= preview.X {
					t.Fatal("overview does not place summaries to the left of the phone")
				}
				frame, screen := u.preview.frame.Rect(), u.preview.screen.Rect()
				if frame.Width <= screen.Width || frame.Height <= screen.Height || screen.X <= frame.X || screen.Y <= frame.Y {
					t.Fatal("screen has no enclosing phone bezel")
				}
				if math.Abs(float64(screen.Width/screen.Height-360.0/800.0)) > 0.001 {
					t.Fatal("phone screenshot stretched")
				}
				if screen.Width > 260 || screen.Height > 520 || frame.Y+frame.Height > float32(cfg.Height)-24 {
					t.Fatalf("phone preview too large for window: frame=%+v screen=%+v", frame, screen)
				}
				if storage.Y+u.overviewVitalsHeight() > float32(cfg.Height) || battery.Y+u.overviewVitalsHeight() > float32(cfg.Height) {
					t.Fatal("battery/storage summary does not fit the normal window")
				}
				for _, text := range []string{"更新于 ", "截图于 ", "预览仅保留在内存中", "常用操作", "快捷操作", "投屏并录屏", "安装 APK", "停止会话"} {
					if s.HasText(text) {
						t.Fatal("removed copy still displayed", text)
					}
				}
				workspaceScreenshot(t, s, fmt.Sprintf("refined-overview-%s-%d", theme, width))
			})
		}
	}
}

type refinementFixture struct {
	*workspaceFixture
	launches []domain.VirtualDisplay
	clears   int
}

func (f *refinementFixture) StartVirtual(serial string, _ domain.Options, d domain.VirtualDisplay, _ bool) {
	f.serial = serial
	f.launches = append(f.launches, d)
}
func (f *refinementFixture) ClearAppCache() {
	f.clears++
	f.state.SharedAppMetas = nil
	f.state.AppMetas = nil
	f.state.AppCacheGeneration++
	for serial, listing := range f.state.Apps {
		listing.Apps = append([]domain.AppInfo(nil), listing.Apps...)
		for i := range listing.Apps {
			listing.Apps[i].Label = ""
			listing.Apps[i].IconPNG = nil
		}
		f.state.Apps[serial] = listing
	}
	if f.notify != nil {
		f.notify()
	}
}
func refinementApps(t *testing.T) *refinementFixture {
	t.Helper()
	f := &refinementFixture{workspaceFixture: &workspaceFixture{fixture: sample(), requested: map[string]bool{}}}
	var apps []domain.AppInfo
	f.state.SharedAppMetas = map[string]domain.AppMeta{}
	icon := bitmapTestPNG(t, color.NRGBA{R: 50, G: 140, B: 210, A: 255}, 96)
	for i := 0; i < 50; i++ {
		pkg := fmt.Sprintf("com.example.app%03d", i)
		apps = append(apps, domain.AppInfo{Package: pkg, APKPath: fmt.Sprintf("/data/app/%d/base.apk", i), VersionCode: "42", System: i >= 40})
		f.state.SharedAppMetas[pkg] = domain.AppMeta{Package: pkg, Label: fmt.Sprintf("示例应用 %03d", i), IconPNG: icon, Resolved: true}
	}
	f.state.Apps = map[string]domain.AppListing{}
	for _, d := range f.state.Devices[:2] {
		f.state.Apps[d.Serial] = domain.AppListing{Apps: append([]domain.AppInfo(nil), apps...)}
	}
	return f
}

func TestRefinedVirtualModalSearchSelectionAndValidation(t *testing.T) {
	t.Setenv("VIVA_HEADLESS", "1")
	for _, theme := range []string{"light", "dark"} {
		for _, width := range []int{1000, 1240} {
			t.Run(fmt.Sprintf("%s-%d", theme, width), func(t *testing.T) {
				f := refinementApps(t)
				f.state.Settings.Theme = theme
				u := New(f)
				cfg := u.Config()
				cfg.Width, cfg.Height = width, 700
				s, err := headless.Open(cfg, nil, headless.Options{Scale: 1.5})
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				workspaceClick(t, s, "虚拟屏幕")
				if !u.vd.open {
					t.Fatal("virtual modal not open")
				}
				if _, ok := s.Hit("投屏参数"); ok {
					t.Fatal("background action remains reachable through modal")
				}
				clean(t, s)
				workspaceScreenshot(t, s, fmt.Sprintf("virtual-modal-%s-%d", theme, width))
				u.vd.search.SetText("示例应用 007")
				u.wake.Invalidate()
				s.Driver().Pump()
				if !s.HasText("示例应用 007") || s.HasText("示例应用 008") {
					t.Fatal("Chinese name search not applied")
				}
				workspaceClick(t, s, "选择应用 · 示例应用 007 · com.example.app007")
				u.vd.search.SetText("APP008")
				u.wake.Invalidate()
				s.Driver().Pump()
				if !s.HasText("com.example.app008") || u.vd.selectedApp != "com.example.app007" {
					t.Fatal("package search changed selection")
				}
				u.vd.width.SetText("invalid")
				workspaceClick(t, s, "启动虚拟屏幕")
				if !u.vd.open || len(f.launches) != 0 || u.notice == "" {
					t.Fatal("invalid display launched or dismissed dialog")
				}
				clean(t, s)
				u.vd.width.SetText("1080")
				u.vd.dpi.SetText("0")
				workspaceClick(t, s, "启动虚拟屏幕")
				if u.vd.open || len(f.launches) != 1 || f.launches[0].StartApp != "com.example.app007" || f.launches[0].DPI != 0 || f.serial != "USB-DEMO-01" {
					t.Fatalf("virtual launch lost selected package: %+v", f.launches)
				}
			})
		}
	}
}

func TestRefinedGlobalIconsShareAcrossTransportsAndClearConfirmation(t *testing.T) {
	f := refinementApps(t)
	u := New(f)
	cfg := u.Config()
	cfg.Height = 1250
	s, err := headless.Open(cfg, nil, headless.Options{Scale: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	workspaceClick(t, s, "应用包")
	first := u.apps.bitmaps.entries["com.example.app000"]
	if first == nil {
		t.Fatal("shared app icon not decoded")
	}
	workspaceClick(t, s, "Xiaomi 15")
	if next := u.apps.bitmaps.entries["com.example.app000"]; next != first {
		t.Fatal("USB and Wi-Fi did not reuse decoded bitmap")
	}
	if len(f.requested) != 0 {
		t.Fatal("shared icons unnecessarily requested per-device metadata")
	}
	workspaceClick(t, s, "设置")
	workspaceClick(t, s, "清理应用缓存")
	clean(t, s)
	workspaceScreenshot(t, s, "clear-global-app-cache")
	workspaceClick(t, s, "取消")
	if f.clears != 0 {
		t.Fatal("cancel cleared cache")
	}
	workspaceClick(t, s, "清理应用缓存")
	workspaceClick(t, s, "确认清理缓存")
	if f.clears != 1 || len(u.apps.bitmaps.entries) != 0 || u.clearAppCache {
		t.Fatal("confirmed clear retained decoded cache or modal")
	}
	if len(f.state.Apps["USB-DEMO-01"].Apps) != 50 || f.state.Apps["USB-DEMO-01"].Apps[0].APKPath == "" {
		t.Fatal("cache clear deleted device installation data")
	}
}

func TestDeviceHeadingConnectionToggleFollowsTransportState(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			f := sample()
			f.state.Settings.Theme = theme
			u, s := openUI(t, f)
			if u.deviceConnection.label != "" {
				t.Fatal("USB device exposes an unsupported disconnect action")
			}
			click(t, s, "Xiaomi 15")
			clean(t, s)
			hit, ok := s.Hit("断开连接")
			if !ok {
				t.Fatal("connected wireless device has no disconnect control")
			}
			title := paintedText(t, s, "Xiaomi 15").Bounds
			// The sidebar contains the same name; use the large workspace heading.
			for _, text := range s.Snapshot().Texts {
				if text.Value == "Xiaomi 15" && text.Bounds.X > 350 {
					title = text.Bounds
					break
				}
			}
			if hit.Bounds.X < title.X+title.Width || math.Abs(float64(hit.Bounds.Y+hit.Bounds.Height/2-title.Y-title.Height/2)) > 8 {
				t.Fatalf("connection control is not beside the name: title=%+v control=%+v", title, hit.Bounds)
			}
			if err := s.Move(hit.Bounds.X+15, hit.Bounds.Y+15); err != nil {
				t.Fatal(err)
			}
			if err := s.Rest(); err != nil {
				t.Fatal(err)
			}
			if !s.HasText("断开连接") {
				t.Fatal("connection icon has no hover explanation")
			}
			workspaceScreenshot(t, s, "device-connection-"+theme)
			click(t, s, "断开连接")
			if f.action != "disconnect" || f.serial != "192.168.1.28:37001" {
				t.Fatal("disconnect targeted the wrong transport")
			}
			f.state.Devices[1].State = "offline"
			f.notify()
			if err := s.Driver().Pump(); err != nil {
				t.Fatal(err)
			}
			if err := s.Rest(); err != nil {
				t.Fatal(err)
			}
			clean(t, s)
			if _, ok := s.Hit("断开连接"); ok {
				t.Fatal("offline device retains disconnect action")
			}
			click(t, s, "连接设备")
			if f.action != "connect" || f.address != "192.168.1.28:37001" {
				t.Fatal("reconnect targeted the wrong transport")
			}
			f.state.Devices[1].State = "device"
			f.state.Sessions[domain.SessionID(f.state.Devices[1].Serial, domain.SessionMirror)] = domain.Session{Status: "running"}
			f.notify()
			if err := s.Driver().Pump(); err != nil {
				t.Fatal(err)
			}
			if err := s.Rest(); err != nil {
				t.Fatal(err)
			}
			f.action = ""
			_ = s.ClickLabel("断开连接")
			if f.action != "" {
				t.Fatal("active session did not protect the disconnect action")
			}
			click(t, s, "Pixel 9 Pro")
			if u.deviceConnection.label != "" {
				t.Fatal("wireless connection action leaked to USB device")
			}
		})
	}
}
