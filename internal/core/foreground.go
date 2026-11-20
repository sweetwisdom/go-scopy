package core

import (
	"context"
	"errors"
	"time"
)

// ResolveForeground uses the normal task center for progress and failures;
// callers only receive immutable data, never a worker-side UI mutation.
func (s *Service) ResolveForeground(serial string, done func(string, error)) {
	id := s.submit("识别当前应用 · "+serial, func(ctx context.Context) (detail, path string, err error) {
		var pkg string
		defer func() { done(pkg, err) }()
		if err = s.online(serial); err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		pkg, err = s.tool.ForegroundApp(ctx, serial)
		return pkg, "", err
	})
	if id == 0 {
		done("", errors.New("程序正在关闭"))
	}
}
