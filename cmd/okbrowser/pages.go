//go:build windows

package main

import (
	"fmt"
	"strconv"
	"strings"
)

// pages.go renders OK Browser's built-in pages: the start page (speed
// dial + search), bookmarks, history and settings. They are generated
// HTML shown via NavigateToString, styled in the same Liquid Glass
// language as the shell, and talk to the host through window.__ok
// (the bridge script is injected into every page, including these).

// pageBase is the shared glass styling for all built-in pages.
const pageBase = `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
:root{color-scheme:light dark}
*{box-sizing:border-box;-webkit-user-select:none}
body{margin:0;min-height:100vh;display:flex;flex-direction:column;align-items:center;
font-family:-apple-system,'Segoe UI Variable Text','Segoe UI',system-ui,sans-serif;
background:linear-gradient(160deg,#f6f7fa 0%,#eceef4 55%,#e7e9f2 100%);
color:#1d1d1f;-webkit-tap-highlight-color:transparent}
@media (prefers-color-scheme:dark){body{
background:linear-gradient(160deg,#151519 0%,#101014 55%,#0c0c10 100%);color:#f2f2f7}}
.wrap{width:min(720px,92vw);padding:48px 0 64px}
h1{font-size:26px;font-weight:700;letter-spacing:-.02em;margin:0 0 20px}
.card{background:rgba(255,255,255,.55);border-radius:18px;
backdrop-filter:blur(28px) saturate(1.8);-webkit-backdrop-filter:blur(28px) saturate(1.8);
box-shadow:0 10px 34px rgba(0,0,0,.10),inset 0 1px 0 rgba(255,255,255,.65),
inset 0 0 0 .5px rgba(255,255,255,.35);overflow:hidden}
@media (prefers-color-scheme:dark){.card{background:rgba(30,30,36,.55);
box-shadow:0 10px 34px rgba(0,0,0,.45),inset 0 1px 0 rgba(255,255,255,.08),
inset 0 0 0 .5px rgba(255,255,255,.06)}}
.row{display:flex;align-items:center;gap:12px;padding:11px 16px;cursor:default;
transition:background .14s}
.row:hover{background:rgba(120,128,138,.10)}
.av{flex:0 0 auto;width:34px;height:34px;border-radius:50%;display:grid;place-items:center;
font-size:15px;font-weight:600;color:#3c4043;background:rgba(120,128,138,.14);
backdrop-filter:blur(10px);-webkit-backdrop-filter:blur(10px)}
@media (prefers-color-scheme:dark){.av{color:#e8eaed;background:rgba(200,205,214,.14)}}
.meta{flex:1 1 auto;min-width:0}
.tt{font-size:13.5px;font-weight:550;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.uu{font-size:11.5px;opacity:.55;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.xx{flex:0 0 auto;width:26px;height:26px;border-radius:50%;display:grid;place-items:center;
opacity:0;transition:opacity .14s,background .14s;cursor:default;font-size:13px;line-height:1}
.row:hover .xx{opacity:.6}
.xx:hover{background:rgba(120,128,138,.22);opacity:1 !important}
button,.pill{font-family:inherit}
input{-webkit-user-select:text}
.fade{animation:fade .3s ease}
@keyframes fade{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:none}}
</style></head><body>`

// pageScriptClose ends every built-in page.
const pageClose = `</body></html>`

// avChar picks the avatar letter for a URL (first letter of the host).
func avChar(u string) string {
	host := u
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if host == "" {
		return "•"
	}
	return strings.ToUpper(host[:1])
}

// avHTML renders a glass letter avatar for a URL.
func avHTML(u string) string {
	host := u
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	ch := "•"
	if host != "" {
		ch = strings.ToUpper(host[:1])
	}
	return `<div class="av">` + htmlEsc(ch) + `</div>`
}

// StartPageHTML renders the start page: big search field plus the
// most-visited speed dial.
func StartPageHTML(tiles []Tile, engine string) string {
	var b strings.Builder
	b.WriteString(pageBase)
	b.WriteString(`<div class="wrap fade" style="text-align:center">`)
	b.WriteString(`<div style="width:64px;height:64px;margin:8vh auto 22px;border-radius:20px;display:grid;place-items:center;font-size:24px;font-weight:800;color:#0a84ff;background:rgba(255,255,255,.6);backdrop-filter:blur(24px) saturate(1.8);-webkit-backdrop-filter:blur(24px) saturate(1.8);box-shadow:0 14px 40px rgba(10,132,255,.18),inset 0 1px 0 rgba(255,255,255,.8),inset 0 0 0 .5px rgba(255,255,255,.4)">OK</div>`)
	b.WriteString(`<div class="card" style="display:flex;align-items:center;gap:10px;padding:6px 8px 6px 18px;margin-bottom:34px">`)
	b.WriteString(`<input id="q" placeholder="Search with ` + htmlEsc(engine) + ` or enter address" spellcheck="false" autocomplete="off" style="all:unset;flex:1;font-size:15px;padding:12px 0;cursor:text">`)
	b.WriteString(`<div id="go" style="flex:0 0 auto;width:38px;height:38px;border-radius:50%;display:grid;place-items:center;color:#fff;background:rgba(10,132,255,.92);cursor:pointer">`)
	b.WriteString(`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h13"/><path d="M13 6l6 6-6 6"/></svg></div>`)
	b.WriteString(`</div><div id="tiles" style="display:grid;grid-template-columns:repeat(4,1fr);gap:12px">`)
	for _, t := range tiles {
		title := t.Title
		if title == "" {
			title = t.URL
		}
		b.WriteString(`<div class="tile" data-u="` + htmlEsc(t.URL) + `" style="padding:16px 8px;border-radius:16px;cursor:pointer;background:rgba(255,255,255,.5);backdrop-filter:blur(20px) saturate(1.7);-webkit-backdrop-filter:blur(20px) saturate(1.7);box-shadow:0 6px 20px rgba(0,0,0,.08),inset 0 1px 0 rgba(255,255,255,.6),inset 0 0 0 .5px rgba(255,255,255,.3);transition:transform .16s,background .16s">` +
			`<div style="margin:0 auto 8px;width:40px;height:40px;border-radius:50%;display:grid;place-items:center;font-size:17px;font-weight:600;color:#3c4043;background:rgba(120,128,138,.14)">` + htmlEsc(avChar(t.URL)) + `</div>` +
			`<div style="font-size:11.5px;font-weight:550;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;padding:0 4px">` + htmlEsc(title) + `</div></div>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<style>@media (prefers-color-scheme:dark){.tile{background:rgba(38,38,42,.5) !important;box-shadow:0 6px 20px rgba(0,0,0,.35),inset 0 1px 0 rgba(255,255,255,.07),inset 0 0 0 .5px rgba(255,255,255,.06) !important}}</style>`)
	b.WriteString(`<script>
(function(){
  var post=function(o){try{window.__ok(o)}catch(e){}};
  var q=document.getElementById('q');
  function go(){ if(q.value.trim()) post({t:'go',u:q.value.trim()}); }
  q.addEventListener('keydown',function(e){ if(e.key==='Enter'){e.preventDefault();go();} });
  document.getElementById('go').addEventListener('click',go);
  var tiles=document.querySelectorAll('.tile');
  for(var i=0;i<tiles.length;i++){
    (function(el){
      var u=el.getAttribute('data-u');
      el.addEventListener('mouseenter',function(){ el.style.transform='translateY(-2px)'; el.style.background='rgba(255,255,255,.8)'; });
      el.addEventListener('mouseleave',function(){ el.style.transform=''; el.style.background='rgba(255,255,255,.5)'; });
      el.addEventListener('click',function(){ post({t:'go',u:u}); });
    })(tiles[i]);
  }
  try{q.focus();}catch(e){}
})();
</script>`)
	b.WriteString(pageClose)
	return b.String()
}

// BookmarksHTML renders the bookmarks manager.
func BookmarksHTML(items []bmEntry) string {
	var b strings.Builder
	b.WriteString(pageBase)
	b.WriteString(`<div class="wrap fade"><h1>Bookmarks</h1><div class="card" id="list">`)
	if len(items) == 0 {
		b.WriteString(`<div class="row"><div class="meta"><div class="tt">No bookmarks yet</div><div class="uu">Tap the ★ in the address bar to save a page</div></div></div>`)
	}
	for _, it := range items {
		b.WriteString(`<div class="row" data-u="` + htmlEsc(it.URL) + `">` + avHTML(it.URL) +
			`<div class="meta"><div class="tt">` + htmlEsc(it.Title) + `</div><div class="uu">` + htmlEsc(it.URL) + `</div></div>` +
			`<div class="xx" title="Remove">✕</div></div>`)
	}
	b.WriteString(`</div></div>`)
	b.WriteString(`<script>
(function(){
  var post=function(o){try{window.__ok(o)}catch(e){}};
  var rows=document.querySelectorAll('.row[data-u]');
  for(var i=0;i<rows.length;i++){
    (function(r){
      var u=r.getAttribute('data-u');
      r.addEventListener('click',function(e){ if(e.target.className==='xx')return; post({t:'go',u:u}); });
      r.querySelector('.xx').addEventListener('click',function(e){ e.stopPropagation();
        post({t:'bm-del',u:u}); r.style.opacity='0'; setTimeout(function(){r.remove();},150); });
    })(rows[i]);
  }
})();
</script>`)
	b.WriteString(pageClose)
	return b.String()
}

// HistoryHTML renders the history page with a client-side filter.
func HistoryHTML(items []histEntry) string {
	var b strings.Builder
	b.WriteString(pageBase)
	b.WriteString(`<div class="wrap fade"><h1>History</h1>`)
	b.WriteString(`<div class="card" style="display:flex;align-items:center;gap:10px;padding:6px 16px;margin-bottom:14px">`)
	b.WriteString(`<input id="f" placeholder="Search history" spellcheck="false" style="all:unset;flex:1;font-size:13.5px;padding:10px 0;cursor:text">`)
	b.WriteString(`<div id="clear" style="flex:0 0 auto;font-size:12.5px;font-weight:550;color:#e0111b;cursor:pointer;padding:8px 10px;border-radius:10px" onmouseover="this.style.background='rgba(224,17,27,.10)'" onmouseout="this.style.background=''">Clear all</div>`)
	b.WriteString(`</div><div class="card" id="list">`)
	if len(items) == 0 {
		b.WriteString(`<div class="row"><div class="meta"><div class="tt">Nothing here yet</div><div class="uu">Pages you visit will appear here</div></div></div>`)
	}
	for _, it := range items {
		b.WriteString(`<div class="row" data-u="` + htmlEsc(it.URL) + `" data-ts="` + strconv.FormatInt(it.TS, 10) + `" data-q="` + htmlEsc(strings.ToLower(it.Title+" "+it.URL)) + `">` +
			avHTML(it.URL) +
			`<div class="meta"><div class="tt">` + htmlEsc(it.Title) + `</div><div class="uu">` + htmlEsc(it.URL) + `</div></div>` +
			`<div style="flex:0 0 auto;font-size:11px;opacity:.45;padding-right:10px"></div>` +
			`<div class="xx" title="Remove">✕</div></div>`)
	}
	b.WriteString(`</div></div>`)
	b.WriteString(`<script>
(function(){
  var post=function(o){try{window.__ok(o)}catch(e){}};
  var rows=document.querySelectorAll('.row[data-u]');
  for(var i=0;i<rows.length;i++){
    (function(r){
      var u=r.getAttribute('data-u'), ts=parseInt(r.getAttribute('data-ts'),10);
      var t=r.querySelector('div[style]');
      if(t){ try{ t.textContent=new Date(ts).toLocaleString([], {month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}); }catch(e){} }
      r.addEventListener('click',function(e){ if(e.target.className==='xx')return; post({t:'go',u:u}); });
      r.querySelector('.xx').addEventListener('click',function(e){ e.stopPropagation();
        post({t:'hist-del',u:u,ts:ts}); r.style.opacity='0'; setTimeout(function(){r.remove();},150); });
    })(rows[i]);
  }
  var f=document.getElementById('f');
  f.addEventListener('input',function(){
    var q=f.value.toLowerCase();
    for(var i=0;i<rows.length;i++){
      rows[i].style.display=(!q||rows[i].getAttribute('data-q').indexOf(q)>-1)?'':'none';
    }
  });
  document.getElementById('clear').addEventListener('click',function(){ post({t:'clear',m:'history'}); window.location.reload(); });
})();
</script>`)
	b.WriteString(pageClose)
	return b.String()
}

// SettingsHTML renders the settings page.
func SettingsHTML(s Settings, version string) string {
	var b strings.Builder
	b.WriteString(pageBase)
	b.WriteString(`<div class="wrap fade"><h1>Settings</h1>`)

	// Search engine
	b.WriteString(`<div style="font-size:12px;font-weight:650;opacity:.5;margin:22px 4px 8px;text-transform:uppercase;letter-spacing:.06em">Search engine</div>`)
	b.WriteString(`<div class="card" style="display:flex;gap:8px;padding:8px" id="eng">`)
	for _, name := range engineList {
		on := ""
		if name == s.Engine {
			on = `;color:#fff;background:rgba(10,132,255,.92)`
		}
		b.WriteString(`<div class="pill" data-v="` + name + `" style="flex:1;text-align:center;padding:10px 0;border-radius:12px;font-size:13.5px;font-weight:550;cursor:pointer;transition:background .15s` + on + `">` + name + `</div>`)
	}
	b.WriteString(`</div>`)

	// Startup
	b.WriteString(`<div style="font-size:12px;font-weight:650;opacity:.5;margin:22px 4px 8px;text-transform:uppercase;letter-spacing:.06em">On startup</div>`)
	b.WriteString(`<div class="card"><div class="row" style="padding:14px 16px"><div class="meta"><div class="tt">Reopen my tabs</div><div class="uu">Restore the tabs from your last session</div></div>`)
	b.WriteString(`<div id="restore" data-v="` + fmt.Sprintf("%t", s.RestoreSession) + `" style="flex:0 0 auto;width:46px;height:28px;border-radius:14px;position:relative;cursor:pointer;transition:background .2s;` +
		func() string {
			if s.RestoreSession {
				return `background:rgba(52,199,89,.95)`
			}
			return `background:rgba(120,128,138,.35)`
		}() + `">` +
		`<div style="position:absolute;top:2px;width:24px;height:24px;border-radius:50%;background:#fff;box-shadow:0 2px 6px rgba(0,0,0,.25);transition:left .2s;` +
		func() string {
			if s.RestoreSession {
				return `left:20px`
			}
			return `left:2px`
		}() + `"></div></div></div></div>`)

	// Privacy
	b.WriteString(`<div style="font-size:12px;font-weight:650;opacity:.5;margin:22px 4px 8px;text-transform:uppercase;letter-spacing:.06em">Privacy</div>`)
	b.WriteString(`<div class="card">`)
	b.WriteString(`<div class="row" id="ch"><div class="meta"><div class="tt">Clear browsing history</div></div><div class="xx" style="opacity:.6">Clear ›</div></div>`)
	b.WriteString(`<div class="row" id="cb"><div class="meta"><div class="tt">Clear bookmarks</div></div><div class="xx" style="opacity:.6">Clear ›</div></div>`)
	b.WriteString(`<div class="row" id="cs"><div class="meta"><div class="tt">Forget saved session</div></div><div class="xx" style="opacity:.6">Clear ›</div></div>`)
	b.WriteString(`</div>`)

	// About
	b.WriteString(`<div style="font-size:12px;font-weight:650;opacity:.5;margin:22px 4px 8px;text-transform:uppercase;letter-spacing:.06em">About</div>`)
	b.WriteString(`<div class="card"><div class="row" style="padding:14px 16px"><div class="av">OK</div><div class="meta"><div class="tt">OK Browser ` + version + `</div><div class="uu">Light and fast · WebView2 edition</div></div></div></div>`)
	b.WriteString(`</div>`)

	b.WriteString(`<script>
(function(){
  var post=function(o){try{window.__ok(o)}catch(e){}};
  var pills=document.querySelectorAll('.pill');
  for(var i=0;i<pills.length;i++){
    pills[i].addEventListener('click',function(){
      var v=this.getAttribute('data-v');
      for(var j=0;j<pills.length;j++){
        pills[j].style.cssText=pills[j].style.cssText.split(';color')[0];
      }
      this.style.cssText=this.style.cssText.split(';color')[0]+';color:#fff;background:rgba(10,132,255,.92)';
      post({t:'set',m:'engine',u:v});
    });
  }
  var r=document.getElementById('restore');
  if(r){
    r.addEventListener('click',function(){
      var on=this.getAttribute('data-v')!=='true';
      this.setAttribute('data-v',on?'true':'false');
      this.style.background=on?'rgba(52,199,89,.95)':'rgba(120,128,138,.35)';
      this.firstChild.style.left=on?'20px':'2px';
      post({t:'set',m:'restore',u:on?'1':'0'});
    });
  }
  function act(id,m){ var el=document.getElementById(id); if(el) el.addEventListener('click',function(){ post({t:'clear',m:m}); }); }
  act('ch','history'); act('cb','bookmarks'); act('cs','session');
})();
</script>`)
	b.WriteString(pageClose)
	return b.String()
}
