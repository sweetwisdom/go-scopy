//go:build windows

// Package windowhost manages scrcpy's native window and its independent controls.
package windowhost

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const Supported = true

var (
	user           = syscall.NewLazyDLL("user32.dll")
	enumerate      = user.NewProc("EnumWindows")
	getPID         = user.NewProc("GetWindowThreadProcessId")
	getText        = user.NewProc("GetWindowTextW")
	isVisible      = user.NewProc("IsWindowVisible")
	getStyle       = user.NewProc("GetWindowLongPtrW")
	setStyle       = user.NewProc("SetWindowLongPtrW")
	getRect        = user.NewProc("GetWindowRect")
	getDPI         = user.NewProc("GetDpiForWindow")
	position       = user.NewProc("SetWindowPos")
	cursorPosition = user.NewProc("GetCursorPos")
	monitorWindow  = user.NewProc("MonitorFromWindow")
	monitorInfo    = user.NewProc("GetMonitorInfoW")
	setRegion      = user.NewProc("SetWindowRgn")
	foreground     = user.NewProc("SetForegroundWindow")
	showWindow     = user.NewProc("ShowWindow")
	isIconic       = user.NewProc("IsIconic")
	gdi            = syscall.NewLazyDLL("gdi32.dll")
	roundRegion    = gdi.NewProc("CreateRoundRectRgn")
	deleteObject   = gdi.NewProc("DeleteObject")
	findMu         sync.Mutex
	findNext       atomic.Uint64
	findRequests   = map[uintptr]*findRequest{}
	findCallback   = syscall.NewCallback(func(hwnd, data uintptr) uintptr {
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
)

type findRequest struct {
	pid   uint32
	title string
	hwnd  uintptr
}
type rect struct{ left, top, right, bottom int32 }
type point struct{ x, y int32 }
type monitorDetails struct {
	size          uint32
	monitor, work rect
	flags         uint32
}

// Host observes the scrcpy process without parenting or changing its window.
// Control-window methods run on Viva's UI thread; Attach runs on a worker.
type Host struct {
	mu                sync.Mutex
	title             string
	target            uintptr
	dock              uintptr
	collapsed, placed bool
	anchorX, anchorY  int
	regionW, regionH  int32
	regionDPI         uintptr
	dragStart         point
	dragRect          rect
	dragging, moved   bool
}

func New(title string) *Host { return &Host{title: title} }

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
		if hwnd := find(uint32(pid), ""); hwnd != 0 {
			h.mu.Lock()
			h.target = hwnd
			h.mu.Unlock()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func workArea(hwnd uintptr) rect {
	m, _, _ := monitorWindow.Call(hwnd, 2)
	d := monitorDetails{size: uint32(unsafe.Sizeof(monitorDetails{}))}
	if ok, _, _ := monitorInfo.Call(m, uintptr(unsafe.Pointer(&d))); ok != 0 {
		return d.work
	}
	return rect{0, 0, 1920, 1080}
}

func dpiOf(hwnd uintptr) uintptr {
	dpi, _, _ := getDPI.Call(hwnd)
	if dpi == 0 {
		return 96
	}
	return dpi
}
func scaled(value int32, dpi uintptr) int32 { return int32((int64(value)*int64(dpi) + 48) / 96) }

// Dock sizes an independent, topmost island. Expanding preserves its screen edge.
func (h *Host) Dock(collapsed bool) {
	if h.dock == 0 {
		h.dock = find(uint32(os.Getpid()), h.title)
		if h.dock == 0 {
			return
		}
		style, _, _ := getStyle.Call(h.dock, ^uintptr(15))
		setStyle.Call(h.dock, ^uintptr(15), style&^uintptr(0x00CF0000))
		ex, _, _ := getStyle.Call(h.dock, ^uintptr(19))
		setStyle.Call(h.dock, ^uintptr(19), (ex&^uintptr(0x40000))|0x80)
	}
	if h.placed && h.collapsed == collapsed {
		h.RefreshDock()
		return
	}
	dpi := dpiOf(h.dock)
	w, height := int32(236), int32(48)
	if !collapsed {
		w, height = 344, 272
	}
	w, height = scaled(w, dpi), scaled(height, dpi)
	var current rect
	getRect.Call(h.dock, uintptr(unsafe.Pointer(&current)))
	work := workArea(h.dock)
	gap := scaled(12, dpi)
	x, y := (current.left+current.right-w)/2, current.top
	if !h.placed {
		x = work.left + (work.right-work.left-w)/2
		h.anchorY = -1
	}
	if h.anchorX == -1 {
		x = work.left + gap
	}
	if h.anchorX == 1 {
		x = work.right - gap - w
	}
	if h.anchorY == -1 {
		y = work.top + gap
	}
	if h.anchorY == 1 {
		y = work.bottom - gap - height
	}
	x = max(work.left, min(x, work.right-w))
	y = max(work.top, min(y, work.bottom-height))
	h.collapsed, h.placed = collapsed, true
	// HWND_TOPMOST, no activation, frame changed. No owner ties it to the phone.
	position.Call(h.dock, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(height), 0x0030)
	h.RefreshDock()
}

// RefreshDock also handles DPI changes while crossing monitors.
func (h *Host) RefreshDock() {
	if h.dock == 0 {
		return
	}
	var r rect
	getRect.Call(h.dock, uintptr(unsafe.Pointer(&r)))
	w, height, dpi := r.right-r.left, r.bottom-r.top, dpiOf(h.dock)
	if w == h.regionW && height == h.regionH && dpi == h.regionDPI {
		return
	}
	radius := scaled(28, dpi)
	if h.collapsed {
		radius = height / 2
	}
	// HRGN has binary coverage. Keep its coarse hit-test shape outside the
	// painted curve so it cannot cut off DirectComposition's alpha fringe.
	radius = max(radius-scaled(3, dpi), 1)
	region, _, _ := roundRegion.Call(0, 0, uintptr(w+1), uintptr(height+1), uintptr(radius*2), uintptr(radius*2))
	if region == 0 {
		return
	}
	if ok, _, _ := setRegion.Call(h.dock, region, 1); ok == 0 {
		deleteObject.Call(region)
		return
	}
	h.regionW, h.regionH, h.regionDPI = w, height, dpi
}

func (h *Host) BeginDrag() {
	if h.dock == 0 {
		return
	}
	cursorPosition.Call(uintptr(unsafe.Pointer(&h.dragStart)))
	getRect.Call(h.dock, uintptr(unsafe.Pointer(&h.dragRect)))
	h.dragging, h.moved = true, false
}
func (h *Host) MoveDrag() {
	if !h.dragging {
		return
	}
	var cursor point
	cursorPosition.Call(uintptr(unsafe.Pointer(&cursor)))
	dx, dy := cursor.x-h.dragStart.x, cursor.y-h.dragStart.y
	threshold := scaled(4, dpiOf(h.dock))
	if !h.moved && abs32(dx) < threshold && abs32(dy) < threshold {
		return
	}
	h.moved = true
	position.Call(h.dock, 0, uintptr(h.dragRect.left+dx), uintptr(h.dragRect.top+dy), 0, 0, 0x0015)
}
func (h *Host) EndDrag() {
	if !h.dragging {
		return
	}
	h.MoveDrag()
	h.dragging = false
	if !h.moved {
		return
	}
	var r rect
	getRect.Call(h.dock, uintptr(unsafe.Pointer(&r)))
	dpi := dpiOf(h.dock)
	r, h.anchorX, h.anchorY = snapRect(r, workArea(h.dock), scaled(24, dpi), scaled(12, dpi))
	position.Call(h.dock, 0, uintptr(r.left), uintptr(r.top), 0, 0, 0x0015)
	h.RefreshDock()
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
func snapRect(r, work rect, threshold, gap int32) (rect, int, int) {
	w, height := r.right-r.left, r.bottom-r.top
	x := max(work.left, min(r.left, work.right-w))
	y := max(work.top, min(r.top, work.bottom-height))
	ax, ay := 0, 0
	if abs32(x-work.left) <= threshold {
		x, ax = work.left+gap, -1
	} else if abs32(work.right-x-w) <= threshold {
		x, ax = work.right-gap-w, 1
	}
	if abs32(y-work.top) <= threshold {
		y, ay = work.top+gap, -1
	} else if abs32(work.bottom-y-height) <= threshold {
		y, ay = work.bottom-gap-height, 1
	}
	return rect{x, y, x + w, y + height}, ax, ay
}

func (h *Host) Focus() {
	h.mu.Lock()
	target := h.target
	h.mu.Unlock()
	if target == 0 {
		return
	}
	if minimized, _, _ := isIconic.Call(target); minimized != 0 {
		showWindow.Call(target, 9)
	}
	foreground.Call(target)
}
func (h *Host) Release() {
	h.mu.Lock()
	h.target = 0
	h.mu.Unlock()
	h.dock, h.dragging = 0, false
}
