//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows/registry"
)

// A few Win32 pieces that lxn/win does not expose.

// Accelerator key modifier flags (Win32 ACCEL.fVirt).
const (
	fVirtKey = 0x01
	fShift   = 0x04
	fControl = 0x08
	fAlt     = 0x10
)

// lparamPoint extracts signed x/y coordinates from an lParam.
func lparamPoint(lp unsafe.Pointer) (x, y int32) {
	v := uintptr(lp)
	x = int32(int16(v & 0xffff))
	y = int32(int16((v >> 16) & 0xffff))
	return
}

// accel mirrors the Win32 ACCEL structure.
type accel struct {
	FVirt byte
	Key   uint16
	Cmd   uint16
}

var (
	modUser32                   = syscall.NewLazyDLL("user32.dll")
	modGdi32                    = syscall.NewLazyDLL("gdi32.dll")
	procSetWindowTextW          = modUser32.NewProc("SetWindowTextW")
	procGetWindowTextLengthW    = modUser32.NewProc("GetWindowTextLengthW")
	procGetWindowTextW          = modUser32.NewProc("GetWindowTextW")
	procCreateAcceleratorTableW = modUser32.NewProc("CreateAcceleratorTableW")
	procTranslateAcceleratorW   = modUser32.NewProc("TranslateAcceleratorW")
	procGetDpiForWindow         = modUser32.NewProc("GetDpiForWindow")
	procFillRect                = modUser32.NewProc("FillRect")
	procSetWindowRgn            = modUser32.NewProc("SetWindowRgn")
	procCreateSolidBrush        = modGdi32.NewProc("CreateSolidBrush")
	procCreateRoundRectRgn      = modGdi32.NewProc("CreateRoundRectRgn")
)

// setWindowText sets a window/control's text.
func setWindowText(hwnd win.HWND, text string) {
	p, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	procSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(p)))
}

// getWindowText reads a control's text (e.g. the address bar).
func getWindowText(hwnd win.HWND) string {
	n, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
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

// fillRect paints a rectangle with a brush (no border).
func fillRect(hdc win.HDC, rc *win.RECT, brush win.HBRUSH) bool {
	r, _, _ := procFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(rc)), uintptr(brush))
	return r != 0
}

// createSolidBrush makes a brush for a RGB color.
func createSolidBrush(color win.COLORREF) win.HBRUSH {
	r, _, _ := procCreateSolidBrush.Call(uintptr(color))
	return win.HBRUSH(r)
}

// createRoundRectRgn builds a rounded-rectangle region - the shape behind
// every pill and circle in the UI.
func createRoundRectRgn(x1, y1, x2, y2, w, h int32) win.HRGN {
	r, _, _ := procCreateRoundRectRgn.Call(
		uintptr(int32ToU32(x1)), uintptr(int32ToU32(y1)),
		uintptr(int32ToU32(x2)), uintptr(int32ToU32(y2)),
		uintptr(int32ToU32(w)), uintptr(int32ToU32(h)))
	return win.HRGN(r)
}

// int32ToU32 converts an int32 coordinate to a uintptr for GDI calls.
func int32ToU32(v int32) uint32 {
	return uint32(int64(v) & 0xffffffff)
}

// setWindowRgn clips a window (e.g. the address bar) to a rounded shape.
// After a successful call the system owns the region.
func setWindowRgn(hwnd win.HWND, rgn win.HRGN, redraw bool) {
	procSetWindowRgn.Call(uintptr(hwnd), uintptr(rgn), boolToInt(redraw))
}

func boolToInt(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// isDarkMode reports whether Windows apps are themed dark.
func isDarkMode() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return false
	}
	return v == 0
}
