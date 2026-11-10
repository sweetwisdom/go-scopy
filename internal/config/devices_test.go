package config

import (
	"strings"
	"testing"
)

func TestDeviceRecordsCRUDAndPersistence(t *testing.T) {
	original := Default()
	created, err := EditDevice(original, "", "10.168.2.20:5555", "我的手机", true)
	if err != nil || !created.SavedWireless["10.168.2.20:5555"] || len(original.DeviceNames) != 0 {
		t.Fatalf("create mutated original or failed: %v", err)
	}
	created.Devices["10.168.2.20:5555"] = created.Defaults
	updated, err := EditDevice(created, "10.168.2.20:5555", "10.168.2.21:5555", "工作手机", true)
	if err != nil || updated.DeviceNames["10.168.2.21:5555"] != "工作手机" || updated.SavedWireless["10.168.2.20:5555"] || len(updated.Devices) != 1 {
		t.Fatalf("edit: %+v %v", updated, err)
	}
	if !created.SavedWireless["10.168.2.20:5555"] {
		t.Fatal("editing mutated previous snapshot")
	}
	dir := t.TempDir()
	if err := Save(dir, updated); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil || loaded.DeviceNames["10.168.2.21:5555"] != "工作手机" || !loaded.SavedWireless["10.168.2.21:5555"] {
		t.Fatalf("record lost on reload: %+v %v", loaded, err)
	}
	deleted := ForgetDevice(loaded, "10.168.2.21:5555")
	if len(deleted.DeviceNames)+len(deleted.SavedWireless)+len(deleted.Devices) != 0 || len(loaded.SavedWireless) != 1 {
		t.Fatal("delete failed or mutated original")
	}
}

func TestDeviceRecordsRejectInvalidAndDuplicateInput(t *testing.T) {
	s := Default()
	for _, address := range []string{"", "10.1.1.1", "host:0", "host:65536", "ho st:5555"} {
		if _, err := EditDevice(s, "", address, "手机", true); err == nil {
			t.Fatalf("invalid address accepted: %q", address)
		}
	}
	for _, name := range []string{strings.Repeat("机", 41), "手机\n二号"} {
		if _, err := EditDevice(s, "USB123", "", name, false); err == nil {
			t.Fatal("invalid name accepted")
		}
	}
	s, _ = EditDevice(s, "", "10.1.1.1:5555", "一号", true)
	if _, err := EditDevice(s, "", "10.1.1.1:5555", "覆盖", true); err == nil {
		t.Fatal("duplicate replaced saved record")
	}
	s, err := EditDevice(s, "USB123", "", "USB 手机", false)
	if err != nil || s.DeviceNames["USB123"] != "USB 手机" || s.SavedWireless["USB123"] {
		t.Fatal("USB rename changed transport")
	}
	s, err = EditDevice(s, "USB123", "", "", false)
	if err != nil || s.DeviceNames["USB123"] != "" {
		t.Fatal("reset name failed")
	}
}
