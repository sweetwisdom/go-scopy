package core

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

func (s *Service) FetchPreview(serial string) {
	s.mu.Lock()
	if s.closed || serial == "" || (s.state.Preview.Serial == serial && s.state.Preview.Loading) {
		s.mu.Unlock()
		return
	}
	if s.previewCancel != nil {
		s.previewCancel()
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	s.previewCancel = cancel
	s.previewGeneration++
	generation := s.previewGeneration
	if s.state.Preview.Serial != serial {
		s.state.Preview = domain.ScreenPreview{Serial: serial}
	}
	s.state.Preview.Loading, s.state.Preview.Error = true, ""
	s.wg.Add(1)
	s.mu.Unlock()
	s.wake()
	go func() {
		defer s.wg.Done()
		defer cancel()
		err := s.online(serial)
		var data []byte
		if err == nil {
			data, err = s.tool.Device(ctx, serial, "exec-out", "screencap", "-p")
		}
		if err == nil {
			cfg, decodeErr := png.DecodeConfig(bytes.NewReader(data))
			if decodeErr != nil || len(data) > 32<<20 || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 32<<20 {
				err = fmt.Errorf("设备未返回有效的屏幕 PNG")
			}
		}
		s.mu.Lock()
		if generation != s.previewGeneration || s.closed {
			s.mu.Unlock()
			return
		}
		preview := &s.state.Preview
		preview.Loading = false
		if err != nil {
			preview.Error = err.Error()
		} else {
			preview.PNG, preview.UpdatedAt = data, time.Now()
		}
		s.previewCancel = nil
		s.mu.Unlock()
		s.wake()
	}()
}

func (s *Service) ClearPreview(serial string) {
	s.mu.Lock()
	if s.state.Preview.Serial != serial {
		s.mu.Unlock()
		return
	}
	if s.previewCancel != nil {
		s.previewCancel()
		s.previewCancel = nil
	}
	s.previewGeneration++
	s.state.Preview = domain.ScreenPreview{}
	s.mu.Unlock()
	s.wake()
}
