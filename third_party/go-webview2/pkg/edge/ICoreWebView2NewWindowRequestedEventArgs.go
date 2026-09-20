package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// OK Browser addition: ICoreWebView2NewWindowRequestedEventArgs.
// Vtable order follows the WebView2 SDK:
//   get_Uri, put_NewWindow, get_NewWindow, put_Handled, get_Handled,
//   get_IsUserInitiated, GetDeferral

type _ICoreWebView2NewWindowRequestedEventArgsVtbl struct {
	_IUnknownVtbl
	GetUri             ComProc
	PutNewWindow       ComProc
	GetNewWindow       ComProc
	PutHandled         ComProc
	GetHandled         ComProc
	GetIsUserInitiated ComProc
	GetDeferral        ComProc
	GetWindowFeatures  ComProc
}

type ICoreWebView2NewWindowRequestedEventArgs struct {
	vtbl *_ICoreWebView2NewWindowRequestedEventArgsVtbl
}

// GetUri returns the target URI of the requested new window.
func (i *ICoreWebView2NewWindowRequestedEventArgs) GetUri() (string, error) {
	var _uri *uint16
	_, _, err := i.vtbl.GetUri.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&_uri)),
	)
	if err != windows.ERROR_SUCCESS {
		return "", err
	}
	if _uri == nil {
		return "", nil
	}
	uri := windows.UTF16PtrToString(_uri)
	windows.CoTaskMemFree(unsafe.Pointer(_uri))
	return uri, nil
}

// PutHandled suppresses the engine's default handling of the new-window
// request (the host takes care of it instead).
func (i *ICoreWebView2NewWindowRequestedEventArgs) PutHandled(handled bool) error {
	_, _, err := i.vtbl.PutHandled.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(boolToInt(handled)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

// GetIsUserInitiated reports whether the request came from a user gesture
// (as opposed to script without activation).
func (i *ICoreWebView2NewWindowRequestedEventArgs) GetIsUserInitiated() (bool, error) {
	var v int32 // COM BOOL is 32-bit
	_, _, err := i.vtbl.GetIsUserInitiated.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&v)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return v != 0, nil
}

// GetWindowFeatures returns the window features the page requested (the
// width/height/left/top of a window.open call). Chrome uses these to decide
// whether to open a small popup window instead of a tab.
func (i *ICoreWebView2NewWindowRequestedEventArgs) GetWindowFeatures() (*ICoreWebView2WindowFeatures, error) {
	var wf *ICoreWebView2WindowFeatures
	_, _, err := i.vtbl.GetWindowFeatures.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&wf)),
	)
	if err != windows.ERROR_SUCCESS {
		return nil, err
	}
	return wf, nil
}
