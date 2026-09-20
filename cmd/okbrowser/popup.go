//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	"github.com/jchv/go-webview2/pkg/edge"
)

// Sign-in flows (Google, Microsoft, GitHub) call window.open with an explicit
// width and height. Chrome honours that by opening a small, separate popup
// window with no tab strip, and the provider's script closes it when the flow
// finishes. Turning those into ordinary tabs breaks the illusion and leaves a
// dead tab behind once window.close() has run.
//
// A popup is a real top-level window hosting its own engine. It deliberately
// has no bar, no tabs and no bridge script: it is the provider's page and
// nothing else.

const popupClassName = "OKBrowserPopup"

// Chrome clamps popups to a sane minimum so a page cannot open a sliver.
const (
	popupMinW = 320
	popupMinH = 240
	popupMaxW = 1600
	popupMaxH = 1200
)

type popup struct {
	hwnd     win.HWND
	chromium *edge.Chromium
}

// livePopups keeps popups reachable so the GC cannot collect a window that
// Windows still owns, and lets shutdown close them.
var livePopups = map[win.HWND]*popup{}

// popupSize clamps the geometry a page requested into something usable.
// Width and height are clamped independently; a page that asks for nothing
// gets Chrome's rough default sign-in size.
func popupSize(w, h uint32, hasSize bool) (int32, int32) {
	if !hasSize || w == 0 || h == 0 {
		return 500, 640 // a typical OAuth consent window
	}
	cw, ch := int32(w), int32(h)
	if cw < popupMinW {
		cw = popupMinW
	}
	if cw > popupMaxW {
		cw = popupMaxW
	}
	if ch < popupMinH {
		ch = popupMinH
	}
	if ch > popupMaxH {
		ch = popupMaxH
	}
	return cw, ch
}

// popupOrigin centres the popup over the owner when the page did not ask for
// a position, and keeps a requested position on screen.
func popupOrigin(owner win.RECT, x, y uint32, hasPos bool, w, h int32) (int32, int32) {
	if hasPos {
		return int32(x), int32(y)
	}
	cx := owner.Left + (owner.Right-owner.Left-w)/2
	cy := owner.Top + (owner.Bottom-owner.Top-h)/2
	return cx, cy
}

func popupProc(hwnd win.HWND, msg uint32, wp uintptr, lp unsafe.Pointer) uintptr {
	switch msg {
	case win.WM_SIZE:
		if p := livePopups[hwnd]; p != nil && p.chromium != nil {
			p.chromium.Resize()
		}
		return 0

	case win.WM_CLOSE:
		win.DestroyWindow(hwnd)
		return 0

	case win.WM_DESTROY:
		if p := livePopups[hwnd]; p != nil {
			if p.chromium != nil {
				p.chromium.Close()
				p.chromium = nil
			}
			delete(livePopups, hwnd)
		}
		return 0
	}
	return win.DefWindowProc(hwnd, msg, wp, uintptr(lp))
}

// openPopup creates the popup window and starts its engine.
//
// ready is called once the engine exists, with the new ICoreWebView2 that the
// caller must hand back to the opener via put_NewWindow; it receives nil when
// the engine could not be created. The popup performs NO navigation of its
// own: once it is adopted as the opener's new window, WebView2 drives the
// navigation, which is what keeps window.close(), postMessage and
// popup.closed working.
func (a *app) openPopup(w, h, x, y int32, ready func(*edge.ICoreWebView2)) bool {
	cn, err := syscall.UTF16PtrFromString(popupClassName)
	if err != nil {
		return false
	}
	title, _ := syscall.UTF16PtrFromString("OK Browser")

	// A normal framed window: the provider's page supplies the content, and
	// Windows supplies the title bar, close button and resize borders. The
	// requested size is the CONTENT size, so grow it by the frame.
	fw := int32(win.GetSystemMetrics(win.SM_CXFRAME)+win.GetSystemMetrics(smCXPaddedBorder)) * 2
	fh := int32(win.GetSystemMetrics(win.SM_CYFRAME)+win.GetSystemMetrics(smCXPaddedBorder))*2 +
		int32(win.GetSystemMetrics(win.SM_CYCAPTION))

	hwnd := win.CreateWindowEx(0, cn, title,
		win.WS_OVERLAPPEDWINDOW,
		x, y, w+fw, h+fh,
		0, 0, a.instance, nil)
	if hwnd == 0 {
		return false
	}

	p := &popup{hwnd: hwnd}
	livePopups[hwnd] = p

	c := edge.NewChromium()
	c.DataPath = dataPath()
	p.chromium = c

	if !c.EmbedAsync(uintptr(hwnd), func(ok bool) {
		if !ok {
			win.DestroyWindow(hwnd)
			ready(nil)
			return
		}
		c.SetDefaultBackgroundColor(edge.COREWEBVIEW2_COLOR{A: 255, R: 28, G: 28, B: 30})
		if st, e := c.GetSettings(); e == nil {
			_ = st.PutAreDefaultContextMenusEnabled(true)
			_ = st.PutIsStatusBarEnabled(false)
			autofill := a.store.SettingsView().Autofill && !incognitoMode
			_ = st.PutIsPasswordAutosaveEnabled(autofill)
			_ = st.PutIsGeneralAutofillEnabled(autofill)
		}
		c.Resize()
		c.Show()
		win.ShowWindow(hwnd, win.SW_SHOW)
		win.SetForegroundWindow(hwnd)
		c.Focus()
		ready(c.CoreWebView2())
	}) {
		win.DestroyWindow(hwnd)
		return false
	}
	return true
}

// closePopups destroys any popup still open (browser shutdown).
func closePopups() {
	for hwnd := range livePopups {
		win.DestroyWindow(hwnd)
	}
}
