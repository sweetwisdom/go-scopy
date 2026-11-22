package config

import (
	"encoding/json"
	"github.com/go-scopy/go-scopy/internal/domain"
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

func TestVirtualPreferencesPersistMigrateAndRejectInvalid(t *testing.T) {
	dir := t.TempDir()
	settings := Default()
	p := domain.DefaultVirtualPreferences()
	p.Preset = "tablet"
	p.Width, p.Height, p.DPI = 1920, 1200, 240
	p.Keyboard = "uhid"
	p.IMEPolicy = "local"
	p.LastApp = "com.example"
	settings.VirtualDevices = map[string]domain.VirtualPreferences{"phone-a": p}
	if err := Save(dir, settings); err != nil {
		t.Fatal(err)
	}
	restored, err := Load(dir)
	if err != nil || restored.VirtualOptionsFor("phone-a") != p {
		t.Fatalf("virtual preference round trip failed: %+v %v", restored, err)
	}
	if got := restored.VirtualOptionsFor("phone-b"); !got.DestroyOnClose || got.LastApp != "" {
		t.Fatal("default profile or per-device isolation incorrect")
	}
	raw, err := json.Marshal(Default())
	if err != nil {
		t.Fatal(err)
	}
	var old map[string]json.RawMessage
	if err = json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	delete(old, "virtualDisplayDefaults")
	delete(old, "virtualDisplayDevices")
	raw, _ = json.Marshal(old)
	if err = os.WriteFile(filepath.Join(dir, "settings.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	migrated, err := Load(dir)
	if err != nil || !migrated.VirtualOptionsFor("phone-a").DestroyOnClose {
		t.Fatal("old configuration did not get safe close defaults", err)
	}
	for _, modify := range []func(*domain.VirtualPreferences){func(p *domain.VirtualPreferences) { p.Width = -1 }, func(p *domain.VirtualPreferences) { p.DPI = -1 }, func(p *domain.VirtualPreferences) { p.Keyboard = "invalid" }, func(p *domain.VirtualPreferences) { p.IMEPolicy = "invalid" }} {
		bad := Default()
		modify(&bad.VirtualDefaults)
		if Validate(bad) == nil {
			t.Fatalf("invalid virtual defaults accepted: %+v", bad.VirtualDefaults)
		}
	}
}

func TestDeviceEditMovesVirtualProfileWithoutMutatingSource(t *testing.T) {
	settings := Default()
	p := domain.DefaultVirtualPreferences()
	p.LastApp = "com.example"
	settings.VirtualDevices = map[string]domain.VirtualPreferences{"old:5555": p}
	edited, err := EditDevice(settings, "old:5555", "new:5555", "Phone", true)
	if err != nil || edited.VirtualDevices["new:5555"] != p {
		t.Fatal("profile not migrated", err)
	}
	if _, ok := settings.VirtualDevices["old:5555"]; !ok {
		t.Fatal("edit changed source profile")
	}
	forgotten := ForgetDevice(edited, "new:5555")
	if len(forgotten.VirtualDevices) != 0 || len(edited.VirtualDevices) != 1 {
		t.Fatal("forget did not isolate/remove profile")
	}
}
