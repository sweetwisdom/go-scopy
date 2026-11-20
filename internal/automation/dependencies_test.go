package automation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

// A local executable stands in for Python/pip. Never install into the host
// interpreter or contact a package index during automated tests.
func TestMain(m *testing.M) {
	dir := os.Getenv("GOSCOPY_DEPENDENCY_HELPER")
	if dir != "" {
		args := os.Args[1:]
		data, _ := json.Marshal(args)
		f, err := os.OpenFile(filepath.Join(dir, "arguments.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(90)
		}
		f.Write(append(data, '\n'))
		f.Close()
		if slices.Contains(args, "pip") {
			fmt.Println("[pip helper] installing locked packages")
			switch os.Getenv("GOSCOPY_DEPENDENCY_MODE") {
			case "slow":
				time.Sleep(30 * time.Second)
			case "fail":
				fmt.Fprintln(os.Stderr, "HASH MISMATCH (simulated)")
				os.Exit(1)
			}
			if err := os.WriteFile(filepath.Join(dir, "installed"), []byte("yes"), 0600); err != nil {
				os.Exit(91)
			}
			os.Exit(0)
		}
		if strings.Contains(strings.Join(args, " "), "GOSCOPY_DEPENDENCY_CHECK") {
			if _, err := os.Stat(filepath.Join(dir, "installed")); err != nil {
				fmt.Println("uiautomator2: 未安装")
				os.Exit(1)
			}
			fmt.Println("[environment] 锁定依赖检查通过 (helper)")
			os.Exit(0)
		}
		os.Exit(92)
	}
	os.Exit(m.Run())
}

func dependencyManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	root, lib := pythonFixture(t)
	lock := filepath.Join(filepath.Dir(lib), "requirements.lock")
	if err := os.WriteFile(lock, []byte("uiautomator2==3.7.1 \\\n    --hash=sha256:00\n"), 0600); err != nil {
		t.Fatal(err)
	}
	helper := t.TempDir()
	t.Setenv("GOSCOPY_DEPENDENCY_HELPER", helper)
	python, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := New(root, t.TempDir(), "", nil)
	t.Cleanup(m.Close)
	return m, python, helper
}

func waitDependencies(t *testing.T, m *Manager) *domain.ScriptDependencies {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		state := m.Snapshot().Dependencies
		if state != nil && state.Status != "running" && state.Status != "stopping" {
			return state
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("environment job did not finish")
	return nil
}

func TestDependenciesCheckInstallAndRecheckSelectedInterpreter(t *testing.T) {
	m, python, helper := dependencyManager(t)
	if err := m.Dependencies(python, false); err != nil {
		t.Fatal(err)
	}
	state := waitDependencies(t, m)
	if state.Ready || state.Status != "failed" || !strings.Contains(state.Log, "未安装") {
		t.Fatalf("missing dependency not reported: %+v", state)
	}
	before, _ := os.ReadFile(filepath.Join(helper, "arguments.jsonl"))
	if strings.Contains(string(before), `"pip"`) {
		t.Fatal("read-only check started installation")
	}
	if err := m.Dependencies(python, true); err != nil {
		t.Fatal(err)
	}
	state = waitDependencies(t, m)
	if !state.Ready || state.Status != "success" || state.Python != python || !strings.Contains(state.Log, "检查通过") {
		t.Fatalf("installation not verified: %+v", state)
	}
	data, err := os.ReadFile(filepath.Join(helper, "arguments.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	installs := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var args []string
		if err := json.Unmarshal([]byte(line), &args); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(args, "pip") {
			continue
		}
		installs++
		for _, flag := range []string{"-m", "pip", "install", "--require-hashes", "-r", "-i", dependencyIndex, "--no-input"} {
			if !slices.Contains(args, flag) {
				t.Fatalf("missing required flag %s: %v", flag, args)
			}
		}
		at := slices.Index(args, "-r")
		if at < 0 || !filepath.IsAbs(args[at+1]) || filepath.Base(args[at+1]) != "requirements.lock" {
			t.Fatal("requirements path depends on cwd", args)
		}
		if slices.Contains(args, "--user") || slices.Contains(args, "--break-system-packages") {
			t.Fatal("installer changed installation scope")
		}
	}
	if installs != 1 {
		t.Fatal("duplicate installation", installs)
	}
}

func TestDependenciesFailureCancellationAndMutualExclusion(t *testing.T) {
	for _, mode := range []string{"fail", "slow"} {
		t.Run(mode, func(t *testing.T) {
			m, python, _ := dependencyManager(t)
			t.Setenv("GOSCOPY_DEPENDENCY_MODE", mode)
			if err := m.Dependencies(python, true); err != nil {
				t.Fatal(err)
			}
			if mode == "slow" {
				if err := m.Dependencies(python, true); err == nil {
					t.Fatal("duplicate pip job accepted")
				}
				path := filepath.Join(t.TempDir(), "blocked.py")
				if err := os.WriteFile(path, []byte("print('no')"), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := m.Start(domain.ScriptRequest{Path: path, Serial: "device"}); err == nil {
					t.Fatal("script started during installation")
				}
				time.Sleep(80 * time.Millisecond)
				m.StopDependencies()
			}
			state := waitDependencies(t, m)
			if state.Ready {
				t.Fatal("failed/stopped installation marked ready")
			}
			if mode == "fail" && (state.Status != "failed" || !strings.Contains(state.Log, "HASH MISMATCH")) {
				t.Fatalf("pip failure hidden: %+v", state)
			}
			if mode == "slow" && state.Status != "cancelled" {
				t.Fatalf("cancellation failed: %+v", state)
			}
		})
	}
}

func TestDependenciesWillNotMutateEnvironmentOfRunningScript(t *testing.T) {
	m, python, _ := dependencyManager(t)
	m.runs = []*liveRun{{run: domain.ScriptRun{Status: "running"}, cancel: func() {}}}
	if err := m.Dependencies(python, true); err == nil {
		t.Fatal("installation accepted with active script")
	}
	m.runs = nil
	m.Close()
	if err := m.Dependencies(python, true); err == nil {
		t.Fatal("closed manager accepted installation")
	}
}

func TestDependencyPinsRejectMissingOrUnpinnedLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requirements.lock")
	if _, err := dependencyPins(path); err == nil {
		t.Fatal("missing lock accepted")
	}
	for _, data := range []string{"", "uiautomator2>=3.7.1", "uiautomator2==1.0", "uiautomator2==3.7.1\nuiautomator2==3.7.1"} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := dependencyPins(path); err == nil {
			t.Fatal("invalid lock accepted", data)
		}
	}
}
