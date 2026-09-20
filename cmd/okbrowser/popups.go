//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/lxn/win"
)

// Real popup windows.
//
// Google sign-in (and every other OAuth provider that uses the popup flow)
// does not open "a page in a new tab" - it opens a *child window of the
// opening document*. The contract it relies on is:
//
//   - window.opener points back at the page that called window.open()
//   - the popup shares the same session, cookies and profile
//   - the callback can postMessage / write to the opener and then
//     window.close() itself
//   - the requested width/height are honored
//
// Creating a fresh tab (our old behavior) or handing the URL to the system
// browser breaks all four: the opener link is severed, so Google's callback
// never reaches the page and the sign-in silently hangs. URL-matching
// "accounts.google.com" would be just as unreliable - every provider has its
// own domains and Google changes them.
//
// The robust fix is the one Microsoft designed the API for: when the engine
// raises NewWindowRequested we take a DEFERRAL, create a dedicated child
// WebView2 *inside the parent's existing environment* (same browser process,
// same user-data folder - hence the same cookies and session), hand it back
// through put_NewWindow, and only then complete the deferral. The engine
// itself then wires up window.opener and resolves the pending window.open()
// call against our view.

const popupClassName = "OKBrowserPopupWnd"

// popup is a top-level window hosting one child WebView2 handed to the
// engine as the answer to a new-window request.
type popup struct {
	hwnd     win.HWND
	chromium *edge.Chromium
	closing  bool
}

// popupMinSize is the floor Chromium itself applies to window.open sizes.
const popupMinSize = 100

// onNewWindowRequested is the engine-level new-window handler. Sized or
// positioned requests (window.open with features - the OAuth case) become
// real popup windows; everything else stays a normal tab, which is what a
// user expects from a plain target=_blank link.
func (a *app) onNewWindowRequested(parent *tab, args *edge.ICoreWebView2NewWindowRequestedEventArgs) {
	uri, _ := args.GetUri()
	user, _ := args.GetIsUserInitiated()

	wantPopup, w, h, x, y := false, 0, 0, 0, 0
	if feat, err := args.GetWindowFeatures(); err == nil && feat != nil {
		if feat.HasSize() {
			wantPopup = true
			w, h = int(feat.Width()), int(feat.Height())
		}
		if feat.HasPosition() {
			wantPopup = true
			x, y = int(feat.Left()), int(feat.Top())
		}
		feat.Release()
	}

	if a.inSelfTest {
		a.stlog("[selftest] engine NewWindowRequested: uri=%s user=%v popup=%v size=%dx%d", uri, user, wantPopup, w, h)
	}

	if !wantPopup {
		// Plain new-window request: keep the familiar tab behavior.
		_ = args.PutHandled(true)
		if uri == "" {
			return
		}
		if user || a.allowSpawn() {
			a.postTask(func() { a.newTab(uri, true) })
		}
		return
	}

	if !user && !a.allowSpawn() {
		_ = args.PutHandled(true) // popup storm: swallow it
		return
	}
	if !a.openPopup(parent, args, w, h, x, y) {
		// Could not build a popup: never lose the user's action, fall
		// back to a tab.
		_ = args.PutHandled(true)
		if uri != "" {
			a.postTask(func() { a.newTab(uri, true) })
		}
	}
}

// openPopup creates the popup window and its dedicated child WebView2, and
// completes the engine's new-window request once that view exists.
// Returns false if the popup could not even be started.
func (a *app) openPopup(parent *tab, args *edge.ICoreWebView2NewWindowRequestedEventArgs, w, h, x, y int) bool {
	if parent == nil || parent.chromium == nil {
		return false
	}
	env := parent.chromium.Environment()
	if env == nil {
		return false
	}

	// The event args must outlive this callback: the deferral keeps the
	// pending window.open() alive while the child view is created
	// asynchronously.
	deferral, err := args.GetDeferral()
	if err != nil || deferral == nil {
		return false
	}
	args.AddRef()

	release := func() {
		deferral.Release()
		args.Release()
	}

	if w < popupMinSize {
		w = 520
	}
	if h < popupMinSize {
		h = 600
	}
	cw, ch := a.scaled(int32(w)), a.scaled(int32(h))
	if x <= 0 && y <= 0 {
		// Center on the browser window, like Chrome does.
		var rc win.RECT
		if win.GetWindowRect(a.hwnd, &rc) {
			x = int(rc.Left) + (int(rc.Right-rc.Left)-int(cw))/2
			y = int(rc.Top) + (int(rc.Bottom-rc.Top)-int(ch))/2
		}
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	cn, _ := syscall.UTF16PtrFromString(popupClassName)
	tn, _ := syscall.UTF16PtrFromString(appName)
	// A popup is its own top-level window, owned by the browser window so
	// it always floats above it and closes with it.
	hwnd := win.CreateWindowEx(win.WS_EX_APPWINDOW, cn, tn,
		win.WS_OVERLAPPED|win.WS_CAPTION|win.WS_SYSMENU|win.WS_THICKFRAME|win.WS_CLIPCHILDREN,
		int32(x), int32(y), cw, ch,
		a.hwnd, 0, a.instance, nil)
	if hwnd == 0 {
		_ = deferral.Complete()
		release()
		return false
	}

	p := &popup{hwnd: hwnd}
	c := edge.NewChromium()
	c.DataPath = dataPath()
	c.MessageCallback = func(string) {}
	c.WindowCloseRequestedCallback = func() {
		// window.close() from the OAuth callback page must really close
		// the window, or the sign-in flow appears to hang.
		a.postTask(func() { a.closePopup(p) })
	}
	c.NewWindowRequestedCallback = func(inner *edge.ICoreWebView2NewWindowRequestedEventArgs) {
		// A popup opening another window: keep it simple and use a tab.
		u, _ := inner.GetUri()
		_ = inner.PutHandled(true)
		if u != "" {
			a.postTask(func() { a.newTab(u, true) })
		}
	}
	p.chromium = c
	a.popups = append(a.popups, p)

	// Creating the view inside the PARENT'S environment is what keeps the
	// profile, cookies and signed-in session shared. It is also the only
	// creation path that is safe from inside a COM event callback, because
	// it does not run a nested message pump.
	ok := c.EmbedInEnvironment(uintptr(hwnd), env, func(ready bool) {
		if !ready {
			_ = deferral.Complete()
			release()
			a.removePopup(p)
			win.DestroyWindow(hwnd)
			return
		}
		c.SetDefaultBackgroundColor(edge.COREWEBVIEW2_COLOR{A: 255, R: 28, G: 28, B: 30})
		if st, serr := c.GetSettings(); serr == nil {
			_ = st.PutAreDefaultContextMenusEnabled(true)
			_ = st.PutAreDevToolsEnabled(true)
			_ = st.PutIsStatusBarEnabled(false)
			_ = st.PutIsPasswordAutosaveEnabled(a.store.Settings().Autofill && !incognitoMode)
			_ = st.PutIsGeneralAutofillEnabled(a.store.Settings().Autofill && !incognitoMode)
		}
		// Hand the finished view to the engine, THEN release the
		// deferral: the engine now owns the navigation and wires up
		// window.opener for us. The popup must not be navigated by the
		// host - doing so would replace the engine's own load.
		_ = args.PutNewWindow(c.WebView2())
		_ = args.PutHandled(true)
		_ = deferral.Complete()
		release()

		win.ShowWindow(hwnd, win.SW_SHOW)
		win.UpdateWindow(hwnd)
		c.Resize()
		c.Focus()
		if a.inSelfTest {
			a.stlog("[selftest] popup window ready for the new-window request")
		}
	})
	if !ok {
		_ = deferral.Complete()
		release()
		a.removePopup(p)
		win.DestroyWindow(hwnd)
		return false
	}
	return true
}

// popupByHwnd finds the popup owning a window handle.
func (a *app) popupByHwnd(h win.HWND) *popup {
	for _, p := range a.popups {
		if p.hwnd == h {
			return p
		}
	}
	return nil
}

// removePopup drops p from the popup list.
func (a *app) removePopup(p *popup) {
	for i, q := range a.popups {
		if q == p {
			a.popups = append(a.popups[:i], a.popups[i+1:]...)
			break
		}
	}
}

// closePopup tears a popup down (window.close(), user close, or shutdown).
func (a *app) closePopup(p *popup) { a.releasePopup(p, true) }

// releasePopup drops the popup's engine and list entry. destroy is false
// when the window is already being destroyed by Windows.
func (a *app) releasePopup(p *popup, destroy bool) {
	if p == nil || p.closing {
		return
	}
	p.closing = true
	if p.chromium != nil {
		p.chromium.Close()
		p.chromium = nil
	}
	a.removePopup(p)
	if destroy && isWnd(p.hwnd) {
		win.DestroyWindow(p.hwnd)
	}
	p.hwnd = 0
}

// closeAllPopups is called when the browser window goes away.
func (a *app) closeAllPopups() {
	for _, p := range append([]*popup(nil), a.popups...) {
		a.closePopup(p)
	}
}

// popupWndProc drives popup window geometry and lifetime.
func popupWndProc(hwnd win.HWND, msg uint32, wp uintptr, lp unsafe.Pointer) uintptr {
	a := theApp
	if a == nil {
		return win.DefWindowProc(hwnd, msg, wp, uintptr(lp))
	}
	switch msg {
	case win.WM_SIZE:
		if p := a.popupByHwnd(hwnd); p != nil && p.chromium != nil {
			p.chromium.Resize()
		}
		return 0
	case win.WM_SETFOCUS:
		if p := a.popupByHwnd(hwnd); p != nil && p.chromium != nil {
			p.chromium.Focus()
		}
		return 0
	case win.WM_CLOSE:
		if p := a.popupByHwnd(hwnd); p != nil {
			a.closePopup(p)
			return 0
		}
	case win.WM_DESTROY:
		// Windows is already tearing the window down: release the engine
		// only, never call DestroyWindow again from here.
		if p := a.popupByHwnd(hwnd); p != nil {
			a.releasePopup(p, false)
		}
		return 0
	}
	return win.DefWindowProc(hwnd, msg, wp, uintptr(lp))
}
