package tools

import "testing"

func TestWiFiRouteAddress(t *testing.T) {
	for _, tc := range []struct{ name, routes, want string }{
		{"USB tethering first", "10.155.160.0/24 dev rndis0 src 10.155.160.129\n10.163.225.60/30 dev rmnet_data2 src 10.163.225.62\n10.168.2.0/24 dev wlan0 src 10.168.2.20", "10.168.2.20"},
		{"alternate Wi-Fi", "default via 192.168.2.1 dev wlan1 src 192.168.2.10", "192.168.2.10"},
		{"no Wi-Fi", "10.155.160.0/24 dev rndis0 src 10.155.160.129\n10.1.0.0/24 dev tun0 src 10.1.0.1", ""},
		{"reject unspecified", "default dev wlan0 src 0.0.0.0", ""},
		{"reject loopback", "default dev wlan0 src 127.0.0.1", ""},
		{"reject link local", "default dev wlan0 src 169.254.1.2", ""},
		{"missing source", "default via 10.168.2.1 dev wlan0", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := wifiRouteAddress(tc.routes); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
