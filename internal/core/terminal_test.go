package core

import (
	"path/filepath"
	"testing"
)

func TestTerminalStartupErrorReturnsNilInterface(t *testing.T) {
	for _, logcat := range []bool{false, true} {
		s := testService(t)
		s.tool.ADB = filepath.Join(t.TempDir(), "missing-adb.exe")
		open := s.OpenShell
		if logcat {
			open = s.OpenLogcat
		}
		stream, err := open("test-serial", 80, 24, nil)
		if err == nil || stream != nil {
			t.Fatalf("startup failure must return a nil interface: logcat=%v stream=%#v err=%v", logcat, stream, err)
		}
	}
}
