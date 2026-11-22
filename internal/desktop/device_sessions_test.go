package desktop

import (
	"github.com/vivaui/viva/headless"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

type virtualFixture struct {
	*fixture
	launches []domain.VirtualDisplay
	options  []domain.Options
}

func (f *virtualFixture) StartVirtual(serial string, o domain.Options, d domain.VirtualDisplay, record bool) {
	f.serial = serial
	f.launches = append(f.launches, d)
	f.options = append(f.options, o)
}

func TestVirtualLaunchFallsBackAndDoesNotReusePreviousLaunch(t *testing.T) {
	t.Setenv("VIVA_HEADLESS", "1")
	f := &virtualFixture{fixture: sample()}
	u := New(f)
	u.openVirtualDialog(u.selected)
	f.state.Apps = map[string]domain.AppListing{u.selected: {Apps: []domain.AppInfo{{Package: "com.example.first"}, {Package: "com.example.second"}}}}
	u.state = f.state
	u.vd.selectedApp = "com.example.first"
	u.launchVirtualDialog()
	u.openVirtualDialog(u.selected)
	u.vd.selectedApp = "com.example.second"
	u.launchVirtualDialog()
	if len(f.launches) != 2 || f.launches[0].StartApp != "com.example.first" || f.launches[1].StartApp != "com.example.second" || f.serial != u.selected {
		t.Fatalf("virtual launch lost: %+v", f.launches)
	}
}

func TestVirtualSessionsFilterOrderAndStopByExactID(t *testing.T) {
	f := sample()
	u := New(f)
	now := time.Now()
	for i, id := range []string{"virtual:USB-DEMO-01:2", "virtual:USB-DEMO-01:1"} {
		f.state.Sessions[id] = domain.Session{ID: id, Serial: u.selected, Kind: domain.SessionVirtual, StartApp: "com.example.app", Status: "running", Started: now.Add(time.Duration(i) * time.Second)}
	}
	f.state.Sessions["mirror"] = domain.Session{ID: "mirror", Serial: u.selected, Kind: domain.SessionMirror}
	f.state.Sessions["other"] = domain.Session{ID: "other", Serial: "other-device", Kind: domain.SessionVirtual}
	u.state = f.state
	got := u.virtualSessions(u.selected)
	if len(got) != 2 || got[0].ID != "virtual:USB-DEMO-01:2" {
		t.Fatalf("session order/filter: %+v", got)
	}
	u.stopVirtualSession(got[1].ID)
	if f.action != "stop" || f.serial != got[1].ID {
		t.Fatalf("wrong stop target: %s", f.serial)
	}
}

func TestVirtualSessionListLayout(t *testing.T) {
	f := sample()
	for _, id := range []string{"virtual:USB-DEMO-01:1", "virtual:USB-DEMO-01:2"} {
		f.state.Sessions[id] = domain.Session{ID: id, Serial: "USB-DEMO-01", Kind: domain.SessionVirtual, StartApp: "com.example.app", Status: "running", Started: time.Now()}
	}
	_, s := openUI(t, f)
	if !s.HasText("com.example.app") {
		t.Fatal("active virtual app missing from workspace")
	}
	clean(t, s)
}

func TestTabletPresetLaunchAndReturnToPhone(t *testing.T) {
	t.Setenv("VIVA_HEADLESS", "1")
	f := &virtualFixture{fixture: sample()}
	f.state.Apps = map[string]domain.AppListing{"USB-DEMO-01": {Apps: []domain.AppInfo{{Package: "com.example"}}}}
	u := New(f)
	session, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	click(t, session, "虚拟屏幕")
	click(t, session, "平板横屏")
	if u.vd.width.Text() != "1920" || u.vd.height.Text() != "1200" || u.vd.dpi.Text() != "240" {
		t.Fatal("tablet preset not applied")
	}
	clean(t, session)
	u.vd.selectedApp = "com.example"
	u.launchVirtualDialog()
	if len(f.launches) != 1 || f.launches[0].Width != 1920 || f.launches[0].Height != 1200 || f.launches[0].DPI != 240 {
		t.Fatalf("wrong tablet launch: %+v", f.launches)
	}
	u.openVirtualDialog(u.selected)
	u.selectVirtualPreset(screenTabletLandscape)
	u.selectVirtualPreset(screenPhoneLandscape)
	if u.vd.width.Text() != "2400" || u.vd.height.Text() != "1080" || u.vd.dpi.Text() != "420" {
		t.Fatal("tablet settings leaked into phone preset")
	}
	u.selectVirtualPreset(screenPhonePortrait)
	if u.vd.width.Text() != "1080" || u.vd.height.Text() != "2400" {
		t.Fatal("phone portrait not restored")
	}
}

func TestVirtualRemembersDeviceProfileAndAppAcrossUIRestart(t *testing.T) {
	t.Setenv("VIVA_HEADLESS", "1")
	f := &virtualFixture{fixture: sample()}
	f.state.Apps = map[string]domain.AppListing{"USB-DEMO-01": {Apps: []domain.AppInfo{{Package: "com.example"}}}}
	u := New(f)
	u.openVirtualDialog(u.selected)
	if !u.vd.destroy {
		t.Fatal("new virtual screen would return tasks to the phone")
	}
	u.selectVirtualPreset(screenTabletLandscape)
	u.vd.selectedApp = "com.example"
	u.launchVirtualDialog()
	if len(f.launches) != 1 || !f.launches[0].DestroyOnClose || f.launches[0].IMEPolicy != "local" || f.options[0].Keyboard != "uhid" {
		t.Fatalf("wrong tablet lifecycle/input: %+v %+v", f.launches, f.options)
	}
	reopened := New(f)
	reopened.openVirtualDialog("USB-DEMO-01")
	if reopened.vd.selectedApp != "com.example" || reopened.vd.preset != screenTabletLandscape || reopened.vd.width.Text() != "1920" || reopened.vd.dpi.Text() != "240" || !reopened.vd.destroy || reopened.vd.keyboard != "uhid" {
		t.Fatal("device profile not restored")
	}
	reopened.openVirtualDialog("192.168.1.28:37001")
	if reopened.vd.selectedApp != "" || reopened.vd.preset != screenPhonePortrait {
		t.Fatal("device preference leaked to another device")
	}
}

func TestVirtualSettingsCanEditDefaultsAndOfflineDevice(t *testing.T) {
	t.Setenv("VIVA_HEADLESS", "1")
	f := sample()
	u := New(f)
	u.section = "settings"
	u.category = "virtual"
	cfg := u.Config()
	cfg.Height = 700
	s, err := headless.Open(cfg, nil, headless.Options{Scale: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	click(t, s, "编辑默认参数")
	click(t, s, "平板横屏")
	click(t, s, "保存虚拟屏设置")
	if f.state.Settings.VirtualDefaults.Preset != "tablet" || !f.state.Settings.VirtualDefaults.DestroyOnClose || f.state.Settings.VirtualDefaults.LastApp != "" {
		t.Fatal("default profile not saved")
	}
	u.openVirtualPreferences("offline-device", true, false)
	u.vd.lastApp.SetText("com.example.offline")
	u.wake.Invalidate()
	if err := s.Driver().Pump(); err != nil {
		t.Fatal(err)
	}
	clean(t, s)
	click(t, s, "保存虚拟屏设置")
	if f.state.Settings.VirtualDevices["offline-device"].LastApp != "com.example.offline" {
		t.Fatal("offline device profile not saved")
	}
}
