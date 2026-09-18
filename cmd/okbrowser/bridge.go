//go:build windows

package main

import (
	"encoding/json"
	"time"

	"github.com/owaiskhanov/okbrowser/internal/nav"
)

// bridgeJS is injected into every page before any of its own scripts run.
// It provides:
//
//   - window.__ok(obj):  post a JSON message to the host application
//   - target=_blank links, middle-clicks and window.open() forwarded to the
//     host so they can open as a new tab
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

// allowSpawn rate-limits new-tab requests coming from web pages.
func (a *app) allowSpawn() bool {
	now := time.Now()
	if now.Sub(a.lastSpawn) < 300*time.Millisecond {
		return false
	}
	a.lastSpawn = now
	return true
}

// onWebMessage receives JSON messages posted by tab t via window.__ok.
func (a *app) onWebMessage(t *tab, msg string) {
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
			a.newTab(m.U, true)
		}

	case "go": // start page search box - parsed exactly like the address bar
		if u := nav.Parse(m.U); u != "" {
			t.isStart = false
			t.url = u
			if a.isActive(t) {
				setWindowText(a.address, u)
			}
			t.chromium.Navigate(u)
		}

	case "nav": // page reported its URL and title
		if m.U == "" || m.U == "about:blank" {
			t.isStart = true
			t.title = "New Tab"
			if a.isActive(t) {
				setWindowText(a.address, "")
			}
		} else {
			t.isStart = false
			t.url = m.U
			if m.D != "" {
				t.title = m.D
			} else {
				t.title = m.U
			}
		}
		if a.isActive(t) {
			a.syncTitle()
		}
		a.invalidateBar() // tab titles are drawn on the bar
	}
}
