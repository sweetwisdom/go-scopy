package tools

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// WiFiAddress excludes USB tethering, cellular and VPN routes, which may also
// expose reachable private addresses while the USB cable is attached.
func (t Tools) WiFiAddress(ctx context.Context, serial string) (string, error) {
	out, routeErr := t.Device(ctx, serial, "shell", "ip", "route")
	if routeErr == nil {
		if host := wifiRouteAddress(string(out)); host != "" {
			return host, nil
		}
	}
	// Some Android versions do not expose Wi-Fi routes in the main table.
	out, err := t.Device(ctx, serial, "shell", "ip", "-o", "-4", "addr", "show", "dev", "wlan0")
	if err == nil {
		fields := strings.Fields(string(out))
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "inet" {
				host, _, _ := strings.Cut(fields[i+1], "/")
				if usableIPv4(host) {
					return host, nil
				}
			}
		}
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return "", errors.New("未找到手机 Wi-Fi IPv4 地址，请先将手机与电脑连接到可互通的 Wi-Fi 网络")
}

func usableIPv4(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() != nil && ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast()
}

func wifiRouteAddress(output string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		var device, host string
		for i := 0; i+1 < len(fields); i++ {
			switch fields[i] {
			case "dev":
				device = fields[i+1]
			case "src":
				host = fields[i+1]
			}
		}
		if (strings.HasPrefix(device, "wlan") || strings.HasPrefix(device, "wifi")) && usableIPv4(host) {
			return host
		}
	}
	return ""
}

// Connect verifies the selected TCP transport, independently of any USB link.
func (t Tools) Connect(ctx context.Context, address string) ([]byte, error) {
	out, err := t.Run(ctx, "connect", address)
	if err != nil {
		return nil, err
	}
	state, err := t.Device(ctx, address, "get-state")
	if err != nil {
		return nil, fmt.Errorf("无线通道 %s 尚未就绪：%w", address, err)
	}
	if strings.TrimSpace(string(state)) != "device" {
		return nil, fmt.Errorf("无线通道 %s 尚未就绪：%s", address, strings.TrimSpace(string(state)))
	}
	return out, nil
}
