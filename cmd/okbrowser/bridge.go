//go:build windows

package main

import (
	"encoding/json"
	"time"

	"github.com/lxn/win"

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
  if (window.top !== window) return;
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

// barJS builds the Liquid Glass bar: a floating frosted capsule with pill
// tabs, a capsule address field and tiny circular buttons, rendered inside
// the page compositor. backdrop-filter gives real blur of the page content
// scrolling underneath - the same technique Apple-style browsers use.
//
// The bar lives in a closed shadow root with constructed stylesheets, so
// page CSS cannot touch it and it cannot leak into the page, even on sites
// with strict Content-Security-Policies.
const barJS = `
(function () {
  if (window.top !== window) return;
  if (window.__okBarInstalled) return;
  window.__okBarInstalled = true;

  var S = { tabs: [{ t: "New Tab" }], a: 0, u: "", b: false, f: false };

  var SV = function (inner) {
    return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' + inner + '</svg>';
  };
  var I_BACK = SV('<path d="M15 18l-6-6 6-6"/>');
  var I_FWD  = SV('<path d="M9 18l6-6-6-6"/>');
  var I_RL   = SV('<polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>');
  var I_PLUS = SV('<path d="M12 5v14M5 12h14"/>');
  var I_X    = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"><path d="M18 6L6 18M6 6l12 12"/></svg>';

  var host = document.createElement('div');
  var root = host.attachShadow({ mode: 'closed' });

  var css = [
    ".bar{position:fixed;top:6px;left:8px;right:8px;height:46px;z-index:2147483647;",
    "display:flex;align-items:center;gap:8px;padding:0 10px;border-radius:23px;box-sizing:border-box;",
    "font-family:-apple-system,'Segoe UI Variable Text','Segoe UI',system-ui,sans-serif;",
    "background:rgba(250,250,252,.62);",
    "backdrop-filter:blur(28px) saturate(1.8);-webkit-backdrop-filter:blur(28px) saturate(1.8);",
    "box-shadow:0 10px 34px rgba(0,0,0,.16),0 2px 8px rgba(0,0,0,.08),",
    "inset 0 1px 0 rgba(255,255,255,.65),inset 0 0 0 .5px rgba(255,255,255,.35)}",
    "@media (prefers-color-scheme:dark){.bar{background:rgba(28,28,32,.60);",
    "box-shadow:0 10px 34px rgba(0,0,0,.42),0 2px 8px rgba(0,0,0,.30),",
    "inset 0 1px 0 rgba(255,255,255,.10),inset 0 0 0 .5px rgba(255,255,255,.08)}}",
    ".tabs{display:flex;gap:4px;flex:0 1 auto;min-width:0;overflow:hidden;height:100%;align-items:center}",
    ".tab{display:flex;align-items:center;gap:2px;height:32px;min-width:0;flex:0 1 150px;",
    "padding:0 7px 0 12px;border-radius:16px;cursor:default;",
    "transition:background .16s ease,transform .16s ease;animation:okpop .18s ease}",
    ".tab:hover{background:rgba(120,128,138,.14)}",
    ".tab:active{transform:scale(.96)}",
    ".tab.on{background:rgba(255,255,255,.82);",
    "box-shadow:0 1px 6px rgba(0,0,0,.10),inset 0 0 0 .5px rgba(0,0,0,.04)}",
    "@media (prefers-color-scheme:dark){.tab.on{background:rgba(255,255,255,.16);",
    "box-shadow:0 1px 6px rgba(0,0,0,.35),inset 0 0 0 .5px rgba(255,255,255,.06)}}",
    ".tt{font-size:12.5px;font-weight:500;color:#3c4043;white-space:nowrap;",
    "overflow:hidden;text-overflow:ellipsis;flex:1 1 auto;min-width:0}",
    "@media (prefers-color-scheme:dark){.tt{color:#e8eaed}}",
    ".x{flex:0 0 auto;width:19px;height:19px;border-radius:50%;display:none;",
    "place-items:center;color:#5f6368;opacity:.8;transition:background .14s,opacity .14s}",
    ".x:hover{background:rgba(120,128,138,.22);opacity:1}",
    "@media (prefers-color-scheme:dark){.x{color:#e8eaed}}",
    ".x svg{width:9px;height:9px}",
    ".tab.on .x,.tab:hover .x{display:grid}",
    ".plus{flex:0 0 auto;width:30px;height:30px;border-radius:50%;display:grid;place-items:center;",
    "color:#5f6368;cursor:default;transition:background .15s,transform .12s}",
    "@media (prefers-color-scheme:dark){.plus{color:#e8eaed}}",
    ".plus:hover{background:rgba(120,128,138,.16)}",
    ".plus:active{transform:scale(.88)}",
    ".plus svg{width:13px;height:13px}",
    ".addr{flex:1 1 auto;max-width:560px;min-width:120px;height:32px;margin:0 auto;",
    "border-radius:16px;display:flex;align-items:center;padding:0 14px;box-sizing:border-box;",
    "background:rgba(255,255,255,.66);box-shadow:inset 0 0 0 .5px rgba(0,0,0,.06);",
    "transition:box-shadow .16s,background .16s}",
    "@media (prefers-color-scheme:dark){.addr{background:rgba(255,255,255,.10);",
    "box-shadow:inset 0 0 0 .5px rgba(255,255,255,.07)}}",
    ".addr:focus-within{background:rgba(255,255,255,.95);",
    "box-shadow:0 0 0 3px rgba(10,132,255,.32),inset 0 0 0 .5px rgba(0,0,0,.04)}",
    "@media (prefers-color-scheme:dark){.addr:focus-within{background:rgba(40,42,46,.95);",
    "box-shadow:0 0 0 3px rgba(10,132,255,.45),inset 0 0 0 .5px rgba(255,255,255,.08)}}",
    ".addr input{all:unset;width:100%;font-size:13.5px;color:#202124;font-family:inherit;",
    "caret-color:#0a84ff}",
    "@media (prefers-color-scheme:dark){.addr input{color:#e8eaed}}",
    ".addr input::placeholder{color:#80868b}",
    ".btns{display:flex;gap:2px;flex:0 0 auto}",
    ".btn{width:30px;height:30px;border-radius:50%;display:grid;place-items:center;",
    "color:#3c4043;cursor:default;transition:background .15s,transform .12s,opacity .15s}",
    "@media (prefers-color-scheme:dark){.btn{color:#e8eaed}}",
    ".btn:hover{background:rgba(120,128,138,.16)}",
    ".btn:active{transform:scale(.88)}",
    ".btn[disabled]{opacity:.26;pointer-events:none}",
    ".btn svg{width:15px;height:15px}",
    "@keyframes okpop{from{transform:scale(.6);opacity:0}to{transform:scale(1);opacity:1}}",
    "@media print{.bar{display:none !important}}"
  ].join("");

  var sheet = new CSSStyleSheet();
  sheet.replaceSync(css);
  root.adoptedStyleSheets = [sheet];

  root.innerHTML =
    '<div class="bar">' +
      '<div class="tabs" id="tabs"></div>' +
      '<div class="plus" id="plus">' + I_PLUS + '</div>' +
      '<div class="addr" id="addr">' +
        '<input id="a" placeholder="Search or enter address" spellcheck="false" autocomplete="off" autocapitalize="off">' +
      '</div>' +
      '<div class="btns">' +
        '<div class="btn" id="back" title="Back">' + I_BACK + '</div>' +
        '<div class="btn" id="fwd" title="Forward">' + I_FWD + '</div>' +
        '<div class="btn" id="rl" title="Reload">' + I_RL + '</div>' +
      '</div>' +
    '</div>';

  var input = root.getElementById('a');
  var post = function (o) { window.__ok(o); };

  function render() {
    var wrap = root.getElementById('tabs');
    wrap.textContent = '';
    var tabs = S.tabs || [];
    for (var i = 0; i < tabs.length; i++) {
      (function (idx) {
        var el = document.createElement('div');
        el.className = 'tab' + (idx === S.a ? ' on' : '');
        var sp = document.createElement('span');
        sp.className = 'tt';
        sp.textContent = tabs[idx].t || 'New Tab';
        el.appendChild(sp);
        var x = document.createElement('div');
        x.className = 'x';
        x.innerHTML = I_X;
        x.addEventListener('click', function (ev) {
          ev.stopPropagation();
          post({ t: 'ui', a: 'close', i: idx });
        });
        el.appendChild(x);
        el.addEventListener('click', function () {
          if (idx !== S.a) post({ t: 'ui', a: 'switch', i: idx });
        });
        el.addEventListener('auxclick', function (ev) {
          if (ev.button === 1) { ev.preventDefault(); post({ t: 'ui', a: 'close', i: idx }); }
        });
        wrap.appendChild(el);
      })(i);
    }
    root.getElementById('back').toggleAttribute('disabled', !S.b);
    root.getElementById('fwd').toggleAttribute('disabled', !S.f);
  }

  function sync() {
    if (document.activeElement !== input) input.value = S.u || '';
  }

  root.getElementById('plus').addEventListener('click', function () { post({ t: 'ui', a: 'new' }); });
  root.getElementById('back').addEventListener('click', function () { post({ t: 'ui', a: 'back' }); });
  root.getElementById('fwd').addEventListener('click', function () { post({ t: 'ui', a: 'forward' }); });
  root.getElementById('rl').addEventListener('click', function () { post({ t: 'ui', a: 'reload' }); });

  input.addEventListener('keydown', function (e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      if (input.value.trim()) post({ t: 'go', u: input.value });
      input.blur();
      post({ t: 'ui', a: 'refocus' });
    } else if (e.key === 'Escape') {
      e.preventDefault();
      sync();
      input.blur();
      post({ t: 'ui', a: 'refocus' });
    }
  });

  document.addEventListener('fullscreenchange', function () {
    host.style.display = document.fullscreenElement ? 'none' : '';
  });

  window.__okBar = function (s) { S = s; render(); sync(); };
  window.__okBarFocus = function () { input.focus(); input.select(); };

  (document.body || document.documentElement).appendChild(host);
  render();
  sync();
})();
`

// barTab is one tab entry for the in-page bar.
type barTab struct {
	T string `json:"t"`
}

// barState is the full state pushed to the active tab's bar.
type barState struct {
	Tabs []barTab `json:"tabs"`
	A    int      `json:"a"`
	U    string   `json:"u"`
	B    bool     `json:"b"`
	F    bool     `json:"f"`
}

// pushBarState sends tab list, address and navigation state to the bar of
// the active tab. Safe to call at any time; no-op when there is no tab.
func (a *app) pushBarState() {
	t := a.active()
	if t == nil || t.chromium == nil {
		return
	}
	tabs := make([]barTab, len(a.tabs))
	for i, tb := range a.tabs {
		title := tb.title
		if title == "" {
			title = "New Tab"
		}
		tabs[i] = barTab{T: title}
	}
	st := barState{
		Tabs: tabs,
		A:    a.activeIdx,
		U:    t.url,
		B:    t.chromium.CanGoBack(),
		F:    t.chromium.CanGoForward(),
	}
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	a.execActive("window.__okBar&&window.__okBar(" + string(b) + ")")
}

// scheduleBarPush re-pushes bar state shortly after a load, covering the
// moment when the bar script has just been installed in a new document.
func (a *app) scheduleBarPush() {
	if a.hwnd != 0 {
		win.SetTimer(a.hwnd, 1, 150, 0)
	}
}

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
		A string `json:"a"`
		I int    `json:"i"`
	}
	if err := json.Unmarshal([]byte(msg), &m); err != nil {
		return
	}

	switch m.T {
	case "open": // link explicitly asking for a new window
		if m.U != "" && a.allowSpawn() {
			a.newTab(m.U, true)
		}

	case "go": // address bar (or start page) - parsed the same way
		if u := nav.Parse(m.U); u != "" && t.chromium != nil {
			t.isStart = false
			t.url = u
			if a.isActive(t) {
				a.pushBarState()
			}
			t.chromium.Navigate(u)
			t.chromium.Focus()
		}

	case "nav": // page reported its URL and title
		if m.U == "" || m.U == "about:blank" {
			t.isStart = true
			t.title = "New Tab"
			t.url = ""
		} else {
			t.isStart = false
			t.url = m.U
			if m.D != "" {
				t.title = m.D
			} else {
				t.title = m.U
			}
		}
		a.syncTitle()
		a.pushBarState()
		a.scheduleBarPush()

	case "ui": // the glass bar
		switch m.A {
		case "back":
			if t.chromium != nil && t.chromium.CanGoBack() {
				t.chromium.GoBack()
			}
		case "forward":
			if t.chromium != nil && t.chromium.CanGoForward() {
				t.chromium.GoForward()
			}
		case "reload":
			if t.chromium != nil {
				if t.isStart {
					a.showStartPage(t)
				} else {
					t.chromium.Reload()
				}
			}
		case "refocus":
			if t.chromium != nil {
				t.chromium.Focus()
			}
		case "new":
			a.newTab("", true)
			return
		case "close":
			a.closeTab(m.I)
			return
		case "switch":
			a.switchToTab(m.I)
			return
		}
		a.pushBarState()
	}
}
