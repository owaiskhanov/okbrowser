package edge

import (
	"unsafe"
	"github.com/jchv/go-webview2/internal/w32"
	"golang.org/x/sys/windows"
)

// WebView2 download support lives on ICoreWebView2_4.
type iCoreWebView2_4Vtbl struct {
	iCoreWebView2_3Vtbl
	AddFrameCreated ComProc
	RemoveFrameCreated ComProc
	AddDownloadStarting ComProc
	RemoveDownloadStarting ComProc
}
type iCoreWebView2_4 struct { vtbl *iCoreWebView2_4Vtbl }
var iidCoreWebView2_4 = NewGUID("{20D02D59-6DF2-42DC-BD06-F98A694B1302}")
func (i *ICoreWebView2) get4() *iCoreWebView2_4 {
	var out *iCoreWebView2_4
	_,_,_ = i.vtbl.QueryInterface.Call(uintptr(unsafe.Pointer(i)), uintptr(unsafe.Pointer(iidCoreWebView2_4)), uintptr(unsafe.Pointer(&out)))
	return out
}

type iDownloadStartingArgsVtbl struct { _IUnknownVtbl; GetDownloadOperation, GetCancel, PutCancel, GetResultFilePath, PutResultFilePath, GetHandled, PutHandled, GetDeferral ComProc }
type ICoreWebView2DownloadStartingEventArgs struct { vtbl *iDownloadStartingArgsVtbl }
func (a *ICoreWebView2DownloadStartingEventArgs) GetDownloadOperation() *ICoreWebView2DownloadOperation { var o *ICoreWebView2DownloadOperation; a.vtbl.GetDownloadOperation.Call(uintptr(unsafe.Pointer(a)),uintptr(unsafe.Pointer(&o))); return o }
func (a *ICoreWebView2DownloadStartingEventArgs) GetResultFilePath() string { var p *uint16; a.vtbl.GetResultFilePath.Call(uintptr(unsafe.Pointer(a)),uintptr(unsafe.Pointer(&p))); s:=w32.Utf16PtrToString(p); if p!=nil { windows.CoTaskMemFree(unsafe.Pointer(p)) }; return s }

type iDownloadOperationVtbl struct {
	_IUnknownVtbl
	AddBytesReceivedChanged ComProc
	RemoveBytesReceivedChanged ComProc
	AddEstimatedEndTimeChanged ComProc
	RemoveEstimatedEndTimeChanged ComProc
	AddStateChanged ComProc
	RemoveStateChanged ComProc
	// Keep this exact COM/IDL order. Calling a method through the wrong slot
	// invokes an unrelated function pointer and crashes the host process.
	GetCanResume ComProc
	GetContentDisposition ComProc
	GetEstimatedEndTime ComProc
	GetInterruptReason ComProc
	GetMimeType ComProc
	GetResultFilePath ComProc
	GetState ComProc
	GetTotalBytesToReceive ComProc
	GetUri ComProc
	GetBytesReceived ComProc
	Cancel ComProc
	Pause ComProc
	Resume ComProc
}
type ICoreWebView2DownloadOperation struct { vtbl *iDownloadOperationVtbl }
func (o *ICoreWebView2DownloadOperation) AddRef() { o.vtbl.AddRef.Call(uintptr(unsafe.Pointer(o))) }
func (o *ICoreWebView2DownloadOperation) Release() { o.vtbl.Release.Call(uintptr(unsafe.Pointer(o))) }
func (o *ICoreWebView2DownloadOperation) stringValue(p ComProc) string { var v *uint16; p.Call(uintptr(unsafe.Pointer(o)),uintptr(unsafe.Pointer(&v))); s:=w32.Utf16PtrToString(v); if v!=nil { windows.CoTaskMemFree(unsafe.Pointer(v)) }; return s }
func (o *ICoreWebView2DownloadOperation) GetURI() string { return o.stringValue(o.vtbl.GetUri) }
func (o *ICoreWebView2DownloadOperation) GetMimeType() string { return o.stringValue(o.vtbl.GetMimeType) }
func (o *ICoreWebView2DownloadOperation) GetResultFilePath() string { return o.stringValue(o.vtbl.GetResultFilePath) }
func (o *ICoreWebView2DownloadOperation) GetTotalBytesToReceive() int64 { var v int64; o.vtbl.GetTotalBytesToReceive.Call(uintptr(unsafe.Pointer(o)),uintptr(unsafe.Pointer(&v))); return v }
func (o *ICoreWebView2DownloadOperation) GetBytesReceived() int64 { var v int64; o.vtbl.GetBytesReceived.Call(uintptr(unsafe.Pointer(o)),uintptr(unsafe.Pointer(&v))); return v }
func (o *ICoreWebView2DownloadOperation) GetState() uint32 { var v uint32; o.vtbl.GetState.Call(uintptr(unsafe.Pointer(o)),uintptr(unsafe.Pointer(&v))); return v }
func (o *ICoreWebView2DownloadOperation) GetInterruptReason() uint32 { var v uint32; o.vtbl.GetInterruptReason.Call(uintptr(unsafe.Pointer(o)),uintptr(unsafe.Pointer(&v))); return v }
func (o *ICoreWebView2DownloadOperation) GetCanResume() bool { var v int32; o.vtbl.GetCanResume.Call(uintptr(unsafe.Pointer(o)),uintptr(unsafe.Pointer(&v))); return v!=0 }
func (o *ICoreWebView2DownloadOperation) Cancel() { o.vtbl.Cancel.Call(uintptr(unsafe.Pointer(o))) }
func (o *ICoreWebView2DownloadOperation) Pause() { o.vtbl.Pause.Call(uintptr(unsafe.Pointer(o))) }
func (o *ICoreWebView2DownloadOperation) Resume() { o.vtbl.Resume.Call(uintptr(unsafe.Pointer(o))) }

type iDownloadStartingHandlerVtbl struct { _IUnknownVtbl; Invoke ComProc }
type iDownloadStartingHandler struct { vtbl *iDownloadStartingHandlerVtbl; impl iDownloadStartingHandlerImpl }
type iDownloadStartingHandlerImpl interface { _IUnknownImpl; DownloadStarting(*ICoreWebView2DownloadStartingEventArgs) uintptr }
func dlQI(h *iDownloadStartingHandler,r,o uintptr)uintptr{return h.impl.QueryInterface(r,o)}
func dlAdd(h *iDownloadStartingHandler)uintptr{return h.impl.AddRef()}
func dlRelease(h *iDownloadStartingHandler)uintptr{return h.impl.Release()}
func dlInvoke(h *iDownloadStartingHandler,_ *ICoreWebView2,a *ICoreWebView2DownloadStartingEventArgs)uintptr{return h.impl.DownloadStarting(a)}
var dlHandlerVtbl=iDownloadStartingHandlerVtbl{_IUnknownVtbl{NewComProc(dlQI),NewComProc(dlAdd),NewComProc(dlRelease)},NewComProc(dlInvoke)}
func newDownloadStartingHandler(i iDownloadStartingHandlerImpl)*iDownloadStartingHandler{return &iDownloadStartingHandler{&dlHandlerVtbl,i}}
