package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// OK Browser addition: ICoreWebView2NavigationStartingEventArgs.
// Only the members we consume are declared; the vtable prefix we rely on
// (IUnknown + GetUri) matches the WebView2 SDK definition.

type _ICoreWebView2NavigationStartingEventArgsVtbl struct {
	_IUnknownVtbl
	GetUri             ComProc
	GetIsUserInitiated ComProc
	GetIsRedirected    ComProc
	GetRequest         ComProc
}

type ICoreWebView2NavigationStartingEventArgs struct {
	vtbl *_ICoreWebView2NavigationStartingEventArgsVtbl
}

// GetUri returns the URI of the navigation that is starting.
func (i *ICoreWebView2NavigationStartingEventArgs) GetUri() (string, error) {
	var _uri *uint16
	_, _, err := i.vtbl.GetUri.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&_uri)),
	)
	if err != nil && err != windows.ERROR_SUCCESS {
		return "", err
	}
	if _uri == nil {
		return "", nil
	}
	uri := windows.UTF16PtrToString(_uri)
	windows.CoTaskMemFree(unsafe.Pointer(_uri))
	return uri, nil
}
