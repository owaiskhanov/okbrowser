//go:build windows

package main

import (
	"fmt"
	"strconv"
	"syscall"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/lxn/win"

	"github.com/owaiskhanov/okbrowser/internal/nav"
)

// tab is one browser tab: its own host window and web engine, switched in
// and out of the shared content area. All tabs share one engine process via
// the shared user-data folder, so a new tab is cheap.
type tab struct {
	host     win.HWND
	chromium *edge.Chromium

	title   string
	url     string
	isStart bool
	zoom    float64
}

// active returns the currently displayed tab, or nil.
func (a *app) active() *tab {
	if a.activeIdx < 0 || a.activeIdx >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.activeIdx]
}

// isActive reports whether t is the displayed tab.
func (a *app) isActive(t *tab) bool { return a.active() == t }

// newTab creates a tab, embeds a web engine in it and navigates to the
// start page or the given URL. Returns nil when the engine fails to start.
func (a *app) newTab(url string, activate bool) *tab {
	tn, _ := syscall.UTF16PtrFromString(tabHostClassName)
	a.hostSeq++
	h := win.CreateWindowEx(0, tn, nil, win.WS_CHILD,
		0, 0, 0, 0, a.hwnd, win.HMENU(uintptr(a.hostSeq)), a.instance, nil)
	if h == 0 {
		return nil
	}

	t := &tab{host: h, title: "New Tab", zoom: 1.0}

	c := edge.NewChromium()
	c.DataPath = dataPath()
	c.MessageCallback = func(msg string) { a.onWebMessage(t, msg) }
	c.NavigationStartingCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationStartingEventArgs) {
		a.onNavStarting(t, args)
	}
	c.NavigationCompletedCallback = func(*edge.ICoreWebView2, *edge.ICoreWebView2NavigationCompletedEventArgs) {
		a.onNavCompleted(t)
	}
	t.chromium = c

	if !c.Embed(uintptr(h)) {
		showRuntimeMissingDialog()
		win.DestroyWindow(h)
		return nil
	}

	if st, err := c.GetSettings(); err == nil {
		_ = st.PutAreDefaultContextMenusEnabled(true)
		_ = st.PutAreDevToolsEnabled(true)
		_ = st.PutIsStatusBarEnabled(true)
		_ = st.PutIsZoomControlEnabled(true)
	}
	c.Init(bridgeJS)

	a.tabs = append(a.tabs, t)
	if activate || len(a.tabs) == 1 {
		a.switchToTab(len(a.tabs) - 1)
	} else {
		a.layout()
	}

	if url != "" {
		t.isStart = false
		c.Navigate(nav.Parse(url))
	} else {
		a.showStartPage(t)
	}
	return t
}

// switchToTab displays tab i and syncs the address bar, title and buttons.
func (a *app) switchToTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	if cur := a.active(); cur != nil {
		win.ShowWindow(cur.host, win.SW_HIDE)
		cur.chromium.Hide()
	}
	a.activeIdx = i
	t := a.tabs[i]
	a.layout() // sizes and shows the host, resizes the engine

	setWindowText(a.address, t.url)
	a.syncTitle()
	a.updateNavButtons()
	a.applyZoomTab(t)
	a.focusContent(t)
}

// closeTab removes tab i. Closing the last tab closes the window.
func (a *app) closeTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	if len(a.tabs) == 1 {
		win.DestroyWindow(a.hwnd)
		return
	}
	t := a.tabs[i]
	t.chromium.Close()
	win.DestroyWindow(t.host)
	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)

	if a.activeIdx == i {
		next := i
		if next >= len(a.tabs) {
			next = len(a.tabs) - 1
		}
		a.activeIdx = -1 // force full re-sync
		a.switchToTab(next)
	} else if a.activeIdx > i {
		a.activeIdx--
	}
	a.layout()
}

// showStartPage navigates tab t to the built-in start page.
func (a *app) showStartPage(t *tab) {
	t.isStart = true
	t.url = ""
	t.title = "New Tab"
	t.chromium.NavigateToString(nav.StartHTML)
	if a.isActive(t) {
		setWindowText(a.address, "")
		a.syncTitle()
	}
}

// syncTitle updates the window title from the active tab.
func (a *app) syncTitle() {
	t := a.active()
	if t == nil || t.isStart {
		setWindowText(a.hwnd, appName)
		return
	}
	title := t.title
	if title == "" {
		title = t.url
	}
	setWindowText(a.hwnd, title+" - "+appName)
}

// navigateActive reads the address bar and navigates the active tab.
func (a *app) navigateActive() {
	t := a.active()
	if t == nil {
		return
	}
	u := nav.Parse(getWindowText(a.address))
	if u == "" {
		a.showStartPage(t)
		return
	}
	t.isStart = false
	t.url = u
	setWindowText(a.address, u)
	t.chromium.Navigate(u)
	t.chromium.Focus()
}

// focusContent gives keyboard focus to the tab's web content.
func (a *app) focusContent(t *tab) {
	if t != nil && t.chromium != nil {
		t.chromium.Focus()
	}
}

// applyZoomTab applies the tab's zoom (CSS based; see notes in app.go).
func (a *app) applyZoomTab(t *tab) {
	if t == nil || t.chromium == nil || t.zoom == 1.0 {
		return
	}
	t.chromium.Eval(fmt.Sprintf("document.documentElement.style.zoom=%q",
		strconv.FormatFloat(t.zoom, 'f', -1, 64)))
}

// onNavStarting fires the instant a navigation begins, so the address bar
// updates immediately instead of after the page loads.
func (a *app) onNavStarting(t *tab, args *edge.ICoreWebView2NavigationStartingEventArgs) {
	uri, err := args.GetUri()
	if err != nil || uri == "" || uri == "about:blank" {
		return
	}
	t.url = uri
	t.isStart = false
	if a.isActive(t) {
		setWindowText(a.address, uri)
	}
}

// onNavCompleted refreshes navigation state, zoom and titles after a load.
func (a *app) onNavCompleted(t *tab) {
	if t.chromium == nil {
		return
	}
	a.applyZoomTab(t)
	if a.isActive(t) {
		a.updateNavButtons()
	}
	t.chromium.Eval(`window.__ok && window.__ok({ t: "nav", u: location.href, d: document.title })`)
}

// mktabMouseHit is a tiny helper shared by mouse handlers.
func inRect(x, y int32, r win.RECT) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}
