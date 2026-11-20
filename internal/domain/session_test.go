package domain

import (
	"strings"
	"sync"
	"testing"
)

func TestVirtualSessionIDsAreUniqueAcrossConcurrentLaunches(t *testing.T) {
	const count = 200
	ids := make(chan string, count)
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() { ids <- NewVirtualSessionID("127.0.0.1:5555") })
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] || !strings.HasPrefix(id, "virtual:127.0.0.1:5555:") {
			t.Fatalf("invalid or duplicate virtual ID: %q", id)
		}
		seen[id] = true
	}
	if SessionID("127.0.0.1:5555", SessionMirror) != "mirror:127.0.0.1:5555" {
		t.Fatal("mirror ID must stay stable")
	}
}
