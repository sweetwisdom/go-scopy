package tools

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PullProgress reports actual local bytes written by adb, not elapsed-time estimates.
// An unavailable remote size is zero and must be displayed as indeterminate.
func (t Tools) PullProgress(ctx context.Context, serial, remote, local string, report func(int64, int64)) error {
	var total int64
	sizeCtx, sizeCancel := context.WithTimeout(ctx, 5*time.Second)
	if out, err := t.Device(sizeCtx, serial, "shell", "stat -c %s "+shellQuote(remote)); err == nil {
		total, _ = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if total < 0 {
			total = 0
		}
	}
	sizeCancel()
	update := func() {
		var bytes int64
		if info, err := os.Stat(local); err == nil {
			bytes = info.Size()
		}
		if report != nil {
			report(bytes, total)
		}
	}
	update()
	done := make(chan error, 1)
	go func() { _, err := t.Pull(ctx, serial, remote, local); done <- err }()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			update()
			if err == nil && total > 0 {
				info, statErr := os.Stat(local)
				if statErr != nil {
					return statErr
				}
				if info.Size() != total {
					return fmt.Errorf("安装包传输不完整：%d / %d 字节", info.Size(), total)
				}
			}
			return err
		case <-ticker.C:
			update()
		}
	}
}
