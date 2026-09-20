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
	GetCancel          ComProc
	PutCancel          ComProc
	GetNavigationId    ComProc
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

// GetNavigationId returns the ID shared by NavigationStarting and its later
// NavigationCompleted event. It lets callers discard completion events from a
// navigation that a newer click has already superseded.
func (i *ICoreWebView2NavigationStartingEventArgs) GetNavigationId() (uint64, error) {
	var id uint64
	_, _, err := i.vtbl.GetNavigationId.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&id)),
	)
	if err != nil && err != windows.ERROR_SUCCESS {
		return 0, err
	}
	return id, nil
}
