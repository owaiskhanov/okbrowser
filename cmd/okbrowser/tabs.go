//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

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
	favicon string // page-reported icon URL ('' = letter avatar)
	isStart bool
	errPage bool // the currently shown page is our error page
	pinned  bool // pinned tabs render as favicon-only pills
	zoom         float64
	inactiveSince time.Time
	sleeping     bool
	audioPlaying bool
	permissions  map[string]map[edge.CoreWebView2PermissionKind]edge.CoreWebView2PermissionState
}

func permissionOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" { return "" }
	return strings.ToLower(u.Scheme + "://" + u.Host)
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
	return a.newTabMode(url, activate, false)
}

func (a *app) newTabMode(url string, activate, secondary bool) *tab {
	tn, _ := syscall.UTF16PtrFromString(tabHostClassName)
	a.hostSeq++
	h := win.CreateWindowEx(0, tn, nil, win.WS_CHILD,
		0, 0, 0, 0, a.hwnd, win.HMENU(uintptr(a.hostSeq)), a.instance, nil)
	if h == 0 {
		if selfTestMode {
			selfTestFileInit(fmt.Sprintf("[selftest] FAIL: tab host window creation failed (seq %d)\n", a.hostSeq))
		}
		return nil
	}

	t := &tab{host: h, title: "New Tab", zoom: 1.0, permissions: make(map[string]map[edge.CoreWebView2PermissionKind]edge.CoreWebView2PermissionState)}

	c := edge.NewChromium()
	c.DataPath = dataPath()
	c.MessageCallback = func(msg string) { a.onWebMessage(t, msg) }
	c.AcceleratorKeyCallback = a.onAccelerator
	c.PermissionRequestedCallback = func(raw string, kind edge.CoreWebView2PermissionKind) edge.CoreWebView2PermissionState {
		origin := permissionOrigin(raw)
		if byKind := t.permissions[origin]; byKind != nil {
			if state, ok := byKind[kind]; ok { return state }
		}
		return edge.CoreWebView2PermissionStateDefault
	}
	// The engine-level safety net for new windows (target=_blank,
	// window.open) - covers cases the page-side bridge cannot see (e.g.
	// links inside closed shadow DOMs).
	c.NewWindowRequestedCallback = func(args *edge.ICoreWebView2NewWindowRequestedEventArgs) {
		uri, _ := args.GetUri()
		user, _ := args.GetIsUserInitiated()
		if a.inSelfTest {
			a.stlog("[selftest] engine NewWindowRequested: uri=%s user=%v", uri, user)
		}
		_ = args.PutHandled(true)
		if uri == "" {
			return
		}
		if user {
			// A trusted user gesture (a real click on a _blank link) must
			// always open its tab - never eat a user action.
			a.postTask(func() { a.newTab(uri, true) })
			return
		}
		if a.allowSpawn() {
			a.postTask(func() { a.newTab(uri, true) })
		}
	}
	c.NavigationStartingCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationStartingEventArgs) {
		a.onNavStarting(t, args)
	}
	c.NavigationCompletedCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		a.onNavCompleted(t, args)
	}
	t.chromium = c

	if !c.Embed(uintptr(h)) {
		win.DestroyWindow(h)
		if selfTestMode {
			selfTestFileInit(fmt.Sprintf("[selftest] FAIL: web engine failed to start for tab %d (profile locked or runtime missing)\n", a.hostSeq))
			os.Exit(1)
		}
		showRuntimeMissingDialog()
		return nil
	}

	// Dark engine background: what the engine paints before the page's own
	// CSS applies - WebView2 defaults to white, which flashed on every new
	// tab and every navigation in dark mode. Must run AFTER Embed: the
	// controller only exists once the engine has been created.
	c.SetDefaultBackgroundColor(edge.COREWEBVIEW2_COLOR{A: 255, R: 28, G: 28, B: 30})

	if st, err := c.GetSettings(); err == nil {
		_ = st.PutAreDefaultContextMenusEnabled(true)
		_ = st.PutAreDevToolsEnabled(true)
		_ = st.PutIsStatusBarEnabled(true)
		_ = st.PutIsZoomControlEnabled(true)
		// Passwords, passkeys and profile autofill stay inside WebView2's
		// Windows-protected profile; the browser host never sees the values.
		_ = st.PutIsPasswordAutosaveEnabled(a.store.Settings().Autofill && !incognitoMode)
		_ = st.PutIsGeneralAutofillEnabled(a.store.Settings().Autofill && !incognitoMode)
	}
	c.Init(bridgeJS)
	if secondary { c.Init("window.__okSecondary=true;") }
	c.Init(barJS)

	a.tabs = append(a.tabs, t)
	win.SetTimer(a.hwnd, 4, 30000, 0) // periodic inactive-tab memory trim
	if activate || len(a.tabs) == 1 {
		// The fade state MUST be armed BEFORE the tab is shown: layout()
		// keeps a pending fade host hidden until its first paint. Setting
		// it afterwards (the old order) let layout() show the unpainted
		// host for one full-opacity frame - the white flash.
		prev := a.active()
		if prev != nil && prev != t && isWnd(prev.host) {
			a.beginTabFade(t, prev.host)
		}
		a.switchToTab(len(a.tabs) - 1)
	} else {
		a.pushBarState() // update the visible tab strip
	}

	if url != "" {
		a.navigateTab(t, url)
	} else {
		a.showStartPage(t)
	}
	a.scheduleBarPush(false)
	if a.inSelfTest {
		a.stlog("[selftest] tab %d created (url=%s)", len(a.tabs), url)
	}
	return t
}

// switchToTab displays tab i and syncs title and glass-bar state.
// postNewTab queues a new tab (used from contexts that may sit inside
// engine callbacks).
func (a *app) postNewTab(url string) {
	a.postTask(func() {
		a.newTab(url, true)
		a.scheduleBarPush(true)
	})
}

// openSplit creates a real second WebView and places it beside the active page.
// The link remains a normal tab, so switching tabs naturally promotes it later.
func (a *app) openSplit(url string) {
	// Two panes is the maximum. Ignore additional edge drops while split.
	if a.splitTab != nil { return }
	t := a.newTabMode(url, false, true)
	if t == nil { return }
	a.splitTab = t
	a.splitRatio = .5
	a.layout()
	a.execActive("window.__okSplitToast&&window.__okSplitToast()")
}

// closeSplit closes the secondary pane and restores the active page to full width.
func (a *app) closeSplit() {
	t := a.splitTab
	if t == nil { return }
	idx := -1
	for i, candidate := range a.tabs { if candidate == t { idx = i; break } }
	a.splitTab = nil
	if idx >= 0 { a.closeTab(idx) }
	a.layout()
	a.pushBarState()
}

func (a *app) resizeSplit(delta float64) {
	if a.splitTab == nil { return }
	var rc win.RECT
	if !win.GetClientRect(a.hwnd, &rc) || rc.Right <= 0 { return }
	a.splitRatio += delta / float64(rc.Right)
	if a.splitRatio < .28 { a.splitRatio = .28 }
	if a.splitRatio > .72 { a.splitRatio = .72 }
	a.layout()
}

func (a *app) swapSplit() {
	if a.splitTab == nil { return }
	old := a.active()
	idx := -1
	for i, t := range a.tabs { if t == a.splitTab { idx = i; break } }
	if old == nil || idx < 0 { return }
	a.activeIdx, a.splitTab = idx, old
	a.splitRatio = 1 - a.splitRatio
	a.layout(); a.syncTitle(); a.pushBarState()
}

func (a *app) promoteSplit() {
	if a.splitTab == nil { return }
	a.splitTab = nil
	a.layout(); a.pushBarState()
}

func (a *app) switchToTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	// Selecting either pane promotes it to a normal full-width tab.
	a.splitTab = nil
	if cur := a.active(); cur != nil {
		cur.inactiveSince = time.Now()
		win.ShowWindow(cur.host, win.SW_HIDE)
		cur.chromium.Hide()
	}
	a.activeIdx = i
	t := a.tabs[i]
	if t.sleeping {
		t.chromium.CallDevToolsProtocol("Page.setWebLifecycleState", `{"state":"active"}`)
		t.sleeping = false
	}
	t.inactiveSince = time.Time{}
	a.layout() // sizes and shows the host, resizes the engine

	a.syncTitle()
	a.pushBarState()
	a.applyZoomTab(t)
	if t.chromium != nil {
		t.chromium.Focus()
	}
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
	if a.splitTab == t { a.splitTab = nil }
	if t.url != "" && !t.isStart {
		// Remember it for Ctrl+Shift+T (reopen closed tab).
		a.closedTabs = append(a.closedTabs, t.url)
		if len(a.closedTabs) > 16 {
			a.closedTabs = a.closedTabs[len(a.closedTabs)-16:]
		}
	}
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
	a.pushBarState()
}

// navigateTab navigates tab t to raw user input: an okbrowser:// built-in
// page or a web URL parsed with the selected search engine.
func (a *app) navigateTab(t *tab, raw string) {
	if t == nil || t.chromium == nil {
		return
	}
	s := strings.TrimSpace(raw)
	if s == "" {
		return
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "okbrowser://") {
		a.showInternal(t, strings.TrimPrefix(low, "okbrowser://"))
		return
	}
	u := nav.ParseWithEngine(s, a.store.Settings().Engine)
	if u == "" {
		return
	}
	t.isStart = false
	t.errPage = false
	t.url = u
	if a.isActive(t) {
		a.pushBarState()
	}
	t.chromium.Navigate(u)
	t.chromium.Focus()
}

// showInternal renders one of the built-in okbrowser:// pages in tab t.
func (a *app) showInternal(t *tab, page string) {
	var html, title string
	isStart := false
	switch page {
	case "bookmarks":
		html, title = BookmarksHTML(a.store.SnapshotBookmarks()), "Bookmarks"
	case "history":
		html, title = HistoryHTML(a.store.SnapshotHistory()), "History"
	case "settings":
		html, title = SettingsHTML(a.store.Settings(), appVersion), "Settings"
	case "downloads":
		html, title = DownloadsHTML(listDownloads()), "Downloads"
	default: // start
		html, title = StartPageHTML(a.store.MostVisited(12), a.store.Settings().Engine), "New Tab"
		page, isStart = "start", true
	}
	t.isStart = isStart
	t.errPage = false
	t.title = title
	if isStart {
		t.url = ""
	} else {
		t.url = "okbrowser://" + page
	}
	if a.isActive(t) {
		a.syncTitle()
		a.pushBarState()
	}
	t.chromium.NavigateToString(html)
}

// sleepInactiveTabs freezes background pages after five idle minutes. Pinned
// tabs and either Split View pane stay live. WebView2 keeps page state in memory
// and resumes it instantly when selected.
func (a *app) sleepInactiveTabs() {
	minutes := a.store.Settings().SleepMinutes
	if minutes <= 0 { return }
	now := time.Now()
	for _, t := range a.tabs {
		if t == a.active() || t == a.splitTab || t.pinned || t.audioPlaying || t.sleeping || t.inactiveSince.IsZero() { continue }
		if now.Sub(t.inactiveSince) < time.Duration(minutes)*time.Minute { continue }
		t.chromium.CallDevToolsProtocol("Page.setWebLifecycleState", `{"state":"frozen"}`)
		t.sleeping = true
	}
	a.pushBarState()
}

// beginTabFade starts the liquid cross-fade for a newly opened tab.
//
// Two-phase, so nothing ever flashes: while the new tab's engine starts
// and renders its first page, its host window stays completely HIDDEN -
// the user keeps seeing the previous tab (an uninitialized layered
// surface shows black, and the engine's default background is white, so
// showing it early is exactly what flashed). Only once the first content
// has painted does the host appear as a soft translucent veil over the
// old tab and liquidly ramp to full opacity.
func (a *app) beginTabFade(t *tab, prevHost win.HWND) {
	a.fading = true
	a.fadeRamping = false
	a.fadeReady = false
	a.fadeHost = t.host
	a.fadePrev = prevHost
	a.fadeAlpha = 60
	a.fadeTicks = 0
	win.SetTimer(a.hwnd, 2, 16, 0)
}

// tabByHost finds the tab owned by a host window.
func (a *app) tabByHost(h win.HWND) *tab {
	for _, t := range a.tabs {
		if t.host == h {
			return t
		}
	}
	return nil
}

// fadeTick advances the new-tab cross-fade (WM_TIMER id 2).
func (a *app) fadeTick() {
	if !a.fading {
		win.KillTimer(a.hwnd, 2)
		return
	}
	a.fadeTicks++
	if !isWnd(a.fadeHost) {
		a.endTabFade()
		return
	}
	// The user switched away mid-fade: settle instantly.
	if cur := a.active(); cur == nil || cur.host != a.fadeHost {
		a.endTabFade()
		return
	}
	if !a.fadeRamping {
		// Pending phase: stay hidden until the first content has painted
		// (fadeReady) or the ~700ms fallback fires - the previous tab
		// keeps showing the whole time.
		if !a.fadeReady && a.fadeTicks <= 45 {
			return
		}
		ex := win.GetWindowLong(a.fadeHost, win.GWL_EXSTYLE)
		win.SetWindowLong(a.fadeHost, win.GWL_EXSTYLE, ex|win.WS_EX_LAYERED)
		if !setLayeredAlpha(a.fadeHost, byte(a.fadeAlpha)) {
			unlayered(a.fadeHost) // layered children unsupported: show at once
			a.endTabFade()
			return
		}
		win.ShowWindow(a.fadeHost, win.SW_SHOW)
		if t := a.tabByHost(a.fadeHost); t != nil && t.chromium != nil {
			t.chromium.Show()
			t.chromium.Resize()
		}
		a.fadeRamping = true // revealed - the liquid ramp begins next tick
		return
	}
	a.fadeAlpha += 13 // gentle ~250ms ramp
	if a.fadeAlpha >= 255 {
		a.endTabFade()
		return
	}
	setLayeredAlpha(a.fadeHost, byte(a.fadeAlpha))
}

// selftestClickTick dispatches the deferred trusted click once the
// cross-fade has fully settled (Chromium ignores synthesized input for
// hidden widgets - dispatching during the pending phase was a race).
func (a *app) selftestClickTick() {
	t := a.stClickTab
	if t == nil || t.chromium == nil {
		win.KillTimer(a.hwnd, 3)
		return
	}
	a.stClickTicks++
	if a.fading && a.stClickTicks < 80 { // up to ~4s
		return
	}
	win.KillTimer(a.hwnd, 3)
	a.stlog("[selftest] dispatching trusted click at %.0f,%.0f", a.stClickX, a.stClickY)
	t.chromium.CallDevToolsProtocol("Input.dispatchMouseEvent",
		fmt.Sprintf(`{"type":"mousePressed","x":%.1f,"y":%.1f,"button":"left","clickCount":1}`, a.stClickX, a.stClickY))
	t.chromium.CallDevToolsProtocol("Input.dispatchMouseEvent",
		fmt.Sprintf(`{"type":"mouseReleased","x":%.1f,"y":%.1f,"button":"left","clickCount":1}`, a.stClickX, a.stClickY))
	a.stClickTab = nil
}

// endTabFade finishes the cross-fade: full opacity, previous view retired.
func (a *app) endTabFade() {
	win.KillTimer(a.hwnd, 2)
	if isWnd(a.fadeHost) {
		setLayeredAlpha(a.fadeHost, 255)
		unlayered(a.fadeHost)
	}
	prev := a.fadePrev
	a.fading = false
	a.fadeHost = 0
	a.fadePrev = 0
	if isWnd(prev) {
		win.ShowWindow(prev, win.SW_HIDE)
	}
	a.layout()
}

// reorderTab moves the tab at index from to index to.
func (a *app) reorderTab(from, to int) {
	if from == to || from < 0 || to < 0 || from >= len(a.tabs) || to >= len(a.tabs) {
		return
	}
	t := a.tabs[from]
	a.tabs = append(a.tabs[:from], a.tabs[from+1:]...)
	a.tabs = append(a.tabs[:to], append([]*tab{t}, a.tabs[to:]...)...)
	cur := a.activeIdx
	switch {
	case cur == from:
		cur = to
	case from < cur && to >= cur:
		cur--
	case from > cur && to <= cur:
		cur++
	}
	a.activeIdx = cur
	a.pushBarState()
}

// closeOthers closes every tab except i.
func (a *app) closeOthers(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	for j := len(a.tabs) - 1; j >= 0; j-- {
		if j != i {
			a.closeTab(j)
		}
	}
}

// showStartPage navigates tab t to the built-in start page.
func (a *app) showStartPage(t *tab) {
	a.showInternal(t, "start")
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

// applyZoomTab applies the tab's zoom (CSS based; the engine's zoom API
// takes a raw double, which cannot be called safely from Go).
func (a *app) applyZoomTab(t *tab) {
	if t == nil || t.chromium == nil || t.zoom == 1.0 {
		return
	}
	t.chromium.Eval(fmt.Sprintf("document.documentElement.style.zoom=%q",
		strconv.FormatFloat(t.zoom, 'f', -1, 64)))
}

// onNavStarting fires the instant a navigation begins, so the glass bar's
// address updates immediately instead of after the page loads.
func (a *app) onNavStarting(t *tab, args *edge.ICoreWebView2NavigationStartingEventArgs) {
	uri, err := args.GetUri()
	if err != nil || uri == "" || uri == "about:blank" {
		return
	}
	t.url = uri
	t.isStart = false
	t.errPage = false
	if a.isActive(t) {
		a.pushBarState()
		a.execActive("window.__okLoad&&window.__okLoad(true)")
	}
}

// onNavCompleted refreshes bar state, zoom and titles after a load, and
// turns failed navigations into a visible glass error page with the reason
// and a retry button - a browser must never fail silently.
func (a *app) onNavCompleted(t *tab, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
	if t.chromium == nil {
		return
	}
	if a.fading && t.host == a.fadeHost {
		a.fadeReady = true // first paint done - begin the liquid ramp
	}
	if a.isActive(t) {
		a.execActive("window.__okLoad&&window.__okLoad(false)")
	}
	if args != nil && !t.errPage {
		if ok, err := args.GetIsSuccess(); err == nil && !ok {
			code, _ := args.GetWebErrorStatus()
			// 14 = OperationCanceled (user stopped or replaced the
			// navigation) - not an error worth showing.
			if code != 0 && code != 14 && t.url != "" {
				a.showErrorPage(t, code)
				return
			}
		}
	}
	a.applyZoomTab(t)
	a.pushBarState()
	a.scheduleBarPush(false)
	a.selftestNavHook(t)
	t.chromium.Eval(`window.__ok && window.__ok({ t: "nav", u: location.href, d: document.title, f: (function(){try{var l=document.querySelector('link[rel~="shortcut icon"],link[rel~="icon"]');return l&&l.href?l.href:(location.origin+'/favicon.ico')}catch(e){return ''}})() })`)
}

// showErrorPage replaces the tab's content with a glass error page that
// explains what went wrong and offers a retry.
func (a *app) showErrorPage(t *tab, code uint32) {
	name, hint := webErrorText(code)
	url := t.url
	t.errPage = true
	t.title = "Can't reach this page"
	if a.isActive(t) {
		a.syncTitle()
		a.pushBarState()
	}
	u, _ := json.Marshal(url)
	t.chromium.NavigateToString(errorHTML(string(u), name, hint))
}

// webErrorText maps a COREWEBVIEW2_WEB_ERROR_STATUS to a friendly message.
func webErrorText(code uint32) (name, hint string) {
	switch code {
	case 1:
		return "Certificate name is incorrect", "The site's security certificate doesn't match its address."
	case 2:
		return "Certificate expired", "The site's security certificate has expired."
	case 3:
		return "Client certificate error", "The client certificate has errors."
	case 4:
		return "Certificate revoked", "The site's security certificate was revoked."
	case 5:
		return "Certificate is invalid", "The site's security certificate is not valid."
	case 6:
		return "Server unreachable", "The host could not be reached. Check your internet connection."
	case 7:
		return "Connection timed out", "The site took too long to respond."
	case 8:
		return "Invalid server response", "The server returned an invalid or unrecognized response."
	case 9:
		return "Connection aborted", "The connection was aborted."
	case 10:
		return "Connection reset", "The connection was reset."
	case 11:
		return "Disconnected", "The internet connection was lost."
	case 12:
		return "Can't connect", "A connection to the site could not be established."
	case 13:
		return "Can't find the site", "The host name could not be resolved (DNS). Check the address or your connection."
	case 15:
		return "Redirect failed", "A redirect failed."
	case 16:
		return "Unexpected error", "An unexpected error occurred."
	}
	return "Can't reach this page", "The navigation failed."
}

// errorHTML renders the minimal glass error page. urlJSON must be a
// pre-marshaled JSON string.
func errorHTML(urlJSON, name, hint string) string {
	return `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Can't reach this page</title>
<style>
  :root { --fg:#202124; --muted:#5f6368; --card:#fff; --border:#e3e3e3; }
  @media (prefers-color-scheme: dark) {
    :root { --fg:#e8eaed; --muted:#9aa0a6; --card:#292a2d; --border:#3c4043; }
  }
  * { box-sizing: border-box; }
  body {
    margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center;
    font-family:'Segoe UI',system-ui,sans-serif; color:var(--fg);
  }
  .card {
    max-width:460px; width:92vw; padding:36px 34px; border-radius:22px;
    background:color-mix(in srgb, var(--card) 72%, transparent);
    backdrop-filter:blur(24px) saturate(1.6); -webkit-backdrop-filter:blur(24px) saturate(1.6);
    box-shadow:0 16px 48px rgba(0,0,0,.14), inset 0 1px 0 rgba(255,255,255,.5),
               inset 0 0 0 .5px var(--border);
    text-align:center;
  }
  .ico { font-size:42px; }
  h1 { font-size:20px; margin:14px 0 6px; }
  p  { color:var(--muted); font-size:13.5px; line-height:1.55; margin:6px 0; word-break:break-all; }
  .url { font-family:ui-monospace,Consolas,monospace; font-size:12px; opacity:.85; }
  button {
    margin-top:20px; padding:11px 30px; border:0; border-radius:18px; cursor:pointer;
    font:600 13.5px 'Segoe UI',system-ui,sans-serif; color:#fff;
    background:rgba(10,132,255,.92); transition:background .15s, transform .12s;
  }
  button:hover { background:rgba(10,132,255,1); }
  button:active { transform:scale(.95); }
</style></head><body>
<div class="card">
  <div class="ico">&#127760;</div>
  <h1>` + name + `</h1>
  <p>` + hint + `</p>
  <p class="url">` + htmlEsc(urlJSON[1:len(urlJSON)-1]) + `</p>
  <button onclick="window.__ok({t:'go',u:` + urlJSON + `})">Try again</button>
</div>
</body></html>`
}

// htmlEsc escapes text for safe interpolation into HTML.
func htmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return r.Replace(s)
}
