package tools

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	propLine = regexp.MustCompile(`^\[([^\]]+)]: \[(.*)]$`)
	inetLine = regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+)/`)
	ssidLine = regexp.MustCompile(`SSID: "?([^",\r\n]+)"?`)
)

// Commands are read-only and use only Android's built-in shell utilities.
const phoneSystemCommand = `printf '\n__GOSCOPY_kernel__\n'; uname -r
printf '\n__GOSCOPY_uptime__\n'; cat /proc/uptime
printf '\n__GOSCOPY_uid__\n'; id -u
printf '\n__GOSCOPY_su__\n'; command -v su
printf '\n__GOSCOPY_cpu_min__\n'; cat /sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_min_freq 2>/dev/null
printf '\n__GOSCOPY_cpu_max__\n'; cat /sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq 2>/dev/null
printf '\n__GOSCOPY_cpu_cur__\n'; cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq 2>/dev/null
printf '\n__GOSCOPY_current__\n'; cat /sys/class/power_supply/battery/current_now 2>/dev/null
:`

// PhoneInfo bounds concurrent ADB processes and each command's lifetime. A
// failed group cannot discard successful groups or block the page indefinitely.
func (t Tools) PhoneInfo(ctx context.Context, serial string) (PhoneInfo, error) {
	return collectPhoneInfo(ctx, func(ctx context.Context, command string) ([]byte, error) {
		return t.Device(ctx, serial, "shell", command)
	}, 4*time.Second)
}

type phoneRead struct {
	name, command string
	parse         func(*PhoneInfo, string) bool
}

func collectPhoneInfo(ctx context.Context, run func(context.Context, string) ([]byte, error), timeout time.Duration) (PhoneInfo, error) {
	info := PhoneInfo{BatteryLevel: -1, MemAvailableKB: -1}
	reads := []phoneRead{
		{"系统属性", "getprop", parsePhoneProps},
		{"电池", "dumpsys battery", parsePhoneBattery},
		{"内存", "cat /proc/meminfo", parsePhoneMemory},
		{"存储", "df -k /data", parsePhoneStorage},
		{"屏幕", "wm size; wm density", parsePhoneDisplay},
		{"系统详情", phoneSystemCommand, parsePhoneSystem},
		{"网络地址", "ip -f inet addr show wlan0", func(i *PhoneInfo, text string) bool {
			if m := inetLine.FindStringSubmatch(text); m != nil {
				i.IPAddress = m[1]
			}
			return true
		}},
		{"Wi-Fi", "cmd wifi status", func(i *PhoneInfo, text string) bool {
			if m := ssidLine.FindStringSubmatch(text); m != nil && m[1] != "<unknown ssid>" {
				i.WiFiSSID = strings.TrimSpace(m[1])
			}
			return true
		}},
	}
	type result struct {
		out []byte
		err error
	}
	results := make([]result, len(reads))
	jobs := make(chan int, len(reads))
	for n := range reads {
		jobs <- n
	}
	close(jobs)
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range jobs {
				child, cancel := context.WithTimeout(ctx, timeout)
				results[n].out, results[n].err = run(child, reads[n].command)
				cancel()
			}
		}()
	}
	wg.Wait()
	succeeded := 0
	for n, r := range reads {
		result := results[n]
		if result.err != nil {
			info.Warnings = append(info.Warnings, r.name+"："+result.err.Error())
		} else if !r.parse(&info, string(result.out)) {
			info.Warnings = append(info.Warnings, r.name+"：设备未返回可识别的数据")
		} else {
			succeeded++
		}
	}
	if succeeded == 0 {
		return info, errors.New("未能读取设备信息，请检查 ADB 连接；详情见读取提示")
	}
	return info, nil
}

func parseProps(text string) map[string]string {
	props := make(map[string]string, 256)
	for _, line := range strings.Split(text, "\n") {
		if m := propLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			props[m[1]] = m[2]
		}
	}
	return props
}

func firstProp(p map[string]string, keys ...string) string {
	for _, k := range keys {
		if p[k] != "" {
			return p[k]
		}
	}
	return ""
}

func parsePhoneProps(i *PhoneInfo, text string) bool {
	p := parseProps(text)
	i.Manufacturer = p["ro.product.manufacturer"]
	i.Brand, i.Model = p["ro.product.brand"], p["ro.product.model"]
	i.Product, i.DeviceCode = p["ro.product.name"], p["ro.product.device"]
	i.AndroidVersion, i.APILevel = p["ro.build.version.release"], p["ro.build.version.sdk"]
	i.SecurityPatch = p["ro.build.version.security_patch"]
	i.SerialNumber = firstProp(p, "ro.serialno", "ro.boot.serialno")
	i.CPUABI, i.CPUABIs = p["ro.product.cpu.abi"], p["ro.product.cpu.abilist"]
	i.Hardware, i.Board = p["ro.hardware"], p["ro.product.board"]
	i.BuildID, i.BuildDisplay = p["ro.build.id"], p["ro.build.display.id"]
	i.Fingerprint, i.Baseband = p["ro.build.fingerprint"], p["gsm.version.baseband"]
	i.VerifiedBoot = p["ro.boot.verifiedbootstate"]
	// A Verified Boot color alone is not a lock-state measurement.
	switch p["ro.boot.flash.locked"] {
	case "1":
		i.BootloaderState = "已锁定（flash.locked=1）"
	case "0":
		i.BootloaderState = "已解锁（flash.locked=0）"
	default:
		switch p["ro.boot.vbmeta.device_state"] {
		case "locked":
			i.BootloaderState = "已锁定（vbmeta）"
		case "unlocked":
			i.BootloaderState = "已解锁（vbmeta）"
		}
	}
	switch firstProp(p, "ro.boot.slot_suffix", "ro.boot.slot") {
	case "_a", "a":
		i.Slot = "Slot A"
	case "_b", "b":
		i.Slot = "Slot B"
	default:
		if p["ro.build.ab_update"] == "false" {
			i.Slot = "非 A/B 分区（设备报告）"
		}
	}
	bootDevice := strings.ToLower(firstProp(p, "ro.boot.bootdevice", "ro.boot.boot_devices"))
	if strings.Contains(bootDevice, "ufs") {
		i.StorageType = "UFS（启动设备属性）"
	}
	if strings.Contains(bootDevice, "mmc") {
		i.StorageType = "eMMC（启动设备属性）"
	}
	i.IPAddress = p["dhcp.wlan0.ipaddress"]
	return len(p) > 0
}

func number(text string) *int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func parsePhoneBattery(i *PhoneInfo, text string) bool {
	fields := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok {
			fields[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	if n := number(fields["level"]); n != nil {
		scale := int64(100)
		if s := number(fields["scale"]); s != nil {
			scale = *s
		}
		if scale > 0 && *n >= 0 && *n <= scale {
			i.BatteryLevel = int(math.Round(float64(*n) * 100 / float64(scale)))
		}
	}
	i.BatteryStatus = map[string]string{"2": "充电中", "3": "放电中", "4": "未充电", "5": "已充满"}[fields["status"]]
	i.BatteryHealth = map[string]string{"2": "良好", "3": "过热", "4": "失效", "5": "过压", "6": "未指定故障", "7": "过冷"}[fields["health"]]
	i.BatteryVoltageMV = number(fields["voltage"])
	if i.BatteryVoltageMV != nil && *i.BatteryVoltageMV <= 0 {
		i.BatteryVoltageMV = nil
	}
	i.BatteryTemperatureTenthsC = number(fields["temperature"])
	i.BatteryChargeCounterUAH = number(fields["charge counter"])
	if i.BatteryChargeCounterUAH != nil && *i.BatteryChargeCounterUAH < 0 {
		i.BatteryChargeCounterUAH = nil
	}
	return i.BatteryLevel >= 0 || i.BatteryStatus != "" || i.BatteryVoltageMV != nil
}

func parsePhoneMemory(i *PhoneInfo, text string) bool {
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		fields := strings.Fields(value)
		if !ok || len(fields) == 0 {
			continue
		}
		n := number(fields[0])
		if n == nil || *n < 0 {
			continue
		}
		switch strings.TrimSpace(key) {
		case "MemTotal":
			i.MemTotalKB = *n
		case "MemAvailable":
			i.MemAvailableKB = *n
		}
	}
	if i.MemAvailableKB > i.MemTotalKB {
		i.MemAvailableKB = -1
	}
	return i.MemTotalKB > 0
}

func parsePhoneStorage(i *PhoneInfo, text string) bool {
	// The queried path is /data, but df can report a bind/overlay mount at a
	// different path. Also accept a wrapped filesystem name on its own line.
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		start := 1
		if strings.HasSuffix(f[3], "%") {
			start = 0
		}
		total, used := number(f[start]), number(f[start+1])
		if total != nil && used != nil && *total > 0 && *used >= 0 && *used <= *total && *total <= math.MaxInt64/1024 {
			i.StorageTotalBytes, i.StorageUsedBytes = *total*1024, *used*1024
			return true
		}
	}
	return false
}

func parsePhoneDisplay(i *PhoneInfo, text string) bool {
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "Physical size":
			i.PhysicalSize = v
		case "Override size":
			i.OverrideSize = v
		case "Physical density":
			i.PhysicalDPI = v
		case "Override density":
			i.OverrideDPI = v
		}
	}
	return i.PhysicalSize != "" || i.PhysicalDPI != ""
}

func parsePhoneSystem(i *PhoneInfo, text string) bool {
	fields := map[string]string{}
	key := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "__GOSCOPY_") && strings.HasSuffix(line, "__") {
			key = strings.TrimSuffix(strings.TrimPrefix(line, "__GOSCOPY_"), "__")
		} else if key != "" && line != "" {
			fields[key] = line
		}
	}
	i.KernelVersion = fields["kernel"]
	if f := strings.Fields(fields["uptime"]); len(f) > 0 {
		n, err := strconv.ParseFloat(f[0], 64)
		if err == nil && n >= 0 && n < float64(math.MaxInt64) {
			v := int64(n)
			i.UptimeSeconds = &v
		}
	}
	switch fields["uid"] {
	case "0":
		i.RootStatus = "ADB shell 已是 root（UID 0）"
	case "":
	default:
		i.RootStatus = "未确认；ADB UID " + fields["uid"]
	}
	if fields["su"] != "" {
		i.RootStatus = fmt.Sprintf("检测到 su：%s（未执行提权）", fields["su"])
	}
	i.CPUFreqMinKHz, i.CPUFreqMaxKHz = number(fields["cpu_min"]), number(fields["cpu_max"])
	i.CPUFreqCurKHz = number(fields["cpu_cur"])
	i.BatteryCurrentUA = number(fields["current"])
	return i.KernelVersion != "" || i.UptimeSeconds != nil
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
