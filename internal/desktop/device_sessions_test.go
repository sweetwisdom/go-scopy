package desktop

import (
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

type virtualFixture struct {
	*fixture
	launches []domain.VirtualDisplay
}

func (f *virtualFixture) StartVirtual(serial string, o domain.Options, d domain.VirtualDisplay, record bool) {
	f.serial = serial
	f.launches = append(f.launches, d)
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
