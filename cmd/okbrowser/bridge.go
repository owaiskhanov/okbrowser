//go:build windows

package main

import (
	"encoding/json"
	"time"

	"github.com/jchv/go-webview2/pkg/edge"

	"github.com/owaiskhanov/okbrowser/internal/nav"
)

// bridgeJS is injected into every page before any of its own scripts run.
// It provides:
//
//   - window.__ok(obj):  post a JSON message to the host application
//   - target=_blank links, middle-clicks and window.open() forwarded to the
//     host so they can open as a new OK Browser window
const bridgeJS = `
window.__ok = function (o) {
  try { window.chrome.webview.postMessage(JSON.stringify(o)); } catch (e) {}
};
(function () {
  function anchor(el) { return el && el.closest ? el.closest("a") : null; }
  document.addEventListener("click", function (e) {
    var a = anchor(e.target);
    if (a && a.target && a.target !== "_self") {
      e.preventDefault();
      window.__ok({ t: "open", u: a.href });
    }
  }, true);
  document.addEventListener("auxclick", function (e) {
    var a = anchor(e.target);
    if (a && e.button === 1) {
      e.preventDefault();
      window.__ok({ t: "open", u: a.href });
    }
  }, true);
  window.open = function (u) {
    if (u) window.__ok({ t: "open", u: String(u) });
    return null;
  };
})();
`

// embedWebView creates the WebView2 engine inside the host child window and
// wires up its callbacks.
func (a *app) embedWebView() bool {
	c := edge.NewChromium()
	c.DataPath = dataPath()
	c.MessageCallback = a.onWebMessage
	c.NavigationCompletedCallback = a.onNavCompleted
	a.chromium = c

	// Embed runs a small nested message pump until the engine is ready;
	// it returns false when the WebView2 runtime is missing.
	if !c.Embed(uintptr(a.host)) {
		showRuntimeMissingDialog()
		return false
	}

	// Browser-like settings (context menu and DevTools are on by default;
	// the status bar and zoom control are not, so we enable them).
	if st, err := c.GetSettings(); err == nil {
		_ = st.PutAreDefaultContextMenusEnabled(true) // right-click menu
		_ = st.PutAreDevToolsEnabled(true)            // F12
		_ = st.PutIsStatusBarEnabled(true)            // link preview on hover
		_ = st.PutIsZoomControlEnabled(true)          // Ctrl + mouse wheel
	}

	c.Init(bridgeJS)
	return true
}

// onNavStarting fires the instant a navigation begins - before any network
// activity - so the address bar updates immediately instead of waiting for
// the page to finish loading.
func (a *app) onNavStarting(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationStartingEventArgs) {
	uri, err := args.GetUri()
	if err != nil || uri == "" || uri == "about:blank" {
		return // our built-in start page (NavigateToString) or unknown
	}
	a.isStart = false
	setWindowText(a.address, uri)
	a.updateNavButtons()
}

// onNavCompleted fires after each navigation; it asks the page to report its
// final URL and title so the address bar and window title stay in sync.
func (a *app) onNavCompleted(*edge.ICoreWebView2, *edge.ICoreWebView2NavigationCompletedEventArgs) {
	a.updateNavButtons()
	a.applyZoom() // new documents start at 100%; restore the window's zoom
	a.chromium.Eval(`window.__ok && window.__ok({ t: "nav", u: location.href, d: document.title })`)
}

// allowSpawn rate-limits new-window requests coming from web pages.
func (a *app) allowSpawn() bool {
	now := time.Now()
	if now.Sub(a.lastSpawn) < 300*time.Millisecond {
		return false
	}
	a.lastSpawn = now
	return true
}

// onWebMessage receives JSON messages posted by pages via window.__ok.
func (a *app) onWebMessage(msg string) {
	var m struct {
		T string `json:"t"`
		U string `json:"u"`
		D string `json:"d"`
	}
	if err := json.Unmarshal([]byte(msg), &m); err != nil {
		return
	}

	switch m.T {
	case "open": // link explicitly asking for a new window
		if m.U != "" && a.allowSpawn() {
			spawnNewWindow(m.U)
		}

	case "go": // start page search box - parsed exactly like the address bar
		if u := nav.Parse(m.U); u != "" {
			a.isStart = false
			setWindowText(a.address, u) // instant feedback
			a.chromium.Navigate(u)
		}

	case "nav": // page reported its URL and title
		if m.U == "" || m.U == "about:blank" {
			// The built-in start page.
			a.isStart = true
			setWindowText(a.hwnd, appName)
			setWindowText(a.address, "")
			return
		}
		a.isStart = false
		setWindowText(a.address, m.U)
		title := m.D
		if title == "" {
			title = m.U
		}
		setWindowText(a.hwnd, title+" - "+appName)
	}
}
