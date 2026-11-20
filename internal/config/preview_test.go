package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewIntervalDefaultMigrationAndValidation(t *testing.T) {
	dir := t.TempDir()
	settings := Default()
	data, _ := json.Marshal(settings)
	var legacy map[string]any
	json.Unmarshal(data, &legacy)
	delete(legacy, "screenPreviewIntervalSeconds")
	data, _ = json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || got.ScreenPreviewIntervalSeconds != 5 {
		t.Fatal("old config missing default", got, err)
	}
	for _, seconds := range []int{0, 1, 5, 300} {
		settings.ScreenPreviewIntervalSeconds = seconds
		if err := Save(dir, settings); err != nil {
			t.Fatal(err)
		}
		got, err := Load(dir)
		if err != nil || got.ScreenPreviewIntervalSeconds != seconds {
			t.Fatal("interval did not persist", seconds, err)
		}
	}
	for _, seconds := range []int{-1, 301} {
		settings.ScreenPreviewIntervalSeconds = seconds
		if Validate(settings) == nil {
			t.Fatal("invalid interval accepted", seconds)
		}
	}
}
