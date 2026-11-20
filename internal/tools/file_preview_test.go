package tools

import (
	"context"
	"strings"
	"testing"
)

func TestPreviewBufferNeverAllocatesUnboundedOutput(t *testing.T) {
	cancelled := false
	b := &previewBuffer{limit: 5, cancel: func() { cancelled = true }}
	if n, err := b.Write([]byte("ab")); n != 2 || err != nil {
		t.Fatal(n, err)
	}
	if n, err := b.Write([]byte("cdefgh")); n != 3 || err == nil {
		t.Fatal(n, err)
	}
	if !b.overflow || !cancelled || string(b.data) != "abcde" {
		t.Fatalf("buffer = %+v", b)
	}
}

func TestReadPreviewRejectsInvalidRequestsBeforeSpawning(t *testing.T) {
	for _, tc := range []struct {
		serial, path string
		limit        int64
	}{{"", "/a", 4}, {"device", "/a\n", 4}, {"device", "/a", 0}, {"device", "/a", 64<<20 + 1}} {
		if _, err := (Tools{ADB: "not-a-real-executable"}).ReadFilePreview(context.Background(), tc.serial, tc.path, tc.limit); err == nil || strings.Contains(err.Error(), "executable") {
			t.Fatalf("request reached a process: %+v %v", tc, err)
		}
	}
}
