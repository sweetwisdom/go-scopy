package core

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools"
)

// User intent is recorded synchronously, before submitting any CLI work. A
// tracker notification arriving during disconnect cannot re-enable recovery.
func (s *Service) setWirelessIntent(serial string, connect bool) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wirelessBlocked == nil {
		s.wirelessBlocked = map[string]bool{}
	}
	if s.wirelessIntent == nil {
		s.wirelessIntent = map[string]uint64{}
	}
	s.wirelessIntent[serial]++
	s.wirelessBlocked[serial] = !connect
	if cancel := s.wirelessAttempt[serial]; cancel != nil {
		cancel()
	}
	return s.wirelessIntent[serial]
}

func (s *Service) wirelessIntentCurrent(serial string, version uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && s.wirelessIntent[serial] == version
}

func (s *Service) rememberWireless(serial string, version uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wirelessBlocked[serial] || s.wirelessIntent[serial] != version {
		return
	}
	if s.wirelessKnown == nil {
		s.wirelessKnown = map[string]domain.Device{}
	}
	device := s.wirelessKnown[serial]
	device.Serial, device.Connection = serial, "Wi-Fi"
	s.wirelessKnown[serial] = device
	s.recoveryMessage = ""
}

func (s *Service) acceptDevices(devices []domain.Device) {
	s.mu.Lock()
	changed := !reflect.DeepEqual(s.state.Devices, devices) || s.state.Connection != "设备监听已连接"
	old := s.state.Devices
	s.state.Devices = devices
	s.state.Connection = "设备监听已连接"
	if s.wirelessKnown == nil {
		s.wirelessKnown = map[string]domain.Device{}
	}
	online := map[string]bool{}
	for _, d := range devices {
		if d.State == "device" {
			online[d.Serial] = true
			if d.Connection == "Wi-Fi" && !s.wirelessBlocked[d.Serial] {
				s.wirelessKnown[d.Serial] = d
			}
		}
	}
	var disconnected []*tools.Process
	for _, before := range old {
		if !online[before.Serial] {
			delete(s.agentBatches, before.Serial)
			for id, process := range s.processes {
				if session, ok := s.state.Sessions[id]; ok && session.Serial == before.Serial {
					disconnected = append(disconnected, process)
				}
			}
		}
	}
	s.mu.Unlock()
	if changed {
		s.wake()
	}
	for _, process := range disconnected {
		process.RequestStop()
	}
}

func (s *Service) wirelessCandidates() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	online := map[string]bool{}
	for _, d := range s.state.Devices {
		if d.State == "device" {
			online[d.Serial] = true
		}
	}
	var serials []string
	for serial := range s.wirelessKnown {
		if !online[serial] && !s.wirelessBlocked[serial] {
			serials = append(serials, serial)
		}
	}
	sort.Strings(serials)
	return serials
}

func (s *Service) recoverWireless(ctx context.Context) {
	if len(s.wirelessCandidates()) == 0 {
		return
	}
	s.mu.Lock()
	s.recoveryMessage = "正在恢复无线连接"
	s.mu.Unlock()
	s.wake()
	failed := false
	for attempt := range 3 {
		failed = false
		for _, serial := range s.wirelessCandidates() {
			if ctx.Err() != nil {
				return
			}
			s.wirelessOpMu.Lock()
			s.mu.Lock()
			blocked := s.wirelessBlocked[serial] || s.closed
			readCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			if s.wirelessAttempt == nil {
				s.wirelessAttempt = map[string]context.CancelFunc{}
			}
			if !blocked {
				s.wirelessAttempt[serial] = cancel
			}
			s.mu.Unlock()
			if !blocked {
				address := serial
				var err error
				if tools.ValidateAddress(address) != nil {
					address, err = s.tool.DiscoverWireless(readCtx, serial)
				}
				if err == nil {
					_, err = s.tool.Connect(readCtx, address)
				}
				if err != nil {
					failed = true
				}
			}
			cancel()
			s.mu.Lock()
			delete(s.wirelessAttempt, serial)
			s.mu.Unlock()
			s.wirelessOpMu.Unlock()
		}
		if !failed || ctx.Err() != nil {
			break
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
	s.mu.Lock()
	s.recoveryMessage = ""
	if failed {
		s.recoveryMessage = "部分无线设备未恢复，可手动重连"
	}
	s.mu.Unlock()
	s.wake()
}
