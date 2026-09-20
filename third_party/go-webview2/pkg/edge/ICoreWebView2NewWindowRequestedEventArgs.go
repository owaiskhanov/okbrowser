package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// OK Browser addition: ICoreWebView2NewWindowRequestedEventArgs.
// Vtable order follows the WebView2 SDK:
//   get_Uri, put_NewWindow, get_NewWindow, put_Handled, get_Handled,
//   get_IsUserInitiated, GetDeferral, get_WindowFeatures

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

// PutNewWindow hands the engine the WebView2 that should serve as the
// popup. The engine then wires window.opener, the shared session and the
// pending window.open() result to it, which is exactly what Google's OAuth
// flow requires. (OK Browser addition.)
func (i *ICoreWebView2NewWindowRequestedEventArgs) PutNewWindow(webview *ICoreWebView2) error {
	_, _, err := i.vtbl.PutNewWindow.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(webview)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

// GetDeferral keeps the new-window request pending while the host creates
// the child WebView2 asynchronously. The caller must Complete() (and
// Release()) the returned deferral. (OK Browser addition.)
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

// GetWindowFeatures returns the size/position the page asked for. The
// caller must Release() the result. (OK Browser addition.)
func (i *ICoreWebView2NewWindowRequestedEventArgs) GetWindowFeatures() (*ICoreWebView2WindowFeatures, error) {
	var f *ICoreWebView2WindowFeatures
	_, _, err := i.vtbl.GetWindowFeatures.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&f)),
	)
	if err != windows.ERROR_SUCCESS {
		return nil, err
	}
	return f, nil
}

// AddRef/Release let the host keep the event args alive across the
// asynchronous popup creation triggered by a deferral.
func (i *ICoreWebView2NewWindowRequestedEventArgs) AddRef() {
	_, _, _ = i.vtbl.AddRef.Call(uintptr(unsafe.Pointer(i)))
}

func (i *ICoreWebView2NewWindowRequestedEventArgs) Release() {
	_, _, _ = i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))
}
