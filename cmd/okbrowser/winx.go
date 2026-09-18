//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

// A few Win32 pieces that lxn/win does not expose.

// Accelerator key modifier flags (Win32 ACCEL.fVirt).
const (
	fVirtKey = 0x01
	fShift   = 0x04
	fControl = 0x08
	fAlt     = 0x10
)

// accel mirrors the Win32 ACCEL structure.
type accel struct {
	FVirt byte
	Key   uint16
	Cmd   uint16
}

var (
	modUser32                   = syscall.NewLazyDLL("user32.dll")
	procSetWindowTextW          = modUser32.NewProc("SetWindowTextW")
	procGetAsyncKeyState        = modUser32.NewProc("GetAsyncKeyState")
	procCreateAcceleratorTableW = modUser32.NewProc("CreateAcceleratorTableW")
	procTranslateAcceleratorW   = modUser32.NewProc("TranslateAcceleratorW")
	procGetDpiForWindow         = modUser32.NewProc("GetDpiForWindow")
)

// keyDown reports whether the virtual key is currently pressed. Used to read
// modifier state inside the engine's accelerator-key callback.
func keyDown(vk uintptr) bool {
	r, _, _ := procGetAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}

// setWindowText sets a window's text (used for the window title).
func setWindowText(hwnd win.HWND, text string) {
	p, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	procSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(p)))
}

// createAcceleratorTable builds a Win32 accelerator (hotkey) table.
func createAcceleratorTable(accels []accel) win.HACCEL {
	if len(accels) == 0 {
		return 0
	}
	r, _, _ := procCreateAcceleratorTableW.Call(
		uintptr(unsafe.Pointer(&accels[0])), uintptr(len(accels)))
	return win.HACCEL(r)
}

// translateAccelerator returns non-zero when the message was a hotkey that
// has been translated into a WM_COMMAND for the window.
func translateAccelerator(hwnd win.HWND, table win.HACCEL, msg *win.MSG) int {
	r, _, _ := procTranslateAcceleratorW.Call(
		uintptr(hwnd), uintptr(table), uintptr(unsafe.Pointer(msg)))
	return int(r)
}

// dpiOf returns the DPI of the given window, or of the primary monitor when
// hwnd is 0. Falls back gracefully on very old systems.
func dpiOf(hwnd win.HWND) int {
	// GetDpiForWindow exists on Windows 10 1607+.
	if procGetDpiForWindow.Find() == nil {
		if r, _, _ := procGetDpiForWindow.Call(uintptr(hwnd)); r != 0 {
			return int(r)
		}
	}
	dc := win.GetDC(hwnd)
	if dc == 0 {
		return 96
	}
	defer win.ReleaseDC(hwnd, dc)
	if v := win.GetDeviceCaps(dc, win.LOGPIXELSY); v > 0 {
		return int(v)
	}
	return 96
}
