package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"
)

// Synthetic devices exercise the actual painter and router, never real networking.
type fixture struct {
	state                         domain.State
	notify                        func()
	action, address, code, serial string
	options                       domain.Options
	record                        bool
}

func sample() *fixture {
	return &fixture{state: domain.State{Settings: config.Default(), Sessions: map[string]domain.Session{}, Connection: "设备监听已连接", ADBVersion: "Android Debug Bridge version 1.0.41", ScrcpyVersion: "scrcpy 5.0.1", Devices: []domain.Device{{Serial: "USB-DEMO-01", Model: "Pixel 9 Pro", State: "device", Connection: "USB", Product: "caiman"}, {Serial: "192.168.1.28:37001", Model: "Xiaomi 15", State: "device", Connection: "Wi-Fi"}, {Serial: "USB-DEMO-03", Model: "Galaxy S25", State: "unauthorized", Connection: "USB"}}}}
}
func (f *fixture) State() domain.State       { return f.state }
func (f *fixture) SetNotify(fn func())       { f.notify = fn }
func (f *fixture) Refresh()                  { f.action = "refresh" }
func (f *fixture) Connect(address string)    { f.action = "connect"; f.address = address }
func (f *fixture) Pair(address, code string) { f.action = "pair"; f.address = address; f.code = code }
func (f *fixture) Disconnect(serial string)  { f.action = "disconnect"; f.serial = serial }
func (f *fixture) EnableTCP(serial string)   { f.action = "tcp"; f.serial = serial }
func (f *fixture) Start(serial string, o domain.Options, record bool) {
	f.action = "start"
	f.serial = serial
	f.options = o
	f.record = record
}
func (f *fixture) Stop(serial string)    { f.action = "stop"; f.serial = serial }
func (f *fixture) Capture(serial string) { f.action = "capture"; f.serial = serial }
func (f *fixture) Install(serial, apk string) {
	f.action = "install"
	f.serial = serial
	f.address = apk
}
func (f *fixture) Save(s domain.Settings) {
	f.action = "save"
	f.state.Settings = s
	if f.notify != nil {
		f.notify()
	}
}
func (f *fixture) Close() {}

func openUI(t *testing.T, f *fixture) (*UI, *headless.Session) {
	t.Helper()
	u := New(f)
	s, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return u, s
}
func click(t *testing.T, s *headless.Session, label string) {
	t.Helper()
	if err := s.ClickLabel(label); err != nil {
		t.Fatal(err)
	}
}
func clean(t *testing.T, s *headless.Session) {
	t.Helper()
	for _, p := range headless.CheckLayout(s.Snapshot()) {
		t.Errorf("layout: %+v", p)
	}
}

func TestDeviceActionsUseSelectedSerialAndValidateParameters(t *testing.T) {
	f := sample()
	u, s := openUI(t, f)
	clean(t, s)
	click(t, s, "开始投屏")
	if f.action != "start" || f.serial != "USB-DEMO-01" || f.record {
		t.Fatalf("unexpected action: %+v", f)
	}
	click(t, s, "Xiaomi 15")
	click(t, s, "投屏并录屏")
	if f.serial != "192.168.1.28:37001" || !f.record {
		t.Fatal("recording targeted wrong device")
	}
	u.fps.SetText("0")
	f.action = ""
	click(t, s, "开始投屏")
	if f.action != "" || !strings.Contains(u.notice, "帧率") {
		t.Fatal("invalid parameters were dispatched")
	}
	click(t, s, "Galaxy S25")
	f.action = ""
	if _, exists := s.Hit("开始投屏"); exists {
		t.Fatal("unauthorized start must not be interactive")
	}
	if f.action != "" {
		t.Fatal("unauthorized device started")
	}
}

func TestWirelessPairingClearsSecretAndKeepsPortsSeparate(t *testing.T) {
	f := sample()
	u, s := openUI(t, f)
	click(t, s, "＋  无线连接")
	clean(t, s)
	u.pairAddress.SetText("192.168.1.2:37000")
	u.code.SetText("123456")
	click(t, s, "配对设备")
	if f.action != "pair" || f.address != "192.168.1.2:37000" || f.code != "123456" || u.code.Text() != "" {
		t.Fatal("pairing dispatch or secret cleanup failed")
	}
	u.address.SetText("192.168.1.2:38000")
	click(t, s, "连接设备")
	if f.action != "connect" || f.address != "192.168.1.2:38000" {
		t.Fatal("connection reused pairing port")
	}
}

func TestSettingsAndTaskFeedback(t *testing.T) {
	f := sample()
	_, s := openUI(t, f)
	click(t, s, "设置")
	clean(t, s)
	click(t, s, "切换为深色")
	s.Driver().Pump()
	if f.state.Settings.Theme != "dark" {
		t.Fatal("theme not saved")
	}
	click(t, s, "运行环境")
	clean(t, s)
	click(t, s, "保存设置")
	if f.action != "save" {
		t.Fatal("settings not dispatched")
	}
	f.state.Tasks = []domain.Task{{ID: 1, Title: "安装 APK", Status: "error", Detail: "设备已断开，请重新连接", Started: time.Now()}}
	f.notify()
	s.Driver().Pump()
	click(t, s, "任务")
	if !s.HasText("设备已断开，请重新连接") {
		t.Fatal("task error is not visible")
	}
	clean(t, s)
}

func TestLayoutsAndScreenshots(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			f := sample()
			f.state.Settings.Theme = theme
			u, s := openUI(t, f)
			for _, size := range [][2]int{{1240, 820}, {1000, 700}, {1600, 1000}} {
				if err := s.Resize(size[0], size[1]); err != nil {
					t.Fatal(err)
				}
				clean(t, s)
			}
			s.Resize(1240, 820)
			if dir := os.Getenv("GOSCOPY_ARTIFACT_DIR"); dir != "" {
				os.MkdirAll(dir, 0755)
				if err := s.Screenshot(filepath.Join(dir, "devices-"+theme+".png")); err != nil {
					t.Fatal(err)
				}
			}
			click(t, s, "＋  无线连接")
			clean(t, s)
			click(t, s, "设置")
			clean(t, s)
			click(t, s, "关于")
			clean(t, s)
			u.state.Devices = nil
			u.section = "devices"
			u.selected = ""
			u.connecting = false
			u.wake.Invalidate()
			s.Driver().Pump()
			clean(t, s)
		})
	}
}

func BenchmarkPaint(b *testing.B) {
	f := sample()
	u := New(f)
	driver, err := app.NewDriver(u.Config(), nil)
	if err != nil {
		b.Fatal(err)
	}
	defer driver.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := driver.Resize(1240, 820); err != nil {
			b.Fatal(err)
		}
	}
}

func Example_fixture() {
	fmt.Println("Screenshots use synthetic device metadata; real ADB is tested separately.") // Output: Screenshots use synthetic device metadata; real ADB is tested separately.
}
