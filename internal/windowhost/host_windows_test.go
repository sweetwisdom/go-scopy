//go:build windows

package windowhost

import "testing"

func TestIslandSnapsWithinMonitorWorkArea(t *testing.T) {
	for _, tc := range []struct {
		name               string
		window, work, want rect
		ax, ay             int
	}{
		{"top", rect{600, 15, 836, 63}, rect{0, 0, 1920, 1040}, rect{600, 12, 836, 60}, 0, -1},
		{"bottom-right", rect{1674, 982, 1910, 1030}, rect{0, 0, 1920, 1040}, rect{1672, 980, 1908, 1028}, 1, 1},
		{"free", rect{500, 400, 736, 448}, rect{0, 0, 1920, 1040}, rect{500, 400, 736, 448}, 0, 0},
		{"negative-monitor", rect{-1930, 300, -1694, 348}, rect{-1920, 0, 0, 1040}, rect{-1908, 300, -1672, 348}, -1, 0},
		{"taskbar", rect{500, 1100, 844, 1372}, rect{0, 40, 1920, 1040}, rect{500, 756, 844, 1028}, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ax, ay := snapRect(tc.window, tc.work, 24, 12)
			if got != tc.want || ax != tc.ax || ay != tc.ay {
				t.Fatalf("got %+v (%d, %d), want %+v (%d, %d)", got, ax, ay, tc.want, tc.ax, tc.ay)
			}
		})
	}
}
