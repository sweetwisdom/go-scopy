package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/headless"
)

func overviewSample() domain.PhoneInfo {
	voltage, temp, charge, current, uptime := int64(4210), int64(315), int64(1234567), int64(-450000), int64(90061)
	return domain.PhoneInfo{
		Brand: "Google", Manufacturer: "Google", Model: "Pixel 9 Pro", DeviceCode: "caiman",
		AndroidVersion: "15", APILevel: "35", SecurityPatch: "2026-11-01", CPUABI: "arm64-v8a",
		Hardware: "Tensor G4", PhysicalSize: "1280x2856", OverrideSize: "960x2142", PhysicalDPI: "480",
		IPAddress: "192.168.1.100", WiFiSSID: "Office Wi-Fi", SerialNumber: "DEMO-SERIAL-001", Slot: "Slot A",
		BootloaderState: "已锁定（flash.locked=1）", VerifiedBoot: "green", StorageType: "UFS（启动设备属性）",
		RootStatus: "未确认；ADB UID 2000", KernelVersion: "6.1.0-android15-example", UptimeSeconds: &uptime,
		BatteryLevel: 82, BatteryStatus: "放电中", BatteryHealth: "良好", BatteryVoltageMV: &voltage,
		BatteryTemperatureTenthsC: &temp, BatteryChargeCounterUAH: &charge, BatteryCurrentUA: &current,
		MemTotalKB: 16 * 1024 * 1024, MemAvailableKB: 6 * 1024 * 1024,
		StorageTotalBytes: 256 * 1024 * 1024 * 1024, StorageUsedBytes: 120 * 1024 * 1024 * 1024,
		Fingerprint: strings.Repeat("google/caiman/build:test-release/", 6), UpdatedAt: time.Now(),
	}
}

func TestOverviewLayoutsAndDataAtWindowSizes(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, width := range []int{1000, 1240, 1600} {
			t.Run(fmt.Sprintf("%s-%d", theme, width), func(t *testing.T) {
				f := sample()
				f.state.Settings.Theme = theme
				f.state.PhoneInfos = map[string]domain.PhoneInfo{"USB-DEMO-01": overviewSample()}
				u := New(f)
				cfg := u.Config()
				cfg.Width, cfg.Height = width, 1000
				s, err := headless.Open(cfg, nil, headless.Options{Scale: 1.5})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(s.Close)
				clean(t, s)
				texts := ""
				for _, text := range s.Snapshot().Texts {
					texts += text.Value + "\n"
				}
				for _, value := range []string{"设备信息", "系统版本", "15 (API 35)", "Pixel 9 Pro", "31.5°C", "全部设备信息"} {
					if !strings.Contains(texts, value) {
						t.Errorf("missing overview text %q", value)
					}
				}
				if strings.Contains(texts, "正在读取") {
					t.Error("completed data still loading")
				}
				if dir := os.Getenv("GOSCOPY_ARTIFACT_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					if err := s.Screenshot(filepath.Join(dir, fmt.Sprintf("overview-%s-%d.png", theme, width))); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestOverviewUnknownDoesNotRetryOrShowFalseZero(t *testing.T) {
	f := sample()
	f.state.PhoneInfos = map[string]domain.PhoneInfo{"USB-DEMO-01": {BatteryLevel: -1, MemAvailableKB: -1, UpdatedAt: time.Now(), Warnings: []string{"设备未提供部分数据"}}}
	_, s := openUI(t, f)
	clean(t, s)
	for _, text := range s.Snapshot().Texts {
		if text.Value == "0%" || strings.Contains(text.Value, "正在读取") {
			t.Fatal("unknown must not become loading/zero", text.Value)
		}
	}
	if memoryRatio(1000, -1) >= 0 || storageRatio(0, 0) >= 0 {
		t.Fatal("unknown ratio must be negative")
	}
	if memoryText(1000, -1) != "未知" || memoryText(1000, 1001) != "未知" {
		t.Fatal("unknown availability")
	}
	if scaledInfo(nil, 1000, " V") != "" {
		t.Fatal("missing measurement became zero")
	}
	zero := int64(0)
	if scaledInfo(&zero, 10, "°C") != "0°C" || uptimeText(&zero) != "0小时 0分" {
		t.Fatal("valid zero lost")
	}
}

// Completed-but-empty fields must not trigger another automatic fetch.
type overviewFixture struct {
	*fixture
	reads int
}

func (f *overviewFixture) FetchPhoneInfo(string) { f.reads++ }

func TestOverviewEmptyCompletionRequiresExplicitRefresh(t *testing.T) {
	base := sample()
	base.state.PhoneInfos = map[string]domain.PhoneInfo{"USB-DEMO-01": {BatteryLevel: -1, MemAvailableKB: -1, UpdatedAt: time.Now()}}
	f := &overviewFixture{fixture: base}
	u := New(f)
	s, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if f.reads != 0 {
		t.Fatal("completed empty info fetched automatically")
	}
	click(t, s, "刷新信息")
	if f.reads != 1 {
		t.Fatal("explicit refresh was not dispatched", f.reads)
	}
}

func TestOverviewExpandedLongFieldsLayout(t *testing.T) {
	f := sample()
	f.state.PhoneInfos = map[string]domain.PhoneInfo{"USB-DEMO-01": overviewSample()}
	u := New(f)
	u.overviewDetailsSerial = "USB-DEMO-01"
	cfg := u.Config()
	cfg.Width, cfg.Height = 1000, 700
	s, err := headless.Open(cfg, nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	clean(t, s)
	found := false
	for _, text := range s.Snapshot().Texts {
		if text.Value == "品牌" {
			found = true
		}
	}
	if !found {
		t.Fatal("advanced information missing")
	}
	if _, ok := s.Hit("刷新信息"); ok {
		t.Fatal("background information refresh remains reachable through details modal")
	}
	u.view.ScrollState(u.id("overview-details-scroll")).ScrollToEnd()
	u.wake.Invalidate()
	s.Driver().Pump()
	if err := s.Rest(); err != nil {
		t.Fatal(err)
	}
	clean(t, s)
	if !s.HasText("构建指纹") || !s.HasText("内核版本") {
		t.Fatal("long diagnostic fields are not reachable by scrolling")
	}
	workspaceScreenshot(t, s, "overview-details-scrolled")
	workspaceClick(t, s, "关闭详情")
	if u.overviewDetailsSerial != "" {
		t.Fatal("advanced section did not collapse")
	}
}

func TestOverviewScreenshotControlsFollowSelectedDevice(t *testing.T) {
	f := &workspaceFixture{fixture: sample()}
	u := New(f)
	s, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, device := range []struct{ label, serial string }{{"Pixel 9 Pro", "USB-DEMO-01"}, {"Xiaomi 15", "192.168.1.28:37001"}} {
		click(t, s, device.label)
		click(t, s, "保存截图")
		if f.action != "capture" || f.serial != device.serial {
			t.Fatal("capture targeted wrong device")
		}
		click(t, s, "刷新截图")
		if f.previewSerial != device.serial {
			t.Fatal("preview targeted wrong device")
		}
	}
	click(t, s, "Galaxy S25")
	f.action, f.previewSerial = "", ""
	_ = s.ClickLabel("保存截图")
	_ = s.ClickLabel("刷新截图")
	if f.action != "" || f.previewSerial != "" {
		t.Fatal("unauthorized device screenshot controls dispatched actions")
	}
}

func TestOverviewLoadingDisablesRefreshAndFailureKeepsCards(t *testing.T) {
	for _, loading := range []bool{false, true} {
		base := sample()
		info := overviewSample()
		info.Loading = loading
		if !loading {
			info.Error = "offline（保留上次结果）"
		}
		base.state.PhoneInfos = map[string]domain.PhoneInfo{"USB-DEMO-01": info}
		f := &overviewFixture{fixture: base}
		u := New(f)
		s, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
		if err != nil {
			t.Fatal(err)
		}
		clean(t, s)
		texts := ""
		for _, text := range s.Snapshot().Texts {
			texts += text.Value + "\n"
		}
		if !strings.Contains(texts, "设备信息") {
			t.Fatal("error hid successful information")
		}
		if loading {
			if !strings.Contains(texts, "正在读取设备信息") {
				t.Fatal("missing loading status")
			}
			_ = s.ClickLabel("刷新信息")
			if f.reads != 0 {
				t.Fatal("loading refresh was not disabled")
			}
		}
		s.Close()
	}
}
