package desktop

import (
	"bytes"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/headless"
)

func TestPreviewRoundedMaskPreservesSourceAndCenter(t *testing.T) {
	bitmap, ok := pngBitmap("rounded-test", portraitPreviewPNG(t), 1280, 8192, 32<<20)
	if !ok {
		t.Fatal("fixture PNG invalid")
	}
	source := append([]byte(nil), bitmap.Pix...)
	rounded := roundedPreviewBitmap(bitmap)
	if !bytes.Equal(bitmap.Pix, source) {
		t.Fatal("source bitmap mutated")
	}
	if rounded.Pix[3] != 0 || rounded.Pix[(bitmap.Width-1)*4+3] != 0 {
		t.Fatal("top corners not masked")
	}
	middle := (bitmap.Height/2)*bitmap.Stride + (bitmap.Width/2)*4
	if !bytes.Equal(source[middle:middle+4], rounded.Pix[middle:middle+4]) {
		t.Fatal("screen center changed")
	}
}

func TestPreviewIntervalSettingsSaveAndRejectInvalid(t *testing.T) {
	f := sample()
	u, s := openUI(t, f)
	s.Resize(1240, 1500)
	click(t, s, "设置")
	clean(t, s)
	u.previewInterval.SetText("301")
	f.action = ""
	click(t, s, "保存刷新频率")
	if f.action != "" || u.notice == "" {
		t.Fatal("invalid interval saved")
	}
	u.previewInterval.SetText("0")
	click(t, s, "保存刷新频率")
	if f.action != "save" || f.state.Settings.ScreenPreviewIntervalSeconds != 0 {
		t.Fatal("disabled interval not saved")
	}
	u.previewInterval.SetText("12")
	click(t, s, "保存刷新频率")
	if f.state.Settings.ScreenPreviewIntervalSeconds != 12 {
		t.Fatal("custom interval not saved")
	}
	workspaceScreenshot(t, s, "preview-refresh-settings")
}

func TestPreviewScheduleCompletionPauseIntervalAndReentry(t *testing.T) {
	var r previewRefreshState
	now := time.Unix(100, 0)
	interval := 5 * time.Second
	if !r.step(now, "phone", interval, false) {
		t.Fatal("entry must refresh immediately")
	}
	if r.step(now.Add(20*time.Second), "phone", interval, true) {
		t.Fatal("slow read queued another request")
	}
	if r.step(now.Add(21*time.Second), "phone", interval, false) {
		t.Fatal("completion should start interval")
	}
	if r.step(now.Add(25*time.Second), "phone", interval, false) {
		t.Fatal("interval measured from request instead of completion")
	}
	if !r.step(now.Add(26*time.Second), "phone", interval, false) {
		t.Fatal("periodic refresh missing")
	}
	r.step(now.Add(27*time.Second), "phone", interval, false)
	if r.step(now.Add(28*time.Second), "phone", 10*time.Second, false) {
		t.Fatal("interval change caused early request")
	}
	if !r.next.Equal(now.Add(38 * time.Second)) {
		t.Fatal("interval change not applied")
	}
	r.step(now.Add(29*time.Second), "", interval, false)
	if !r.step(now.Add(30*time.Second), "phone", 0, false) {
		t.Fatal("entry must refresh with timer disabled")
	}
	r.step(now.Add(31*time.Second), "phone", 0, false)
	if r.step(now.Add(time.Hour), "phone", 0, false) {
		t.Fatal("disabled periodic refresh dispatched")
	}
	if !r.step(now.Add(time.Hour), "other-phone", interval, false) {
		t.Fatal("device switch must refresh")
	}
}

type autoPreviewFixture struct {
	*workspaceFixture
	captures int
}

func (f *autoPreviewFixture) FetchPreview(serial string) {
	f.captures++
	f.previewSerial = serial
	f.state.Preview = domain.ScreenPreview{Serial: serial, Loading: true}
	if f.notify != nil {
		f.notify()
	}
}

func TestAutomaticPreviewOnlyOnVisibleOnlineOverview(t *testing.T) {
	f := &autoPreviewFixture{workspaceFixture: &workspaceFixture{fixture: sample()}}
	u := New(f)
	s, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if f.captures != 1 || f.previewSerial != u.selected {
		t.Fatal("initial connected device not captured")
	}
	click(t, s, "应用包")
	f.state.Preview.Loading = false
	f.notify()
	s.Driver().Pump()
	if f.captures != 1 || u.previewRefresh.timer != nil {
		t.Fatal("background refresh not paused")
	}
	click(t, s, "设备概况")
	if f.captures != 2 {
		t.Fatal("overview activation did not capture")
	}
	u.Config().OnMinimize(true)
	s.Driver().Pump()
	if u.previewRefresh.timer != nil {
		t.Fatal("minimized window retained timer")
	}
	f.state.Preview.Loading = false
	u.Config().OnMinimize(false)
	s.Driver().Pump()
	if f.captures != 3 {
		t.Fatal("restore did not capture")
	}
	f.state.Devices[0].State = "offline"
	f.notify()
	s.Driver().Pump()
	if u.previewRefresh.timer != nil {
		t.Fatal("offline device retained timer")
	}
	f.state.Preview.Loading = false
	f.state.Devices[0].State = "device"
	f.notify()
	s.Driver().Pump()
	if f.captures != 4 {
		t.Fatal("reconnected device not captured")
	}
}
