package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

func TestDevicesCoverTransportAndAuthorization(t *testing.T) {
	d := ParseDevices("List of devices attached\nUSB123 unauthorized usb:1-2\n192.168.1.9:5555 device product:panther model:Pixel_7 transport_id:3\nUSB456 offline\n")
	if len(d) != 3 || d[0].Model != "Pixel 7" || d[0].Connection != "Wi-Fi" || d[1].State != "unauthorized" || d[2].State != "offline" {
		t.Fatalf("unexpected devices: %+v", d)
	}
}

func TestTrackerFramesHandleEmptyAndTruncatedPayload(t *testing.T) {
	r := strings.NewReader("0000000aabc device")
	if text, err := readFrame(r); err != nil || text != "" {
		t.Fatalf("empty frame: %q %v", text, err)
	}
	if text, err := readFrame(r); err != nil || text != "abc device" {
		t.Fatalf("device frame: %q %v", text, err)
	}
	for _, s := range []string{"zzzz", "000fabc", "00"} {
		if _, err := readFrame(strings.NewReader(s)); err == nil {
			t.Fatalf("accepted truncated frame %q", s)
		}
	}
}

func TestScrcpyArgumentsKeepPathsAndSerialAsSingleArguments(t *testing.T) {
	o := domain.Options{MaxSize: 1920, FPS: 60, Bitrate: 8, ScreenOff: true, StayAwake: true, AlwaysOnTop: true}
	path := `C:\录像 文件\a $test; 1.mp4`
	serial := `device-$(echo)`
	a, err := ScrcpyArgs(serial, o, path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--serial", serial, "--window-title", "Go-Scopy · " + serial, "--max-fps", "60", "--video-bit-rate", "8M", "--max-size", "1920", "--no-audio", "--turn-screen-off", "--stay-awake", "--always-on-top", "--record", path}
	if !reflect.DeepEqual(a, want) {
		t.Fatalf("%q", a)
	}
	o.FPS = 0
	if _, err := ScrcpyArgs(serial, o, ""); err == nil {
		t.Fatal("invalid parameters accepted")
	}
}

func TestResolveExplicitPathNeverSilentlyFallsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "不存在.exe")
	if _, err := Resolve(path, "adb", dir); err == nil {
		t.Fatal("missing custom path accepted")
	}
	if _, err := Resolve(dir, "adb", dir); err == nil {
		t.Fatal("directory accepted as executable")
	}
	if err := os.WriteFile(path, []byte("stub"), 0700); err != nil {
		t.Fatal(err)
	}
	if got, err := Resolve(path, "adb", dir); err != nil || got != path {
		t.Fatalf("explicit path %q %v", got, err)
	}
}

func TestAddressValidation(t *testing.T) {
	for _, address := range []string{"192.168.1.2:5555", "[::1]:12345", "android.local:37001"} {
		if err := ValidateAddress(address); err != nil {
			t.Fatal(err)
		}
	}
	for _, address := range []string{"", ":5555", "192.168.1.2", "host:0", "host:65536", "ho st:5555"} {
		if err := ValidateAddress(address); err == nil {
			t.Fatalf("accepted %q", address)
		}
	}
}

func TestOutputTailIsBounded(t *testing.T) {
	var b limitedBuffer
	input := bytes.Repeat([]byte("x"), 32768)
	if n, err := b.Write(input); n != len(input) || err != nil {
		t.Fatal(n, err)
	}
	io.WriteString(&b, "last message")
	if got := b.String(); len(got) > 16384 || !strings.HasSuffix(got, "last message") {
		t.Fatal("tail did not preserve final diagnostic")
	}
}

func TestTrackingHandshakeUpdatesAndCancellation(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan []domain.Device, 2)
	finished := make(chan error, 1)
	go func() { finished <- trackConnection(ctx, client, func(d []domain.Device) { updates <- d }) }()
	request, err := readFrame(server)
	if err != nil || request != "host:track-devices-l" {
		t.Fatalf("handshake %q %v", request, err)
	}
	if _, err = io.WriteString(server, "OKAY0000"); err != nil {
		t.Fatal(err)
	}
	text := "phone unauthorized\n"
	if _, err = fmt.Fprintf(server, "%04x%s", len(text), text); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case got := <-updates:
			if i == 0 && len(got) != 0 || i == 1 && (len(got) != 1 || got[0].State != "unauthorized") {
				t.Fatalf("update %d: %+v", i, got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("missing device update")
		}
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not unblock tracker")
	}
}

func TestADBEnvironmentUsesOneLocalServer(t *testing.T) {
	t.Setenv("ADB_SERVER_SOCKET", "tcp:remote.invalid:9999")
	t.Setenv("ANDROID_ADB_SERVER_ADDRESS", "remote.invalid")
	t.Setenv("ANDROID_ADB_SERVER_PORT", "9999")
	t.Setenv("ADB", "other-adb")
	env := localADBEnvironment("selected-adb")
	for _, entry := range env {
		if strings.Contains(entry, "remote.invalid") || entry == "ANDROID_ADB_SERVER_PORT=9999" || entry == "ADB=other-adb" {
			t.Fatalf("inherited conflicting adb environment: %s", entry)
		}
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "ADB=selected-adb") || !strings.Contains(joined, "ANDROID_ADB_SERVER_PORT=5037") {
		t.Fatal("local adb settings missing")
	}
}
