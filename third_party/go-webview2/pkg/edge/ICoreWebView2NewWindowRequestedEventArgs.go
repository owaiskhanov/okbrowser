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

// GetDeferral puts the event into a deferred state so the host can supply a
// NewWindow asynchronously. The opener's window.open() call does not return
// until Complete is called on the returned deferral.
func (i *ICoreWebView2NewWindowRequestedEventArgs) GetDeferral() (*ICoreWebView2Deferral, error) {
	var d *ICoreWebView2Deferral
	_, _, err := i.vtbl.GetDeferral.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&d)),
	)
	if err != windows.ERROR_SUCCESS {
		return nil, err
	}
	return d, nil
}

// PutNewWindow supplies the WebView that should back the opened window.
//
// This is what makes window.open() return a LIVE handle to the opener. With
// Handled set but no NewWindow, WebView2 hands the page a proxy for a testing
// window that never loads: window.close() from the popup does nothing,
// postMessage back to the opener is dropped and popup.closed never turns true
// - which is exactly how an OAuth sign-in flow hangs.
func (i *ICoreWebView2NewWindowRequestedEventArgs) PutNewWindow(w *ICoreWebView2) error {
	_, _, err := i.vtbl.PutNewWindow.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(w)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

// AddRef keeps the args alive across a deferral.
func (i *ICoreWebView2NewWindowRequestedEventArgs) AddRef() error {
	_, _, err := i.vtbl.AddRef.Call(uintptr(unsafe.Pointer(i)))
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

// Release drops a reference taken with AddRef.
func (i *ICoreWebView2NewWindowRequestedEventArgs) Release() error {
	_, _, err := i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}
