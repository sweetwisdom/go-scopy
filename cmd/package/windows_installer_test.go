package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsInstallerIncludesAllPayloadAndSafeUninstall(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Go-Scopy.exe", "Go-Scopy.ico", "tools/scrcpy/adb.exe", "tools/python/DEVELOPMENT.zh-CN.md", "docs/中文文档.md", "licenses/MIT.txt", "README.md", "file$dollar.txt"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	b, err := windowsInstallerScript(root, "0.1.8", filepath.Join(t.TempDir(), "setup.exe"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, text := range []string{`RequestExecutionLevel user`, `Go-Scopy.exe`, `File "/oname=adb.exe"`, `File "/oname=DEVELOPMENT.zh-CN.md"`, `Delete "$INSTDIR\tools\scrcpy\adb.exe"`, `RMDir "$INSTDIR\tools\scrcpy"`, `RMDir "$INSTDIR\tools"`, `file$$dollar.txt`} {
		if !strings.Contains(s, text) {
			t.Fatal("missing", text)
		}
	}
	if strings.Contains(strings.ToLower(s), "rmdir /r") {
		t.Fatal("recursive uninstall could delete user data")
	}
	if strings.Index(s, `RMDir "$INSTDIR\tools\scrcpy"`) > strings.Index(s, `RMDir "$INSTDIR\tools"`) {
		t.Fatal("parent removed before child")
	}
}
