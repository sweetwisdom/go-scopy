package automation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
)

func pythonFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	lib := filepath.Join(root, "tools", "python", "lib")
	if err := os.MkdirAll(filepath.Join(lib, "goscrcpy"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "goscrcpy", "__init__.py"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	return root, lib
}
func fakePython(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("not executed"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPythonUsesPATHAndIgnoresOldBundledManifest(t *testing.T) {
	root, lib := pythonFixture(t)
	bin := t.TempDir()
	t.Setenv("GOSCRCPY_PYTHON", "")
	t.Setenv("PATH", bin)
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	name := "python3"
	if runtime.GOOS == "windows" {
		name = "python"
	}
	want := fakePython(t, bin, name)
	// A stale or malformed manifest must neither override PATH nor block it.
	if err := os.WriteFile(filepath.Join(root, "tools", "python", "runtime.json"), []byte("invalid old manifest"), 0600); err != nil {
		t.Fatal(err)
	}
	got, sdk, err := resolve(root, "")
	if err != nil || got != want || sdk != lib {
		t.Fatalf("got %q %q %v", got, sdk, err)
	}
	if name == "python" {
		name = "python3"
	} else {
		name = "python"
	}
	// The secondary command is used when the preferred one is absent.
	other := t.TempDir()
	want = fakePython(t, other, name)
	t.Setenv("PATH", other)
	if got, _, err = resolve(root, ""); err != nil || got != want {
		t.Fatalf("fallback: %q %v", got, err)
	}
}

func TestPythonExplicitOverridesAndMissingPATH(t *testing.T) {
	root, _ := pythonFixture(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	t.Setenv("GOSCRCPY_PYTHON", "")
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	if _, _, err := resolve(root, ""); err == nil || !strings.Contains(err.Error(), "PATH") {
		t.Fatalf("missing Python error: %v", err)
	}
	explicit := fakePython(t, t.TempDir(), "chosen-python")
	env := fakePython(t, t.TempDir(), "environment-python")
	t.Setenv("GOSCRCPY_PYTHON", env)
	if got, _, err := resolve(root, ""); err != nil || got != env {
		t.Fatalf("environment override: %q %v", got, err)
	}
	if got, _, err := resolve(root, explicit); err != nil || got != explicit {
		t.Fatalf("explicit override: %q %v", got, err)
	}
	if _, _, err := resolve(root, filepath.Join(bin, "missing-python")); err == nil {
		t.Fatal("invalid explicit path silently fell back")
	}
}

func TestPythonEnvironmentDoesNotInjectBundledDependencies(t *testing.T) {
	root, lib := pythonFixture(t)
	old := filepath.Join(root, "tools", "python", "runtime", "packages")
	if err := os.MkdirAll(old, 0755); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(t.TempDir(), "host-libs")
	t.Setenv("PYTHONPATH", host)
	t.Setenv("GOSCRCPY_PYTHON", "")
	values := map[string]string{}
	for _, v := range environment(lib, "adb", domain.ScriptRequest{}, domain.ScriptRun{}) {
		k, v, _ := strings.Cut(v, "=")
		values[k] = v
	}
	if got := values["PYTHONPATH"]; got != lib+string(os.PathListSeparator)+host || strings.Contains(got, old) {
		t.Fatalf("PYTHONPATH: %q", got)
	}
}
