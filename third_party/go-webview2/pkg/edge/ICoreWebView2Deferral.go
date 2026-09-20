package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// OK Browser addition: ICoreWebView2Deferral.
//
// Vtable order follows WebView2.idl: the interface adds a single method,
// Complete, after IUnknown.
//
// A deferral holds an event open past the end of its handler. NewWindowRequested
// needs one because the replacement WebView cannot be supplied synchronously:
// creating an engine is asynchronous, and the opener's script is blocked until
// either the handler returns or the deferral completes.

type _ICoreWebView2DeferralVtbl struct {
	_IUnknownVtbl
	Complete ComProc
}

type ICoreWebView2Deferral struct {
	vtbl *_ICoreWebView2DeferralVtbl
}

// Complete releases the deferred event, letting the opener's script continue.
func (i *ICoreWebView2Deferral) Complete() error {
	if i == nil {
		return nil
	}
	_, _, err := i.vtbl.Complete.Call(uintptr(unsafe.Pointer(i)))
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

// Release drops the reference taken by GetDeferral.
func (i *ICoreWebView2Deferral) Release() error {
	if i == nil {
		return nil
	}
	_, _, err := i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}
