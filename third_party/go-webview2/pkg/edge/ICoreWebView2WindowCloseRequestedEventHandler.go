package edge

// OK Browser addition: ICoreWebView2WindowCloseRequestedEventHandler.
//
// Raised when the content calls window.close(). A real popup must actually
// disappear when the OAuth provider closes it, so the host listens for this
// and destroys the popup's host window.

type iCoreWebView2WindowCloseRequestedEventHandlerVtbl struct {
	_IUnknownVtbl
	Invoke ComProc
}

type iCoreWebView2WindowCloseRequestedEventHandler struct {
	vtbl *iCoreWebView2WindowCloseRequestedEventHandlerVtbl
	impl iCoreWebView2WindowCloseRequestedEventHandlerImpl
}

type iCoreWebView2WindowCloseRequestedEventHandlerImpl interface {
	_IUnknownImpl
	WindowCloseRequested(sender *ICoreWebView2, args uintptr) uintptr
}

func windowCloseQI(h *iCoreWebView2WindowCloseRequestedEventHandler, r, o uintptr) uintptr {
	return h.impl.QueryInterface(r, o)
}
func windowCloseAddRef(h *iCoreWebView2WindowCloseRequestedEventHandler) uintptr {
	return h.impl.AddRef()
}
func windowCloseRelease(h *iCoreWebView2WindowCloseRequestedEventHandler) uintptr {
	return h.impl.Release()
}
func windowCloseInvoke(h *iCoreWebView2WindowCloseRequestedEventHandler, s *ICoreWebView2, a uintptr) uintptr {
	return h.impl.WindowCloseRequested(s, a)
}

var windowCloseRequestedHandlerFn = iCoreWebView2WindowCloseRequestedEventHandlerVtbl{
	_IUnknownVtbl{
		NewComProc(windowCloseQI),
		NewComProc(windowCloseAddRef),
		NewComProc(windowCloseRelease),
	},
	NewComProc(windowCloseInvoke),
}

func newICoreWebView2WindowCloseRequestedEventHandler(impl iCoreWebView2WindowCloseRequestedEventHandlerImpl) *iCoreWebView2WindowCloseRequestedEventHandler {
	return &iCoreWebView2WindowCloseRequestedEventHandler{vtbl: &windowCloseRequestedHandlerFn, impl: impl}
}
