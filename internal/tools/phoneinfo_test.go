package tools

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPhonePropsCRLFAndConservativeStates(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		text := strings.Join([]string{
			"[ro.product.manufacturer]: [vivo]", "[ro.product.model]: [V2362A]",
			"[ro.product.brand]: [vivo]", "[ro.build.version.release]: [12]",
			"[ro.build.version.sdk]: [32]", "[ro.boot.verifiedbootstate]: [green]",
			"[ro.boot.slot_suffix]: [_b]", "[ro.boot.bootdevice]: [platform/soc/unknown]",
		}, newline) + newline
		var i PhoneInfo
		if !parsePhoneProps(&i, text) || i.Model != "V2362A" || i.Manufacturer != "vivo" || i.APILevel != "32" {
			t.Fatalf("newline %q: %+v", newline, i)
		}
		if i.BootloaderState != "" || i.StorageType != "" || i.Slot != "Slot B" {
			t.Fatalf("unsupported lock/storage inference: %+v", i)
		}
	}
	for _, tc := range []struct{ prop, want string }{
		{"[ro.boot.flash.locked]: [1]", "已锁定"},
		{"[ro.boot.flash.locked]: [0]", "已解锁"},
		{"[ro.boot.vbmeta.device_state]: [locked]", "已锁定"},
	} {
		var i PhoneInfo
		parsePhoneProps(&i, tc.prop)
		if !strings.HasPrefix(i.BootloaderState, tc.want) {
			t.Fatal(i.BootloaderState)
		}
	}
	if len(parseProps("warning: no properties\r\n")) != 0 {
		t.Fatal("accepted malformed properties")
	}
}

func TestPhoneStorageMountAliasesAndWrappedRows(t *testing.T) {
	for _, body := range []string{
		"/dev/block/sdc3 3030800 1379096 1478036 49% /storage/emulated/0/Android/obb\r\n",
		"/dev/mapper/userdata 3030800 1379096 1478036 49% /data\n",
		"/dev/very-long-filesystem\n 3030800 1379096 1478036 49% /data\n",
	} {
		var i PhoneInfo
		if !parsePhoneStorage(&i, "Filesystem 1K-blocks Used Available Use% Mounted on\n"+body) || i.StorageTotalBytes != 3030800*1024 || i.StorageUsedBytes != 1379096*1024 {
			t.Fatalf("body %q: %+v", body, i)
		}
	}
	for _, body := range []string{"permission denied", "/dev/a 0 0 0 0% /data", "/dev/a 100 101 0 99% /data", "/dev/a 9223372036854775807 0 0 0% /data"} {
		var i PhoneInfo
		if parsePhoneStorage(&i, body) {
			t.Fatal("accepted invalid df", body)
		}
	}
}

func TestPhoneBatteryUnitsMissingAndZero(t *testing.T) {
	i := PhoneInfo{BatteryLevel: -1}
	text := "Current Battery Service state:\r\n  level: 42\r\n  scale: 200\r\n  status: 2\r\n  health: 2\r\n  voltage: 4210\r\n  temperature: 0\r\n  Charge counter: 1234567\r\n"
	if !parsePhoneBattery(&i, text) || i.BatteryLevel != 21 || i.BatteryStatus != "充电中" || i.BatteryHealth != "良好" || *i.BatteryVoltageMV != 4210 || *i.BatteryTemperatureTenthsC != 0 || *i.BatteryChargeCounterUAH != 1234567 {
		t.Fatalf("battery %+v", i)
	}
	missing := PhoneInfo{BatteryLevel: -1}
	parsePhoneBattery(&missing, "level: 999\nscale: 0\nvoltage: 0\nCharge counter: -1")
	if missing.BatteryLevel != -1 || missing.BatteryVoltageMV != nil || missing.BatteryTemperatureTenthsC != nil || missing.BatteryChargeCounterUAH != nil {
		t.Fatal("unknown data became zero/valid", missing)
	}
}

func TestPhoneMemoryMissingAvailability(t *testing.T) {
	i := PhoneInfo{MemAvailableKB: -1}
	if !parsePhoneMemory(&i, "MemTotal: 4096 kB\r\nMemFree: 100 kB\r\n") || i.MemAvailableKB != -1 {
		t.Fatal(i)
	}
	parsePhoneMemory(&i, "MemAvailable: 0 kB\r\n")
	if i.MemAvailableKB != 0 {
		t.Fatal("zero availability must remain valid")
	}
}

func TestPhoneDisplayAndSystemEvidence(t *testing.T) {
	var i PhoneInfo
	if !parsePhoneDisplay(&i, "Physical size: 900x1600\r\nOverride size: 720x1280\r\nPhysical density: 240\r\nOverride density: 200\r\n") || i.OverrideSize != "720x1280" || i.PhysicalDPI != "240" || i.OverrideDPI != "200" {
		t.Fatal(i)
	}
	text := "__GOSCOPY_kernel__\r\n5.10.0\r\n__GOSCOPY_uptime__\r\n90061.25 40000.00\r\n__GOSCOPY_uid__\r\n2000\r\n__GOSCOPY_su__\r\n__GOSCOPY_cpu_min__\r\n300000\r\n__GOSCOPY_current__\r\n-450000\r\n"
	if !parsePhoneSystem(&i, text) || *i.UptimeSeconds != 90061 || *i.CPUFreqMinKHz != 300000 || *i.BatteryCurrentUA != -450000 || !strings.HasPrefix(i.RootStatus, "未确认") {
		t.Fatal(i)
	}
	parsePhoneSystem(&i, "__GOSCOPY_uid__\n0\n__GOSCOPY_su__\n/system/bin/su\n")
	if !strings.Contains(i.RootStatus, "未执行提权") {
		t.Fatal(i.RootStatus)
	}
	if strings.Contains(phoneSystemCommand, "su -") || strings.Contains(phoneSystemCommand, "su -c") {
		t.Fatal("root probe must not elevate")
	}
}

func TestPhoneCollectionPartialFailureAndBoundedConcurrency(t *testing.T) {
	var active, peak atomic.Int32
	start := time.Now()
	i, err := collectPhoneInfo(context.Background(), func(ctx context.Context, command string) ([]byte, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); n > p; p = peak.Load() {
			if peak.CompareAndSwap(p, n) {
				break
			}
		}
		switch command {
		case "getprop":
			return []byte("[ro.product.model]: [demo]\r\n"), nil
		case "dumpsys battery":
			return []byte("level: 50\n"), nil
		default:
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}, 25*time.Millisecond)
	if err != nil || i.Model != "demo" || i.BatteryLevel != 50 || len(i.Warnings) != 6 {
		t.Fatalf("partial results: %+v, %v", i, err)
	}
	if peak.Load() > 3 || peak.Load() < 2 {
		t.Fatal("concurrency", peak.Load())
	}
	if time.Since(start) > time.Second {
		t.Fatal("timeouts were not bounded")
	}
}

func TestPhoneCollectionAllFailuresAndCancellation(t *testing.T) {
	i, err := collectPhoneInfo(context.Background(), func(context.Context, string) ([]byte, error) {
		return nil, errors.New("offline")
	}, time.Second)
	if err == nil || len(i.Warnings) != 8 || i.BatteryLevel != -1 || i.MemAvailableKB != -1 {
		t.Fatalf("%+v %v", i, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = collectPhoneInfo(ctx, func(ctx context.Context, _ string) ([]byte, error) { return nil, ctx.Err() }, time.Second)
	if err == nil {
		t.Fatal("cancelled request succeeded")
	}
}
