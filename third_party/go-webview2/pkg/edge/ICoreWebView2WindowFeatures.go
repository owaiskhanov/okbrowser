package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// OK Browser addition: ICoreWebView2WindowFeatures - the position, size and
// chrome the page asked for in window.open()'s third argument. Google's
// OAuth popup requests an explicit width/height, and honoring it is part of
// looking like a real popup window.
//
// Vtable order follows the WebView2 SDK IDL:
//   get_HasPosition, get_HasSize, get_Left, get_Top, get_Height, get_Width,
//   get_ShouldDisplayMenuBar, get_ShouldDisplayStatus,
//   get_ShouldDisplayToolbar, get_ShouldDisplayScrollBars

type _ICoreWebView2WindowFeaturesVtbl struct {
	_IUnknownVtbl
	GetHasPosition             ComProc
	GetHasSize                 ComProc
	GetLeft                    ComProc
	GetTop                     ComProc
	GetHeight                  ComProc
	GetWidth                   ComProc
	GetShouldDisplayMenuBar    ComProc
	GetShouldDisplayStatus     ComProc
	GetShouldDisplayToolbar    ComProc
	GetShouldDisplayScrollBars ComProc
}

type ICoreWebView2WindowFeatures struct {
	vtbl *_ICoreWebView2WindowFeaturesVtbl
}

func (i *ICoreWebView2WindowFeatures) getBool(p ComProc) bool {
	var v int32 // COM BOOL is 32-bit
	_, _, err := p.Call(uintptr(unsafe.Pointer(i)), uintptr(unsafe.Pointer(&v)))
	if err != windows.ERROR_SUCCESS {
		return false
	}
	return v != 0
}

func (i *ICoreWebView2WindowFeatures) getUint32(p ComProc) uint32 {
	var v uint32
	_, _, err := p.Call(uintptr(unsafe.Pointer(i)), uintptr(unsafe.Pointer(&v)))
	if err != windows.ERROR_SUCCESS {
		return 0
	}
	return v
}

// HasPosition reports whether the page requested left/top coordinates.
func (i *ICoreWebView2WindowFeatures) HasPosition() bool {
	return i.getBool(i.vtbl.GetHasPosition)
}

// HasSize reports whether the page requested a width/height.
func (i *ICoreWebView2WindowFeatures) HasSize() bool { return i.getBool(i.vtbl.GetHasSize) }

func (i *ICoreWebView2WindowFeatures) Left() uint32   { return i.getUint32(i.vtbl.GetLeft) }
func (i *ICoreWebView2WindowFeatures) Top() uint32    { return i.getUint32(i.vtbl.GetTop) }
func (i *ICoreWebView2WindowFeatures) Width() uint32  { return i.getUint32(i.vtbl.GetWidth) }
func (i *ICoreWebView2WindowFeatures) Height() uint32 { return i.getUint32(i.vtbl.GetHeight) }

// Release drops the reference obtained from GetWindowFeatures.
func (i *ICoreWebView2WindowFeatures) Release() {
	_, _, _ = i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))
}
