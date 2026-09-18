//go:build windows

package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// OK Browser addition: ICoreWebView2CallDevToolsProtocolMethodCompletedHandler.
// Used only by the built-in self test to synthesize trusted input events
// (Input.dispatchMouseEvent) - the equivalent of what Puppeteer does - so a
// target=_blank click can be tested without a human.

type _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl struct {
	_IUnknownVtbl
	Invoke ComProc
}

type ICoreWebView2CallDevToolsProtocolMethodCompletedHandler struct {
	vtbl *_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl
	impl _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerImpl
}

type _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerImpl interface {
	_IUnknownImpl
	DevToolsProtocolMethodCompleted(errorCode uintptr, returnObjectAsJson string) uintptr
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownQueryInterface(this *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler, refiid, object uintptr) uintptr {
	return this.impl.QueryInterface(refiid, object)
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownAddRef(this *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler) uintptr {
	return this.impl.AddRef()
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownRelease(this *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler) uintptr {
	return this.impl.Release()
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerInvoke(this *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler, errorCode uintptr, returnObjectAsJson *uint16) uintptr {
	var json string
	if returnObjectAsJson != nil {
		json = windows.UTF16PtrToString(returnObjectAsJson)
	}
	return this.impl.DevToolsProtocolMethodCompleted(errorCode, json)
}

var _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerFn = _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl{
	_IUnknownVtbl{
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownQueryInterface),
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownAddRef),
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownRelease),
	},
	NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerInvoke),
}

func newICoreWebView2CallDevToolsProtocolMethodCompletedHandler(impl _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerImpl) *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler {
	return &ICoreWebView2CallDevToolsProtocolMethodCompletedHandler{
		vtbl: &_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerFn,
		impl: impl,
	}
}

// CallDevToolsProtocolMethod runs a DevTools protocol method on the webview.
func (i *ICoreWebView2) CallDevToolsProtocolMethod(methodName string, parametersAsJSON string, handler *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler) error {
	_methodName, err := windows.UTF16PtrFromString(methodName)
	if err != nil {
		return err
	}
	_parameters, err := windows.UTF16PtrFromString(parametersAsJSON)
	if err != nil {
		return err
	}
	_, _, err = i.vtbl.CallDevToolsProtocolMethod.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(_methodName)),
		uintptr(unsafe.Pointer(_parameters)),
		uintptr(unsafe.Pointer(handler)),
	)
	if err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}
