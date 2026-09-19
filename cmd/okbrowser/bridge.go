//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/lxn/win"
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
  if (window.top !== window) {
    document.addEventListener("mousemove", function (e) {
      if (e.clientY < 180) window.__ok({ t: "proximity" });
    }, true);
    return;
  }
  function anchor(el) { return el && el.closest ? el.closest("a") : null; }
  function reportAudio() {
    var media = document.querySelectorAll('audio,video'), playing = false;
    for (var i=0;i<media.length;i++) if (!media[i].paused && !media[i].ended) { playing=true; break; }
    window.__ok({t:'audio', a:playing?'1':'0'});
  }
  document.addEventListener('play', reportAudio, true);
  document.addEventListener('pause', reportAudio, true);
  document.addEventListener('ended', reportAudio, true);
  var formDirty=false;
  document.addEventListener('input',function(e){if(!formDirty && e.target && /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName)){formDirty=true;window.__ok({t:'form-dirty',a:'1'});}},true);
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
  // Link edge gestures. Drag any ordinary link toward an edge: left previews
  // a split view, right queues it as a background tab, top opens a tab.
  var edgeLink = "";
  function edgeSignal(phase, e) {
    document.dispatchEvent(new CustomEvent("ok-link-edge", { detail: {
      phase: phase, url: edgeLink, x: e.clientX || 0, y: e.clientY || 0
    }}));
  }
  document.addEventListener("dragstart", function (e) {
    if (window.__okSplitActive) return; // two panes is the hard maximum
    var a = anchor(e.target);
    if (!a || !a.href || a.href.indexOf("javascript:") === 0) return;
    edgeLink = a.href;
    if (e.dataTransfer) { e.dataTransfer.effectAllowed = "copy"; e.dataTransfer.setData("text/uri-list", edgeLink); }
    edgeSignal("start", e);
  }, true);
  document.addEventListener("dragover", function (e) {
    if (!edgeLink) return;
    edgeSignal("move", e);
    if (e.clientX < 92 || e.clientX > innerWidth - 92 || e.clientY < 72) e.preventDefault();
  }, true);
  document.addEventListener("drop", function (e) {
    if (!edgeLink) return;
    edgeSignal("drop", e); e.preventDefault(); edgeLink = "";
  }, true);
  document.addEventListener("dragend", function (e) {
    if (edgeLink) edgeSignal("end", e);
    edgeLink = "";
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

  var S = { tabs: [{ t: "New Tab" }], a: 0, u: "", b: false, f: false, m: false, k: false, e: "Google" };

  var SV = function (inner) {
    return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' + inner + '</svg>';
  };
  var I_BACK = SV('<path d="M15 18l-6-6 6-6"/>');
  var I_FWD  = SV('<path d="M9 18l6-6-6-6"/>');
  var I_RL   = SV('<polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>');
  var I_PLUS = SV('<path d="M12 5v14M5 12h14"/>');
  var I_X    = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"><path d="M18 6L6 18M6 6l12 12"/></svg>';
  var I_LENS = SV('<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.35-4.35"/>');
  var I_LOCK = SV('<rect x="5" y="10" width="14" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>');
  var I_GO   = SV('<path d="M5 12h13"/><path d="M13 6l6 6-6 6"/>');
  var I_MIN  = SV('<path d="M5 12h14"/>');
  var I_MAX  = SV('<rect x="5.5" y="5.5" width="13" height="13" rx="2"/>');
  var I_RST  = SV('<rect x="8.5" y="5.5" width="10" height="10" rx="2"/><path d="M5.5 15.5a3.5 3.5 0 0 0 3.5 3.5h7"/>');
  var I_UP   = SV('<path d="M6 15l6-6 6 6"/>');
  var I_DOWN = SV('<path d="M6 9l6 6 6-6"/>');
  var I_STAR = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"><path d="M12 3.2l2.7 5.5 6.1.9-4.4 4.3 1 6.1-5.4-2.9-5.4 2.9 1-6.1L3.2 9.6l6.1-.9z"/></svg>';
  var I_STARF = '<svg viewBox="0 0 24 24" fill="currentColor" stroke="none"><path d="M12 3.2l2.7 5.5 6.1.9-4.4 4.3 1 6.1-5.4-2.9-5.4 2.9 1-6.1L3.2 9.6l6.1-.9z"/></svg>';
  var I_MENU = '<svg viewBox="0 0 24 24" fill="currentColor" stroke="none"><circle cx="12" cy="5" r="1.7"/><circle cx="12" cy="12" r="1.7"/><circle cx="12" cy="19" r="1.7"/></svg>';
  var I_DL = SV('<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>');
  var I_INC = SV('<path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/><path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/><path d="M14.12 14.12a3 3 0 1 1-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/>');
  var I_CLK  = SV('<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 3"/>');
  var I_SET  = SV('<line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/>');

  var host = document.createElement('div');
  var root = host.attachShadow({ mode: 'closed' });

  var css = [
    ":host{all:initial}",
    "*{-webkit-user-select:none}",
    ".edge{position:fixed;top:0;left:0;right:0;height:4px;z-index:2147483645;",
    "pointer-events:auto;cursor:default;",
    "background:linear-gradient(90deg,transparent,rgba(120,120,128,.28),transparent)}",
    ".strip{position:fixed;top:0;left:0;right:0;height:36px;z-index:2147483646;",
    "display:flex;align-items:center;gap:5px;padding:0 152px 0 8px;pointer-events:none;",
    "font-family:-apple-system,'Segoe UI Variable Text','Segoe UI',system-ui,sans-serif;",
    "background:rgba(250,250,252,.52);",
    "backdrop-filter:blur(26px) saturate(1.7);-webkit-backdrop-filter:blur(26px) saturate(1.7);",
    "box-shadow:0 1px 12px rgba(0,0,0,.08),inset 0 -.5px 0 rgba(0,0,0,.07);",
    "transform:translateY(calc(-100% + (100% * var(--ok-proximity,0))));",
    "opacity:calc(.18 + (.82 * var(--ok-proximity,0)));",
    "transition:transform .10s ease-out,opacity .10s ease-out}",
    ".strip.open{transform:translateY(0);opacity:1}",
    ".strip.paneactive{box-shadow:inset 0 -2px 0 #0a84ff,0 1px 12px rgba(0,0,0,.12)}",
    "@media (prefers-color-scheme:dark){.strip{background:rgba(24,24,28,.55);",
    "box-shadow:0 1px 12px rgba(0,0,0,.32),inset 0 -.5px 0 rgba(255,255,255,.06)}}",
    ".tz{display:flex;gap:4px;align-items:center;min-width:0;height:100%;",
    "flex:0 1 auto;overflow:hidden;pointer-events:auto}",
    ".tab{display:flex;align-items:center;gap:2px;height:23px;min-width:0;flex:0 1 118px;",
    "padding:0 5px 0 9px;border-radius:12px;cursor:default;",
    "background:rgba(250,250,252,.30);",
    "backdrop-filter:blur(20px) saturate(1.6);-webkit-backdrop-filter:blur(20px) saturate(1.6);",
    "box-shadow:0 1px 6px rgba(0,0,0,.08),inset 0 1px 0 rgba(255,255,255,.42),",
    "inset 0 0 0 .5px rgba(255,255,255,.20);",
    "transition:background .16s ease,transform .16s ease,flex-basis .22s ease,width .22s ease}",
    ".tab.in{animation:okin .24s cubic-bezier(.2,.8,.3,1)}",
    ".tab.sleep{opacity:.62}.tab.sleep .ic{filter:saturate(.35)}",
    ".ic{flex:0 0 auto;width:16px;height:16px;border-radius:5px;display:grid;place-items:center;",
    "font-size:10px;font-weight:700;color:#5f6368;overflow:hidden}",
    "@media (prefers-color-scheme:dark){.ic{color:#9aa0a6}}",
    ".fi{width:15px;height:15px;border-radius:4px;object-fit:contain}",
    ".tz.mini .tab,.tab.pin{flex:0 0 auto;width:23px;padding:0 3px;justify-content:center;gap:0}",
    ".tz.mini .tt,.tab.pin .tt{display:none}",
    ".tz.mini .tx,.tab.pin .tx{display:none !important}",
    ".prog{position:fixed;top:0;left:0;height:2.5px;width:0;z-index:2147483644;pointer-events:none;",
    "background:linear-gradient(90deg,#0a84ff,#5ac8fa);border-radius:0 2px 2px 0;opacity:0;",
    "transition:opacity .25s}",
    ".prog.on{opacity:1;animation:okload 5s ease-out forwards}",
    ".prog.done{width:100% !important;opacity:0;transition:width .2s,opacity .35s}",
    "@keyframes okload{0%{width:8%}25%{width:38%}55%{width:62%}85%{width:78%}100%{width:86%}}",
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
    ".wcap{position:fixed;top:5px;right:6px;height:26px;display:flex;align-items:center;",
    "padding:0 3px;border-radius:14px;z-index:2147483647;pointer-events:auto;",
    "transform:translateY(0);transition:transform .10s ease-out,opacity .10s ease-out}",
    "background:rgba(250,250,252,.5);",
    "backdrop-filter:blur(24px) saturate(1.7);-webkit-backdrop-filter:blur(24px) saturate(1.7);",
    "box-shadow:0 2px 12px rgba(0,0,0,.14),inset 0 1px 0 rgba(255,255,255,.5),",
    "inset 0 0 0 .5px rgba(255,255,255,.28)}",
    "@media (prefers-color-scheme:dark){.wcap{background:rgba(28,28,32,.55);",
    "box-shadow:0 2px 12px rgba(0,0,0,.4),inset 0 1px 0 rgba(255,255,255,.09),",
    "inset 0 0 0 .5px rgba(255,255,255,.08)}}",
    ".wcap.hid{transform:translateY(calc(-160% + (160% * var(--ok-proximity,0))));",
    "opacity:var(--ok-proximity,0);pointer-events:none}",
    ".wcap.hid.near{pointer-events:auto}",
    ".wbtn{width:40px;height:22px;border-radius:11px;display:grid;place-items:center;color:#3c4043;",
    "cursor:default;transition:background .12s,opacity .12s}",
    ".sitepanel{position:fixed;top:40px;left:8px;width:290px;padding:12px;border-radius:18px;z-index:2147483647;",
    "display:none;pointer-events:auto;font:12px -apple-system,'Segoe UI',sans-serif;color:#202124;",
    "background:rgba(250,250,252,.88);backdrop-filter:blur(32px) saturate(1.8);box-shadow:0 16px 50px rgba(0,0,0,.28)}",
    ".sitepanel.open{display:block;animation:okin .16s ease}.sitehead{font-size:14px;font-weight:700;margin:2px 4px 3px}",
    ".siteorigin{font-size:11px;opacity:.58;margin:0 4px 10px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}",
    ".permrow{display:flex;align-items:center;justify-content:space-between;padding:7px 4px;border-top:1px solid rgba(120,128,138,.14)}",
    ".permrow select{border:0;border-radius:9px;padding:4px 6px;background:rgba(120,128,138,.13);color:inherit}",
    "@media(prefers-color-scheme:dark){.sitepanel{background:rgba(28,28,32,.9);color:#f2f2f7}}",
    ".menu,.ctx{position:fixed;top:34px;right:6px;width:224px;padding:6px;border-radius:16px;",
    "z-index:2147483647;pointer-events:auto;display:none;",
    "font-family:-apple-system,'Segoe UI Variable Text','Segoe UI',system-ui,sans-serif;",
    "background:rgba(250,250,252,.72);",
    "backdrop-filter:blur(30px) saturate(1.8);-webkit-backdrop-filter:blur(30px) saturate(1.8);",
    "box-shadow:0 14px 44px rgba(0,0,0,.22),inset 0 1px 0 rgba(255,255,255,.6),",
    "inset 0 0 0 .5px rgba(255,255,255,.35)}",
    ".menu.open,.ctx.open{display:block;animation:okin .16s ease}",
    ".ctx{right:auto}",
    "@media (prefers-color-scheme:dark){.menu,.ctx{background:rgba(30,30,34,.76);",
    "box-shadow:0 14px 44px rgba(0,0,0,.5),inset 0 1px 0 rgba(255,255,255,.09),",
    "inset 0 0 0 .5px rgba(255,255,255,.07)}}",
    ".mrow{display:flex;align-items:center;gap:10px;padding:9px 12px;border-radius:11px;",
    "font-size:13px;font-weight:500;color:#202124;cursor:default;transition:background .13s}",
    ".mrow:hover{background:rgba(120,128,138,.16)}",
    ".mrow svg{width:14px;height:14px;color:#5f6368}",
    "@media (prefers-color-scheme:dark){.mrow{color:#e8eaed}.mrow svg{color:#9aa0a6}}",
    ".mfoot{padding:8px 12px 5px;font-size:11px;color:#80868b}",
    ".sug{position:fixed;top:42px;width:340px;max-width:calc(100vw - 20px);padding:6px;",
    "border-radius:16px;z-index:2147483647;pointer-events:auto;display:none;",
    "font-family:-apple-system,'Segoe UI Variable Text','Segoe UI',system-ui,sans-serif;",
    "background:rgba(250,250,252,.78);",
    "backdrop-filter:blur(30px) saturate(1.8);-webkit-backdrop-filter:blur(30px) saturate(1.8);",
    "box-shadow:0 14px 44px rgba(0,0,0,.22),inset 0 1px 0 rgba(255,255,255,.6),",
    "inset 0 0 0 .5px rgba(255,255,255,.35)}",
    ".sug.open{display:block;animation:okin .14s ease}",
    "@media (prefers-color-scheme:dark){.sug{background:rgba(30,30,34,.8);",
    "box-shadow:0 14px 44px rgba(0,0,0,.5),inset 0 1px 0 rgba(255,255,255,.09),",
    "inset 0 0 0 .5px rgba(255,255,255,.07)}}",
    ".srow{display:flex;align-items:center;gap:10px;padding:8px 10px;border-radius:11px;",
    "cursor:default;transition:background .1s}",
    ".srow:hover,.srow.son{background:rgba(10,132,255,.14)}",
    ".sic{flex:0 0 auto;width:26px;height:26px;border-radius:50%;display:grid;place-items:center;",
    "color:#5f6368;background:rgba(120,128,138,.12)}",
    ".sic svg{width:12px;height:12px}",
    "@media (prefers-color-scheme:dark){.sic{color:#9aa0a6;background:rgba(200,205,214,.12)}}",
    ".smeta{flex:1 1 auto;min-width:0}",
    ".stt{font-size:12.5px;font-weight:550;color:#202124;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}",
    "@media (prefers-color-scheme:dark){.stt{color:#e8eaed}}",
    ".suu{font-size:11px;color:#80868b;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}",
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
    ".edgeact{position:fixed;z-index:2147483643;pointer-events:none;opacity:0;display:flex;",
    "align-items:center;justify-content:center;font:600 13px -apple-system,'Segoe UI',sans-serif;",
    "color:#fff;background:rgba(18,20,26,.42);backdrop-filter:blur(34px) saturate(1.8);",
    "-webkit-backdrop-filter:blur(34px) saturate(1.8);border:1px solid rgba(255,255,255,.2);",
    "box-shadow:0 24px 70px rgba(0,0,0,.28),inset 0 1px rgba(255,255,255,.22);",
    "transition:opacity .22s ease,transform .48s cubic-bezier(.16,1,.3,1),background .2s}",
    ".edgeact.show{opacity:1}.edgeact.hot{background:rgba(10,132,255,.68)}",
    ".edge-left{left:12px;top:12%;bottom:12%;width:min(42vw,520px);border-radius:28px;transform:translateX(-105%) scale(.94)}",
    ".edge-left.show{transform:translateX(-84%) scale(.97)}.edge-left.hot{transform:translateX(0) scale(1)}",
    ".edge-right{right:12px;top:26%;width:180px;height:48%;border-radius:26px;transform:translateX(115%) scale(.9)}",
    ".edge-right.show{transform:translateX(74%) scale(.96)}.edge-right.hot{transform:translateX(0) scale(1)}",
    ".edge-top{left:32%;right:32%;top:8px;height:64px;border-radius:32px;transform:translateY(-125%) scale(.9)}",
    ".edge-top.show{transform:translateY(-72%) scale(.96)}.edge-top.hot{transform:translateY(0) scale(1)}",
    ".edgeact span{padding:10px 16px;border-radius:20px;background:rgba(0,0,0,.2);text-align:center}",
    ".splitclose{position:fixed;top:42px;left:50%;transform:translateX(-50%);z-index:2147483647;",
    "display:none;gap:2px;padding:4px;border-radius:18px;pointer-events:auto;cursor:default;",
    "font:600 11px -apple-system,'Segoe UI',sans-serif;color:#fff;background:rgba(35,35,40,.68);",
    "backdrop-filter:blur(24px);box-shadow:0 8px 30px rgba(0,0,0,.25)}",
    ".splitclose.show{display:flex}.splitclose .splitact{padding:6px 9px;border-radius:13px;font-weight:600}",
    ".splitclose .splitact:hover{background:rgba(255,255,255,.16)}.splitclose .danger:hover{background:#e81123}",
    ".splitdivider{position:fixed;right:0;top:80px;bottom:0;width:8px;z-index:2147483647;",
    "display:none;cursor:col-resize;pointer-events:auto}.splitdivider.show{display:block}",
    ".splitdivider:after{content:'';position:absolute;left:3px;top:35%;width:2px;height:30%;",
    "border-radius:2px;background:rgba(120,128,138,.45)}",
    "*:focus-visible{outline:2px solid #0a84ff !important;outline-offset:2px}",
    ".sr{position:fixed;width:1px;height:1px;overflow:hidden;clip-path:inset(50%)}",
    ":host(.large) .strip{height:46px}:host(.large) .tab{height:32px;border-radius:16px}",
    ":host(.large) .wcap{height:34px}:host(.large) .wbtn{height:30px;width:46px}",
    ":host(.large) .okb{height:34px;border-radius:18px}",
    "@media (prefers-reduced-motion:reduce){*{animation:none !important;transition-duration:.01ms !important}}",
    "@media (forced-colors:active){.strip,.wcap,.okb,.menu,.ctx,.sug,.find{background:Canvas;border:1px solid CanvasText;backdrop-filter:none}.tab.on{outline:2px solid Highlight}}",
    "@media print{.strip,.wcap,.edge,.find,.edgeact{display:none !important}}"
  ].join("");

  var sheet = new CSSStyleSheet();
  sheet.replaceSync(css);
  root.adoptedStyleSheets = [sheet];

  root.innerHTML =
    '<div class="sr" id="live" role="status" aria-live="polite"></div>' +
    '<div class="prog" id="prog"></div>' +
    '<div class="edge" id="edge"></div>' +
    '<div class="strip" id="strip">' +
      '<div class="tz" id="tz"></div>' +
      '<div class="plus" id="plus">' + I_PLUS + '</div>' +
      '<div class="okb" id="okb">' +
        '<div class="lens" id="lens">' + I_LENS + '</div>' +
        '<div class="inner">' +
          '<div class="bb" id="bsite" title="Site information">' + I_LOCK + '</div>' +
          '<div class="bb" id="bback" title="Back">' + I_BACK + '</div>' +
          '<div class="bb" id="bfwd" title="Forward">' + I_FWD + '</div>' +
          '<div class="bb" id="brl" title="Reload">' + I_RL + '</div>' +
          '<input id="q" placeholder="Search or enter address" spellcheck="false" autocomplete="off" autocapitalize="off">' +
          '<div class="bb" id="bstar" title="Bookmark this page (Ctrl+D)">' + I_STAR + '</div>' +
          '<div class="go" id="go" title="Go">' + I_GO + '</div>' +
        '</div>' +
      '</div>' +
      '<div class="drag" id="drag"></div>' +
    '</div>' +
    '<div class="wcap" id="wcap">' +
      '<div class="wbtn" id="wmenu" title="Menu">' + I_MENU + '</div>' +
      '<div class="wbtn" id="wmin" title="Minimize">' + I_MIN + '</div>' +
      '<div class="wbtn" id="wmax" title="Maximize">' + I_MAX + '</div>' +
      '<div class="wbtn close" id="wclose" title="Close">' + I_X + '</div>' +
    '</div>' +
    '<div class="sitepanel" id="sitepanel"><div class="sitehead" id="sitehead">Site information</div><div class="siteorigin" id="siteorigin"></div>' +
      '<div class="permrow">Camera<select data-perm="camera"><option value="default">Ask</option><option value="allow">Allow</option><option value="deny">Block</option></select></div>' +
      '<div class="permrow">Microphone<select data-perm="microphone"><option value="default">Ask</option><option value="allow">Allow</option><option value="deny">Block</option></select></div>' +
      '<div class="permrow">Location<select data-perm="location"><option value="default">Ask</option><option value="allow">Allow</option><option value="deny">Block</option></select></div>' +
      '<div class="permrow">Notifications<select data-perm="notifications"><option value="default">Ask</option><option value="allow">Allow</option><option value="deny">Block</option></select></div>' +
      '<div class="permrow">Clipboard<select data-perm="clipboard"><option value="default">Ask</option><option value="allow">Allow</option><option value="deny">Block</option></select></div>' +
      '<div class="permrow">Sensors<select data-perm="sensors"><option value="default">Ask</option><option value="allow">Allow</option><option value="deny">Block</option></select></div>' +
      '<div class="permrow"><div class="mrow" id="clear-perms">Reset permissions</div><div class="mrow" id="clear-site">Clear site data</div></div></div>' +
    '<div class="menu" id="menu">' +
      '<div class="mrow" id="m-newtab" data-m="newtab">' + I_PLUS + 'New tab</div>' +
      '<div class="mrow" id="m-incognito" data-m="incognito">' + I_INC + 'New incognito window</div>' +
      '<div class="mrow" id="m-bookmarks" data-m="bookmarks">' + I_STARF + 'Bookmarks</div>' +
      '<div class="mrow" id="m-history" data-m="history">' + I_CLK + 'History</div>' +
      '<div class="mrow" id="m-downloads" data-m="downloads">' + I_DL + 'Downloads</div>' +
      '<div class="mrow" id="m-settings" data-m="settings">' + I_SET + 'Settings</div>' +
      '<div class="mfoot" id="m-foot">OK Browser</div>' +
    '</div>' +
    '<div class="ctx" id="ctx">' +
      '<div class="mrow" id="c-newtab">New tab</div>' +
      '<div class="mrow" id="c-dup">Duplicate</div>' +
      '<div class="mrow" id="c-pin">Pin tab</div>' +
      '<div class="mrow" id="c-split">Open in Split View</div>' +
      '<div class="mrow" id="c-sleep">Sleep tab</div>' +
      '<div class="mrow" id="c-never">Never sleep this site</div>' +
      '<div class="mrow" id="c-close">Close tab</div>' +
      '<div class="mrow" id="c-others">Close other tabs</div>' +
    '</div>' +
    '<div class="sug" id="sug"></div>' +
    '<div class="edgeact edge-left" id="edge-left"><span>Drop to Queue for Later</span></div>' +
    '<div class="edgeact edge-right" id="edge-right"><span>Drop for Peek & Split</span></div>' +
    '<div class="edgeact edge-top" id="edge-top"><span></span></div>' +
    '<div class="splitclose" id="splitclose"><div class="splitact" id="split-swap">⇄ Swap</div><div class="splitact" id="split-tab">↗ Tabs</div><div class="splitact danger" id="split-close">× Close</div></div>' +
    '<div class="splitdivider" id="splitdivider"></div>' +
    '<div class="find" id="find">' +
      '<input id="fq" placeholder="Find in page" spellcheck="false">' +
      '<div class="fc" id="fc">0/0</div>' +
      '<div class="fb" id="fprev" title="Previous (Shift+Enter)">' + I_UP + '</div>' +
      '<div class="fb" id="fnext" title="Next (Enter)">' + I_DOWN + '</div>' +
      '<div class="fb" id="fclose" title="Close (Esc)">' + I_X + '</div>' +
    '</div>';

  var live = root.getElementById('live');
  function announce(text) { live.textContent = ''; setTimeout(function(){ live.textContent = text; }, 20); }
  var buttonIDs = ['plus','lens','bsite','bback','bfwd','brl','bstar','go','wmenu','wmin','wmax','wclose','fprev','fnext','fclose','split-swap','split-tab','split-close'];
  for (var ai=0;ai<buttonIDs.length;ai++) {
    var control=root.getElementById(buttonIDs[ai]); if(!control)continue;
    control.setAttribute('role','button'); control.setAttribute('tabindex','0');
    if(control.title) control.setAttribute('aria-label',control.title);
    control.addEventListener('keydown',function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();this.click();}});
  }
  input = root.getElementById('q'); input.setAttribute('aria-label','Address and search');
  root.getElementById('tz').setAttribute('role','tablist');
  root.getElementById('menu').setAttribute('role','menu');
  root.getElementById('sitepanel').setAttribute('role','dialog');
  root.getElementById('sitepanel').setAttribute('aria-label','Site information and permissions');

  var post = function (o) { window.__ok(o); };
  document.addEventListener('pointerdown', function () { post({t:'pane-focus'}); }, true);
  var tz = root.getElementById('tz');
  var okb = root.getElementById('okb');
  var input = root.getElementById('q');
  var hovering = false;

  // --- immersive auto-hide --------------------------------------------------
  // The page fills the whole window. The glass bar hides away and glides
  // back when the mouse touches the top edge (or on Ctrl+T / Ctrl+L /
  // tab switches); the window buttons stay visible as a small floating
  // capsule at the top right.
  var strip = root.getElementById('strip');
  var barPinned = false; // the mouse is over the bar area
  var hideTimer = 0;
  var briefTimer = 0;

  function hideBar() {
    strip.classList.remove('open');
    var wc = root.getElementById('wcap');
    if (wc) wc.classList.add('hid'); // the capsule hides with the bar
    closeMenu();
    closeCtx();
    strip.style['--ok-proximity'] = '0';
    var cap = root.getElementById('wcap');
    if (cap) { cap.style['--ok-proximity'] = '0'; cap.classList.remove('near'); }
  }
  function uiOpen(id) {
    var el = root.getElementById(id);
    return !!el && el.classList.contains('open');
  }
  function hideIfIdle() {
    // New Tab is a persistent command surface: its tabs and URL field never
    // retreat, even when the pointer leaves the top of the window.
    if (!S.u || barPinned || document.activeElement === input) return;
    // Popovers anchored to the bar (menu, tab menu, suggestions, find)
    // are part of it: the bar must never retire while one is open.
    if (uiOpen('menu') || uiOpen('ctx') || uiOpen('sug') || uiOpen('find') || uiOpen('sitepanel')) return;
    hideBar();
  }
  function revealBar(brief) {
    strip.classList.add('open');
    var wc = root.getElementById('wcap');
    if (wc) wc.classList.remove('hid');
    if (hideTimer) { clearTimeout(hideTimer); hideTimer = 0; }
    if (briefTimer) { clearTimeout(briefTimer); briefTimer = 0; }
    if (brief && !barPinned) {
      briefTimer = setTimeout(function () { briefTimer = 0; hideIfIdle(); }, 2000);
    }
  }
  // Proximity reveal: the chrome begins following the pointer before it
  // reaches the edge, then becomes fully interactive near the top.  A pointer
  // moving upward quickly gets a wider magnetic range so the controls meet it.
  var lastPointerY = window.innerHeight || 10000;
  var clockNow = function () { return window.performance ? window.performance.now() : Date.now(); };
  var nextFrame = window.requestAnimationFrame || function (fn) { fn(); return 0; };
  var dropFrame = window.cancelAnimationFrame || function () {};
  var lastPointerAt = clockNow();
  var proximityFrame = 0;
  function paintProximity(y, upwardSpeed) {
    if (barPinned || strip.classList.contains('open')) return;
    var range = upwardSpeed > 0.65 ? 210 : 160;
    var fullAt = 32;
    var amount = Math.max(0, Math.min(1, (range - y) / (range - fullAt)));
    // Ease the first hint in, while retaining a direct, cursor-linked finish.
    amount = amount * amount * (3 - 2 * amount);
    strip.style['--ok-proximity'] = amount.toFixed(3);
    wcap.style['--ok-proximity'] = amount.toFixed(3);
    wcap.classList.toggle('near', amount > 0.82);
    if (y <= fullAt) revealBar(false);
  }
  root.getElementById('edge').addEventListener('mouseenter', function () { revealBar(false); });
  document.addEventListener('mousemove', function (e) {
    var now = clockNow();
    var dt = Math.max(1, now - lastPointerAt);
    var upwardSpeed = Math.max(0, (lastPointerY - e.clientY) / dt);
    lastPointerY = e.clientY;
    lastPointerAt = now;
    if (proximityFrame) dropFrame(proximityFrame);
    proximityFrame = nextFrame(function () {
      proximityFrame = 0;
      paintProximity(e.clientY, upwardSpeed);
    });
  }, true);
  strip.addEventListener('mouseenter', function () { barPinned = true; });
  strip.addEventListener('mouseleave', function () {
    barPinned = false;
    if (hideTimer) clearTimeout(hideTimer);
    hideTimer = setTimeout(hideIfIdle, 350);
  });
  var wcap = root.getElementById('wcap');
  wcap.addEventListener('mouseenter', function () {
    barPinned = true;
    if (hideTimer) { clearTimeout(hideTimer); hideTimer = 0; }
    revealBar(false);
  });
  wcap.addEventListener('mouseleave', function () {
    barPinned = false;
    if (hideTimer) clearTimeout(hideTimer);
    hideTimer = setTimeout(hideIfIdle, 350);
  });

  // Link-edge gesture surface. The large left preview follows the same soft
  // spring curve as iOS sheets; committing it asks the native host for a real
  // second WebView, so sites that block iframes still work in Split View.
  var edgeLeft = root.getElementById('edge-left');
  var edgeRight = root.getElementById('edge-right');
  var edgeTop = root.getElementById('edge-top');
  var edgeZone = '';
  function clearEdgeGesture() {
    [edgeLeft, edgeRight, edgeTop].forEach(function (el) { el.classList.remove('show', 'hot'); });
    edgeZone = '';
  }
  document.addEventListener('ok-link-edge', function (e) {
    var d = e.detail || {};
    if (S.v) { clearEdgeGesture(); return; }
    if (d.phase === 'end') { clearEdgeGesture(); return; }
    var zone = d.x < 92 ? 'later' : (d.x > innerWidth - 92 ? 'split' : '');
    [edgeLeft, edgeRight, edgeTop].forEach(function (el) { el.classList.toggle('show', d.phase !== 'drop'); });
    edgeLeft.classList.toggle('hot', zone === 'later');
    edgeRight.classList.toggle('hot', zone === 'split');
    edgeTop.classList.remove('show', 'hot');
    edgeZone = zone;
    if (d.url) {
      try { edgeRight.firstElementChild.textContent = 'Peek & Split  ·  ' + new URL(d.url).hostname; } catch (_) {}
    }
    if (d.phase === 'drop') {
      if (zone && d.url) post({ t: 'edge-link', a: zone, u: d.url });
      clearEdgeGesture();
    }
  }, true);

  // Existing tabs can be dragged to either window edge to form Split View.
  // The side of the drop determines which pane receives the dragged tab.
  var tabEdgeZone = '';
  document.addEventListener('dragover', function(e) {
    if (dragFrom < 0) return;
    if (S.v) tabEdgeZone = e.clientY < 110 ? 'merge' : '';
    else tabEdgeZone = e.clientX < 92 ? 'left' : (e.clientX > innerWidth - 92 ? 'right' : '');
    edgeLeft.classList.toggle('show', !!tabEdgeZone);
    edgeRight.classList.toggle('show', !!tabEdgeZone);
    edgeLeft.classList.toggle('hot', tabEdgeZone === 'left');
    edgeRight.classList.toggle('hot', tabEdgeZone === 'right');
    edgeTop.classList.toggle('show', tabEdgeZone === 'merge'); edgeTop.classList.toggle('hot', tabEdgeZone === 'merge');
    edgeLeft.firstElementChild.textContent = 'Drop tab on Left';
    edgeRight.firstElementChild.textContent = 'Drop tab on Right';
    edgeTop.firstElementChild.textContent = 'Drop to Return to Tabs';
    if (tabEdgeZone) e.preventDefault();
  }, true);
  document.addEventListener('drop', function(e) {
    if (dragFrom < 0 || !tabEdgeZone) return;
    e.preventDefault(); e.stopPropagation();
    if (tabEdgeZone === 'merge') post({t:'ui', a:'promote-split'});
    else post({t:'ui', a:'tab-split-' + tabEdgeZone, i:dragFrom});
    dragFrom=-1; tabEdgeZone=''; clearEdgeGesture();
  }, true);

  // Tabs render by keyed diff: existing pills are updated in place and only
  // genuinely new pills animate in - no rebuild, no flicker, no re-animation
  // of already-open tabs.
  var tabEls = [];
  var prevA = 0;
  var stateLive = false; // true once the first real state push rendered
  var dragFrom = -1;

  // letterOf picks the avatar letter for a URL (first letter of the host).
  function letterOf(u) {
    try {
      var h = String(u || '');
      var i = h.indexOf('://');
      if (i >= 0) h = h.slice(i + 3);
      var j = h.search(/[\/?#]/);
      if (j >= 0) h = h.slice(0, j);
      if (!h) return '\u2022';
      return h.charAt(0).toUpperCase();
    } catch (e) { return '\u2022'; }
  }

  function buildTab() {
    var el = document.createElement('div');
    el.className = 'tab';
    el.draggable = true;
    el.setAttribute('role','tab'); el.setAttribute('tabindex','0');
    var ic = document.createElement('div');
    ic.className = 'ic';
    el.appendChild(ic);
    var sp = document.createElement('span');
    sp.className = 'tt';
    el.appendChild(sp);
    var x = document.createElement('div');
    x.className = 'tx';
    x.innerHTML = I_X; x.setAttribute('role','button'); x.setAttribute('aria-label','Close tab'); x.setAttribute('tabindex','-1');
    x.addEventListener('click', function (ev) {
      ev.stopPropagation();
      post({ t: 'ui', a: 'close', i: el.__idx });
    });
    el.appendChild(x);
    el.addEventListener('click', function () {
      if (el.__idx !== S.a) post({ t: 'ui', a: 'switch', i: el.__idx });
    });
    el.addEventListener('keydown', function(ev) {
      if(ev.key==='Enter'||ev.key===' '){ev.preventDefault();post({t:'ui',a:'switch',i:el.__idx});}
      else if(ev.key==='Delete'){ev.preventDefault();post({t:'ui',a:'close',i:el.__idx});}
      else if(ev.key==='ArrowRight'||ev.key==='ArrowLeft'){ev.preventDefault();var n=(el.__idx+(ev.key==='ArrowRight'?1:-1)+tabEls.length)%tabEls.length;if(tabEls[n])tabEls[n].focus();}
    });
    el.addEventListener('auxclick', function (ev) {
      if (ev.button === 1) { ev.preventDefault(); post({ t: 'ui', a: 'close', i: el.__idx }); }
    });
    el.addEventListener('animationend', function () { el.classList.remove('in'); });
    // Drag & drop reordering.
    el.addEventListener('dragstart', function (e) {
      dragFrom = el.__idx;
      try { e.dataTransfer.setData('text/plain', 'ok'); } catch (err) {}
    });
    el.addEventListener('dragend', function () { dragFrom=-1; tabEdgeZone=''; clearEdgeGesture(); });
    el.addEventListener('dragover', function (e) { e.preventDefault(); });
    el.addEventListener('drop', function (e) {
      e.preventDefault();
      if (dragFrom >= 0 && dragFrom !== el.__idx) {
        post({ t: 'ui', a: 'reorder', i: dragFrom, to: el.__idx });
      }
      dragFrom = -1;
    });
    // Right-click: the tab context menu.
    el.addEventListener('contextmenu', function (e) {
      e.preventDefault();
      openCtx(el.__idx, e.clientX || 40, e.clientY || 40);
    });
    el.__fav = null;
    el.__set = function (t, u, f) {
      if (sp.textContent !== t) sp.textContent = t;
      el.title = t;
      if (f !== el.__fav) {
        el.__fav = f;
        ic.textContent = '';
        if (f) {
          var img = document.createElement('img');
          img.className = 'fi';
          img.src = f;
          img.alt = '';
          img.addEventListener('error', function () { if (el.__fav === f) el.__set(t, u, ''); });
          ic.appendChild(img);
        } else {
          ic.textContent = letterOf(u || t);
        }
      }
    };
    return el;
  }

  // --- tab context menu ------------------------------------------------------
  var ctx = root.getElementById('ctx');
  var ctxIdx = -1;
  function closeCtx() { ctx.classList.remove('open'); ctxIdx = -1; }
  function openCtx(idx, x, y) {
    ctxIdx = idx;
    var pinRow = root.getElementById('c-pin');
    var tab = (S.tabs || [])[idx];
    if (pinRow) pinRow.textContent = (tab && tab.p) ? 'Unpin tab' : 'Pin tab';
    var sleepRow=root.getElementById('c-sleep');if(sleepRow)sleepRow.textContent=(tab&&tab.s)?'Wake tab':'Sleep tab';
    var neverRow=root.getElementById('c-never');if(neverRow)neverRow.textContent=(tab&&tab.n)?'Allow this site to sleep':'Never sleep this site';
    ctx.classList.add('open');
    try {
      var iw = window.innerWidth || 900, ih = window.innerHeight || 700;
      ctx.style.left = Math.max(6, Math.min(x, iw - 240)) + 'px';
      ctx.style.top = Math.max(6, Math.min(y, ih - 310)) + 'px';
    } catch (e) {}
  }
  function ctxAction(id, act) {
    var row = root.getElementById(id);
    if (!row) return;
    row.addEventListener('click', function () {
      if (ctxIdx >= 0) post({ t: 'ui', a: act, i: ctxIdx });
      closeCtx();
    });
  }
  root.getElementById('c-newtab').addEventListener('click', function () {
    post({ t: 'ui', a: 'new' });
    closeCtx();
  });
  ctxAction('c-dup', 'dup');
  ctxAction('c-pin', 'pin');
  ctxAction('c-split', 'tab-split');
  ctxAction('c-sleep', 'sleep');
  ctxAction('c-never', 'never-sleep');
  ctxAction('c-close', 'close');
  ctxAction('c-others', 'close-others');
  document.addEventListener('mousedown', function (e) {
    if (!ctx.classList.contains('open')) return;
    if (inRect(ctx, e.clientX, e.clientY)) return;
    closeCtx();
  }, true);
  function render() {
    var tabs = S.tabs || [];
    // Auto-collapse: pills shrink to favicon-only when the tab strip gets
    // crowded, and grow back when there is room again. Pinned pills are
    // always favicon-only (hover shows the full title as a tooltip).
    var mini = false;
    try {
      mini = tabs.length > 1 && (tabs.length * 104) > ((window.innerWidth || 1200) - 250);
    } catch (e) {}
    tz.classList.toggle('mini', mini);
    while (tabEls.length > tabs.length) tabEls.pop().remove();
    for (var i = 0; i < tabs.length; i++) {
      var el = tabEls[i];
      if (!el) {
        el = buildTab();
        el.classList.add('in');
        tabEls[i] = el;
        tz.appendChild(el);
        if (stateLive) revealBar(true); // show the new tab appearing
      }
      el.__idx = i;
      el.classList.toggle('on', i === S.a);
      el.setAttribute('aria-selected', i === S.a ? 'true' : 'false');
      el.classList.toggle('pin', !!(tabs[i] && tabs[i].p));
      el.classList.toggle('sleep', !!(tabs[i] && tabs[i].s));
      if (tabs[i] && tabs[i].s) el.title = (tabs[i].t || 'Tab') + ' — sleeping';
      if (tabs[i] && tabs[i].a) el.title = (tabs[i].t || 'Tab') + ' — playing audio';
      el.__set(tabs[i].t || 'New Tab', (tabs[i] && tabs[i].u) || '', (tabs[i] && tabs[i].f) || '');
    }
    root.getElementById('bback').toggleAttribute('disabled', !S.b);
    root.getElementById('bfwd').toggleAttribute('disabled', !S.f);
    root.getElementById('wmax').innerHTML = S.m ? I_RST : I_MAX;
    root.getElementById('bstar').innerHTML = S.k ? I_STARF : I_STAR;
    if (stateLive && prevA !== S.a) revealBar(true); // tab switched
    prevA = S.a;
  }

  // --- site identity and per-site permissions -------------------------------
  var sitepanel = root.getElementById('sitepanel');
  root.getElementById('bsite').addEventListener('click', function (e) {
    e.stopPropagation();
    var secure = /^https:\/\//i.test(S.u || '');
    root.getElementById('sitehead').textContent = secure ? 'Connection is secure' : 'Connection is not secure';
    root.getElementById('siteorigin').textContent = S.u || 'New Tab';
    var selects = sitepanel.querySelectorAll ? sitepanel.querySelectorAll('select[data-perm]') : [];
    for (var i=0;i<selects.length;i++) selects[i].value = (S.pms && S.pms[selects[i].getAttribute('data-perm')]) || 'default';
    sitepanel.classList.toggle('open');
  });
  var permissionSelects = sitepanel.querySelectorAll ? sitepanel.querySelectorAll('select[data-perm]') : [];
  for (var pi=0;pi<permissionSelects.length;pi++) permissionSelects[pi].addEventListener('change', function () {
    post({t:'ui',a:'permission',m:this.getAttribute('data-perm'),u:this.value});
  });
  root.getElementById('clear-perms').addEventListener('click', function(){post({t:'ui',a:'clear-permissions'});sitepanel.classList.remove('open');});
  root.getElementById('clear-site').addEventListener('click', function(){if(confirm('Clear cookies and storage for this site?'))post({t:'ui',a:'clear-site-data'});sitepanel.classList.remove('open');});

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
    if (hideTimer) clearTimeout(hideTimer);
    hideTimer = setTimeout(hideIfIdle, 350);
  });
  input.addEventListener('focus', function () { setOpen(true); });

  function submit(url) {
    var v = url || input.value.trim();
    if (v) post({ t: 'go', u: v });
    input.blur();
    post({ t: 'ui', a: 'refocus' });
  }

  // --- suggestions (history + bookmarks) ------------------------------------
  var sug = root.getElementById('sug');
  var sugItems = [];
  var sugSel = -1; // -1 = the typed/search row, 0.. = items
  var sugTimer = 0;

  function hideSug() {
    sug.classList.remove('open');
    sug.textContent = '';
    sugItems = [];
    sugSel = -1;
  }
  function sugPaint() {
    var rows = sug.children;
    for (var i = 0; i < rows.length; i++) rows[i].classList.toggle('son', i === sugSel + 1);
  }
  function sugBuild() {
    sug.textContent = '';
    try {
      var br = okb.getBoundingClientRect();
      var iw = window.innerWidth || 1200;
      sug.style.left = Math.max(8, Math.min(br.left, iw - 352)) + 'px';
      sug.style.top = (br.bottom + 6) + 'px';
    } catch (e) {}
    var q = input.value.trim();
    var r0 = document.createElement('div');
    r0.className = 'srow';
    var ic0 = document.createElement('div');
    ic0.className = 'sic';
    ic0.innerHTML = I_LENS;
    r0.appendChild(ic0);
    var m0 = document.createElement('div');
    m0.className = 'smeta';
    var t0 = document.createElement('div');
    t0.className = 'stt';
    t0.textContent = 'Search ' + (S.e || 'Google') + ' for \u201C' + q.replace(/[<>&]/g, '') + '\u201D';
    m0.appendChild(t0);
    r0.appendChild(m0);
    r0.addEventListener('mousedown', function (e) { e.preventDefault(); submit(); });
    sug.appendChild(r0);
    for (var i = 0; i < sugItems.length; i++) {
      (function (it, idx) {
        var r = document.createElement('div');
        r.className = 'srow';
        var ic = document.createElement('div');
        ic.className = 'sic';
        ic.innerHTML = it.s === 'b' ? I_STARF : I_CLK;
        r.appendChild(ic);
        var meta = document.createElement('div');
        meta.className = 'smeta';
        var tt = document.createElement('div');
        tt.className = 'stt';
        var uu = document.createElement('div');
        uu.className = 'suu';
        meta.appendChild(tt);
        meta.appendChild(uu);
        r.appendChild(meta);
        tt.textContent = it.t || it.u;
        uu.textContent = it.u;
        r.addEventListener('mousedown', function (e) { e.preventDefault(); submit(it.u); });
        r.addEventListener('mouseenter', function () { sugSel = idx; sugPaint(); });
        sug.appendChild(r);
      })(sugItems[i], i);
    }
    sug.classList.add('open');
    sugPaint();
  }
  window.__okSuggest = function (list) {
    sugItems = list || [];
    if (document.activeElement === input && input.value.trim()) {
      sugBuild();
    }
  };
  input.addEventListener('input', function () {
    if (sugTimer) clearTimeout(sugTimer);
    if (!input.value.trim()) { hideSug(); return; }
    sugTimer = setTimeout(function () {
      sugTimer = 0;
      post({ t: 'suggest', u: input.value });
    }, 120);
  });
  input.addEventListener('blur', function () {
    setTimeout(hideSug, 150); // let a mousedown-click land first
  });

  input.addEventListener('keydown', function (e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      if (sug.classList.contains('open') && sugSel >= 0 && sugItems[sugSel]) {
        submit(sugItems[sugSel].u);
      } else {
        submit();
      }
    } else if (e.key === 'ArrowDown' && sug.classList.contains('open')) {
      e.preventDefault();
      sugSel = Math.min(sugSel + 1, sugItems.length - 1);
      sugPaint();
    } else if (e.key === 'ArrowUp' && sug.classList.contains('open')) {
      e.preventDefault();
      sugSel = Math.max(sugSel - 1, -1);
      sugPaint();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      if (sug.classList.contains('open')) { hideSug(); return; }
      input.value = S.u || '';
      input.blur();
      post({ t: 'ui', a: 'refocus' });
    }
  });
  root.getElementById('go').addEventListener('click', function () { submit(); });
  root.getElementById('bstar').addEventListener('click', function () {
    post({ t: 'bm' });
  });

  // --- top bar buttons ------------------------------------------------------
  root.getElementById('plus').addEventListener('click', function () { post({ t: 'ui', a: 'new' }); });

  // --- the menu beside minimize ----------------------------------------------
  var menu = root.getElementById('menu');
  var wmenu = root.getElementById('wmenu');
  function closeMenu() { menu.classList.remove('open'); }
  wmenu.addEventListener('click', function (e) {
    e.stopPropagation();
    menu.classList.toggle('open');
    if(menu.classList.contains('open')){var first=root.getElementById('m-newtab');if(first)first.focus();}
  });
  // Hovering a popover anchored to the bar pins it (the capsule's
  // mouseleave must not retire the bar while the user is INSIDE the menu).
  // Leaving the popover retires it (and then the bar, if nothing else
  // holds it) - otherwise the bar would wait for the menu forever.
  function pinFromPopover(el, retire) {
    el.addEventListener('mouseenter', function () {
      barPinned = true;
      if (hideTimer) { clearTimeout(hideTimer); hideTimer = 0; }
      if (briefTimer) { clearTimeout(briefTimer); briefTimer = 0; }
    });
    el.addEventListener('mouseleave', function () {
      barPinned = false;
      if (hideTimer) clearTimeout(hideTimer);
      hideTimer = setTimeout(function () {
        if (retire) retire();
        hideIfIdle();
      }, 350);
    });
  }
  pinFromPopover(menu, closeMenu);
  pinFromPopover(ctx, closeCtx);
  pinFromPopover(sitepanel, function(){ sitepanel.classList.remove('open'); });

  var mids = ['m-newtab', 'm-incognito', 'm-bookmarks', 'm-history', 'm-downloads', 'm-settings'];
  for (var mi = 0; mi < mids.length; mi++) {
    var mrow = root.getElementById(mids[mi]);
    if (!mrow) continue;
    mrow.setAttribute('role','menuitem'); mrow.setAttribute('tabindex','-1');
    (function (r, act) {
      r.addEventListener('click', function () {
        post({ t: 'menu', m: act });
        closeMenu();
      });
    })(mrow, mids[mi].slice(2));
  }
  menu.addEventListener('keydown', function(e){
    if(e.key==='Escape'){closeMenu();wmenu.focus();return;}
    if(e.key!=='ArrowDown'&&e.key!=='ArrowUp'&&e.key!=='Enter'&&e.key!==' ')return;
    var rows=[];for(var i=0;i<mids.length;i++){var r=root.getElementById(mids[i]);if(r)rows.push(r);}
    var at=rows.indexOf(document.activeElement);
    if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();at=(at+(e.key==='ArrowDown'?1:-1)+rows.length)%rows.length;rows[at].focus();}
    else if(at>=0){e.preventDefault();rows[at].click();}
  });

  // Events crossing out of a CLOSED shadow root are retargeted: at
  // document level every click's target is the shadow host - even for the
  // menu's own rows - and composedPath() hides closed-root internals too.
  // So the closer hit-tests COORDINATES instead (mouse coordinates are
  // never retargeted): clicks inside the menu or the menu button keep it
  // open, everything else closes it. (This was the dead-menu bug.)
  function inRect(el, x, y) {
    try {
      var r = el.getBoundingClientRect();
      return x >= r.left && x <= r.right && y >= r.top && y <= r.bottom;
    } catch (e) { return false; }
  }
  document.addEventListener('mousedown', function (e) {
    if (!menu.classList.contains('open')) return;
    if (inRect(menu, e.clientX, e.clientY) || inRect(wmenu, e.clientX, e.clientY)) return;
    closeMenu();
  }, true);
  root.getElementById('wmin').addEventListener('click', function () { post({ t: 'ui', a: 'wmin' }); });
  root.getElementById('wmax').addEventListener('click', function () { post({ t: 'ui', a: 'wmaxtoggle' }); });
  root.getElementById('wclose').addEventListener('click', function () { post({ t: 'ui', a: 'wclose' }); });
  root.getElementById('split-close').addEventListener('click', function () { post({ t: 'ui', a: 'close-split' }); });
  root.getElementById('split-swap').addEventListener('click', function () { post({ t: 'ui', a: 'swap-split' }); });
  root.getElementById('split-tab').addEventListener('click', function () { post({ t: 'ui', a: 'promote-split' }); });
  var splitDivider = root.getElementById('splitdivider');
  splitDivider.addEventListener('pointerdown', function (e) { e.preventDefault(); post({ t: 'ui', a: 'resize-split-start' }); });

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
    if (e.button === 0 && !document.fullscreenElement) {
      var open = strip.classList.contains('open');
      if (e.clientY < (open ? 3 : 6)) {
        e.preventDefault();
        e.stopPropagation();
        post({ t: 'ui', a: 'wtopresize' });
      }
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

  window.__okBar = function (s) {
    S = s; window.__okSplitActive = !!S.v; render(); sync(); stateLive = true;
    root.getElementById('wclose').title = S.v ? 'Close Split View' : 'Close';
    if (!S.u) { revealBar(false); setOpen(true); if (!stateLive) { input.focus(); input.select(); } }
  };
  window.__okProximityReveal = function () { revealBar(true); };

  // Liquid loading hairline at the top edge while a page loads.
  var prog = root.getElementById('prog');
  window.__okLoad = function (on) {
    if (on) {
      announce('Page loading');
      prog.classList.remove('done');
      prog.classList.add('on');
    } else {
      announce('Page loaded');
      prog.classList.remove('on');
      prog.classList.add('done');
      setTimeout(function () { prog.classList.remove('done'); }, 450);
    }
  };
  try { window.addEventListener('resize', function () { render(); }); } catch (e) {}
  window.__okBubbleFocus = function () {
    revealBar(false);
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
	U string `json:"u"` // for the letter avatar fallback
	F string `json:"f"` // favicon URL ('' = letter)
	P bool   `json:"p"` // pinned (favicon-only pill)
	S bool   `json:"s"` // sleeping to save memory
	A bool   `json:"a"` // currently playing audio
	N bool   `json:"n"` // site excluded from sleeping
}

// barState is the full state pushed to the active tab's shell UI.
type barState struct {
	Tabs []barTab `json:"tabs"`
	A    int      `json:"a"`
	U    string   `json:"u"`
	B    bool     `json:"b"`
	F    bool     `json:"f"`
	M    bool     `json:"m"` // window maximized
	K    bool     `json:"k"` // current page bookmarked
	E    string   `json:"e"` // search engine name
	V    bool              `json:"v"` // split view is active
	Pms  map[string]string `json:"pms,omitempty"`
	Q    bool              `json:"q"` // this is the focused split pane
	L    bool              `json:"l"` // this is the left/original pane
	G    bool              `json:"g"` // larger browser controls
}

func (a *app) permissionStateFor(t *tab) map[string]string {
	out := map[string]string{"camera":"default", "microphone":"default", "location":"default", "notifications":"default", "clipboard":"default", "sensors":"default"}
	if t == nil { return out }
	origin := permissionOrigin(t.url)
	for name := range out { if state := a.store.Permission(origin, name); state != "" { out[name] = state } }
	return out
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
		tabs[i] = barTab{T: title, U: tb.url, F: tb.favicon, P: tb.pinned, S: tb.sleeping, A: tb.audioPlaying, N: a.store.Settings().NeverSleep[permissionOrigin(tb.url)]}
	}
	push := func(view *tab, idx int) {
		if view == nil || view.chromium == nil {
			return
		}
		st := barState{
			Tabs: tabs,
			A:    idx,
			U:    view.url,
			B:    view.chromium.CanGoBack(),
			F:    view.chromium.CanGoForward(),
			M:    a.maximized,
			K:    view.url != "" && !view.isStart && a.store.IsBookmarked(view.url),
			E:    a.store.Settings().Engine,
			V:    a.splitTab != nil,
			Pms:  a.permissionStateFor(view),
			Q:    a.commandTab() == view,
			L:    view == a.active(),
			G:    a.store.Settings().LargeControls,
		}
		b, err := json.Marshal(st)
		if err == nil {
			view.chromium.Eval("window.__okBar&&window.__okBar(" + string(b) + ")")
		}
	}
	push(t, a.activeIdx)
	if a.splitTab != nil {
		for i, candidate := range a.tabs {
			if candidate == a.splitTab {
				push(candidate, i)
				break
			}
		}
	}
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
		if a.inSelfTest {
			a.stlog("[selftest] spawn throttled (last spawn %v ago)", now.Sub(a.lastSpawn))
		}
		return false
	}
	a.lastSpawn = now
	return true
}

// isDownloadPath confines file actions requested by web messages to the
// user's Downloads directory. Every web page has the bridge, so never trust a
// path merely because it arrived through the built-in downloads UI.
func isDownloadPath(path string) bool {
	home := os.Getenv("USERPROFILE")
	if home == "" || path == "" { return false }
	dir, err1 := filepath.Abs(filepath.Join(home, "Downloads"))
	p, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil { return false }
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".."
}

// onWebMessage receives JSON messages posted by tab t via window.__ok.
func (a *app) onWebMessage(t *tab, msg string) {
	var m struct {
		T  string  `json:"t"`
		U  string  `json:"u"`
		D  string  `json:"d"`
		F  string  `json:"f"`
		A  string  `json:"a"`
		M  string  `json:"m"`
		I  int     `json:"i"`
		To int     `json:"to"`
		X  float64 `json:"x"`
		Y  float64 `json:"y"`
		Ts int64   `json:"ts"`
	}
	if err := json.Unmarshal([]byte(msg), &m); err != nil {
		return
	}

	switch m.T {
	case "stclick": // self test only: dispatch a trusted click at x,y
		if a.inSelfTest && t != nil && t.chromium != nil {
			a.stlog("[selftest] trusted click requested at %.0f,%.0f", m.X, m.Y)
			// Chromium drops synthesized input for not-yet-visible widgets,
			// and the target tab may still be mid cross-fade - wait for it.
			a.stClickTab, a.stClickX, a.stClickY, a.stClickTicks = t, m.X, m.Y, 0
			win.SetTimer(a.hwnd, 3, 50, 0)
		}

	case "form-dirty":
		t.dirtyForm = m.A == "1"

	case "pane-focus":
		if t == a.active() || t == a.splitTab { a.focusedTab = t; a.pushBarState() }

	case "audio":
		t.audioPlaying = m.A == "1"
		a.pushBarState()

	case "proximity":
		if t == a.active() { a.execActive("window.__okProximityReveal&&window.__okProximityReveal()") }

	case "edge-link": // a dragged link committed at a window edge
		if m.U != "" {
			url, action := m.U, m.A
			a.postTask(func() {
				switch action {
				case "split":
					a.openSplit(url)
				case "later":
					a.newTab(url, false) // queued as a background tab
				case "tab":
					a.newTab(url, true)
				}
			})
		}

	case "open": // link explicitly asking for a new window
		if a.inSelfTest {
			a.stlog("[selftest] bridge open request: %s", m.U)
		}
		if m.U != "" && a.allowSpawn() {
			// Never create engines from inside the message callback: post
			// the work to the window-proc context instead.
			url := m.U
			if a.inSelfTest {
				a.stlog("[selftest] open accepted, posting new tab for %s", url)
			}
			a.postTask(func() { a.newTab(url, true) })
		}

	case "go": // address bubble, start page or built-in pages
		a.navigateTab(t, m.U)

	case "nav": // page reported its URL, title and favicon
		if !a.inSelfTest && m.U != "" && m.U != "about:blank" && !strings.HasPrefix(m.U, "okbrowser://") {
			a.store.AddHistory(m.U, m.D)
		}
		if m.F != "" && m.U != "" && m.U != "about:blank" {
			t.favicon = m.F
			a.store.SetFavicon(m.U, m.F)
		}
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

	case "bm": // star button: toggle bookmark for the active tab
		if t != nil && t.url != "" && !t.isStart && !strings.HasPrefix(t.url, "okbrowser://") {
			a.store.ToggleBookmark(t.url, t.title)
			a.pushBarState()
		}

	case "bm-del": // bookmarks page: remove one
		a.store.RemoveBookmark(m.U)

	case "hist-del": // history page: remove one visit
		a.store.RemoveHistory(m.U, m.Ts)

	case "clear": // settings / history page: clear a data set
		switch m.M {
		case "history":
			a.store.ClearHistory()
		case "bookmarks":
			a.store.ClearBookmarks()
		case "session":
			a.store.ClearSession()
		}

	case "set": // settings page: change a setting
		st := a.store.Settings()
		switch m.M {
		case "engine":
			st.Engine = m.U
		case "restore":
			st.RestoreSession = m.U == "1"
		case "autofill":
			st.Autofill = m.U == "1"
		case "large":
			st.LargeControls = m.U == "1"
		case "sleep":
			if n, err := strconv.Atoi(m.U); err == nil && n >= 0 && n <= 120 { st.SleepMinutes = n }
		}
		a.store.SetSettings(st)
		if m.M == "autofill" {
			for _, tab := range a.tabs {
				if settings, err := tab.chromium.GetSettings(); err == nil {
					on := st.Autofill && !incognitoMode
					_ = settings.PutIsPasswordAutosaveEnabled(on)
					_ = settings.PutIsGeneralAutofillEnabled(on)
				}
			}
		}
		a.pushBarState() // the address suggestions label follows the engine

	case "suggest": // address bubble typing: reply with suggestions
		q := m.U
		a.postTask(func() {
			sug := a.store.Suggest(q, 6)
			b, err := json.Marshal(sug)
			if err != nil {
				return
			}
			a.execActive("window.__okSuggest&&window.__okSuggest(" + string(b) + ")")
		})

	case "menu": // the menu button beside minimize
		switch m.M {
		case "newtab":
			a.postNewTab("")
		case "bookmarks":
			a.postTask(func() { a.navigateTab(a.active(), "okbrowser://bookmarks") })
		case "history":
			a.postTask(func() { a.navigateTab(a.active(), "okbrowser://history") })
		case "settings":
			a.postTask(func() { a.navigateTab(a.active(), "okbrowser://settings") })
		case "downloads":
			a.postTask(func() { a.navigateTab(a.active(), "okbrowser://downloads") })
		case "incognito":
			spawnIncognito()
		}

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
		case "dup": // tab context menu: duplicate
			i := m.I
			a.postTask(func() {
				if i >= 0 && i < len(a.tabs) {
					a.newTab(a.tabs[i].url, true)
				}
			})
			return
		case "pin": // tab context menu: pin / unpin
			i := m.I
			a.postTask(func() {
				if i >= 0 && i < len(a.tabs) {
					a.tabs[i].pinned = !a.tabs[i].pinned
					a.pushBarState()
				}
			})
			return
		case "sleep":
			i := m.I
			a.postTask(func() { if i >= 0 && i < len(a.tabs) { a.setTabSleeping(i, !a.tabs[i].sleeping) } })
			return
		case "never-sleep":
			i := m.I
			a.postTask(func() {
				if i >= 0 && i < len(a.tabs) {
					st := a.store.Settings(); origin := permissionOrigin(a.tabs[i].url)
					if st.NeverSleep == nil { st.NeverSleep = make(map[string]bool) }
					st.NeverSleep[origin] = !st.NeverSleep[origin]; if !st.NeverSleep[origin] { delete(st.NeverSleep, origin) }
					a.store.SetSettings(st); a.pushBarState()
				}
			})
			return
		case "tab-split-left", "tab-split-right":
			i, side := m.I, m.A
			a.postTask(func() { a.splitExistingTab(i, side == "tab-split-left") })
			return
		case "tab-split":
			i := m.I
			a.postTask(func() {
				if a.splitTab == nil && i >= 0 && i < len(a.tabs) && i != a.activeIdx {
					a.splitTab = a.tabs[i]; a.focusedTab = a.splitTab; a.splitRatio = .5; a.layout(); a.pushBarState()
				}
			})
			return
		case "close-others": // tab context menu
			i := m.I
			a.postTask(func() { a.closeOthers(i) })
			return
		case "reorder": // drag & drop
			from, to := m.I, m.To
			a.postTask(func() { a.reorderTab(from, to) })
			return
		case "permission":
			kinds := map[string]edge.CoreWebView2PermissionKind{"camera":edge.CoreWebView2PermissionKindCamera,"microphone":edge.CoreWebView2PermissionKindMicrophone,"location":edge.CoreWebView2PermissionKindGeolocation,"notifications":edge.CoreWebView2PermissionKindNotifications,"clipboard":edge.CoreWebView2PermissionKindClipboardRead,"sensors":edge.CoreWebView2PermissionKindOtherSensors}
			kind, ok := kinds[m.M]
			if ok {
				state := edge.CoreWebView2PermissionStateDefault
				if m.U == "allow" { state = edge.CoreWebView2PermissionStateAllow }
				if m.U == "deny" { state = edge.CoreWebView2PermissionStateDeny }
				origin := permissionOrigin(t.url)
				a.store.SetPermission(origin, m.M, m.U)
				t.chromium.SetPermission(kind, state)
				a.pushBarState()
			}
			return
		case "clear-permissions":
			origin := permissionOrigin(t.url)
			a.store.ClearPermissions(origin)
			for _, kind := range []edge.CoreWebView2PermissionKind{edge.CoreWebView2PermissionKindCamera,edge.CoreWebView2PermissionKindMicrophone,edge.CoreWebView2PermissionKindGeolocation,edge.CoreWebView2PermissionKindNotifications,edge.CoreWebView2PermissionKindClipboardRead,edge.CoreWebView2PermissionKindOtherSensors} { t.chromium.SetPermission(kind, edge.CoreWebView2PermissionStateDefault) }
			a.pushBarState()
			return
		case "clear-site-data":
			origin := permissionOrigin(t.url)
			if origin != "" {
				params, _ := json.Marshal(map[string]string{"origin":origin,"storageTypes":"all"})
				t.chromium.CallDevToolsProtocol("Storage.clearDataForOrigin", string(params))
				a.store.ClearPermissions(origin)
				t.chromium.Reload()
			}
			return
		case "close-split":
			a.postTask(func() { a.closeSplit() })
			return
		case "swap-split":
			a.postTask(func() { a.swapSplit() })
			return
		case "promote-split":
			a.postTask(func() { a.promoteSplit() })
			return
		case "resize-split-start":
			a.postTask(func() { if a.splitTab != nil { a.splitResizing = true; win.SetCapture(a.hwnd) } })
			return
		case "wclose":
			// In Split View the familiar close control dismisses the second
			// pane, never the whole browser window.
			if a.splitTab != nil {
				a.postTask(func() { a.closeSplit() })
			} else {
				a.postTask(func() { a.windowAction("wclose") })
			}
			return
			case "updates":
			a.checkForUpdates()
			return
		case "dl-open": // downloads page: open a validated file
			if isDownloadPath(m.U) { openPath(m.U) }
			return
		case "dl-show": // downloads page: reveal a validated file in Explorer
			if isDownloadPath(m.U) { showInFolder(m.U) }
			return
		case "dl-control":
			if isDownloadPath(m.U) { a.downloadAction(m.U, m.A) }
			return
		case "dl-refresh":
			a.postTask(func() { if t == a.active() { a.showInternal(t, "downloads") } })
			return
		case "dl-remove": // cancel a partial or delete one downloaded file
			path := m.U
			if isDownloadPath(path) {
				_ = os.Remove(path)
				a.postTask(func() { a.showInternal(t, "downloads") })
			}
			return
		case "wdrag", "wtopresize", "wmaxtoggle", "wmin":
			act := m.A
			a.postTask(func() { a.windowAction(act) })
			return
		}
		a.pushBarState()
	}
}
