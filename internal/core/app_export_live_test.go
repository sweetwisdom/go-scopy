package core

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools"
)

// Opt-in APK download; reads the device only and writes to an explicitly selected local artifact directory.
func TestAppExportLiveReadOnly(t *testing.T) {
	serial := os.Getenv("GOSCOPY_APP_EXPORT_LIVE_SERIAL")
	if serial == "" {
		t.Skip("set GOSCOPY_APP_EXPORT_LIVE_SERIAL for a read-only real-device APK download")
	}
	adb, pkg := os.Getenv("GOSCOPY_ADB"), os.Getenv("GOSCOPY_APP_EXPORT_LIVE_PACKAGE")
	if adb == "" || pkg == "" {
		t.Fatal("select GOSCOPY_ADB and GOSCOPY_APP_EXPORT_LIVE_PACKAGE explicitly")
	}
	dir := t.TempDir()
	if artifacts := os.Getenv("GOSCOPY_ARTIFACT_DIR"); artifacts != "" {
		dir = filepath.Join(artifacts, "live-"+time.Now().Format("20060102-150405"))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{ctx: ctx, cancel: cancel, tool: tools.Tools{ADB: adb}, state: domain.State{Devices: []domain.Device{{Serial: serial, State: "device"}}}}
	defer s.Close()
	dest := filepath.Join(dir, "selected-app.apk")
	s.ExportApps(serial, []string{pkg}, dest, true)
	deadline := time.Now().Add(60 * time.Second)
	var task domain.Task
	for time.Now().Before(deadline) {
		task = s.State().Tasks[0]
		if task.Status != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if task.Status != "done" || task.Path != dest {
		t.Fatalf("live export failed %+v", task)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= 0 || task.Export.Bytes != info.Size() {
		t.Fatal("export size/progress mismatch", task.Export, info.Size())
	}
	archive, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatal("exported APK is not a ZIP", err)
	}
	defer archive.Close()
	manifest := false
	for _, f := range archive.File {
		if f.Name == "AndroidManifest.xml" {
			manifest = true
			break
		}
	}
	if !manifest {
		t.Fatal("APK missing AndroidManifest.xml")
	}
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "export-result.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("read-only export %s: %d bytes, progress total=%d; output=%s", pkg, info.Size(), task.Export.TotalBytes, dest)
}
