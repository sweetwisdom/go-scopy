package automation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

// These checks execute only local Python code, never connect to an Android
// device and skip when the selected host Python lacks the locked dependency.
func testManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	python, lib, err := resolve(root, "")
	if err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-c", `import importlib.metadata as m; assert m.version("uiautomator2")=="3.7.1"`)
	cmd.Env = environment(lib, "adb", domain.ScriptRequest{}, domain.ScriptRun{})
	if err := cmd.Run(); err != nil {
		t.Skip("host Python needs uiautomator2==3.7.1 for integration tests:", err)
	}
	data := t.TempDir()
	m := New(root, data, "adb", nil)
	t.Cleanup(m.Close)
	return m, root, data
}
func waitScript(t *testing.T, m *Manager, id string, ready bool) domain.ScriptRun {
	t.Helper()
	end := time.Now().Add(8 * time.Second)
	for time.Now().Before(end) {
		for _, r := range m.Snapshot().Runs {
			if r.ID == id && ((!ready && !r.Active()) || (ready && strings.Contains(r.Log, "READY"))) {
				return r
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("script did not reach expected state: %+v", m.Snapshot())
	return domain.ScriptRun{}
}
func TestScriptSuccessAndHistory(t *testing.T) {
	m, root, data := testManager(t)
	path := filepath.Join(t.TempDir(), "脚本 with spaces.py")
	code := "import os,sys,goscrcpy\nassert goscrcpy.device_ids()==['TEST-DEVICE']\nassert sys.argv[1]=='argument with spaces'\ngoscrcpy.output_path('result.txt').write_text('ok',encoding='utf-8')\nprint('中文日志',flush=True)\n"
	if err := os.WriteFile(path, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := m.Start(domain.ScriptRequest{Path: path, Serial: "TEST-DEVICE", TimeoutSeconds: 10, Args: []string{"argument with spaces"}})
	if err != nil {
		t.Fatal(err)
	}
	r := waitScript(t, m, id, false)
	if r.Status != "success" || r.ExitCode != 0 || !strings.Contains(r.Log, "中文日志") {
		t.Fatalf("unexpected result: %+v", r)
	}
	m.Close()
	restored := New(root, data, "adb", nil)
	defer restored.Close()
	state := restored.Snapshot()
	if len(state.Runs) != 1 || state.Runs[0].ID != id || state.Runs[0].Status != "success" {
		t.Fatalf("history lost: %+v", state)
	}
	if _, err = os.Stat(filepath.Join(r.OutputDir, "result.txt")); err != nil {
		t.Fatal(err)
	}
}
func TestScriptCancelAndDeviceExclusion(t *testing.T) {
	m, _, _ := testManager(t)
	path := filepath.Join(t.TempDir(), "wait.py")
	if err := os.WriteFile(path, []byte("import time\nprint('READY',flush=True)\ntime.sleep(30)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	req := domain.ScriptRequest{Path: path, Serial: "TEST-DEVICE", TimeoutSeconds: 10}
	id, err := m.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	waitScript(t, m, id, true)
	if _, err = m.Start(req); err == nil {
		t.Fatal("allowed duplicate script on same device")
	}
	m.Stop(id)
	if r := waitScript(t, m, id, false); r.Status != "cancelled" {
		t.Fatalf("unexpected status: %+v", r)
	}
}
func TestScriptTimeout(t *testing.T) {
	m, _, _ := testManager(t)
	path := filepath.Join(t.TempDir(), "timeout.py")
	if err := os.WriteFile(path, []byte("import time\ntime.sleep(30)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := m.Start(domain.ScriptRequest{Path: path, Serial: "TEST-DEVICE", TimeoutSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	if r := waitScript(t, m, id, false); r.Status != "timeout" {
		t.Fatalf("unexpected status: %+v", r)
	}
}

// This uses PATH's real interpreter, but never installs packages or contacts a device.
func TestDefaultPythonReportsMissingDependencies(t *testing.T) {
	t.Setenv("GOSCRCPY_PYTHON", "")
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	python, lib, err := resolve(root, "")
	if err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-c", `import importlib.metadata as m; m.version("uiautomator2")`)
	cmd.Env = environment(lib, "adb", domain.ScriptRequest{}, domain.ScriptRun{})
	if err := cmd.Run(); err == nil {
		t.Skip("PATH Python already has uiautomator2")
	}
	m := New(root, t.TempDir(), "adb", nil)
	t.Cleanup(m.Close)
	path := filepath.Join(t.TempDir(), "no-device.py")
	if err := os.WriteFile(path, []byte("raise AssertionError('script should not run without dependencies')"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := m.Start(domain.ScriptRequest{Path: path, Serial: "TEST-DEVICE", TimeoutSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	r := waitScript(t, m, id, false)
	if r.Status != "failed" || !strings.Contains(r.Log, python) || !strings.Contains(r.Log, "当前 Python 未安装 uiautomator2") {
		t.Fatalf("missing dependency feedback: %+v", r)
	}
}
