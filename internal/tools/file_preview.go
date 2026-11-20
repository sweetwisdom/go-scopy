package tools

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ReadFilePreview caps both device output and host allocation. Unlike Run it
// does not interpret file bytes beginning with "error:" as an ADB error.
func (t Tools) ReadFilePreview(ctx context.Context, serial, remote string, limit int64) ([]byte, error) {
	if serial == "" || t.ADB == "" {
		return nil, errors.New("设备或 adb 未就绪")
	}
	if err := validPath(remote); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 64<<20 {
		return nil, errors.New("预览大小上限无效")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	command := "head -c " + strconv.FormatInt(limit+1, 10) + " -- " + shellQuote(remote)
	c := Command(ctx, t.ADB, "-P", "5037", "-s", serial, "exec-out", command)
	c.Env = localADBEnvironment(t.ADB)
	out := &previewBuffer{limit: int(limit + 1), cancel: cancel}
	var stderr limitedBuffer
	c.Stdout, c.Stderr = out, &stderr
	err := c.Run()
	if out.overflow || int64(len(out.data)) > limit {
		return nil, errors.New("文件超过预览大小上限，请下载后打开")
	}
	if err != nil {
		return nil, fmt.Errorf("读取预览失败：%s (%w)", strings.TrimSpace(stderr.String()), err)
	}
	return out.data, nil
}

type previewBuffer struct {
	data     []byte
	limit    int
	overflow bool
	cancel   context.CancelFunc
}

func (b *previewBuffer) Write(p []byte) (int, error) {
	n := min(len(p), b.limit-len(b.data))
	b.data = append(b.data, p[:n]...)
	if n != len(p) {
		b.overflow = true
		b.cancel()
		return n, errors.New("preview output limit exceeded")
	}
	return n, nil
}
