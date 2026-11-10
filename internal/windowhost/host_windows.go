//go:build windows

// Package windowhost embeds scrcpy's SDL window in a native Viva container.
package windowhost

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const Supported = true

var (
	user          = syscall.NewLazyDLL("user32.dll")
	enumerate     = user.NewProc("EnumWindows")
	getPID        = user.NewProc("GetWindowThreadProcessId")
	getText       = user.NewProc("GetWindowTextW")
	isVisible     = user.NewProc("IsWindowVisible")
	getStyle      = user.NewProc("GetWindowLongPtrW")
	setStyle      = user.NewProc("SetWindowLongPtrW")
	setParent     = user.NewProc("SetParent")
	getParent     = user.NewProc("GetParent")
	getClient     = user.NewProc("GetClientRect")
	getRect       = user.NewProc("GetWindowRect")
	toClient      = user.NewProc("ScreenToClient")
	getDPI        = user.NewProc("GetDpiForWindow")
	position      = user.NewProc("SetWindowPos")
	isZoomed      = user.NewProc("IsZoomed")
	clientScreen  = user.NewProc("ClientToScreen")
	monitorWindow = user.NewProc("MonitorFromWindow")
	monitorInfo   = user.NewProc("GetMonitorInfoW")
	setRegion     = user.NewProc("SetWindowRgn")
	gdi           = syscall.NewLazyDLL("gdi32.dll")
	roundRegion   = gdi.NewProc("CreateRoundRectRgn")
	combineRegion = gdi.NewProc("CombineRgn")
	deleteObject  = gdi.NewProc("DeleteObject")
	setFocus      = user.NewProc("SetFocus")
	attachInput   = user.NewProc("AttachThreadInput")
	currentThread = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")
	setHook       = user.NewProc("SetWinEventHook")
	unhook        = user.NewProc("UnhookWinEvent")
	hookMu        sync.Mutex
	hooks         = map[uintptr]*Host{}
	findMu        sync.Mutex
	findNext      atomic.Uint64
	findRequests  = map[uintptr]*findRequest{}
	findCallback  = syscall.NewCallback(func(hwnd, data uintptr) uintptr {
		findMu.Lock()
		r := findRequests[data]
		findMu.Unlock()
		if r == nil {
			return 0
		}
		var pid uint32
		getPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != r.pid {
			return 1
		}
		if r.title != "" {
			var title [512]uint16
			getText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
			if syscall.UTF16ToString(title[:]) != r.title {
				return 1
			}
		} else if visible, _, _ := isVisible.Call(hwnd); visible == 0 {
			return 1
		}
		r.hwnd = hwnd
		return 0
	})
	locationCallback = syscall.NewCallback(func(hook, event, hwnd, object, child, thread, stamp uintptr) uintptr {
		hookMu.Lock()
		host := hooks[hook]
		hookMu.Unlock()
		if host != nil && object == 0 {
			host.mu.Lock()
			matches := host.child == hwnd
			host.mu.Unlock()
			if matches {
				host.updateOrientation()
				host.Resize()
			}
		}
		return 0
	})
)

type findRequest struct {
	pid   uint32
	title string
	hwnd  uintptr
}
type rect struct{ left, top, right, bottom int32 }
type point struct{ x, y int32 }

// Host is safe to resize from a worker while attachment is in progress.
// Sync and Release must run on Viva's window thread for WinEvent delivery.
type Host struct {
	mu                  sync.Mutex
	title               string
	top                 int
	parent, child, hook uintptr
	pid                 uint32
	topmost             bool
	videoW, videoH      int32
	layoutW, layoutH    int32
	childW, childH      int32
	dock                uintptr
	dockTitle           string
	collapsed           bool
	regionDPI           uintptr
	regionCollapsed     bool
}

func New(title string, toolbarHeight int, topmost bool) *Host {
	return &Host{title: title, top: toolbarHeight, topmost: topmost}
}

func find(pid uint32, title string) uintptr {
	r := findRequest{pid: pid, title: title}
	id := uintptr(findNext.Add(1))
	findMu.Lock()
	findRequests[id] = &r
	findMu.Unlock()
	enumerate.Call(findCallback, id)
	findMu.Lock()
	delete(findRequests, id)
	findMu.Unlock()
	return r.hwnd
}

func (h *Host) Attach(ctx context.Context, pid int) error {
	timer := time.NewTicker(50 * time.Millisecond)
	defer timer.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		parent, child := find(uint32(os.Getpid()), h.title), find(uint32(pid), "")
		if parent != 0 && child != 0 {
			var original rect
			getClient.Call(child, uintptr(unsafe.Pointer(&original)))
			style, _, _ := getStyle.Call(child, ^uintptr(15))                           // GWL_STYLE = -16
			setStyle.Call(child, ^uintptr(15), (style&^uintptr(0x80C40000))|0x40000000) // WS_CHILD
			ex, _, _ := getStyle.Call(child, ^uintptr(19))                              // GWL_EXSTYLE = -20
			setStyle.Call(child, ^uintptr(19), ex&^uintptr(0x40000))                    // WS_EX_APPWINDOW
			setParent.Call(child, parent)
			if actual, _, _ := getParent.Call(child); actual != parent {
				return errors.New("Windows 未能将 scrcpy 嵌入投屏窗口")
			}
			parentStyle, _, _ := getStyle.Call(parent, ^uintptr(15))
			setStyle.Call(parent, ^uintptr(15), parentStyle|0x02000000) // WS_CLIPCHILDREN
			h.mu.Lock()
			h.parent, h.child, h.pid = parent, child, uint32(pid)
			h.videoW, h.videoH = original.right, original.bottom
			h.mu.Unlock()
			h.Resize()
			if h.topmost {
				position.Call(parent, ^uintptr(0), 0, 0, 0, 0, 0x0013)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Resize uses native pixels so the video and toolbar agree at any display DPI.
func (h *Host) Resize() {
	h.fit()
	h.followDock()
	h.mu.Lock()
	parent, child, top := h.parent, h.child, h.top
	h.mu.Unlock()
	if parent == 0 || child == 0 {
		return
	}
	var area, actual rect
	if ok, _, _ := getClient.Call(parent, uintptr(unsafe.Pointer(&area))); ok == 0 {
		return
	}
	dpi, _, _ := getDPI.Call(parent)
	if dpi == 0 {
		dpi = 96
	}
	y := int32((top*int(dpi) + 48) / 96)
	width, height := area.right, max(area.bottom-y, 1)
	getRect.Call(child, uintptr(unsafe.Pointer(&actual)))
	origin := point{actual.left, actual.top}
	toClient.Call(parent, uintptr(unsafe.Pointer(&origin)))
	if origin.x == 0 && origin.y == y && actual.right-actual.left == width && actual.bottom-actual.top == height {
		return
	}
	h.mu.Lock()
	h.childW, h.childH = width, height
	h.mu.Unlock()
	position.Call(child, 0, 0, uintptr(y), uintptr(width), uintptr(height), 0x4070) // async, show, frame changed, no activate
}

// fit retains the video's aspect ratio while allowing either edge to resize it.
func (h *Host) fit() {
	h.mu.Lock()
	parent, vw, vh, lastW, lastH := h.parent, h.videoW, h.videoH, h.layoutW, h.layoutH
	h.mu.Unlock()
	if parent == 0 || vw <= 0 || vh <= 0 {
		return
	}
	if zoomed, _, _ := isZoomed.Call(parent); zoomed != 0 {
		return
	}
	var client, outer rect
	getClient.Call(parent, uintptr(unsafe.Pointer(&client)))
	getRect.Call(parent, uintptr(unsafe.Pointer(&outer)))
	w, height := client.right, client.bottom
	if w <= 0 || height <= 0 {
		return
	}
	nw, nh := int32(float64(height)*float64(vw)/float64(vh)+0.5), height
	if lastW > 0 && w != lastW && height == lastH {
		nw = w
		nh = int32(float64(w)*float64(vh)/float64(vw) + 0.5)
	}
	extraW, extraH := outer.right-outer.left-w, outer.bottom-outer.top-height
	work := workArea(parent)
	maxW, maxH := max(work.right-work.left-extraW-88, 120), max(work.bottom-work.top-extraH-16, 180)
	if nw > maxW {
		nw = maxW
		nh = int32(float64(nw)*float64(vh)/float64(vw) + 0.5)
	}
	if nh > maxH {
		nh = maxH
		nw = int32(float64(nh)*float64(vw)/float64(vh) + 0.5)
	}
	h.mu.Lock()
	h.layoutW, h.layoutH = nw, nh
	h.mu.Unlock()
	if nw != w || nh != height {
		position.Call(parent, 0, 0, 0, uintptr(nw+extraW), uintptr(nh+extraH), 0x0016)
	}
}

func (h *Host) updateOrientation() {
	h.mu.Lock()
	child, cw, ch, vw, vh := h.child, h.childW, h.childH, h.videoW, h.videoH
	h.mu.Unlock()
	if child == 0 {
		return
	}
	var r rect
	getClient.Call(child, uintptr(unsafe.Pointer(&r)))
	if r.right > 0 && r.bottom > 0 && (r.right != cw || r.bottom != ch) && (r.right > r.bottom) != (vw > vh) {
		h.mu.Lock()
		h.videoW, h.videoH = r.right, r.bottom
		h.layoutW, h.layoutH = 0, 0
		h.mu.Unlock()
	}
}

type monitorDetails struct {
	size          uint32
	monitor, work rect
	flags         uint32
}

func workArea(hwnd uintptr) rect {
	m, _, _ := monitorWindow.Call(hwnd, 2)
	d := monitorDetails{size: uint32(unsafe.Sizeof(monitorDetails{}))}
	if ok, _, _ := monitorInfo.Call(m, uintptr(unsafe.Pointer(&d))); ok != 0 {
		return d.work
	}
	return rect{0, 0, 1920, 1080}
}

// Dock uses an owned tool window: it follows its video window and stays out of
// the taskbar. Its region contains only the rounded buttons, leaving gaps clear.
func (h *Host) Dock(title string, collapsed bool) {
	dock := find(uint32(os.Getpid()), title)
	parent := find(uint32(os.Getpid()), h.title)
	if dock == 0 || parent == 0 {
		return
	}
	setStyle.Call(dock, ^uintptr(7), parent) // GWLP_HWNDPARENT: owner, not child
	style, _, _ := getStyle.Call(dock, ^uintptr(15))
	setStyle.Call(dock, ^uintptr(15), style&^uintptr(0x00C40000))
	ex, _, _ := getStyle.Call(dock, ^uintptr(19))
	setStyle.Call(dock, ^uintptr(19), (ex&^uintptr(0x40000))|0x80)
	h.mu.Lock()
	h.dock, h.dockTitle, h.collapsed = dock, title, collapsed
	h.mu.Unlock()
	h.followDock()
}

func (h *Host) followDock() {
	h.mu.Lock()
	parent, dock, collapsed := h.parent, h.dock, h.collapsed
	h.mu.Unlock()
	if dock == 0 {
		return
	}
	if parent == 0 {
		parent = find(uint32(os.Getpid()), h.title)
	}
	if parent == 0 {
		return
	}
	dpi, _, _ := getDPI.Call(parent)
	if dpi == 0 {
		dpi = 96
	}
	scale := func(v int32) int32 { return int32((int64(v)*int64(dpi) + 48) / 96) }
	w, rows := int32(76), 11
	if collapsed {
		w, rows = 52, 1
	}
	height := int32(12 + rows*40 + (rows-1)*6)
	var outer rect
	getRect.Call(parent, uintptr(unsafe.Pointer(&outer)))
	origin := point{}
	clientScreen.Call(parent, uintptr(unsafe.Pointer(&origin)))
	work := workArea(parent)
	x, y := outer.right+scale(6), origin.y+scale(6)
	if x+scale(w) > work.right {
		x = outer.right - scale(w) - scale(6)
	}
	y = max(work.top, min(y, work.bottom-scale(height)))
	position.Call(dock, 0, uintptr(x), uintptr(y), uintptr(scale(w)), uintptr(scale(height)), 0x0010)
	h.mu.Lock()
	rebuild := h.regionDPI != dpi || h.regionCollapsed != collapsed
	h.regionDPI, h.regionCollapsed = dpi, collapsed
	h.mu.Unlock()
	if !rebuild {
		return
	}
	region, _, _ := roundRegion.Call(0, 0, 0, 0, 0, 0)
	for i := 0; i < rows; i++ {
		top := int32(6 + i*46)
		part, _, _ := roundRegion.Call(uintptr(scale(6)), uintptr(scale(top)), uintptr(scale(w-6)+1), uintptr(scale(top+40)+1), uintptr(scale(16)), uintptr(scale(16)))
		combineRegion.Call(region, region, part, 2)
		deleteObject.Call(part)
	}
	if ok, _, _ := setRegion.Call(dock, region, 1); ok == 0 {
		deleteObject.Call(region)
	}
}

// Sync installs an event-driven size guard on the UI thread. SDL may otherwise
// change its child window size when the phone rotates or a resize shortcut runs.
func (h *Host) Sync() {
	h.mu.Lock()
	pid, child, existing := h.pid, h.child, h.hook
	h.mu.Unlock()
	if child == 0 {
		return
	}
	if existing == 0 {
		hook, _, _ := setHook.Call(0x800B, 0x800B, 0, locationCallback, uintptr(pid), 0, 0)
		if hook != 0 {
			hookMu.Lock()
			hooks[hook] = h
			hookMu.Unlock()
			h.mu.Lock()
			h.hook = hook
			h.mu.Unlock()
		}
	}
	h.Resize()
}

func (h *Host) Focus() {
	h.mu.Lock()
	child := h.child
	h.mu.Unlock()
	if child == 0 {
		return
	}
	thread, _, _ := getPID.Call(child, 0)
	current, _, _ := currentThread.Call()
	if thread != current {
		attachInput.Call(current, thread, 1)
		defer attachInput.Call(current, thread, 0)
	}
	setFocus.Call(child)
}

func (h *Host) Release() {
	h.mu.Lock()
	hook := h.hook
	h.hook, h.child, h.parent = 0, 0, 0
	h.mu.Unlock()
	if hook != 0 {
		unhook.Call(hook)
		hookMu.Lock()
		delete(hooks, hook)
		hookMu.Unlock()
	}
}
