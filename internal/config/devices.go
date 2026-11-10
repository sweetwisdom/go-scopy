package config

import (
	"errors"
	"strings"

	"github.com/go-scopy/go-scopy/internal/domain"
)

func copyMap[K comparable, V any](source map[K]V) map[K]V {
	result := make(map[K]V, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}

// EditDevice changes local records only; active transports are left untouched.
func EditDevice(s domain.Settings, previous, address, name string, wireless bool) (domain.Settings, error) {
	address, name = strings.TrimSpace(address), strings.TrimSpace(name)
	if wireless {
		if err := ValidateDeviceAddress(address); err != nil {
			return s, err
		}
		if address != previous && s.SavedWireless[address] {
			return s, errors.New("该无线地址已有保存记录，请编辑已有设备")
		}
	} else {
		address = previous
	}
	s.DeviceNames = copyMap(s.DeviceNames)
	s.SavedWireless = copyMap(s.SavedWireless)
	s.Devices = copyMap(s.Devices)
	if previous != "" && previous != address {
		delete(s.DeviceNames, previous)
		delete(s.SavedWireless, previous)
		if options, exists := s.Devices[previous]; exists {
			if _, occupied := s.Devices[address]; !occupied {
				s.Devices[address] = options
			}
			delete(s.Devices, previous)
		}
	}
	if name == "" {
		delete(s.DeviceNames, address)
	} else {
		s.DeviceNames[address] = name
	}
	if wireless {
		s.SavedWireless[address] = true
	}
	return s, Validate(s)
}

func ForgetDevice(s domain.Settings, address string) domain.Settings {
	s.DeviceNames = copyMap(s.DeviceNames)
	s.SavedWireless = copyMap(s.SavedWireless)
	s.Devices = copyMap(s.Devices)
	delete(s.DeviceNames, address)
	delete(s.SavedWireless, address)
	delete(s.Devices, address)
	return s
}
