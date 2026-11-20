package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

// A real executable simulates slow adb writes so progress, failure and cleanup are exercised.
func runExportHelper(args []string) {
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "shell pm path "):
		pkg := strings.TrimPrefix(args[len(args)-1], "pm path ")
		pkg = strings.Trim(pkg, "'")
		fmt.Printf("package:/data/app/%s/split_config.apk\npackage:/data/app/%s/base.apk\n", pkg, pkg)
	case strings.Contains(joined, "shell stat -c %s "):
		if os.Getenv("GOSCOPY_EXPORT_HELPER") == "unknown" {
			os.Exit(1)
		}
		fmt.Println(32768)
	case strings.Contains(joined, " pull "):
		remote, local := args[len(args)-2], args[len(args)-1]
		if !strings.HasSuffix(remote, "/base.apk") {
			fmt.Fprintln(os.Stderr, "wrong APK selected")
			os.Exit(1)
		}
		f, err := os.Create(local)
		if err != nil {
			os.Exit(1)
		}
		for n := 0; n < 4; n++ {
			if _, err := f.Write(bytes.Repeat([]byte("x"), 8192)); err != nil {
				os.Exit(1)
			}
			time.Sleep(90 * time.Millisecond)
			if strings.Contains(remote, "com.fail") || os.Getenv("GOSCOPY_EXPORT_HELPER") == "fail" {
				f.Close()
				fmt.Fprintln(os.Stderr, "device disconnected")
				os.Exit(1)
			}
			if os.Getenv("GOSCOPY_EXPORT_HELPER") == "short" {
				break
			}
		}
		f.Close()
	default:
		fmt.Fprintln(os.Stderr, "unexpected adb call", joined)
		os.Exit(1)
	}
	os.Exit(0)
}

func exportService(t *testing.T) *Service {
	t.Helper()
	s := testService(t)
	t.Setenv("GOSCOPY_EXPORT_HELPER", "ok")
	s.state.Apps = map[string]domain.AppListing{"test-serial": {Apps: []domain.AppInfo{{Package: "com.demo", Label: "演示应用", VersionName: "1.0"}}}}
	return s
}

func TestExportSingleUsesExactPathAndReportsRealProgress(t *testing.T) {
	s := exportService(t)
	dest := filepath.Join(t.TempDir(), "自选 位置.apk")
	if err := os.WriteFile(dest, []byte("previous complete APK"), 0600); err != nil {
		t.Fatal(err)
	}
	s.ExportApps("test-serial", []string{"com.demo"}, dest, true)
	observed := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task := s.State().Tasks[0]
		if task.Status != "running" {
			break
		}
		p := task.Export
		if p.Bytes > 0 && p.Bytes < p.TotalBytes {
			observed = true
			data, err := os.ReadFile(dest)
			if err != nil || string(data) != "previous complete APK" {
				t.Fatal("existing file changed before successful transfer")
			}
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !observed {
		t.Fatal("no intermediate real-byte progress reported")
	}
	task := taskResult(t, s)
	if task.Status != "done" || task.Path != dest || task.Export.Completed != 1 || task.Export.Bytes != 32768 {
		t.Fatalf("unexpected result %+v", task)
	}
	data, err := os.ReadFile(dest)
	if err != nil || len(data) != 32768 {
		t.Fatal("exact destination not exported", err)
	}
	assertNoExportTemps(t, filepath.Dir(dest))
}

func TestExportBatchDoesNotOverwriteAndContinuesAfterFailure(t *testing.T) {
	s := exportService(t)
	dir := t.TempDir()
	existing := filepath.Join(dir, "演示应用-1.0.apk")
	os.WriteFile(existing, []byte("keep"), 0600)
	s.ExportApps("test-serial", []string{"com.demo", "com.fail", "com.other"}, dir, false)
	task := taskResult(t, s)
	if task.Status != "error" || task.Export.Completed != 3 || task.Export.Failed != 1 || !strings.Contains(task.Detail, "2/3") {
		t.Fatalf("batch result %+v", task)
	}
	for _, name := range []string{"演示应用-1.0 (1).apk", "com.other.apk"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Size() != 32768 {
			t.Fatal("missing completed file", name, err)
		}
	}
	data, _ := os.ReadFile(existing)
	if string(data) != "keep" {
		t.Fatal("batch overwrote existing file")
	}
	if _, err := os.Stat(filepath.Join(dir, "com.fail.apk")); !os.IsNotExist(err) {
		t.Fatal("failed transfer left a final APK")
	}
	assertNoExportTemps(t, dir)
}

func TestExportFailedOrShortTransferKeepsPreviousFile(t *testing.T) {
	for _, mode := range []string{"fail", "short"} {
		t.Run(mode, func(t *testing.T) {
			s := exportService(t)
			t.Setenv("GOSCOPY_EXPORT_HELPER", mode)
			dest := filepath.Join(t.TempDir(), "existing.apk")
			os.WriteFile(dest, []byte("keep"), 0600)
			s.ExportApps("test-serial", []string{"com.demo"}, dest, true)
			task := taskResult(t, s)
			data, _ := os.ReadFile(dest)
			if task.Status != "error" || string(data) != "keep" {
				t.Fatalf("failed export corrupted previous file: %+v", task)
			}
			assertNoExportTemps(t, filepath.Dir(dest))
		})
	}
}

func TestExportUnknownSizeAndCancelledTransfer(t *testing.T) {
	s := exportService(t)
	t.Setenv("GOSCOPY_EXPORT_HELPER", "unknown")
	dest := filepath.Join(t.TempDir(), "unknown.apk")
	s.ExportApps("test-serial", []string{"com.demo"}, dest, true)
	task := taskResult(t, s)
	if task.Status != "done" || task.Export.TotalBytes != 0 || task.Export.Bytes != 32768 {
		t.Fatalf("unknown size %+v", task)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.exportAPK(ctx, "test-serial", "com.demo", filepath.Join(filepath.Dir(dest), "cancelled.apk"), true, nil); err == nil {
		t.Fatal("cancelled export succeeded")
	}
	assertNoExportTemps(t, filepath.Dir(dest))
}

func assertNoExportTemps(t *testing.T, dir string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, ".goscopy-export-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary files not cleaned: %v %v", files, err)
	}
}

func TestExportCancelledMidTransferKeepsDestination(t *testing.T) {
	s := exportService(t)
	dest := filepath.Join(t.TempDir(), "cancelled.apk")
	os.WriteFile(dest, []byte("keep"), 0600)
	s.ExportApps("test-serial", []string{"com.demo"}, dest, true)
	deadline := time.Now().Add(5 * time.Second)
	observed := false
	for time.Now().Before(deadline) {
		task := s.State().Tasks[0]
		if task.Status != "running" {
			break
		}
		if task.Export.Bytes > 0 && task.Export.Bytes < task.Export.TotalBytes {
			observed = true
			s.cancel()
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !observed {
		t.Fatal("did not observe transfer before cancellation")
	}
	task := taskResult(t, s)
	data, _ := os.ReadFile(dest)
	if task.Status != "error" || string(data) != "keep" {
		t.Fatalf("cancel did not preserve destination: %+v", task)
	}
	assertNoExportTemps(t, filepath.Dir(dest))
}

func TestExportBatchSameAppNamesGetDistinctFiles(t *testing.T) {
	s := exportService(t)
	listing := s.state.Apps["test-serial"]
	listing.Apps = append(listing.Apps, domain.AppInfo{Package: "com.other", Label: "演示应用", VersionName: "1.0"})
	s.state.Apps["test-serial"] = listing
	dir := t.TempDir()
	s.ExportApps("test-serial", []string{"com.demo", "com.other"}, dir, false)
	task := taskResult(t, s)
	if task.Status != "done" || task.Export.Completed != 2 {
		t.Fatalf("same labels failed: %+v", task)
	}
	for _, name := range []string{"演示应用-1.0.apk", "演示应用-1.0 (1).apk"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err != nil || info.Size() != 32768 {
			t.Fatal("same-name collision", name, err)
		}
	}
}
