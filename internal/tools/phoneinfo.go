package tools

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

var (
	propLine   = regexp.MustCompile(`^\[([^\]]+)]: \[(.*)]$`)
	battLevel  = regexp.MustCompile(`(?m)^\s*level:\s*(\d+)`)
	battStatus = regexp.MustCompile(`(?m)^\s*status:\s*(\d+)`)
	inetLine   = regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+)/`)
	ssidLine   = regexp.MustCompile(`SSID: "?([^",\n]+)"?`)
)

// PhoneInfo collects the overview fields in a handful of batched shell calls.
// Fields that a device does not report are left empty rather than failing the
// whole page.
func (t Tools) PhoneInfo(ctx context.Context, serial string) (PhoneInfo, error) {
	info := PhoneInfo{BatteryLevel: -1, BatteryStatus: "未知"}
	propOut, err := t.Device(ctx, serial, "shell", "getprop")
	if err != nil {
		return info, err
	}
	props := parseProps(string(propOut))
	info.Manufacturer = props["ro.product.manufacturer"]
	info.Model = props["ro.product.model"]
	info.AndroidVersion = props["ro.build.version.release"]
	info.APILevel = props["ro.build.version.sdk"]

	// Battery, memory, storage and network are best effort: one failure each
	// must not blank the other cards.
	if out, err := t.Device(ctx, serial, "shell", "dumpsys battery"); err == nil {
		text := string(out)
		if m := battLevel.FindStringSubmatch(text); m != nil {
			info.BatteryLevel, _ = strconv.Atoi(m[1])
		}
		if m := battStatus.FindStringSubmatch(text); m != nil {
			switch m[1] {
			case "2":
				info.BatteryStatus = "充电中"
			case "5":
				info.BatteryStatus = "已充满"
			case "3":
				info.BatteryStatus = "放电中"
			case "4":
				info.BatteryStatus = "未充电"
			}
		}
	}
	if out, err := t.Device(ctx, serial, "shell", "cat /proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			fields := strings.Fields(value)
			if len(fields) == 0 {
				continue
			}
			n, _ := strconv.ParseInt(fields[0], 10, 64)
			switch strings.TrimSpace(key) {
			case "MemTotal":
				info.MemTotalKB = n
			case "MemAvailable":
				info.MemAvailableKB = n
			}
		}
	}
	if out, err := t.Device(ctx, serial, "shell", "df -k /data"); err == nil {
		// toybox: "Filesystem 1K-blocks Used Available Use% Mounted on"
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 5 || fields[0] == "Filesystem" {
				continue
			}
			total, e1 := strconv.ParseInt(fields[1], 10, 64)
			used, e2 := strconv.ParseInt(fields[2], 10, 64)
			if e1 == nil && e2 == nil && strings.Contains(line, "/data") {
				info.StorageTotalBytes, info.StorageUsedBytes = total*1024, used*1024
				break
			}
		}
	}
	if out, err := t.Device(ctx, serial, "shell", "ip -f inet addr show wlan0"); err == nil {
		if m := inetLine.FindStringSubmatch(string(out)); m != nil {
			info.IPAddress = m[1]
		}
	}
	if info.IPAddress == "" {
		if out, err := t.Device(ctx, serial, "shell", "getprop dhcp.wlan0.ipaddress"); err == nil {
			info.IPAddress = strings.TrimSpace(string(out))
		}
	}
	if out, err := t.Device(ctx, serial, "shell", "cmd wifi status"); err == nil {
		if m := ssidLine.FindStringSubmatch(string(out)); m != nil {
			info.WiFiSSID = strings.TrimSpace(m[1])
		}
	}
	return info, nil
}

func parseProps(text string) map[string]string {
	props := make(map[string]string, 256)
	for _, line := range strings.Split(text, "\n") {
		if m := propLine.FindStringSubmatch(line); m != nil {
			props[m[1]] = m[2]
		}
	}
	return props
}

// QuickAction runs one of the overview page's ADB shortcuts. Unsupported
// actions fail locally without touching the device.
func (t Tools) QuickAction(ctx context.Context, serial, action string) (string, error) {
	var command string
	switch action {
	case "expand-notifications":
		command = "cmd statusbar expand-notifications"
	case "expand-settings":
		command = "cmd statusbar expand-settings"
	case "collapse":
		command = "cmd statusbar collapse"
	case "settings":
		command = "am start -a android.settings.SETTINGS"
	case "wifi":
		command = "am start -a android.settings.WIFI_SETTINGS"
	case "developer":
		command = "am start -a android.settings.APPLICATION_DEVELOPMENT_SETTINGS"
	default:
		return "", errUnsupported("不支持的快捷操作：" + action)
	}
	out, err := t.Device(ctx, serial, "shell", command)
	return strings.TrimSpace(string(out)), err
}
