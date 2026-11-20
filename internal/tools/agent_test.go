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
