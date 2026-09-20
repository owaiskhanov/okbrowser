//go:build windows
// +build windows

package edge

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"unsafe"

	"github.com/jchv/go-webview2/internal/w32"
	"golang.org/x/sys/windows"
)

type Chromium struct {
	hwnd                  uintptr
	focusOnInit           bool
	controller            *ICoreWebView2Controller
	webview               *ICoreWebView2
	inited                uintptr
	initFailed            uintptr // OK Browser addition: engine creation failed
	// initDone is invoked once engine creation settles when EmbedAsync was
	// used. Called on the UI thread from the COM completion handler.
	initDone func(bool) // OK Browser addition
	envCompleted          *iCoreWebView2CreateCoreWebView2EnvironmentCompletedHandler
	controllerCompleted   *iCoreWebView2CreateCoreWebView2ControllerCompletedHandler
	webMessageReceived    *iCoreWebView2WebMessageReceivedEventHandler
	permissionRequested   *iCoreWebView2PermissionRequestedEventHandler
	webResourceRequested  *iCoreWebView2WebResourceRequestedEventHandler
	acceleratorKeyPressed *ICoreWebView2AcceleratorKeyPressedEventHandler
	navigationCompleted   *ICoreWebView2NavigationCompletedEventHandler
	navigationStarting    *ICoreWebView2NavigationStartingEventHandler // OK Browser addition
	newWindowRequested    *ICoreWebView2NewWindowRequestedEventHandler // OK Browser addition
	processFailed         *iCoreWebView2ProcessFailedEventHandler       // OK Browser addition
	downloadStarting      *iDownloadStartingHandler                      // OK Browser addition

	// OK Browser addition: keeps per-call DevTools handlers referenced so
	// the GC cannot collect them while native code still holds a pointer.
	devToolsHandlers []*ICoreWebView2CallDevToolsProtocolMethodCompletedHandler

	environment *ICoreWebView2Environment

	// Settings
	DataPath string

	// permissions
	permissions      map[CoreWebView2PermissionKind]CoreWebView2PermissionState
	globalPermission *CoreWebView2PermissionState

	// Callbacks
	MessageCallback              func(string)
	WebResourceRequestedCallback func(request *ICoreWebView2WebResourceRequest, args *ICoreWebView2WebResourceRequestedEventArgs)
	NavigationCompletedCallback  func(sender *ICoreWebView2, args *ICoreWebView2NavigationCompletedEventArgs)
	NavigationStartingCallback   func(sender *ICoreWebView2, args *ICoreWebView2NavigationStartingEventArgs) // OK Browser addition
	NewWindowRequestedCallback   func(args *ICoreWebView2NewWindowRequestedEventArgs)                        // OK Browser addition
	PermissionRequestedCallback  func(uri string, kind CoreWebView2PermissionKind) CoreWebView2PermissionState // OK Browser addition
	ProcessFailedCallback         func(kind CoreWebView2ProcessFailedKind)                              // OK Browser addition
	DownloadStartingCallback      func(args *ICoreWebView2DownloadStartingEventArgs)                    // OK Browser addition
	AcceleratorKeyCallback       func(uint) bool
}

func NewChromium() *Chromium {
	e := &Chromium{}
	/*
	 All these handlers are passed to native code through syscalls with 'uintptr(unsafe.Pointer(handler))' and we know
	 that a pointer to those will be kept in the native code. Furthermore these handlers als contain pointer to other Go
	 structs like the vtable.
	 This violates the unsafe.Pointer rule '(4) Conversion of a Pointer to a uintptr when calling syscall.Syscall.' because
	 theres no guarantee that Go doesn't move these objects.
	 AFAIK currently the Go runtime doesn't move HEAP objects, so we should be safe with these handlers. But they don't
	 guarantee it, because in the future Go might use a compacting GC.
	 There's a proposal to add a runtime.Pin function, to prevent moving pinned objects, which would allow to easily fix
	 this issue by just pinning the handlers. The https://go-review.googlesource.com/c/go/+/367296/ should land in Go 1.19.
	*/
	e.envCompleted = newICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler(e)
	e.controllerCompleted = newICoreWebView2CreateCoreWebView2ControllerCompletedHandler(e)
	e.webMessageReceived = newICoreWebView2WebMessageReceivedEventHandler(e)
	e.permissionRequested = newICoreWebView2PermissionRequestedEventHandler(e)
	e.webResourceRequested = newICoreWebView2WebResourceRequestedEventHandler(e)
	e.acceleratorKeyPressed = newICoreWebView2AcceleratorKeyPressedEventHandler(e)
	e.navigationCompleted = newICoreWebView2NavigationCompletedEventHandler(e)
	e.navigationStarting = newICoreWebView2NavigationStartingEventHandler(e) // OK Browser addition
	e.newWindowRequested = newICoreWebView2NewWindowRequestedEventHandler(e) // OK Browser addition
	e.processFailed = newICoreWebView2ProcessFailedEventHandler(e)           // OK Browser addition
	e.downloadStarting = newDownloadStartingHandler(e)                        // OK Browser addition
	e.permissions = make(map[CoreWebView2PermissionKind]CoreWebView2PermissionState)

	return e
}

// Embed starts the engine and BLOCKS until it is ready, by running a nested
// message loop. Every caller on the UI thread therefore freezes the whole
// application for as long as WebView2 takes to create an environment and a
// controller. Prefer EmbedAsync. (OK Browser note.)
func (e *Chromium) Embed(hwnd uintptr) bool {
	if !e.startEmbed(hwnd) {
		return false
	}
	var msg w32.Msg
	for {
		if atomic.LoadUintptr(&e.inited) != 0 {
			break
		}
		if atomic.LoadUintptr(&e.initFailed) != 0 { // OK Browser addition
			return false
		}
		r, _, _ := w32.User32GetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)
		if r == 0 {
			break
		}
		_, _, _ = w32.User32TranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = w32.User32DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	return true
}

// EmbedAsync starts the engine WITHOUT blocking. done is invoked later, on
// this same thread from the application's own message loop, with true once the
// engine is usable or false if it could not be created. It returns false only
// when creation could not even be started.
//
// This exists because Embed's nested message loop makes opening a tab freeze
// the UI until WebView2 is ready. (OK Browser addition.)
func (e *Chromium) EmbedAsync(hwnd uintptr, done func(bool)) bool {
	e.initDone = done
	if !e.startEmbed(hwnd) {
		e.initDone = nil
		return false
	}
	return true
}

// startEmbed kicks off environment creation and returns immediately.
func (e *Chromium) startEmbed(hwnd uintptr) bool {
	e.hwnd = hwnd

	dataPath := e.DataPath
	if dataPath == "" {
		currentExePath := make([]uint16, windows.MAX_PATH)
		_, err := windows.GetModuleFileName(windows.Handle(0), &currentExePath[0], windows.MAX_PATH)
		if err != nil {
			// What to do here?
			return false
		}
		currentExeName := filepath.Base(windows.UTF16ToString(currentExePath))
		dataPath = filepath.Join(os.Getenv("AppData"), currentExeName)
	}

	dataPathPtr, ok := utf16Ptr(dataPath)
	if !ok {
		log.Printf("Invalid WebView2 data path %q", dataPath)
		return false
	}
	res, err := createCoreWebView2EnvironmentWithOptions(nil, dataPathPtr, 0, e.envCompleted)
	if err != nil {
		log.Printf("Error calling Webview2Loader: %v", err)
		return false
	} else if res != 0 {
		log.Printf("Result: %08x", res)
		return false
	}
	return true
}

// utf16Ptr converts s for a COM call. windows.StringToUTF16Ptr PANICS when s
// contains a NUL, and several of these strings are attacker-controlled (a page
// can post any URL through the bridge, or set any document.title), so a single
// embedded NUL would take the whole browser down. Reject the string instead.
// (OK Browser addition.)
func utf16Ptr(s string) (*uint16, bool) {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return nil, false
	}
	return p, true
}

// CoreWebView2 exposes the raw ICoreWebView2 pointer.
//
// OK Browser addition: needed to answer a NewWindowRequested event with
// put_NewWindow, which is the only way the opener script keeps a live handle
// to the window it opened.
func (e *Chromium) CoreWebView2() *ICoreWebView2 {
	return e.webview
}

func (e *Chromium) Navigate(url string) {
	if e.webview == nil { // OK Browser addition: engine not ready yet
		return
	}
	p, ok := utf16Ptr(url)
	if !ok {
		return
	}
	_, _, _ = e.webview.vtbl.Navigate.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(p)),
	)
}

func (e *Chromium) NavigateToString(htmlContent string) {
	if e.webview == nil { // OK Browser addition: engine not ready yet
		return
	}
	p, ok := utf16Ptr(htmlContent)
	if !ok {
		return
	}
	_, _, _ = e.webview.vtbl.NavigateToString.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(p)),
	)
}

func (e *Chromium) Init(script string) {
	if e.webview == nil { // OK Browser addition: engine not ready yet
		return
	}
	p, ok := utf16Ptr(script)
	if !ok {
		return
	}
	_, _, _ = e.webview.vtbl.AddScriptToExecuteOnDocumentCreated.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(p)),
		0,
	)
}

func (e *Chromium) Eval(script string) {
	// Never log.Fatal here: this is reachable with page-derived content.
	if e.webview == nil { // OK Browser addition: engine not ready yet
		return
	}
	_script, ok := utf16Ptr(script)
	if !ok {
		return
	}

	_, _, _ = e.webview.vtbl.ExecuteScript.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(_script)),
		0,
	)
}

func (e *Chromium) Show() error {
	if e.controller == nil { // OK Browser addition: engine not ready yet
		return nil
	}
	return e.controller.PutIsVisible(true)
}

func (e *Chromium) Hide() error {
	if e.controller == nil { // OK Browser addition: engine not ready yet
		return nil
	}
	return e.controller.PutIsVisible(false)
}

func (e *Chromium) QueryInterface(_, _ uintptr) uintptr {
	return 0
}

func (e *Chromium) AddRef() uintptr {
	return 1
}

func (e *Chromium) Release() uintptr {
	return 1
}

func (e *Chromium) EnvironmentCompleted(res uintptr, env *ICoreWebView2Environment) uintptr {
	// OK Browser addition: mirror CreateCoreWebView2ControllerCompleted and
	// unblock Embed with a failure instead of log.Fatalf, which killed the
	// process with a console message no GUI user ever sees. The host then
	// shows the "WebView2 runtime missing" dialog.
	if int64(res) < 0 || env == nil {
		log.Printf("Creating environment failed with %08x", res)
		atomic.StoreUintptr(&e.initFailed, 1)
		if d := e.initDone; d != nil { // OK Browser addition
			e.initDone = nil
			d(false)
		}
		return 1
	}
	_, _, _ = env.vtbl.AddRef.Call(uintptr(unsafe.Pointer(env)))
	e.environment = env

	_, _, _ = env.vtbl.CreateCoreWebView2Controller.Call(
		uintptr(unsafe.Pointer(env)),
		e.hwnd,
		uintptr(unsafe.Pointer(e.controllerCompleted)),
	)
	return 0
}

func (e *Chromium) CreateCoreWebView2ControllerCompleted(res uintptr, controller *ICoreWebView2Controller) uintptr {
	// OK Browser addition: engine creation can legitimately fail (e.g. the
	// shared profile is still held by a dying engine process). Unblock
	// Embed with a failure instead of killing the process (log.Fatalf) or
	// dereferencing a nil controller.
	if int64(res) < 0 || controller == nil {
		log.Printf("Creating controller failed with %08x", res)
		atomic.StoreUintptr(&e.initFailed, 1)
		if d := e.initDone; d != nil { // OK Browser addition
			e.initDone = nil
			d(false)
		}
		return 1
	}
	_, _, _ = controller.vtbl.AddRef.Call(uintptr(unsafe.Pointer(controller)))
	e.controller = controller

	var token _EventRegistrationToken
	_, _, _ = controller.vtbl.GetCoreWebView2.Call(
		uintptr(unsafe.Pointer(controller)),
		uintptr(unsafe.Pointer(&e.webview)),
	)
	_, _, _ = e.webview.vtbl.AddRef.Call(
		uintptr(unsafe.Pointer(e.webview)),
	)
	_, _, _ = e.webview.vtbl.AddWebMessageReceived.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.webMessageReceived)),
		uintptr(unsafe.Pointer(&token)),
	)
	_, _, _ = e.webview.vtbl.AddPermissionRequested.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.permissionRequested)),
		uintptr(unsafe.Pointer(&token)),
	)
	_, _, _ = e.webview.vtbl.AddWebResourceRequested.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.webResourceRequested)),
		uintptr(unsafe.Pointer(&token)),
	)
	_, _, _ = e.webview.vtbl.AddNavigationCompleted.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.navigationCompleted)),
		uintptr(unsafe.Pointer(&token)),
	)
	_, _, _ = e.webview.vtbl.AddNavigationStarting.Call( // OK Browser addition
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.navigationStarting)),
		uintptr(unsafe.Pointer(&token)),
	)
	_, _, _ = e.webview.vtbl.AddNewWindowRequested.Call( // OK Browser addition
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.newWindowRequested)),
		uintptr(unsafe.Pointer(&token)),
	)
	_, _, _ = e.webview.vtbl.AddProcessFailed.Call( // OK Browser addition
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.processFailed)),
		uintptr(unsafe.Pointer(&token)),
	)
	if web4 := e.webview.get4(); web4 != nil { // OK Browser addition
		_, _, _ = web4.vtbl.AddDownloadStarting.Call(uintptr(unsafe.Pointer(web4)), uintptr(unsafe.Pointer(e.downloadStarting)), uintptr(unsafe.Pointer(&token)))
		web4.vtbl.Release.Call(uintptr(unsafe.Pointer(web4)))
	}

	_ = e.controller.AddAcceleratorKeyPressed(e.acceleratorKeyPressed, &token)

	// Previously installed by Embed after its nested loop returned; it must
	// happen here so the async path gets it too.
	e.Init("window.external={invoke:s=>window.chrome.webview.postMessage(s)}")

	atomic.StoreUintptr(&e.inited, 1)

	if e.focusOnInit {
		e.Focus()
	}
	if d := e.initDone; d != nil { // OK Browser addition
		e.initDone = nil
		d(true)
	}

	return 0
}

func (e *Chromium) MessageReceived(sender *ICoreWebView2, args *iCoreWebView2WebMessageReceivedEventArgs) uintptr {
	var message *uint16
	_, _, _ = args.vtbl.TryGetWebMessageAsString.Call(
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(&message)),
	)
	if e.MessageCallback != nil {
		e.MessageCallback(w32.Utf16PtrToString(message))
	}
	// OK Browser patch: the upstream code echoes every message back into the
	// page via PostWebMessageAsString. We removed the echo so that pages
	// listening for their own postMessage events never see our host bridge
	// traffic.
	windows.CoTaskMemFree(unsafe.Pointer(message))
	return 0
}

func (e *Chromium) SetPermission(kind CoreWebView2PermissionKind, state CoreWebView2PermissionState) {
	e.permissions[kind] = state
}

func (e *Chromium) SetGlobalPermission(state CoreWebView2PermissionState) {
	e.globalPermission = &state
}

func (e *Chromium) PermissionRequested(_ *ICoreWebView2, args *iCoreWebView2PermissionRequestedEventArgs) uintptr {
	var kind CoreWebView2PermissionKind
	_, _, _ = args.vtbl.GetPermissionKind.Call(
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(&kind)),
	)
	var uriPtr *uint16
	_, _, _ = args.vtbl.GetURI.Call(uintptr(unsafe.Pointer(args)), uintptr(unsafe.Pointer(&uriPtr)))
	uri := w32.Utf16PtrToString(uriPtr)
	if uriPtr != nil { windows.CoTaskMemFree(unsafe.Pointer(uriPtr)) }
	var result CoreWebView2PermissionState
	if e.PermissionRequestedCallback != nil {
		result = e.PermissionRequestedCallback(uri, kind)
	} else if e.globalPermission != nil {
		result = *e.globalPermission
	} else {
		var ok bool
		result, ok = e.permissions[kind]
		if !ok {
			result = CoreWebView2PermissionStateDefault
		}
	}
	_, _, _ = args.vtbl.PutState.Call(
		uintptr(unsafe.Pointer(args)),
		uintptr(result),
	)
	return 0
}

func (e *Chromium) WebResourceRequested(sender *ICoreWebView2, args *ICoreWebView2WebResourceRequestedEventArgs) uintptr {
	// OK Browser addition: a failed request lookup is not worth killing the
	// browser over (this used to be log.Fatal).
	req, err := args.GetRequest()
	if err != nil {
		log.Printf("WebResourceRequested: %v", err)
		return 0
	}
	if e.WebResourceRequestedCallback != nil {
		e.WebResourceRequestedCallback(req, args)
	}
	return 0
}

func (e *Chromium) AddWebResourceRequestedFilter(filter string, ctx COREWEBVIEW2_WEB_RESOURCE_CONTEXT) {
	// OK Browser addition: report instead of terminating the process.
	if err := e.webview.AddWebResourceRequestedFilter(filter, ctx); err != nil {
		log.Printf("AddWebResourceRequestedFilter(%q): %v", filter, err)
	}
}

func (e *Chromium) Environment() *ICoreWebView2Environment {
	return e.environment
}

// AcceleratorKeyPressed is called when an accelerator key is pressed.
// If the AcceleratorKeyCallback method has been set, it will defer handling of the keypress
// to the callback. That callback returns a bool indicating if the event was handled.
func (e *Chromium) AcceleratorKeyPressed(sender *ICoreWebView2Controller, args *ICoreWebView2AcceleratorKeyPressedEventArgs) uintptr {
	if e.AcceleratorKeyCallback == nil {
		return 0
	}
	eventKind, _ := args.GetKeyEventKind()
	if eventKind == COREWEBVIEW2_KEY_EVENT_KIND_KEY_DOWN ||
		eventKind == COREWEBVIEW2_KEY_EVENT_KIND_SYSTEM_KEY_DOWN {
		virtualKey, _ := args.GetVirtualKey()
		status, _ := args.GetPhysicalKeyStatus()
		if !status.WasKeyDown {
			_ = args.PutHandled(e.AcceleratorKeyCallback(virtualKey))
			return 0
		}
	}
	_ = args.PutHandled(false)
	return 0
}

func (e *Chromium) GetSettings() (*ICoreWebViewSettings, error) {
	if e.webview == nil { // OK Browser addition: engine not ready yet
		return nil, errors.New("webview not ready")
	}
	return e.webview.GetSettings()
}

func (e *Chromium) GetController() *ICoreWebView2Controller {
	return e.controller
}

func boolToInt(input bool) int {
	if input {
		return 1
	}
	return 0
}

func (e *Chromium) NavigationCompleted(sender *ICoreWebView2, args *ICoreWebView2NavigationCompletedEventArgs) uintptr {
	if e.NavigationCompletedCallback != nil {
		e.NavigationCompletedCallback(sender, args)
	}
	return 0
}

// NavigationStarting is invoked as soon as a navigation begins, before any
// network activity. (OK Browser addition.)
func (e *Chromium) NavigationStarting(sender *ICoreWebView2, args *ICoreWebView2NavigationStartingEventArgs) uintptr {
	if e.NavigationStartingCallback != nil {
		e.NavigationStartingCallback(sender, args)
	}
	return 0
}

// NewWindowRequested is raised when page content asks for a new window
// (target=_blank, window.open). The host marks it handled and opens a tab.
// (OK Browser addition.)
func (e *Chromium) NewWindowRequested(sender *ICoreWebView2, args *ICoreWebView2NewWindowRequestedEventArgs) uintptr {
	if e.NewWindowRequestedCallback != nil {
		e.NewWindowRequestedCallback(args)
	}
	return 0
}

// DownloadStarting exposes the native WebView2 operation to the host.
func (e *Chromium) DownloadStarting(args *ICoreWebView2DownloadStartingEventArgs) uintptr {
	if e.DownloadStartingCallback != nil { e.DownloadStartingCallback(args) }
	return 0
}

// ProcessFailed reports renderer/browser/GPU failures to the host.
func (e *Chromium) ProcessFailed(_ *ICoreWebView2, args *ICoreWebView2ProcessFailedEventArgs) uintptr {
	if e.ProcessFailedCallback != nil { e.ProcessFailedCallback(args.GetProcessFailedKind()) }
	return 0
}

// DevToolsProtocolMethodCompleted ignores CDP results (fire-and-forget).
// (OK Browser addition.)
func (e *Chromium) DevToolsProtocolMethodCompleted(errorCode uintptr, returnObjectAsJSON string) uintptr {
	return 0
}

// CallDevToolsProtocol runs a DevTools protocol method, e.g.
// Input.dispatchMouseEvent to synthesize a trusted click. The result is
// ignored. (OK Browser addition; used by the self test.)
func (e *Chromium) CallDevToolsProtocol(method, paramsJSON string) {
	if e.webview == nil {
		return
	}
	h := newICoreWebView2CallDevToolsProtocolMethodCompletedHandler(e)
	e.devToolsHandlers = append(e.devToolsHandlers, h)
	_ = e.webview.CallDevToolsProtocolMethod(method, paramsJSON, h)
}

// iidController2 is ICoreWebView2Controller2
// {C979903E-D4CA-4228-92EB-47EE3FA96EAB}, verified against Microsoft's
// official .NET interop dumps. (OK Browser addition.)
var iidController2 = windows.GUID{
	Data1: 0xC979903E, Data2: 0xD4CA, Data3: 0x4228,
	Data4: [8]byte{0x92, 0xEB, 0x47, 0xEE, 0x3F, 0xA9, 0x6E, 0xAB},
}

// SetDefaultBackgroundColor sets the color the engine paints BEFORE any
// web content has rendered (WebView2's default is white - the classic
// white-flash on tab creation and dark-mode navigation). (OK Browser
// addition.)
func (e *Chromium) SetDefaultBackgroundColor(col COREWEBVIEW2_COLOR) {
	if e.controller == nil {
		return
	}
	var c2 *ICoreWebView2Controller2
	r, _, _ := e.controller.vtbl.QueryInterface.Call(
		uintptr(unsafe.Pointer(e.controller)),
		uintptr(unsafe.Pointer(&iidController2)),
		uintptr(unsafe.Pointer(&c2)),
	)
	if r != 0 || c2 == nil {
		return
	}
	defer c2.vtbl.Release.Call(uintptr(unsafe.Pointer(c2)))
	_ = c2.PutDefaultBackgroundColor(col)
}

// OpenDevTools opens the engine's DevTools window. (OK Browser addition.)
func (e *Chromium) OpenDevTools() {
	if e.webview == nil {
		return
	}
	_, _, _ = e.webview.vtbl.OpenDevToolsWindow.Call(
		uintptr(unsafe.Pointer(e.webview)))
}

// CanGoBack reports whether there is back history. (OK Browser addition.)
func (e *Chromium) CanGoBack() bool {
	if e.webview == nil {
		return false
	}
	r, _, _ := e.webview.vtbl.GetCanGoBack.Call(uintptr(unsafe.Pointer(e.webview)))
	return r != 0
}

// CanGoForward reports whether there is forward history. (OK Browser addition.)
func (e *Chromium) CanGoForward() bool {
	if e.webview == nil {
		return false
	}
	r, _, _ := e.webview.vtbl.GetCanGoForward.Call(uintptr(unsafe.Pointer(e.webview)))
	return r != 0
}

// GoBack navigates back one step using the engine API. (OK Browser addition.)
func (e *Chromium) GoBack() {
	if e.webview == nil {
		return
	}
	e.webview.vtbl.GoBack.Call(uintptr(unsafe.Pointer(e.webview)))
}

// GoForward navigates forward one step using the engine API. (OK Browser addition.)
func (e *Chromium) GoForward() {
	if e.webview == nil {
		return
	}
	e.webview.vtbl.GoForward.Call(uintptr(unsafe.Pointer(e.webview)))
}

// Reload reloads the page using the engine API. (OK Browser addition.)
func (e *Chromium) Reload() {
	if e.webview == nil {
		return
	}
	e.webview.vtbl.Reload.Call(uintptr(unsafe.Pointer(e.webview)))
}

// Close releases the engine controller for this tab. (OK Browser addition.)
func (e *Chromium) Close() {
	if e.controller == nil {
		return
	}
	e.controller.vtbl.Close.Call(uintptr(unsafe.Pointer(e.controller)))
	e.controller = nil
	e.webview = nil
}

// NOTE (OK Browser): the controller's PutZoomFactor takes a raw `double`
// parameter. Go's syscall ABI on windows/amd64 passes arguments in integer
// registers only, so a double argument would arrive as garbage (the runtime
// explicitly does not spill float args to XMM registers). Zoom is therefore
// applied from the application side via CSS (ExecuteScript), which only ever
// passes strings.

func (e *Chromium) NotifyParentWindowPositionChanged() error {
	//It looks like the wndproc function is called before the controller initialization is complete.
	//Because of this the controller is nil
	if e.controller == nil {
		return nil
	}
	return e.controller.NotifyParentWindowPositionChanged()
}

func (e *Chromium) Focus() {
	if e.controller == nil {
		e.focusOnInit = true
		return
	}
	_ = e.controller.MoveFocus(COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC)
}
