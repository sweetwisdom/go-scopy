//go:build windows

package desktop

// File-dialog COM flow adapted from Viva v0.2.3 (Apache-2.0, Copyright 2026
// The Viva Authors; see licenses/Viva-Apache-2.0.txt). This narrow adapter adds
// IFileDialog.SetFolder, which Viva's public options do not currently expose.
import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/vivaui/viva/app"
	"golang.org/x/sys/windows"
)

var exportOle = windows.NewLazySystemDLL("ole32.dll")
var exportShell = windows.NewLazySystemDLL("shell32.dll")
var exportUser = windows.NewLazySystemDLL("user32.dll")
var exportCOMInitialize = exportOle.NewProc("CoInitializeEx")
var exportCOMUninitialize = exportOle.NewProc("CoUninitialize")
var exportCOMCreate = exportOle.NewProc("CoCreateInstance")
var exportCOMFree = exportOle.NewProc("CoTaskMemFree")
var exportShellItem = exportShell.NewProc("SHCreateItemFromParsingName")
var exportDialogOwner = exportUser.NewProc("GetActiveWindow")

// Both dialogs are the Windows common item dialog, not a custom folder UI.
func pickAppExportDestination(initialDir, title, filename string, single, native bool) (string, error) {
	if !native || os.Getenv("VIVA_HEADLESS") == "1" || initialDir == "" {
		return basicAppExportPicker(title, filename, single)
	}
	if info, err := os.Stat(initialDir); err != nil || !info.IsDir() {
		return basicAppExportPicker(title, filename, single)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	status, _, _ := exportCOMInitialize.Call(0, 2)
	if uint32(status) != 0 && uint32(status) != 1 {
		return "", exportDialogError(status)
	}
	defer exportCOMUninitialize.Call()
	classID, _ := windows.GUIDFromString("{DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}")
	interfaceID, _ := windows.GUIDFromString("{D57C7288-D4AD-4768-BE02-9D969532D960}")
	flags := uint32(0x40 | 0x800 | 0x8 | 0x20) // filesystem, existing path, no chdir, folders
	if single {
		classID, _ = windows.GUIDFromString("{C0B4E2F3-BA21-4773-8DBA-335EC946EB8B}")
		interfaceID, _ = windows.GUIDFromString("{84BCCD23-5FDE-4CDB-AEA4-AF64B83D78AB}")
		flags = 0x40 | 0x800 | 0x8 | 0x2 // save requires the normal overwrite prompt
	}
	var dialog *exportDialogCOM
	status, _, _ = exportCOMCreate.Call(uintptr(unsafe.Pointer(&classID)), 0, 1, uintptr(unsafe.Pointer(&interfaceID)), uintptr(unsafe.Pointer(&dialog)))
	if err := exportDialogError(status); err != nil {
		return "", err
	}
	defer dialog.release()
	var current uint32
	if err := exportDialogError(dialog.call(10, uintptr(unsafe.Pointer(&current)))); err != nil {
		return "", err
	}
	if err := exportDialogError(dialog.call(9, uintptr(current|flags))); err != nil {
		return "", err
	}
	titleText, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return "", err
	}
	if err := exportDialogError(dialog.call(17, uintptr(unsafe.Pointer(titleText)))); err != nil {
		return "", err
	}
	pathText, err := windows.UTF16PtrFromString(initialDir)
	if err != nil {
		return "", err
	}
	itemID, _ := windows.GUIDFromString("{43826D1E-E718-42EE-BC55-A1E261C37BFE}")
	var folder *exportDialogCOM
	status, _, _ = exportShellItem.Call(uintptr(unsafe.Pointer(pathText)), 0, uintptr(unsafe.Pointer(&itemID)), uintptr(unsafe.Pointer(&folder)))
	if err := exportDialogError(status); err != nil {
		return "", err
	}
	defer folder.release()
	if err := exportDialogError(dialog.call(12, uintptr(unsafe.Pointer(folder)))); err != nil {
		return "", err
	} // SetFolder
	if single {
		name, err := windows.UTF16PtrFromString(filename)
		if err != nil {
			return "", err
		}
		if err := exportDialogError(dialog.call(15, uintptr(unsafe.Pointer(name)))); err != nil {
			return "", err
		}
		extension, _ := windows.UTF16PtrFromString("apk")
		if err := exportDialogError(dialog.call(22, uintptr(unsafe.Pointer(extension)))); err != nil {
			return "", err
		}
		filterName, _ := windows.UTF16PtrFromString("Android 安装包")
		pattern, _ := windows.UTF16PtrFromString("*.apk")
		filter := struct{ Name, Pattern *uint16 }{filterName, pattern}
		if err := exportDialogError(dialog.call(4, 1, uintptr(unsafe.Pointer(&filter)))); err != nil {
			return "", err
		}
	}
	owner, _, _ := exportDialogOwner.Call()
	status = dialog.call(3, owner)
	if uint32(status) == 0x800704c7 {
		return "", app.ErrCanceled
	}
	if err := exportDialogError(status); err != nil {
		return "", err
	}
	var result *exportDialogCOM
	if err := exportDialogError(dialog.call(20, uintptr(unsafe.Pointer(&result)))); err != nil {
		return "", err
	}
	defer result.release()
	var value *uint16
	if err := exportDialogError(result.call(5, 0x80058000, uintptr(unsafe.Pointer(&value)))); err != nil {
		return "", err
	}
	defer exportCOMFree.Call(uintptr(unsafe.Pointer(value)))
	return windows.UTF16PtrToString(value), nil
}

type exportDialogCOM struct{ vtable *[32]uintptr }

//go:uintptrescapes
func (object *exportDialogCOM) call(index int, args ...uintptr) uintptr {
	var values [8]uintptr
	values[0] = uintptr(unsafe.Pointer(object))
	copy(values[1:], args)
	status, _, _ := syscall.SyscallN(object.vtable[index], values[:len(args)+1]...)
	return status
}

func (object *exportDialogCOM) release() {
	if object != nil {
		object.call(2)
	}
}
func exportDialogError(status uintptr) error {
	if int32(uint32(status)) < 0 {
		return fmt.Errorf("Windows 导出位置选择失败 (HRESULT %#x)", uint32(status))
	}
	return nil
}
