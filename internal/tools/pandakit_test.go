package tools

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestReadPandaBlock(t *testing.T) {
	var encoded bytes.Buffer
	_ = binary.Write(&encoded, binary.BigEndian, int32(3))
	encoded.Write([]byte{1, 2, 3})
	got, err := readPandaBlock(&encoded)
	if err != nil || !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Fatalf("readPandaBlock() = %v, %v", got, err)
	}
}

func TestReadPandaBool(t *testing.T) {
	for _, tc := range []struct {
		input int32
		want  bool
	}{{0, false}, {1, true}} {
		var encoded bytes.Buffer
		_ = binary.Write(&encoded, binary.BigEndian, tc.input)
		got, err := readPandaBool(&encoded)
		if err != nil || got != tc.want {
			t.Fatalf("readPandaBool(%d) = %v, %v", tc.input, got, err)
		}
	}
}
