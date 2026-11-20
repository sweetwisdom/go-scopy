// Package domain defines the desktop application's backend contract.
package domain

import (
	"context"
	"strconv"
	"sync/atomic"
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
	ScreenPreviewIntervalSeconds int                `json:"screenPreviewIntervalSeconds"`
	Theme                        string             `json:"theme"`
	ADBPath                      string             `json:"adbPath,omitempty"`
	ScrcpyPath                   string             `json:"scrcpyPath,omitempty"`
	OutputDir                    string             `json:"outputDir"`
	AppExportDir                 string             `json:"appExportDir,omitempty"`
	Defaults                     Options            `json:"defaults"`
	Devices                      map[string]Options `json:"devices,omitempty"`
	DeviceNames                  map[string]string  `json:"deviceNames,omitempty"`
	SavedWireless                map[string]bool    `json:"savedWireless,omitempty"`
}

func (s Settings) OptionsFor(serial string) Options {
	if o, ok := s.Devices[serial]; ok {
		return o
	}
	return s.Defaults
}

// SessionKind separates a device's ordinary mirroring window from a virtual
// display window; one device may run a mirror and multiple virtual displays.
type SessionKind string

const (
	SessionMirror  SessionKind = "mirror"
	SessionVirtual SessionKind = "virtual"
)

// SessionID is the stable per-device session key (used to reuse a mirror).
// Virtual launches must use NewVirtualSessionID instead.
func SessionID(serial string, kind SessionKind) string { return string(kind) + ":" + serial }

var nextVirtualSession atomic.Uint64

// NewVirtualSessionID allocates a distinct key for every launch, including
// repeated launches of the same app. IDs only need to be unique in this process.
func NewVirtualSessionID(serial string) string {
	return SessionID(serial, SessionVirtual) + ":" + strconv.FormatUint(nextVirtualSession.Add(1), 10)
}

type Session struct {
	ID, Serial, Status, Recording string
	StartApp                      string
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
	ReturnHome         bool   // shortcut: return display 0 home only after this app is resumed on a secondary display
}

// PhoneInfo is the device overview page's snapshot of one phone.
type PhoneInfo struct {
	Manufacturer, Brand, Model, Product, DeviceCode string
	AndroidVersion, APILevel, SecurityPatch         string
	SerialNumber, CPUABI, CPUABIs, Hardware, Board  string
	BuildID, BuildDisplay, Fingerprint, Baseband    string
	PhysicalSize, OverrideSize                      string
	PhysicalDPI, OverrideDPI                        string
	KernelVersion, BootloaderState, VerifiedBoot    string
	Slot, StorageType, RootStatus                   string
	UptimeSeconds                                   *int64
	CPUFreqMinKHz, CPUFreqMaxKHz, CPUFreqCurKHz     *int64
	WiFiSSID, IPAddress                             string
	BatteryLevel                                    int // 0–100; -1 when unknown
	BatteryStatus, BatteryHealth                    string
	BatteryVoltageMV, BatteryTemperatureTenthsC     *int64
	BatteryChargeCounterUAH, BatteryCurrentUA       *int64
	MemTotalKB, MemAvailableKB                      int64 // available is -1 when unknown
	StorageTotalBytes, StorageUsedBytes             int64
	Loading                                         bool
	UpdatedAt                                       time.Time
	Warnings                                        []string
	Error                                           string
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

// AppInfo is one installed application. Go-Scrcpy Agent may return the label and icon
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
	Apps       []AppInfo
	Error      string
	Cached     bool // A disk snapshot, pending an authoritative device refresh.
	Refreshing bool
}

// AppMeta is the lazily resolved label and icon of one application.
type AppMeta struct {
	Resolved bool // Includes a successful lookup with no app-specific raster icon.
	Package  string
	Label    string
	IconPNG  []byte
	Error    string
}

// AppExportProgress is a snapshot; zero Total means this is not an export task.
type AppExportProgress struct {
	Phase                    string
	Serial, Current          string
	Completed, Failed, Total int
	Bytes, TotalBytes        int64
}

type Task struct {
	ID                          uint64
	Title, Status, Detail, Path string
	Started                     time.Time
	Export                      AppExportProgress
}

// ScreenPreview is an ephemeral in-memory capture, never a saved screenshot.
type ScreenPreview struct {
	Serial    string
	PNG       []byte
	Loading   bool
	UpdatedAt time.Time
	Error     string
}

// ScreenPreviewBackend is optional so read-only/offline backends remain usable.
type ScreenPreviewBackend interface {
	FetchPreview(serial string)
	ClearPreview(serial string)
}

// AppCacheBackend clears only app metadata/icon caches, never device records or exports.
type AppCacheBackend interface{ ClearAppCache() }

type State struct {
	SharedAppMetas                                 map[string]AppMeta
	AppCacheGeneration                             uint64
	Preview                                        ScreenPreview
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
	ExportApps(serial string, packages []string, destination string, singleFile bool)
	AppBackup(serial, pkg, localDir string)
	AppUninstall(serial, pkg string, system bool)

	// Terminal opens an interactive shell or a logcat stream on a pseudo
	// terminal. The returned Terminal is owned by the caller; failures return
	// a nil interface, never a typed-nil pointer.
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

// ReadySessionHost optionally receives readiness after Attach and any app
// handoff have completed, so controls cannot restart an unfinished launch.
type ReadySessionHost interface {
	Ready()
}

// HostedBackend runs scrcpy beside an island control window.
type HostedBackend interface {
	StartHosted(serial, session string, options Options, display *VirtualDisplay, record bool, host SessionHost)
	Key(serial, action string)
}

// ForegroundBackend resolves the main display's foreground app off the UI thread.
// The callback is called exactly once and must not access UI state directly.
type ForegroundBackend interface {
	ResolveForeground(serial string, done func(string, error))
}
