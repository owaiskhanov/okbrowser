package edge

import "unsafe"

// CoreWebView2ProcessFailedKind mirrors COREWEBVIEW2_PROCESS_FAILED_KIND.
type CoreWebView2ProcessFailedKind uint32

const (
	CoreWebView2ProcessFailedKindBrowserProcessExited CoreWebView2ProcessFailedKind = iota
	CoreWebView2ProcessFailedKindRenderProcessExited
	CoreWebView2ProcessFailedKindRenderProcessUnresponsive
	CoreWebView2ProcessFailedKindFrameRenderProcessExited
	CoreWebView2ProcessFailedKindUtilityProcessExited
	CoreWebView2ProcessFailedKindSandboxHelperProcessExited
	CoreWebView2ProcessFailedKindGPUProcessExited
	CoreWebView2ProcessFailedKindPPAPIPluginProcessExited
	CoreWebView2ProcessFailedKindPPAPIBrokerProcessExited
	CoreWebView2ProcessFailedKindUnknownProcessExited
)

type iCoreWebView2ProcessFailedEventArgsVtbl struct {
	_IUnknownVtbl
	GetProcessFailedKind ComProc
}

type ICoreWebView2ProcessFailedEventArgs struct { vtbl *iCoreWebView2ProcessFailedEventArgsVtbl }

func (a *ICoreWebView2ProcessFailedEventArgs) GetProcessFailedKind() CoreWebView2ProcessFailedKind {
	var kind CoreWebView2ProcessFailedKind
	_, _, _ = a.vtbl.GetProcessFailedKind.Call(uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(&kind)))
	return kind
}

type iCoreWebView2ProcessFailedEventHandlerVtbl struct { _IUnknownVtbl; Invoke ComProc }
type iCoreWebView2ProcessFailedEventHandler struct {
	vtbl *iCoreWebView2ProcessFailedEventHandlerVtbl
	impl iCoreWebView2ProcessFailedEventHandlerImpl
}
type iCoreWebView2ProcessFailedEventHandlerImpl interface {
	_IUnknownImpl
	ProcessFailed(sender *ICoreWebView2, args *ICoreWebView2ProcessFailedEventArgs) uintptr
}

func processFailedQI(h *iCoreWebView2ProcessFailedEventHandler, r, o uintptr) uintptr { return h.impl.QueryInterface(r,o) }
func processFailedAddRef(h *iCoreWebView2ProcessFailedEventHandler) uintptr { return h.impl.AddRef() }
func processFailedRelease(h *iCoreWebView2ProcessFailedEventHandler) uintptr { return h.impl.Release() }
func processFailedInvoke(h *iCoreWebView2ProcessFailedEventHandler, s *ICoreWebView2, a *ICoreWebView2ProcessFailedEventArgs) uintptr { return h.impl.ProcessFailed(s,a) }

var processFailedHandlerFn = iCoreWebView2ProcessFailedEventHandlerVtbl{
	_IUnknownVtbl{NewComProc(processFailedQI), NewComProc(processFailedAddRef), NewComProc(processFailedRelease)},
	NewComProc(processFailedInvoke),
}
func newICoreWebView2ProcessFailedEventHandler(impl iCoreWebView2ProcessFailedEventHandlerImpl) *iCoreWebView2ProcessFailedEventHandler {
	return &iCoreWebView2ProcessFailedEventHandler{vtbl:&processFailedHandlerFn, impl:impl}
}
