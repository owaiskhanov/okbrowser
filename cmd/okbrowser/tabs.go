//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
	dirtyForm    bool
	crashCount   int
	lastCrash    time.Time

	// ready is set once the engine exists. Until then the tab has a host
	// window but no usable controller, so engine calls must be deferred.
	ready bool
	// pendingURL is the navigation requested before the engine was ready.
	pendingURL string
	// secondary marks a Split View pane (it gets an extra init script).
	secondary bool
	// startRev is the store revision the rendered start page was built
	// from, so a warmed page can be refreshed if it went stale.
	startRev uint64

	// Cached host geometry and visibility, so layout() can skip Win32 and
	// engine calls that would not change anything. layout() runs on every
	// WM_SIZE, i.e. every frame of a window drag-resize.
	bx, by, bw, bh int32
	shown          bool
	engineShown    bool

	// downloadAt is when this tab last turned a navigation into a download.
	// A link that downloads (GitHub release assets, WhatsApp Web media)
	// reports NavigationCompleted with IsSuccess=FALSE, so without this the
	// page the user was on would be replaced by our error page.
	downloadAt time.Time
}

func permissionName(kind edge.CoreWebView2PermissionKind) string {
	switch kind {
	case edge.CoreWebView2PermissionKindCamera: return "camera"
	case edge.CoreWebView2PermissionKindMicrophone: return "microphone"
	case edge.CoreWebView2PermissionKindGeolocation: return "location"
	case edge.CoreWebView2PermissionKindNotifications: return "notifications"
	case edge.CoreWebView2PermissionKindClipboardRead: return "clipboard"
	case edge.CoreWebView2PermissionKindOtherSensors: return "sensors"
	}
	return "unknown"
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

// commandTab is the pane that most recently received pointer/keyboard focus.
func (a *app) commandTab() *tab {
	if a.focusedTab != nil && (a.focusedTab == a.active() || a.focusedTab == a.splitTab) { return a.focusedTab }
	return a.active()
}

// isActive reports whether t is the displayed tab.
func (a *app) isActive(t *tab) bool { return a.active() == t }

// newTab creates a tab, embeds a web engine in it and navigates to the
// start page or the given URL. Returns nil when the engine fails to start.
func (a *app) newTab(url string, activate bool) *tab {
	return a.newTabMode(url, activate, false)
}

func (a *app) newTabMode(url string, activate, secondary bool) *tab {
	// A spare engine was warmed in the background: adopting it makes the
	// tab appear with its start page already painted, with no engine
	// startup on the critical path at all.
	if t := a.takeSpare(secondary); t != nil {
		a.attachTab(t, url, activate)
		a.warmSpareLater()
		return t
	}
	t := a.createTab(secondary, url)
	if t == nil {
		a.engineStartFailed()
		return nil
	}
	a.attachTab(t, url, activate)
	a.warmSpareLater()
	return t
}

// createTab builds the host window and starts the engine WITHOUT waiting for
// it. The engine reports readiness later through onEngineReady, so opening a
// tab never blocks the UI thread.
func (a *app) createTab(secondary bool, url string) *tab {
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

	// pendingURL MUST be set before EmbedAsync: WebView2 can invoke the
	// completion handler synchronously when the browser process and
	// environment already exist, which is the normal case for a popup opened
	// from a page that is already loaded (a "Sign in with Google" window, for
	// example). Assigning it after the call left the URL stranded and the tab
	// showed an empty New Tab instead of the sign-in page.
	t := &tab{host: h, title: "New Tab", zoom: 1.0, secondary: secondary, pendingURL: url}

	c := edge.NewChromium()
	c.DataPath = dataPath()
	c.MessageCallback = func(msg string) { a.onWebMessage(t, msg) }
	c.AcceleratorKeyCallback = func(vk uint) bool { a.focusedTab = t; return a.onAccelerator(vk) }
	c.PermissionRequestedCallback = func(raw string, kind edge.CoreWebView2PermissionKind) edge.CoreWebView2PermissionState {
		name := permissionName(kind)
		saved := a.store.Permission(permissionOrigin(raw), name)
		if saved == "allow" { return edge.CoreWebView2PermissionStateAllow }
		if saved == "deny" { return edge.CoreWebView2PermissionStateDeny }
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
		if uri == "" {
			_ = args.PutHandled(true)
			return
		}

		// A page that asks for an explicit size wants a popup, not a tab:
		// this is how sign-in flows (Google, Microsoft, GitHub) open their
		// consent window. Read the features here - the args are only valid
		// inside this callback unless a deferral is taken.
		popW, popH, popX, popY := int32(0), int32(0), int32(0), int32(0)
		wantPopup, havePos := false, false
		if wf, e := args.GetWindowFeatures(); e == nil && wf != nil {
			hasSize, _ := wf.HasSize()
			hasPos, _ := wf.HasPosition()
			if hasSize {
				rw, _ := wf.Width()
				rh, _ := wf.Height()
				popW, popH = popupSize(rw, rh, true)
				wantPopup = true
			}
			if hasPos {
				rx, _ := wf.Left()
				ry, _ := wf.Top()
				popX, popY = int32(rx), int32(ry)
				havePos = true
			}
			_ = wf.Release()
		}

		if wantPopup && (user || a.allowSpawn()) {
			// The opener MUST end up with a live handle to this window, or
			// the sign-in flow breaks in ways that look like "the popup did
			// nothing": window.close() from the provider is ignored,
			// postMessage back to the opener is dropped and popup.closed
			// never turns true. That means answering with put_NewWindow, and
			// because creating an engine is asynchronous it has to be done
			// under a deferral - the opener's script stays blocked until
			// Complete, so the handle is never observed empty.
			deferral, derr := args.GetDeferral()
			if derr != nil || deferral == nil {
				// No deferral: a tab is still better than a dead window.
				_ = args.PutHandled(true)
				a.postTask(func() { a.newTab(uri, true) })
				return
			}
			_ = args.AddRef() // the args must outlive this callback

			var owner win.RECT
			win.GetWindowRect(a.hwnd, &owner)
			x, y := popupOrigin(owner, uint32(popX), uint32(popY), havePos, popW, popH)

			finish := func(view *edge.ICoreWebView2) {
				if view != nil {
					if err := args.PutNewWindow(view); err == nil {
						_ = args.PutHandled(true)
					}
				} else {
					// The engine failed: let WebView2 open its own window
					// rather than handing the page a handle to nothing.
					_ = args.PutHandled(false)
				}
				_ = deferral.Complete()
				_ = deferral.Release()
				_ = args.Release()
			}

			if !a.openPopup(popW, popH, x, y, finish) {
				finish(nil)
			}
			return
		}

		_ = args.PutHandled(true)
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
	c.DownloadStartingCallback = func(args *edge.ICoreWebView2DownloadStartingEventArgs) { a.onDownloadStarting(t, args) }
	c.ProcessFailedCallback = func(kind edge.CoreWebView2ProcessFailedKind) {
		a.postTask(func() { a.recoverFailedTab(t, kind) })
	}
	c.NavigationStartingCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationStartingEventArgs) {
		t.dirtyForm = false
		a.onNavStarting(t, args)
	}
	c.NavigationCompletedCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		a.onNavCompleted(t, args)
	}
	t.chromium = c

	if !c.EmbedAsync(uintptr(h), func(ok bool) { a.onEngineReady(t, ok) }) {
		win.DestroyWindow(h)
		return nil
	}
	return t
}

// onEngineReady runs once the engine for t exists. It applies every setting
// that needs a live controller and then performs the navigation that was
// requested while the engine was still starting.
func (a *app) onEngineReady(t *tab, ok bool) {
	if !ok {
		// This runs inside WebView2's own completion handler. Destroying the
		// host window or opening a modal dialog here would re-enter the
		// engine while it is still unwinding, so defer it to the message
		// loop. A background spare that fails stays silent: the user never
		// asked for it, and a real tab will report the problem itself.
		wasSpare := t == a.spare
		a.postTask(func() {
			a.dropTab(t)
			if !wasSpare {
				a.engineStartFailed()
			}
		})
		return
	}
	t.ready = true
	c := t.chromium
	if c == nil {
		return
	}

	// Dark engine background: what the engine paints before the page's own
	// CSS applies - WebView2 defaults to white, which flashed on every new
	// tab and every navigation in dark mode. Needs the controller, so it
	// cannot run before the engine is ready.
	c.SetDefaultBackgroundColor(edge.COREWEBVIEW2_COLOR{A: 255, R: 28, G: 28, B: 30})

	if st, err := c.GetSettings(); err == nil {
		_ = st.PutAreDefaultContextMenusEnabled(true)
		_ = st.PutAreDevToolsEnabled(true)
		_ = st.PutIsStatusBarEnabled(true)
		_ = st.PutIsZoomControlEnabled(true)
		// Passwords, passkeys and profile autofill stay inside WebView2's
		// Windows-protected profile; the browser host never sees the values.
		autofill := a.store.SettingsView().Autofill && !incognitoMode
		_ = st.PutIsPasswordAutosaveEnabled(autofill)
		_ = st.PutIsGeneralAutofillEnabled(autofill)
	}
	// Document-created scripts must be registered before the first
	// navigation, which is why the navigation waits for this point.
	c.Init(bridgeJS)
	if t.secondary { c.Init("window.__okSecondary=true;") }
	c.Init(barJS)

	if t.pendingURL != "" {
		url := t.pendingURL
		t.pendingURL = ""
		a.navigateTab(t, url)
	} else {
		a.showStartPage(t)
	}
	if a.isActive(t) {
		c.Resize()
		c.Show()
		c.Focus()
	}
	a.scheduleBarPush(false)
}

// attachTab puts an already-created tab into the tab strip and, when asked,
// makes it the visible one.
func (a *app) attachTab(t *tab, url string, activate bool) {
	a.tabs = append(a.tabs, t)
	win.SetTimer(a.hwnd, 4, 30000, 0) // periodic inactive-tab memory trim
	if activate || len(a.tabs) == 1 {
		// Shown immediately. There used to be a cross-fade here that kept
		// the new tab hidden until its first paint (or a 700ms fallback)
		// and then ramped its alpha for another ~240ms, which is pure
		// added latency on the one action that must feel instant. The tab
		// host class paints a dark background, so revealing it at once
		// cannot flash white.
		a.switchToTab(len(a.tabs) - 1)
	} else {
		a.pushBarState() // update the visible tab strip
	}

	// A warmed spare is already showing its start page; only navigate when
	// the caller actually asked for a URL. t.pendingURL is empty here when the
	// engine came up synchronously and onEngineReady already performed the
	// navigation, which stops it being issued twice.
	if t.ready {
		if url != "" && t.pendingURL == "" && t.url != url {
			a.navigateTab(t, url)
		} else if url == "" && t.isStart && t.startRev != a.store.Rev() {
			// Browsing happened since this page was warmed, so its
			// most-visited tiles are out of date.
			a.showStartPage(t)
		}
	}
	a.scheduleBarPush(false)
	if a.inSelfTest {
		a.stlog("[selftest] tab %d created (url=%s)", len(a.tabs), url)
	}
}

// dropTab removes a tab whose engine never started.
func (a *app) dropTab(t *tab) {
	for i, x := range a.tabs {
		if x == t {
			a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
			if a.activeIdx >= len(a.tabs) {
				a.activeIdx = len(a.tabs) - 1
			}
			break
		}
	}
	if a.spare == t {
		a.spare = nil
	}
	if a.splitTab == t {
		a.splitTab = nil
	}
	if a.focusedTab == t {
		a.focusedTab = nil
	}
	t.chromium = nil
	if isWnd(t.host) {
		win.DestroyWindow(t.host)
	}
}

// engineStartFailed reports an engine that could not be created at all.
func (a *app) engineStartFailed() {
	if selfTestMode {
		selfTestFileInit(fmt.Sprintf("[selftest] FAIL: web engine failed to start for tab %d (profile locked or runtime missing)\n", a.hostSeq))
		os.Exit(1)
	}
	showRuntimeMissingDialog()
	if len(a.tabs) == 0 && a.hwnd != 0 {
		win.DestroyWindow(a.hwnd)
	}
}

// ---- spare engine ----------------------------------------------------------
//
// Creating a WebView2 engine is the slow part of opening a tab - hundreds of
// milliseconds of process and profile work. One spare engine is therefore
// warmed in the background while the browser is idle, with its start page
// already rendered, so the next Ctrl+T is a window show rather than an engine
// launch.

// warmSpareLater schedules the spare to be built once the foreground work has
// settled, so warming never competes with the page the user is waiting for.
func (a *app) warmSpareLater() {
	if a.spare != nil || a.hwnd == 0 || a.inSelfTest {
		return
	}
	win.SetTimer(a.hwnd, 6, 1200, 0)
}

// warmSpare builds the spare engine. Skipped under memory pressure: a spare
// renderer is not worth pushing a loaded machine into swapping.
func (a *app) warmSpare() {
	win.KillTimer(a.hwnd, 6)
	if a.spare != nil || a.hwnd == 0 || a.inSelfTest {
		return
	}
	if systemMemoryLoad() >= 85 {
		return
	}
	a.spare = a.createTab(false, "")
}

// takeSpare returns the warmed tab when one is ready for use. A spare that is
// still starting is left alone - waiting for it would reintroduce the stall.
func (a *app) takeSpare(secondary bool) *tab {
	t := a.spare
	if t == nil || secondary || !t.ready || t.chromium == nil || !isWnd(t.host) {
		return nil
	}
	a.spare = nil
	return t
}

// discardSpare destroys an unused warmed engine (on shutdown).
func (a *app) discardSpare() {
	t := a.spare
	if t == nil {
		return
	}
	a.spare = nil
	if t.chromium != nil {
		t.chromium.Close()
	}
	if isWnd(t.host) {
		win.DestroyWindow(t.host)
	}
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
	a.focusedTab = t
	a.splitRatio = .5
	a.layout()
	a.execActive("window.__okSplitToast&&window.__okSplitToast()")
}

// splitExistingTab turns a tab-strip edge drop into a two-pane layout.
// Dropping left promotes the dragged tab to the primary pane; dropping right
// keeps it secondary. If the active tab itself is dragged, its nearest sibling
// becomes the companion pane.
func (a *app) splitExistingTab(i int, left bool) {
	if a.splitTab != nil || i < 0 || i >= len(a.tabs) || len(a.tabs) < 2 { return }
	dragged, current := a.tabs[i], a.active()
	if dragged == current {
		companion := 0
		if i == 0 { companion = 1 }
		if left {
			a.splitTab = a.tabs[companion]
		} else {
			a.activeIdx = companion
			a.splitTab = dragged
		}
	} else if left {
		a.splitTab = current
		a.activeIdx = i
	} else {
		a.splitTab = dragged
	}
	a.focusedTab = dragged
	a.splitRatio = .5
	a.layout(); a.syncTitle(); a.pushBarState()
}

// closeSplit closes the secondary pane and restores the active page to full width.
func (a *app) closeSplit() {
	t := a.splitTab
	if t == nil { return }
	idx := -1
	for i, candidate := range a.tabs { if candidate == t { idx = i; break } }
	a.splitTab = nil
	a.focusedTab = a.active()
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
	a.focusedTab = a.tabs[idx]
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
		// Mark it inactive but do NOT hide it here. layout() below shows the
		// incoming tab first and only then hides this one, so the screen is
		// never left without a mapped tab host (which flashed the parent
		// window's background on every switch).
		cur.inactiveSince = time.Now()
	}
	a.activeIdx = i
	t := a.tabs[i]
	a.focusedTab = t
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
	// The tab is going away, but its downloads keep running and are still
	// listed on the Downloads page. Detach them so they never point at a
	// freed tab.
	a.detachDownloads(t)
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
	// Control characters never belong in a URL. A NUL in particular used to
	// reach the engine binding and panic the whole browser, and CR/LF would
	// let a crafted address smuggle extra lines into the request.
	if hasControlChars(s) {
		return
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "okbrowser://") {
		a.showInternal(t, strings.TrimPrefix(low, "okbrowser://"))
		return
	}
	u := nav.ParseWithEngine(s, a.store.SettingsView().Engine)
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
		html, title = DownloadsHTML(a.downloadFiles()), "Downloads"
	default: // start
		html, title = StartPageHTML(a.store.MostVisited(12), a.store.SettingsView().Engine), "New Tab"
		page, isStart = "start", true
	}
	t.isStart = isStart
	t.errPage = false
	t.title = title
	if isStart {
		t.url = ""
		t.startRev = a.store.Rev()
	} else {
		t.url = "okbrowser://" + page
	}
	if a.isActive(t) {
		a.syncTitle()
		a.pushBarState()
	}
	t.chromium.NavigateToString(html)
}

// recoverFailedTab reloads an isolated renderer/GPU failure without taking
// down the browser. Repeated failures stop auto-reloading and show a stable
// recovery page so a bad site cannot create an endless crash loop.
func (a *app) recoverFailedTab(t *tab, kind edge.CoreWebView2ProcessFailedKind) {
	if t == nil || t.chromium == nil { return }
	now := time.Now()
	if now.Sub(t.lastCrash) > time.Minute { t.crashCount = 0 }
	t.lastCrash, t.crashCount = now, t.crashCount+1
	_ = os.MkdirAll(dataDir(), 0o755)
	f, _ := os.OpenFile(filepath.Join(dataDir(), "crash.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if f != nil { fmt.Fprintf(f, "%s kind=%d url=%s\n", now.Format(time.RFC3339), kind, t.url); _ = f.Close() }
	if t.crashCount <= 2 {
		t.chromium.Reload()
		return
	}
	t.errPage = true
	t.title = "Page crashed"
	html := `<!doctype html><meta name="viewport" content="width=device-width"><style>body{background:#151519;color:#f2f2f7;font:15px system-ui;display:grid;place-items:center;height:100vh;margin:0}.c{text-align:center;max-width:460px}button{border:0;border-radius:18px;padding:11px 18px;background:#0a84ff;color:white}</style><div class=c><h1>This page keeps crashing</h1><p>OK Browser stopped the reload loop. Your other tabs are safe.</p><button onclick="location.reload()">Try again</button></div>`
	t.chromium.NavigateToString(html)
	a.pushBarState()
}

func (a *app) setTabSleeping(i int, sleep bool) {
	if i < 0 || i >= len(a.tabs) { return }
	t := a.tabs[i]
	if sleep {
		if t == a.active() || t == a.splitTab || t.audioPlaying || t.dirtyForm || a.hasActiveDownload(t) { return }
		t.chromium.CallDevToolsProtocol("Page.setWebLifecycleState", `{"state":"frozen"}`)
	} else { t.chromium.CallDevToolsProtocol("Page.setWebLifecycleState", `{"state":"active"}`) }
	t.sleeping = sleep; a.pushBarState()
}

// sleepInactiveTabs freezes background pages after five idle minutes. Pinned
// tabs and either Split View pane stay live. WebView2 keeps page state in memory
// and resumes it instantly when selected.
func (a *app) sleepInactiveTabs() {
	// Read-only: no NeverSleep copy needed.
	settings := a.store.SettingsView()
	minutes := settings.SleepMinutes
	pressure := systemMemoryLoad() >= 88
	if minutes <= 0 && !pressure { return }
	now := time.Now()
	froze := false
	for _, t := range a.tabs {
		if t == a.active() || t == a.splitTab || t.pinned || t.audioPlaying || t.dirtyForm || a.hasActiveDownload(t) || t.sleeping || t.inactiveSince.IsZero() { continue }
		if settings.NeverSleep[permissionOrigin(t.url)] { continue }
		if !pressure && now.Sub(t.inactiveSince) < time.Duration(minutes)*time.Minute { continue }
		t.chromium.CallDevToolsProtocol("Page.setWebLifecycleState", `{"state":"frozen"}`)
		t.sleeping = true
		froze = true
	}
	// Only touch the shell when something actually changed. This sweep runs
	// every 30 seconds for the life of the process, and it used to serialise
	// and evaluate a full bar state each time even when nothing was frozen.
	if froze {
		a.pushBarState()
	}
}

// selftestClickTick dispatches the deferred trusted click once the target tab
// is actually usable. Chromium ignores synthesized input aimed at a widget
// that is not showing yet, and tab engines now start asynchronously, so wait
// for readiness (previously this waited for the cross-fade to settle).
func (a *app) selftestClickTick() {
	t := a.stClickTab
	if t == nil || t.chromium == nil {
		win.KillTimer(a.hwnd, 3)
		return
	}
	a.stClickTicks++
	if !t.ready && a.stClickTicks < 80 { // up to ~4s
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
	if a.isActive(t) {
		a.execActive("window.__okLoad&&window.__okLoad(false)")
	}
	if args != nil && !t.errPage {
		if ok, err := args.GetIsSuccess(); err == nil && !ok {
			code, _ := args.GetWebErrorStatus()
			// A navigation that turned into a download always completes
			// "unsuccessfully" (typically 9 = ConnectionAborted) because no
			// document was loaded. The download itself is fine, so keep the
			// current page instead of blowing it away with an error page.
			if !t.downloadAt.IsZero() && time.Since(t.downloadAt) < 5*time.Second {
				t.downloadAt = time.Time{}
				return
			}
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
// hasControlChars reports whether s contains a C0/DEL control character
// (tab excepted). Such bytes are never valid in an address and one of them,
// NUL, used to panic the WebView2 string conversion and kill the browser.
func hasControlChars(s string) bool {
	for _, r := range s {
		if r == 0x7f || (r < 0x20 && r != '\t') {
			return true
		}
	}
	return false
}

func htmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return r.Replace(s)
}
