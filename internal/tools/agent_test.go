package tools

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestReadAgentBlock(t *testing.T) {
	var encoded bytes.Buffer
	_ = binary.Write(&encoded, binary.BigEndian, int32(3))
	encoded.Write([]byte{1, 2, 3})
	got, err := readAgentBlock(&encoded)
	if err != nil || !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Fatalf("readAgentBlock() = %v, %v", got, err)
	}
}

func TestReadAgentBool(t *testing.T) {
	for _, tc := range []struct {
		input int32
		want  bool
	}{{0, false}, {1, true}} {
		var encoded bytes.Buffer
		_ = binary.Write(&encoded, binary.BigEndian, tc.input)
		got, err := readAgentBool(&encoded)
		if err != nil || got != tc.want {
			t.Fatalf("readAgentBool(%d) = %v, %v", tc.input, got, err)
		}
	}
}

// Command 10 has no status prefix; booleans occupy four bytes.
func TestReadAgentAppsResponse(t *testing.T) {
	for _, missingDefault := range []bool{false, true} {
		var wire bytes.Buffer
		i32 := func(v int32) {
			t.Helper()
			if err := binary.Write(&wire, binary.BigEndian, v); err != nil {
				t.Fatal(err)
			}
		}
		i64 := func(v int64) {
			t.Helper()
			if err := binary.Write(&wire, binary.BigEndian, v); err != nil {
				t.Fatal(err)
			}
		}
		block := func(v []byte) { i32(int32(len(v))); wire.Write(v) }
		defaultIcon := []byte("default PNG")
		if missingDefault {
			defaultIcon = nil
		}
		block(defaultIcon)
		i32(2)
		for n := range 2 {
			block([]byte("com.example.app"))
			block([]byte("1.2"))
			i64(123)
			block([]byte("Example"))
			i32(1)
			i32(2)
			i32(0)
			block([]byte("installer"))
			block(nil)
			i32(32)
			i32(21)
			i32(128)
			i32(1)
			i32(1)
			i64(12345)
			i64(0)
			i64(0)
			if n == 0 {
				block([]byte("application PNG"))
			} else {
				block(nil)
			}
		}
		apps, err := readAgentAppsResponse(&wire)
		if err != nil {
			t.Fatal(err)
		}
		if len(apps) != 2 || wire.Len() != 0 {
			t.Fatalf("incorrect protocol consumption: %d apps, %d unread", len(apps), wire.Len())
		}
		if string(apps[0].IconPNG) != "application PNG" || !bytes.Equal(apps[1].IconPNG, defaultIcon) {
			t.Fatal("native/default icon selection incorrect")
		}
		if a := apps[1]; a.Package != "com.example.app" || a.VersionCode != "123" || !a.CanLaunch || !a.System || a.SizeBytes != 12345 || a.InstalledAt.Unix() != 1 {
			t.Fatalf("protocol fields shifted: %+v", a)
		}
	}
	if agentIconSize != 1024 {
		t.Fatal("request must preserve native icon sizes; bundled Agent cannot safely downscale")
	}
}

func TestReadAgentAppsResponseRejectsTruncation(t *testing.T) {
	for _, wire := range [][]byte{nil, {0, 0, 0, 0}, {0, 0, 0, 0, 255, 255, 255, 255}, {0, 0, 0, 0, 0, 0, 0, 1}} {
		if _, err := readAgentAppsResponse(bytes.NewReader(wire)); err == nil {
			t.Fatalf("accepted invalid response %v", wire)
		}
	}
}
