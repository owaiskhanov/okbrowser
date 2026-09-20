package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// OK Browser addition: ICoreWebView2WindowFeatures.
//
// Vtable order follows the WebView2 SDK IDL exactly:
//
//	get_HasPosition, get_HasSize, get_Left, get_Top, get_Height, get_Width,
//	get_ShouldDisplayMenuBar, get_ShouldDisplayStatus,
//	get_ShouldDisplayToolbar, get_ShouldDisplayScrollBars
//
// Height comes BEFORE Width - the opposite of the usual (width, height)
// convention, and of the order the MS Learn summary table lists them in.
// Swapping them would silently produce popups with the two axes exchanged.

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

func (i *ICoreWebView2WindowFeatures) Release() error {
	if i == nil {
		return nil
	}
	_, _, err := i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

func (i *ICoreWebView2WindowFeatures) getBool(p ComProc) (bool, error) {
	if i == nil {
		return false, nil
	}
	var v int32 // COM BOOL is 32-bit
	_, _, err := p.Call(uintptr(unsafe.Pointer(i)), uintptr(unsafe.Pointer(&v)))
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return v != 0, nil
}

func (i *ICoreWebView2WindowFeatures) getU32(p ComProc) (uint32, error) {
	if i == nil {
		return 0, nil
	}
	var v uint32
	_, _, err := p.Call(uintptr(unsafe.Pointer(i)), uintptr(unsafe.Pointer(&v)))
	if err != windows.ERROR_SUCCESS {
		return 0, err
	}
	return v, nil
}

// HasPosition reports whether the page asked for a specific left/top.
func (i *ICoreWebView2WindowFeatures) HasPosition() (bool, error) {
	return i.getBool(i.vtbl.GetHasPosition)
}

// HasSize reports whether the page asked for a specific width/height.
func (i *ICoreWebView2WindowFeatures) HasSize() (bool, error) {
	return i.getBool(i.vtbl.GetHasSize)
}

func (i *ICoreWebView2WindowFeatures) Left() (uint32, error) {
	return i.getU32(i.vtbl.GetLeft)
}

func (i *ICoreWebView2WindowFeatures) Top() (uint32, error) {
	return i.getU32(i.vtbl.GetTop)
}

func (i *ICoreWebView2WindowFeatures) Height() (uint32, error) {
	return i.getU32(i.vtbl.GetHeight)
}

func (i *ICoreWebView2WindowFeatures) Width() (uint32, error) {
	return i.getU32(i.vtbl.GetWidth)
}
