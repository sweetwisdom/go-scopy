// Package domain defines the desktop application's backend contract.
package domain

import (
	"context"
	"time"
)

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
	// Keyboard selects how scrcpy sends keys: "" or "sdk" injects through the
	// Android API, "uhid" emulates a physical keyboard. Changing it restarts
	// the session.
	Keyboard string `json:"keyboard,omitempty"`
}

// Keyboard modes persisted in Options.Keyboard.
const (
	KeyboardSDK  = ""
	KeyboardUHID = "uhid"
)

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

// SessionKind separates a device's ordinary mirroring window from a virtual
// display window; one device may run one of each at the same time.
type SessionKind string

const (
	SessionMirror  SessionKind = "mirror"
	SessionVirtual SessionKind = "virtual"
)

// SessionID names a session uniquely: a device may hold a mirror and a
// virtual-display session at once.
func SessionID(serial string, kind SessionKind) string { return string(kind) + ":" + serial }

type Session struct {
	ID, Serial, Status, Recording string
	Kind                          SessionKind
	Started                       time.Time
}

// VirtualDisplay are the parameters of a scrcpy virtual display window.
type VirtualDisplay struct {
	Width, Height, DPI int
	SystemDecorations  bool // default true
	DestroyOnClose     bool // default false: content moves to the main screen
	IMEPolicy          string
	StartApp           string // package name; required
}

// PhoneInfo is the device overview page's snapshot of one phone.
type PhoneInfo struct {
	Manufacturer, Model, AndroidVersion, APILevel string
	WiFiSSID, IPAddress                           string
	BatteryLevel                                  int    // 0–100; -1 when unknown
	BatteryStatus                                 string // 充电中 / 已充满 / 放电中 / 未充电 / 未知
	MemTotalKB, MemAvailableKB                    int64
	StorageTotalBytes, StorageUsedBytes           int64
	Error                                         string
}

// FileEntry is one row of a device directory listing.
type FileEntry struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime string // "2026-06-17 09:40", empty when the device did not report one
}

// FileListing is one directory read, cached in State for the files tab.
type FileListing struct {
	Path    string
	Entries []FileEntry
	Error   string
}

// AppInfo is one installed application. PandaKit may return the label and icon
// alongside the package metadata; APK parsing remains the fallback.
type AppInfo struct {
	Package     string
	Label       string
	IconPNG     []byte
	CanLaunch   bool
	VersionName string
	VersionCode string
	SizeBytes   int64
	System      bool
	APKPath     string // base.apk; where the label, icon and export come from
}

// AppListing is one installed-package read, cached in State for the apps tab.
type AppListing struct {
	Apps  []AppInfo
	Error string
}

// AppMeta is the lazily resolved label and icon of one application.
type AppMeta struct {
	Package string
	Label   string
	IconPNG []byte
	Error   string
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
	PhoneInfos                                     map[string]PhoneInfo
	Files                                          map[string]FileListing
	Apps                                           map[string]AppListing
	AppMetas                                       map[string]AppMeta
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
	StartVirtual(serial string, options Options, display VirtualDisplay, record bool)
	Stop(session string)
	Capture(serial string)
	Install(serial, apk string)
	Save(Settings)
	Close()

	// Device management reads. Each fetch stores its result in State and wakes
	// the UI; repeated calls are cheap for an unchanged cache.
	FetchPhoneInfo(serial string)
	FetchFiles(serial, dir string)
	FetchApps(serial string)
	FetchAppMeta(serial, apkPath, pkg string)
	QuickAction(serial, action string)

	// File operations run as tasks in the task center.
	FileMkdir(serial, path string)
	FileRename(serial, from, to string)
	FileRemove(serial, path string, recursive bool)
	FileUpload(serial, local, remote string)
	FileDownload(serial, remote, localDir string)

	// App operations run as tasks in the task center. Export saves one
	// reinstallable base APK; backup keeps every split APK plus a manifest.
	AppExport(serial, pkg, localDir string)
	AppBackup(serial, pkg, localDir string)
	AppUninstall(serial, pkg string, system bool)

	// Terminal opens an interactive shell or a logcat stream on a pseudo
	// terminal. The returned Terminal is owned by the caller.
	OpenShell(serial string, cols, rows int, notify func()) (Terminal, error)
	OpenLogcat(serial string, cols, rows int, notify func()) (Terminal, error)
}

// Terminal is one running interactive stream: a device shell or a logcat.
type Terminal interface {
	Write(p []byte) bool
	Resize(cols, rows int)
	Drain(dst []byte) []byte
	Done() <-chan struct{}
	Err() error
	Close()
}

// SessionHost manages session controls while the backend owns scrcpy's process.
type SessionHost interface {
	Attach(context.Context, int) error
	Done() <-chan struct{}
	Finished(error)
}

// HostedBackend runs scrcpy beside an island control window.
type HostedBackend interface {
	StartHosted(serial, session string, options Options, display *VirtualDisplay, record bool, host SessionHost)
	Key(serial, action string)
}
