package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackagePythonExcludesInterpreterDependenciesAndCaches(t *testing.T) {
	t.Chdir(t.TempDir())
	base := filepath.Join("resources", "python")
	for _, path := range []string{"lib/goscrcpy/__init__.py", "lib/goscrcpy/__pycache__/cache.pyc", "examples/device_info.py", "requirements.in", "requirements.lock", "README.md", "runtime.json", "runtime/python.exe", "runtime/packages/uiautomator2/__init__.py"} {
		path = filepath.Join(base, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dest := filepath.Join("out", "tools", "python")
	if err := packagePython(dest); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"lib/goscrcpy/__init__.py", "examples/device_info.py", "requirements.in", "requirements.lock", "README.md"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(path))); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"runtime", "runtime.json", "lib/goscrcpy/__pycache__"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(path))); !os.IsNotExist(err) {
			t.Fatalf("unwanted package content %s: %v", path, err)
		}
	}
}
