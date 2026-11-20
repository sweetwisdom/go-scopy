package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"
)

type appExportFixture struct {
	*fixture
	calls       int
	packages    []string
	destination string
	single      bool
}

func (f *appExportFixture) ExportApps(serial string, packages []string, destination string, single bool) {
	f.calls++
	f.serial = serial
	f.packages = append([]string(nil), packages...)
	f.destination, f.single = destination, single
}

func openExportUI(t *testing.T, packages []string) (*UI, *appExportFixture, *headless.Session) {
	t.Helper()
	f := &appExportFixture{fixture: sample()}
	f.state.Apps = map[string]domain.AppListing{"USB-DEMO-01": {Apps: []domain.AppInfo{{Package: "com.demo", Label: "Demo", VersionName: "1.0"}, {Package: "com.other", Label: "Other"}}}}
	u := New(f)
	u.deviceTab = "packages"
	u.apps.selected = map[string]bool{}
	for _, pkg := range packages {
		u.apps.selected[pkg] = true
	}
	s, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return u, f, s
}

func TestAppExportSinglePickerAndExactDestination(t *testing.T) {
	u, f, s := openExportUI(t, []string{"com.demo"})
	dest := filepath.Join(t.TempDir(), "自己指定.apk")
	if err := s.Driver().QueueSaveFile(dest, nil); err != nil {
		t.Fatal(err)
	}
	click(t, s, "导出")
	if err := s.Driver().Pump(); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || !f.single || f.destination != dest || len(f.packages) != 1 || f.packages[0] != "com.demo" {
		t.Fatalf("wrong export request %+v", f)
	}
	request, ok := s.Driver().LastSaveFileRequest()
	if !ok || request.SuggestedName != "Demo-1.0.apk" || request.DefaultExtension != "apk" {
		t.Fatalf("wrong SaveFile request %+v", request)
	}
	if len(u.apps.selected) != 0 {
		t.Fatal("submitted selection not cleared")
	}
}

func TestAppExportBatchPickerIncludesHiddenSelection(t *testing.T) {
	u, f, s := openExportUI(t, []string{"com.other", "com.demo"})
	u.apps.search.SetText("Demo")
	dest := t.TempDir()
	if err := s.Driver().QueueOpenDirectory(dest, nil); err != nil {
		t.Fatal(err)
	}
	click(t, s, "导出")
	if err := s.Driver().Pump(); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || f.single || f.destination != dest || strings.Join(f.packages, ",") != "com.demo,com.other" {
		t.Fatalf("batch snapshot incorrect %+v", f)
	}
	if _, ok := s.Driver().LastOpenDirectoryRequest(); !ok {
		t.Fatal("directory picker not opened")
	}
}

func TestAppExportCancelOrDialogFailureDoesNotSubmit(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, dialogErr := range []error{app.ErrCanceled, errors.New("picker unavailable")} {
			t.Run(fmt.Sprintf("batch-%v-%v", batch, dialogErr), func(t *testing.T) {
				packages := []string{"com.demo"}
				if batch {
					packages = append(packages, "com.other")
				}
				u, f, s := openExportUI(t, packages)
				var err error
				if batch {
					err = s.Driver().QueueOpenDirectory("", dialogErr)
				} else {
					err = s.Driver().QueueSaveFile("", dialogErr)
				}
				if err != nil {
					t.Fatal(err)
				}
				click(t, s, "导出")
				if err := s.Driver().Pump(); err != nil {
					t.Fatal(err)
				}
				if f.calls != 0 || len(u.apps.selected) != len(packages) {
					t.Fatal("cancel/failure submitted or lost selection")
				}
				if errors.Is(dialogErr, app.ErrCanceled) && strings.Contains(u.notice, "失败") {
					t.Fatal("cancel shown as error")
				}
			})
		}
	}
}

func TestAppExportProgressLayouts(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, width := range []int{1000, 1240, 1600} {
			for _, known := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s-%d-known-%v", theme, width, known), func(t *testing.T) {
					f := sample()
					f.state.Settings.Theme = theme
					totalBytes := int64(4096)
					if !known {
						totalBytes = 0
					}
					f.state.Tasks = []domain.Task{
						{ID: 1, Title: "导出安装包", Status: "running", Export: domain.AppExportProgress{Serial: "USB-DEMO-01", Current: "com.example.demo", Total: 3, Completed: 1, Bytes: 2048, TotalBytes: totalBytes}},
						{ID: 2, Title: "导出安装包", Status: "error", Detail: "已导出 1/2 个应用；com.other：设备断开", Path: filepath.Join(t.TempDir(), "应用.apk"), Export: domain.AppExportProgress{Serial: "USB-DEMO-01", Total: 2, Completed: 2, Failed: 1}},
					}
					u := New(f)
					u.deviceTab = "packages"
					cfg := u.Config()
					cfg.Width, cfg.Height = width, 1100
					s, err := headless.Open(cfg, nil, headless.Options{Scale: 1.5})
					if err != nil {
						t.Fatal(err)
					}
					defer s.Close()
					clean(t, s)
					texts := strings.Join(s.Texts(), "\n")
					want := "传输 50%"
					if !known {
						want = "总大小未知"
					}
					if !strings.Contains(texts, want) || !strings.Contains(texts, "1/3 已处理") || !strings.Contains(texts, "设备断开") {
						t.Fatalf("missing progress state: %s", texts)
					}
					if _, ok := s.Hit("打开保存目录"); !ok {
						t.Fatal("output directory action unavailable")
					}
					if dir := os.Getenv("GOSCOPY_ARTIFACT_DIR"); dir != "" && width == 1240 && known {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						if err := s.Screenshot(filepath.Join(dir, "export-progress-"+theme+".png")); err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		}
	}
}

func TestAppExportRowMenuUsesPicker(t *testing.T) {
	u, f, s := openExportUI(t, nil)
	u.apps.menuFor = "com.demo"
	u.wake.Invalidate()
	if err := s.Driver().Pump(); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "menu.apk")
	if err := s.Driver().QueueSaveFile(dest, nil); err != nil {
		t.Fatal(err)
	}
	click(t, s, "导出安装包")
	if err := s.Driver().Pump(); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || !f.single || f.destination != dest || len(f.packages) != 1 || f.packages[0] != "com.demo" {
		t.Fatalf("row menu bypassed picker: %+v", f)
	}
}

func TestAppExportPendingPickerCannotExportSwitchedDevice(t *testing.T) {
	u, f, s := openExportUI(t, []string{"com.demo"})
	u.requestAppExport("USB-DEMO-01", []string{"com.demo"})
	u.selected = "192.168.1.28:37001"
	if err := s.Driver().QueueSaveFile(filepath.Join(t.TempDir(), "switch.apk"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Driver().Pump(); err != nil {
		t.Fatal(err)
	}
	if f.calls != 0 || !strings.Contains(u.notice, "设备已切换") {
		t.Fatal("stale export request submitted")
	}
}

func TestAppExportRemembersDirectoryAndRetiresOldCards(t *testing.T) {
	u, f, s := openExportUI(t, []string{"com.demo"})
	f.state.Tasks = []domain.Task{
		{ID: 10, Status: "done", Export: domain.AppExportProgress{Serial: u.selected, Total: 1}},
		{ID: 11, Status: "running", Export: domain.AppExportProgress{Serial: u.selected, Total: 1}},
		{ID: 12, Status: "done", Export: domain.AppExportProgress{Serial: "other-device", Total: 1}},
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, "Demo.apk")
	if err := s.Driver().QueueSaveFile(dest, nil); err != nil {
		t.Fatal(err)
	}
	click(t, s, "导出")
	if err := s.Driver().Pump(); err != nil {
		t.Fatal(err)
	}
	if f.state.Settings.AppExportDir != dir || u.appExportDir.Text() != dir {
		t.Fatal("last selected directory not saved")
	}
	if !u.apps.exportDismissed[10] || !u.apps.exportDismissed[11] || u.apps.exportDismissed[12] {
		t.Fatal("previous results not retired for the correct device")
	}
	texts := strings.Join(s.Texts(), "\n")
	if strings.Contains(texts, "安装包导出 · 已完成") || !strings.Contains(texts, "安装包导出 · 进行中") {
		t.Fatal("old result should hide without hiding active transfer", texts)
	}
}

func TestAppExportCancelledSelectionPreservesDirectoryAndCards(t *testing.T) {
	u, f, s := openExportUI(t, []string{"com.demo"})
	previous := t.TempDir()
	f.state.Settings.AppExportDir = previous
	f.state.Tasks = []domain.Task{{ID: 10, Status: "done", Export: domain.AppExportProgress{Serial: u.selected, Total: 1}}}
	if err := s.Driver().QueueSaveFile("", app.ErrCanceled); err != nil {
		t.Fatal(err)
	}
	click(t, s, "导出")
	if err := s.Driver().Pump(); err != nil {
		t.Fatal(err)
	}
	if f.calls != 0 || f.state.Settings.AppExportDir != previous || u.apps.exportDismissed[10] {
		t.Fatal("cancel changed directory or removed previous result")
	}
}
