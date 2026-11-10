// Package domain defines the desktop application's backend contract.
package domain

import "time"

type Device struct {
	Serial, Model, Product, State, Connection string
	Alias                                     string
}

func (d Device) Name() string {
	if d.Alias != "" {
		return d.Alias
	}
	if d.Model != "" {
		return d.Model
	}
	return d.Serial
}

type Options struct {
	MaxSize     int  `json:"maxSize"`
	FPS         int  `json:"fps"`
	Bitrate     int  `json:"bitrateMbps"`
	Audio       bool `json:"audio"`
	ScreenOff   bool `json:"screenOff"`
	StayAwake   bool `json:"stayAwake"`
	AlwaysOnTop bool `json:"alwaysOnTop"`
}

type Settings struct {
	Theme         string             `json:"theme"`
	ADBPath       string             `json:"adbPath,omitempty"`
	ScrcpyPath    string             `json:"scrcpyPath,omitempty"`
	OutputDir     string             `json:"outputDir"`
	Defaults      Options            `json:"defaults"`
	Devices       map[string]Options `json:"devices,omitempty"`
	DeviceNames   map[string]string  `json:"deviceNames,omitempty"`
	SavedWireless map[string]bool    `json:"savedWireless,omitempty"`
}

func (s Settings) OptionsFor(serial string) Options {
	if o, ok := s.Devices[serial]; ok {
		return o
	}
	return s.Defaults
}

type Session struct {
	Serial, Status, Recording string
	Started                   time.Time
}

type Task struct {
	ID                          uint64
	Title, Status, Detail, Path string
	Started                     time.Time
}

type State struct {
	Devices                                        []Device
	Sessions                                       map[string]Session
	Tasks                                          []Task
	Settings                                       Settings
	ADBVersion, ScrcpyVersion, ADBPath, ScrcpyPath string
	Connection, ToolError                          string
}

// Backend methods enqueue work; State returns an independent snapshot.
// Notify may be called from a worker and must only wake the UI thread.
type Backend interface {
	State() State
	SetNotify(func())
	Refresh()
	Connect(address string)
	Pair(address, code string)
	Disconnect(serial string)
	EnableTCP(serial string)
	Start(serial string, options Options, record bool)
	Stop(serial string)
	Capture(serial string)
	Install(serial, apk string)
	Save(Settings)
	Close()
}
