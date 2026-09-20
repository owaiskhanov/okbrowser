package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// OK Browser addition: ICoreWebView2Deferral.
//
// A deferral lets the host return from an event handler while the event
// itself stays pending. We need it for NewWindowRequested: creating the
// popup's WebView2 is asynchronous, and the engine must keep the pending
// window.open() call alive until we hand it the finished child view.

type _ICoreWebView2DeferralVtbl struct {
	_IUnknownVtbl
	Complete ComProc
}

type ICoreWebView2Deferral struct {
	vtbl *_ICoreWebView2DeferralVtbl
}

// Complete resumes the deferred event. It must be called exactly once,
// after the host has finished its asynchronous work.
func (i *ICoreWebView2Deferral) Complete() error {
	_, _, err := i.vtbl.Complete.Call(uintptr(unsafe.Pointer(i)))
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

// Release drops the reference the engine handed us with GetDeferral.
func (i *ICoreWebView2Deferral) Release() {
	_, _, _ = i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))
}
