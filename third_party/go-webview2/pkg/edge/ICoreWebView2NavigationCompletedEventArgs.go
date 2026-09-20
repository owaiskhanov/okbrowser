package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type _ICoreWebView2NavigationCompletedEventArgsVtbl struct {
	_IUnknownVtbl
	GetIsSuccess      ComProc
	GetWebErrorStatus ComProc
	GetNavigationId   ComProc
}

type ICoreWebView2NavigationCompletedEventArgs struct {
	vtbl *_ICoreWebView2NavigationCompletedEventArgsVtbl
}

func (i *ICoreWebView2NavigationCompletedEventArgs) AddRef() uintptr {
	r, _, _ := i.vtbl.AddRef.Call()
	return r
}

// GetNavigationId returns the ID of the NavigationStarting event this
// completion belongs to. Hosts must compare it before reacting to an error:
// WebView2 can report a canceled, older request after a user has clicked a
// newer link.
func (i *ICoreWebView2NavigationCompletedEventArgs) GetNavigationId() (uint64, error) {
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
