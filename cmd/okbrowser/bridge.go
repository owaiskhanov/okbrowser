//go:build windows

package main

import (
	"encoding/json"
	"fmt"
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
    if (a && a.target && a.target !== "_self" && !a.hasAttribute("data-ok-engine")) {
      e.preventDefault();
      window.__ok({ t: "open", u: a.href });
    }
  }, true);
  document.addEventListener("auxclick", function (e) {
    var a = anchor(e.target);
    if (a && e.button === 1 && !a.hasAttribute("data-ok-engine")) {
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

// barJS builds the Liquid Glass shell UI, rendered inside the page
// compositor in a closed shadow root (immune to page CSS and CSP):
//
//   - a frameless top bar: glass tab pills, a "+", the address bubble
//     (it expands in place on hover / click / Ctrl+L) and Windows
//     min/max/close buttons; the empty middle is a native drag zone
//     (drag to move the window, double-click to maximize)
//   - a find-in-page capsule (Ctrl+F) with next / previous and a match
//     counter, matching Chrome's behavior
const barJS = `
(function () {
  if (window.top !== window) return;
  if (window.__okBarInstalled) return;
  window.__okBarInstalled = true;

  var S = { tabs: [{ t: "New Tab" }], a: 0, u: "", b: false, f: false, m: false };

  var SV = function (inner) {
    return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' + inner + '</svg>';
  };
  var I_BACK = SV('<path d="M15 18l-6-6 6-6"/>');
  var I_FWD  = SV('<path d="M9 18l6-6-6-6"/>');
  var I_RL   = SV('<polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>');
  var I_PLUS = SV('<path d="M12 5v14M5 12h14"/>');
  var I_X    = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"><path d="M18 6L6 18M6 6l12 12"/></svg>';
  var I_LENS = SV('<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.35-4.35"/>');
  var I_GO   = SV('<path d="M5 12h13"/><path d="M13 6l6 6-6 6"/>');
  var I_MIN  = SV('<path d="M5 12h14"/>');
  var I_MAX  = SV('<rect x="5.5" y="5.5" width="13" height="13" rx="2"/>');
  var I_RST  = SV('<rect x="8.5" y="5.5" width="10" height="10" rx="2"/><path d="M5.5 15.5a3.5 3.5 0 0 0 3.5 3.5h7"/>');
  var I_UP   = SV('<path d="M6 15l6-6 6 6"/>');
  var I_DOWN = SV('<path d="M6 9l6 6 6-6"/>');

  var host = document.createElement('div');
  var root = host.attachShadow({ mode: 'closed' });

  var css = [
    ":host{all:initial}",
    "*{-webkit-user-select:none}",
    ".strip{position:fixed;top:0;left:0;right:0;height:34px;z-index:2147483647;",
    "display:flex;align-items:center;gap:5px;padding:0 2px 0 8px;pointer-events:none;",
    "font-family:-apple-system,'Segoe UI Variable Text','Segoe UI',system-ui,sans-serif}",
    ".tz{display:flex;gap:4px;align-items:center;min-width:0;height:100%;",
    "flex:0 1 auto;overflow:hidden;pointer-events:auto}",
    ".tab{display:flex;align-items:center;gap:2px;height:23px;min-width:0;flex:0 1 118px;",
    "padding:0 5px 0 9px;border-radius:12px;cursor:default;",
    "background:rgba(250,250,252,.30);",
    "backdrop-filter:blur(20px) saturate(1.6);-webkit-backdrop-filter:blur(20px) saturate(1.6);",
    "box-shadow:0 1px 6px rgba(0,0,0,.08),inset 0 1px 0 rgba(255,255,255,.42),",
    "inset 0 0 0 .5px rgba(255,255,255,.20);",
    "transition:background .16s ease,transform .16s ease}",
    ".tab.in{animation:okin .24s cubic-bezier(.2,.8,.3,1)}",
    "@keyframes okin{from{transform:scale(.72);opacity:0}to{transform:scale(1);opacity:1}}",
    ".tab:hover{background:rgba(250,250,252,.48)}",
    ".tab:active{transform:scale(.95)}",
    ".tab.on{background:rgba(255,255,255,.72);",
    "box-shadow:0 2px 10px rgba(0,0,0,.12),inset 0 1px 0 rgba(255,255,255,.6),",
    "inset 0 0 0 .5px rgba(0,0,0,.03)}",
    "@media (prefers-color-scheme:dark){.tab{background:rgba(38,38,42,.30);",
    "box-shadow:0 1px 6px rgba(0,0,0,.26),inset 0 1px 0 rgba(255,255,255,.06),",
    "inset 0 0 0 .5px rgba(255,255,255,.05)}",
    ".tab:hover{background:rgba(38,38,42,.52)}",
    ".tab.on{background:rgba(255,255,255,.18);",
    "box-shadow:0 2px 10px rgba(0,0,0,.34),inset 0 1px 0 rgba(255,255,255,.09),",
    "inset 0 0 0 .5px rgba(255,255,255,.06)}}",
    ".tt{font-size:11px;font-weight:500;color:#3c4043;white-space:nowrap;",
    "overflow:hidden;text-overflow:ellipsis;flex:1 1 auto;min-width:0}",
    "@media (prefers-color-scheme:dark){.tt{color:#e8eaed}}",
    ".tx{flex:0 0 auto;width:16px;height:16px;border-radius:50%;display:none;",
    "place-items:center;color:#5f6368;opacity:.85;transition:background .14s,opacity .14s}",
    ".tx:hover{background:rgba(120,128,138,.24);opacity:1}",
    "@media (prefers-color-scheme:dark){.tx{color:#e8eaed}}",
    ".tx svg{width:8px;height:8px}",
    ".tab.on .tx,.tab:hover .tx{display:grid}",
    ".plus{flex:0 0 auto;width:25px;height:25px;border-radius:50%;display:grid;place-items:center;",
    "color:#5f6368;cursor:default;background:rgba(250,250,252,.34);",
    "backdrop-filter:blur(20px) saturate(1.6);-webkit-backdrop-filter:blur(20px) saturate(1.6);",
    "box-shadow:0 2px 10px rgba(0,0,0,.10),inset 0 0 0 .5px rgba(255,255,255,.28);",
    "transition:background .15s,transform .12s;pointer-events:auto}",
    "@media (prefers-color-scheme:dark){.plus{color:#e8eaed;background:rgba(38,38,42,.34)}}",
    ".plus:hover{background:rgba(250,250,252,.6)}",
    "@media (prefers-color-scheme:dark){.plus:hover{background:rgba(38,38,42,.58)}}",
    ".plus:active{transform:scale(.86)}",
    ".plus svg{width:11px;height:11px}",
    ".drag{flex:1 1 auto;height:100%;pointer-events:auto}",
    ".wbtns{display:flex;height:100%;pointer-events:auto}",
    ".wbtn{width:42px;height:100%;display:grid;place-items:center;color:#3c4043;",
    "cursor:default;transition:background .12s,opacity .12s}",
    "@media (prefers-color-scheme:dark){.wbtn{color:#e8eaed}}",
    ".wbtn svg{width:11px;height:11px}",
    ".wbtn:hover{background:rgba(120,128,138,.20)}",
    ".wbtn:active{background:rgba(120,128,138,.32)}",
    ".wbtn.close:hover{background:#e81123;color:#fff}",
    ".wbtn.close:active{background:#c50f1d;color:#fff}",
    ".okb{position:relative;flex:0 1 auto;height:26px;width:28px;border-radius:14px;",
    "display:flex;align-items:center;overflow:hidden;cursor:default;pointer-events:auto;",
    "background:rgba(250,250,252,.42);",
    "backdrop-filter:blur(24px) saturate(1.7);-webkit-backdrop-filter:blur(24px) saturate(1.7);",
    "box-shadow:0 2px 10px rgba(0,0,0,.12),inset 0 1px 0 rgba(255,255,255,.55),",
    "inset 0 0 0 .5px rgba(255,255,255,.30);",
    "transition:width .3s cubic-bezier(.32,.72,.24,1)}",
    "@media (prefers-color-scheme:dark){.okb{background:rgba(28,28,32,.46);",
    "box-shadow:0 2px 10px rgba(0,0,0,.34),inset 0 1px 0 rgba(255,255,255,.10),",
    "inset 0 0 0 .5px rgba(255,255,255,.08)}}",
    ".okb.open{width:min(430px,44vw)}",
    ".lens{position:absolute;inset:0;display:grid;place-items:center;color:#3c4043}",
    "@media (prefers-color-scheme:dark){.lens{color:#e8eaed}}",
    ".lens svg{width:14px;height:14px}",
    ".okb.open .lens{display:none}",
    ".inner{display:flex;align-items:center;gap:2px;width:100%;height:100%;",
    "padding:0 5px 0 4px;opacity:0;transition:opacity .16s;pointer-events:none}",
    ".okb.open .inner{opacity:1;pointer-events:auto}",
    ".bb{flex:0 0 auto;width:24px;height:24px;border-radius:50%;display:grid;place-items:center;",
    "color:#3c4043;cursor:default;transition:background .14s,transform .12s,opacity .14s}",
    "@media (prefers-color-scheme:dark){.bb{color:#e8eaed}}",
    ".bb:hover{background:rgba(120,128,138,.16)}",
    ".bb:active{transform:scale(.86)}",
    ".bb[disabled]{opacity:.26;pointer-events:none}",
    ".bb svg{width:12px;height:12px}",
    ".inner input{all:unset;flex:1 1 auto;min-width:40px;font-size:12.5px;color:#202124;",
    "font-family:inherit;caret-color:#0a84ff;cursor:text;-webkit-user-select:text}",
    "@media (prefers-color-scheme:dark){.inner input{color:#e8eaed}}",
    ".inner input::placeholder{color:#80868b}",
    ".go{flex:0 0 auto;width:24px;height:24px;border-radius:50%;display:grid;place-items:center;",
    "color:#fff;background:rgba(10,132,255,.92);cursor:default;",
    "transition:transform .12s,background .14s}",
    ".go:hover{background:rgba(10,132,255,1)}",
    ".go:active{transform:scale(.88)}",
    ".go svg{width:12px;height:12px}",
    ".find{position:fixed;top:38px;right:10px;height:32px;display:none;align-items:center;gap:2px;",
    "padding:0 5px 0 12px;border-radius:16px;z-index:2147483647;pointer-events:auto;",
    "font-family:-apple-system,'Segoe UI Variable Text','Segoe UI',system-ui,sans-serif;",
    "background:rgba(250,250,252,.72);",
    "backdrop-filter:blur(28px) saturate(1.8);-webkit-backdrop-filter:blur(28px) saturate(1.8);",
    "box-shadow:0 8px 28px rgba(0,0,0,.22),inset 0 1px 0 rgba(255,255,255,.6),",
    "inset 0 0 0 .5px rgba(255,255,255,.35)}",
    "@media (prefers-color-scheme:dark){.find{background:rgba(28,28,32,.74);",
    "box-shadow:0 8px 28px rgba(0,0,0,.5),inset 0 1px 0 rgba(255,255,255,.10),",
    "inset 0 0 0 .5px rgba(255,255,255,.08)}}",
    ".find.open{display:flex;animation:okin .16s ease}",
    ".find input{all:unset;width:180px;font-size:12.5px;color:#202124;",
    "font-family:inherit;caret-color:#0a84ff;cursor:text;-webkit-user-select:text}",
    "@media (prefers-color-scheme:dark){.find input{color:#e8eaed}}",
    ".fc{flex:0 0 auto;font-size:11px;color:#5f6368;padding:0 6px;min-width:36px;text-align:center}",
    "@media (prefers-color-scheme:dark){.fc{color:#9aa0a6}}",
    ".fb{flex:0 0 auto;width:24px;height:24px;border-radius:50%;display:grid;place-items:center;",
    "color:#3c4043;cursor:default;transition:background .14s}",
    "@media (prefers-color-scheme:dark){.fb{color:#e8eaed}}",
    ".fb:hover{background:rgba(120,128,138,.16)}",
    ".fb svg{width:13px;height:13px}",
    "@media print{.strip,.find{display:none !important}}"
  ].join("");

  var sheet = new CSSStyleSheet();
  sheet.replaceSync(css);
  root.adoptedStyleSheets = [sheet];

  root.innerHTML =
    '<div class="strip">' +
      '<div class="tz" id="tz"></div>' +
      '<div class="plus" id="plus">' + I_PLUS + '</div>' +
      '<div class="okb" id="okb">' +
        '<div class="lens" id="lens">' + I_LENS + '</div>' +
        '<div class="inner">' +
          '<div class="bb" id="bback" title="Back">' + I_BACK + '</div>' +
          '<div class="bb" id="bfwd" title="Forward">' + I_FWD + '</div>' +
          '<div class="bb" id="brl" title="Reload">' + I_RL + '</div>' +
          '<input id="q" placeholder="Search or enter address" spellcheck="false" autocomplete="off" autocapitalize="off">' +
          '<div class="go" id="go" title="Go">' + I_GO + '</div>' +
        '</div>' +
      '</div>' +
      '<div class="drag" id="drag"></div>' +
      '<div class="wbtns">' +
        '<div class="wbtn" id="wmin" title="Minimize">' + I_MIN + '</div>' +
        '<div class="wbtn" id="wmax" title="Maximize">' + I_MAX + '</div>' +
        '<div class="wbtn close" id="wclose" title="Close">' + I_X + '</div>' +
      '</div>' +
    '</div>' +
    '<div class="find" id="find">' +
      '<input id="fq" placeholder="Find in page" spellcheck="false">' +
      '<div class="fc" id="fc">0/0</div>' +
      '<div class="fb" id="fprev" title="Previous (Shift+Enter)">' + I_UP + '</div>' +
      '<div class="fb" id="fnext" title="Next (Enter)">' + I_DOWN + '</div>' +
      '<div class="fb" id="fclose" title="Close (Esc)">' + I_X + '</div>' +
    '</div>';

  var post = function (o) { window.__ok(o); };
  var tz = root.getElementById('tz');
  var okb = root.getElementById('okb');
  var input = root.getElementById('q');
  var hovering = false;

  // Tabs render by keyed diff: existing pills are updated in place and only
  // genuinely new pills animate in - no rebuild, no flicker, no re-animation
  // of already-open tabs.
  var tabEls = [];
  function buildTab() {
    var el = document.createElement('div');
    el.className = 'tab';
    var sp = document.createElement('span');
    sp.className = 'tt';
    el.appendChild(sp);
    var x = document.createElement('div');
    x.className = 'tx';
    x.innerHTML = I_X;
    x.addEventListener('click', function (ev) {
      ev.stopPropagation();
      post({ t: 'ui', a: 'close', i: el.__idx });
    });
    el.appendChild(x);
    el.addEventListener('click', function () {
      if (el.__idx !== S.a) post({ t: 'ui', a: 'switch', i: el.__idx });
    });
    el.addEventListener('auxclick', function (ev) {
      if (ev.button === 1) { ev.preventDefault(); post({ t: 'ui', a: 'close', i: el.__idx }); }
    });
    el.addEventListener('animationend', function () { el.classList.remove('in'); });
    el.__set = function (t) { if (sp.textContent !== t) sp.textContent = t; };
    return el;
  }
  function render() {
    var tabs = S.tabs || [];
    while (tabEls.length > tabs.length) tabEls.pop().remove();
    for (var i = 0; i < tabs.length; i++) {
      var el = tabEls[i];
      if (!el) {
        el = buildTab();
        el.classList.add('in');
        tabEls[i] = el;
        tz.appendChild(el);
      }
      el.__idx = i;
      el.classList.toggle('on', i === S.a);
      el.__set(tabs[i].t || 'New Tab');
    }
    root.getElementById('bback').toggleAttribute('disabled', !S.b);
    root.getElementById('bfwd').toggleAttribute('disabled', !S.f);
    root.getElementById('wmax').innerHTML = S.m ? I_RST : I_MAX;
  }

  // --- the address bubble (in the top bar, beside the +) --------------------
  function setOpen(v) { okb.className = v ? 'okb open' : 'okb'; }

  okb.addEventListener('mouseenter', function () { hovering = true; setOpen(true); });
  okb.addEventListener('mouseleave', function () {
    hovering = false;
    if (document.activeElement !== input) setOpen(false);
  });
  root.getElementById('lens').addEventListener('click', function () {
    setOpen(true);
    input.focus();
    input.select();
  });
  input.addEventListener('blur', function () {
    if (!hovering) setOpen(false);
  });
  input.addEventListener('focus', function () { setOpen(true); });

  function submit() {
    if (input.value.trim()) post({ t: 'go', u: input.value });
    input.blur();
    post({ t: 'ui', a: 'refocus' });
  }
  input.addEventListener('keydown', function (e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      submit();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      input.value = S.u || '';
      input.blur();
      post({ t: 'ui', a: 'refocus' });
    }
  });
  root.getElementById('go').addEventListener('click', submit);

  // --- top bar buttons ------------------------------------------------------
  root.getElementById('plus').addEventListener('click', function () { post({ t: 'ui', a: 'new' }); });
  root.getElementById('wmin').addEventListener('click', function () { post({ t: 'ui', a: 'wmin' }); });
  root.getElementById('wmax').addEventListener('click', function () { post({ t: 'ui', a: 'wmaxtoggle' }); });
  root.getElementById('wclose').addEventListener('click', function () { post({ t: 'ui', a: 'wclose' }); });

  // Empty strip area: drag to move the window, double-click to maximize.
  var drag = root.getElementById('drag');
  drag.addEventListener('mousedown', function (e) {
    if (e.button === 0) { e.preventDefault(); post({ t: 'ui', a: 'wdrag' }); }
  });
  drag.addEventListener('dblclick', function (e) {
    e.preventDefault();
    post({ t: 'ui', a: 'wmaxtoggle' });
  });

  // The very top of the window (the bar's own backdrop) is a resize grip:
  // like any native window, drag it to resize from the top edge.
  document.addEventListener('mousedown', function (e) {
    if (e.button === 0 && e.clientY < 6 && !document.fullscreenElement) {
      e.preventDefault();
      e.stopPropagation();
      post({ t: 'ui', a: 'wtopresize' });
    }
  }, true);

  // --- find in page (Ctrl+F) ------------------------------------------------
  var find = root.getElementById('find');
  var fq = root.getElementById('fq');
  var fc = root.getElementById('fc');
  var findMarks = [];
  var findPos = -1;
  var findTimer = 0;

  function findClearMarks() {
    for (var i = 0; i < findMarks.length; i++) {
      var m = findMarks[i];
      if (m && m.parentNode) {
        while (m.firstChild) m.parentNode.insertBefore(m.firstChild, m);
        m.remove();
      }
    }
    findMarks = [];
    findPos = -1;
  }
  function findCurrent() {
    for (var i = 0; i < findMarks.length; i++) {
      var m = findMarks[i];
      m.style.background = i === findPos ? '#0a84ff' : '#ffe08a';
      m.style.color = i === findPos ? '#fff' : '#111';
    }
    fc.textContent = findMarks.length ? (findPos + 1) + '/' + findMarks.length : '0/0';
  }
  function findRun(q) {
    findClearMarks();
    q = (q || '').trim();
    if (!q || !document.createTreeWalker) { findCurrent(); return; }
    var ql = q.toLowerCase();
    var walker = document.createTreeWalker(document.body || document.documentElement, 4 /* SHOW_TEXT */, {
      acceptNode: function (n) {
        var p = n.parentNode;
        if (!p) return 2;
        var tn = p.nodeName;
        if (tn === 'SCRIPT' || tn === 'STYLE' || tn === 'NOSCRIPT' || tn === 'TEXTAREA') return 2;
        return n.data.toLowerCase().indexOf(ql) === -1 ? 2 : 1;
      }
    });
    var nodes = [];
    var n;
    while ((n = walker.nextNode()) && nodes.length < 400) nodes.push(n);
    for (var i = 0; i < nodes.length && findMarks.length < 400; i++) {
      var node = nodes[i];
      while (findMarks.length < 400) {
        var lower = node.data.toLowerCase();
        var idx = lower.indexOf(ql);
        if (idx === -1) break;
        var match = node.splitText(idx);
        var rest = match.splitText(ql.length);
        var mark = document.createElement('mark');
        mark.style.cssText = 'background:#ffe08a;color:#111;border-radius:2px';
        match.parentNode.replaceChild(mark, match);
        mark.appendChild(match);
        findMarks.push(mark);
        node = rest;
      }
    }
  }
  function findShow(pos) {
    if (!findMarks.length) { findCurrent(); return; }
    findPos = ((pos % findMarks.length) + findMarks.length) % findMarks.length;
    findCurrent();
    try { findMarks[findPos].scrollIntoView({ block: 'center', inline: 'nearest' }); } catch (e) {}
  }
  function openFind() {
    find.classList.add('open');
    fq.focus();
    fq.select();
    findRun(fq.value);
    findShow(0);
  }
  function closeFind() {
    find.classList.remove('open');
    findClearMarks();
    fc.textContent = '0/0';
  }
  window.__okFind = openFind;
  window.__okFindCycle = function (dir) {
    if (!find.classList.contains('open')) { openFind(); return; }
    findShow(findPos < 0 ? 0 : findPos + (dir || 1));
  };
  fq.addEventListener('input', function () {
    if (findTimer) clearTimeout(findTimer);
    findTimer = setTimeout(function () { findTimer = 0; findRun(fq.value); findShow(0); }, 160);
  });
  fq.addEventListener('keydown', function (e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      findShow(findPos < 0 ? 0 : findPos + (e.shiftKey ? -1 : 1));
    } else if (e.key === 'Escape') {
      e.preventDefault();
      closeFind();
      post({ t: 'ui', a: 'refocus' });
    }
  });
  root.getElementById('fnext').addEventListener('click', function () { findShow(findPos + 1); });
  root.getElementById('fprev').addEventListener('click', function () { findShow(findPos - 1); });
  root.getElementById('fclose').addEventListener('click', closeFind);

  // --- state ----------------------------------------------------------------
  function sync() {
    if (document.activeElement !== input) input.value = S.u || '';
  }

  document.addEventListener('fullscreenchange', function () {
    host.style.display = document.fullscreenElement ? 'none' : '';
  });

  window.__okBar = function (s) { S = s; render(); sync(); };
  window.__okBubbleFocus = function () {
    setOpen(true);
    input.focus();
    input.select();
  };

  render();
  sync();

  // Mount the shell. This script runs at document-start, when the page may
  // not have a root element yet - so attach as soon as one exists, and
  // re-attach if a page ever removes the host node.
  function mount() {
    var target = document.body || document.documentElement;
    if (target && !host.parentNode) {
      try { target.appendChild(host); } catch (e) {}
    }
  }
  if (document.documentElement) {
    mount();
  } else {
    document.addEventListener('readystatechange', function onrs() {
      if (document.documentElement) {
        document.removeEventListener('readystatechange', onrs);
        mount();
      }
    });
  }
  document.addEventListener('DOMContentLoaded', function () { mount(); });
  var tries = 0;
  var iv = setInterval(function () {
    if (!host.parentNode) mount();
    if (++tries > 15) clearInterval(iv);
  }, 800);
})();
`

// barTab is one tab entry for the in-page shell.
type barTab struct {
	T string `json:"t"`
}

// barState is the full state pushed to the active tab's shell UI.
type barState struct {
	Tabs []barTab `json:"tabs"`
	A    int      `json:"a"`
	U    string   `json:"u"`
	B    bool     `json:"b"`
	F    bool     `json:"f"`
	M    bool     `json:"m"` // window maximized
}

// pushBarState sends tab list, address and window state to the shell of the
// active tab. Safe to call at any time; no-op when there is no tab.
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
		M:    a.maximized,
	}
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	a.execActive("window.__okBar&&window.__okBar(" + string(b) + ")")
}

// scheduleBarPush re-pushes bar state shortly after a load, covering the
// moment when the shell script has just been installed in a new document.
// It also performs a deferred address-bubble focus (e.g. after Ctrl+T).
func (a *app) scheduleBarPush(focusBubble bool) {
	a.pendingBubbleFocus = a.pendingBubbleFocus || focusBubble
	if a.hwnd != 0 {
		win.SetTimer(a.hwnd, 1, 150, 0)
	}
}

// windowAction performs a frameless-window operation requested by the shell.
func (a *app) windowAction(act string) {
	switch act {
	case "wdrag":
		// Classic trick: release the mouse, then let Windows run its own
		// caption-drag loop for the main window.
		win.ReleaseCapture()
		win.SendMessage(a.hwnd, win.WM_NCLBUTTONDOWN, win.HTCAPTION, 0)
	case "wtopresize":
		// The web content hosts the top edge, so top-edge resizing is
		// forwarded here: let Windows run its own resize loop.
		win.ReleaseCapture()
		win.SendMessage(a.hwnd, win.WM_NCLBUTTONDOWN, win.HTTOP, 0)
	case "wmaxtoggle":
		if win.IsZoomed(a.hwnd) {
			win.ShowWindow(a.hwnd, win.SW_RESTORE)
		} else {
			win.ShowWindow(a.hwnd, win.SW_MAXIMIZE)
		}
	case "wmin":
		win.ShowWindow(a.hwnd, win.SW_MINIMIZE)
	case "wclose":
		win.DestroyWindow(a.hwnd)
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
		T string  `json:"t"`
		U string  `json:"u"`
		D string  `json:"d"`
		A string  `json:"a"`
		I int     `json:"i"`
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := json.Unmarshal([]byte(msg), &m); err != nil {
		return
	}

	switch m.T {
	case "stclick": // self test only: dispatch a trusted click at x,y
		if a.inSelfTest && t != nil && t.chromium != nil {
			a.stlog("[selftest] dispatching trusted click at %.0f,%.0f", m.X, m.Y)
			t.chromium.CallDevToolsProtocol("Input.dispatchMouseEvent",
				fmt.Sprintf(`{"type":"mousePressed","x":%.1f,"y":%.1f,"button":"left","clickCount":1}`, m.X, m.Y))
			t.chromium.CallDevToolsProtocol("Input.dispatchMouseEvent",
				fmt.Sprintf(`{"type":"mouseReleased","x":%.1f,"y":%.1f,"button":"left","clickCount":1}`, m.X, m.Y))
		}

	case "open": // link explicitly asking for a new window
		if a.inSelfTest {
			a.stlog("[selftest] bridge open request: %s", m.U)
		}
		if m.U != "" && a.allowSpawn() {
			// Never create engines from inside the message callback: post
			// the work to the window-proc context instead.
			url := m.U
			a.postTask(func() { a.newTab(url, true) })
		}

	case "go": // address bubble (or start page) - parsed the same way
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
		a.scheduleBarPush(false)

	case "ui": // the glass shell
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
			a.postTask(func() {
				a.newTab("", true)
				a.scheduleBarPush(true) // focus the address bubble once ready
			})
			return
		case "close":
			i := m.I
			a.postTask(func() { a.closeTab(i) })
			return
		case "switch":
			i := m.I
			a.postTask(func() { a.switchToTab(i) })
			return
		case "wdrag", "wtopresize", "wmaxtoggle", "wmin", "wclose":
			act := m.A
			a.postTask(func() { a.windowAction(act) })
			return
		}
		a.pushBarState()
	}
}
