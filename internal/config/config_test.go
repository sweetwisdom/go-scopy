package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveRoundTripAndRejectedWritePreservesFile(t *testing.T) {
	dir := t.TempDir()
	s := Default()
	s.OutputDir = filepath.Join(dir, "截图 与录像")
	s.Devices["serial:5555"] = s.Defaults
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	s.Theme = "dark"
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || got.Theme != "dark" || got.OutputDir != s.OutputDir || len(got.Devices) != 1 {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	previous, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	s.Defaults.FPS = 0
	if err := Save(dir, s); err == nil {
		t.Fatal("invalid FPS accepted")
	}
	now, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if string(previous) != string(now) {
		t.Fatal("invalid write changed saved settings")
	}
}

func TestCorruptConfigIsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte("{broken"), 0600)
	if _, err := Load(dir); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "{broken" {
		t.Fatal("configuration was overwritten")
	}
}

func TestAppExportDirectoryPersistsIndependently(t *testing.T) {
	dir := t.TempDir()
	settings := Default()
	settings.AppExportDir = filepath.Join(dir, "应用导出")
	if err := Save(dir, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil || loaded.AppExportDir != settings.AppExportDir || loaded.OutputDir != settings.OutputDir {
		t.Fatalf("export directory did not round trip: %+v %v", loaded, err)
	}
	loaded.AppExportDir = "relative/path"
	if err := Validate(loaded); err == nil {
		t.Fatal("relative application export directory accepted")
	}
	loaded.AppExportDir = ""
	if err := Validate(loaded); err != nil {
		t.Fatal("old settings with no export directory must remain valid", err)
	}
}
