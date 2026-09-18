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

// procDwmExtendFrameIntoClientArea keeps the DWM frame (and with it the
// soft drop shadow) alive for the borderless window.
var procDwmExtendFrameIntoClientArea = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmExtendFrameIntoClientArea")

// dwmExtendFrame applies the classic 1px DWM frame extension so a
// borderless window keeps its drop shadow.
func dwmExtendFrame(hwnd win.HWND) {
	m := struct{ L, R, T, B int32 }{1, 1, 1, 1}
	_, _, _ = procDwmExtendFrameIntoClientArea.Call(
		uintptr(hwnd), uintptr(unsafe.Pointer(&m)))
}

// placementMaximized reports the committed maximize state. IsZoomed can
// momentarily disagree with the actual placement during window-state
// transitions, which must never leak into WM_NCCALCSIZE geometry.
func placementMaximized(hwnd win.HWND) bool {
	var wp win.WINDOWPLACEMENT
	wp.Length = uint32(unsafe.Sizeof(wp))
	if !win.GetWindowPlacement(hwnd, &wp) {
		return false
	}
	return wp.ShowCmd == win.SW_SHOWMAXIMIZED
}

// smCXPaddedBorder is the GetSystemMetrics index of SM_CXPADDEDBORDER -
// the invisible padding added to the resize frame. The ID is 92; the value
// it returns is only a few pixels and must never be used as a pixel count.
const smCXPaddedBorder = 92

// ncCalcSizeParams mirrors the Win32 NCCALCSIZE_PARAMS structure.
type ncCalcSizeParams struct {
	Rc0, Rc1, Rc2 win.RECT
	Lppos         uintptr // WINDOWPOS*
}

// procCreateSolidBrush makes the dark window-class brush (gdi32).
var procCreateSolidBrush = syscall.NewLazyDLL("gdi32.dll").NewProc("CreateSolidBrush")

// darkBrush returns the brush used to erase tab-host (and main) windows:
// dark, so a shown-but-unpainted window can never flash white.
func darkBrush() win.HBRUSH {
	c, _, _ := procCreateSolidBrush.Call(0x001E1C1C) // COLORREF 0x00BBGGRR = #1C1C1E
	return win.HBRUSH(c)
}

// procSetLayeredWindowAttributes and procIsWindow cover two user32 calls
// lxn/win does not expose (whole-window alpha and window liveness).
var (
	procSetLayeredWindowAttributes = syscall.NewLazyDLL("user32.dll").NewProc("SetLayeredWindowAttributes")
	procIsWindow                   = syscall.NewLazyDLL("user32.dll").NewProc("IsWindow")
)

// isWnd reports whether the window handle is still a live window.
func isWnd(h win.HWND) bool {
	if h == 0 {
		return false
	}
	r, _, _ := procIsWindow.Call(uintptr(h))
	return r != 0
}

// setLayeredAlpha sets the whole-window alpha of a layered window
// (LWA_ALPHA). Returns false when the platform refuses (fade unsupported).
func setLayeredAlpha(hwnd win.HWND, alpha byte) bool {
	r, _, _ := procSetLayeredWindowAttributes.Call(uintptr(hwnd), 0, uintptr(alpha), 2 /*LWA_ALPHA*/)
	return r != 0
}

// unlayered removes the layered style so the window renders at full speed.
func unlayered(hwnd win.HWND) {
	if ex := win.GetWindowLong(hwnd, win.GWL_EXSTYLE); ex&win.WS_EX_LAYERED != 0 {
		win.SetWindowLong(hwnd, win.GWL_EXSTYLE, ex&^win.WS_EX_LAYERED)
		win.SetWindowPos(hwnd, 0, 0, 0, 0, 0,
			win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_NOACTIVATE|win.SWP_FRAMECHANGED)
	}
}

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
