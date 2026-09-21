// Shell UI logic test - validates the Liquid Glass shell JavaScript that is
// embedded in cmd/okbrowser/bridge.go (frameless tab bar + address bubble),
// without needing Windows.
//
//   node scripts/test-bar.js
const fs = require('fs');
const path = require('path');
const assert = require('assert');

const src = fs.readFileSync(path.join(__dirname, '..', 'cmd', 'okbrowser', 'bridge.go'), 'utf8');
const m = src.match(/const barJS = `\r?\n([\s\S]*?)`/);
assert(m, 'barJS not found in bridge.go');
const js = m[1];

class FakeElement {
  constructor(tag) {
    this.tagName = tag; this.children = []; this.style = { cssText: '', display: '' };
    this._html = ''; this._text = ''; this.className = ''; this.disabled = false;
    this._listeners = {}; this.attributes = {}; this.id = '';
  }
  set innerHTML(v) { this._html = v; this.children = []; this._parse(v); }
  get innerHTML() { return this._html; }
  _parse(html) {
    const re = /<(div|input)[^>]*id="([^"]+)"[^>]*>/g; let mm;
    while ((mm = re.exec(html))) { const e = new FakeElement(mm[1]); e.id = mm[2]; this.children.push(e); }
  }
  set textContent(v) { this._text = v; this.children = []; }
  get textContent() { return this._text; }
  appendChild(c) { c.parentNode = this; this.children.push(c); return c; }
  remove() {
    if (this.parentNode) {
      const i = this.parentNode.children.indexOf(this);
      if (i >= 0) this.parentNode.children.splice(i, 1);
    }
  }
  get classList() {
    const self = this;
    const parts = () => self.className.split(/\s+/).filter(Boolean);
    return {
      add(c) { if (!parts().includes(c)) self.className = (self.className + ' ' + c).trim(); },
      remove(c) { self.className = parts().filter(x => x !== c).join(' '); },
      toggle(c, v) {
        const has = parts().includes(c);
        const want = v === undefined ? !has : v;
        if (want && !has) this.add(c);
        if (!want && has) this.remove(c);
      },
      contains(c) { return parts().includes(c); }
    };
  }
  addEventListener(t, f) { (this._listeners[t] = this._listeners[t] || []).push(f); }
  dispatch(t, ev) {
    ev = Object.assign({ stopPropagation() {}, preventDefault() {} }, ev);
    (this._listeners[t] || []).forEach(f => f.call(this, ev));
  }
  attachShadow() { const r = new FakeElement('#shadow'); this.shadowRoot = r; global.__shadow = r; return r; }
  getElementById(id) {
    let found = null;
    const walk = el => { if (found) return; if (el.id === id) { found = el; return; } (el.children || []).forEach(walk); };
    walk(this); return found;
  }
  toggleAttribute(n, v) { if (v) this.attributes[n] = ''; else delete this.attributes[n]; }
  setAttribute(n, v) { this.attributes[n] = String(v); if (n === 'id') this.id = String(v); }
  getAttribute(n) { return n in this.attributes ? this.attributes[n] : null; }
  get firstChild() { return this.children[0] || null; }
  focus() { this.focused = true; global.document.activeElement = this; }
  getBoundingClientRect() { return { left: 100, top: 100, right: 500, bottom: 126, width: 400, height: 26 }; }
  contains(el) { let n = el; while (n) { if (n === this) return true; n = n.parentNode; } return false; }
  select() { this.selected = true; }
  blur() {
    this.focused = false;
    if (global.document.activeElement === this) global.document.activeElement = null;
    (this._listeners['blur'] || []).forEach(f => f({}));
  }
}

const EV = { stopPropagation() {}, preventDefault() {}, button: 0 };
const sent = [];
let createdHost = null;
const docListeners = {};
global.setInterval = () => 0;
global.clearInterval = () => {};
const win = { __ok: o => sent.push(o), __okBarInstalled: false };
win.top = win; // act as the top frame
global.window = win;
global.document = {
  activeElement: null,
  createElement: t => { const e = new FakeElement(t); if (t === 'div' && !createdHost) createdHost = e; return e; },
  addEventListener(t, f) { (docListeners[t] = docListeners[t] || []).push(f); },
  removeEventListener(t, f) { docListeners[t] = (docListeners[t] || []).filter(g => g !== f); },
  body: new FakeElement('body'),
  documentElement: new FakeElement('html'),
};
global.CSSStyleSheet = class { replaceSync() {} };

new Function(js)();
const shadow = global.__shadow;
assert.strictEqual(createdHost.parentNode, global.document.body, 'shell should mount immediately when the document is ready');
assert.strictEqual(typeof win.__okBar, 'function', 'shell API not installed');
assert.strictEqual(typeof win.__okBubbleFocus, 'function', 'bubble focus API not installed');
assert.ok(!shadow.getElementById('strip').classList.contains('open'), 'bar starts hidden (immersive)');

// --- tabs in the frameless top bar ---
win.__okBar({ tabs: [{ t: 'A' }, { t: 'B' }, { t: 'C' }], a: 1, u: 'https://example.com/x', b: true, f: false, m: false });
const tz = shadow.getElementById('tz');
assert.strictEqual(tz.children.length, 3, 'tab pills not rendered');
assert.ok(tz.children[1].className.includes('on'), 'active tab not marked');
assert.strictEqual(tz.children[1].children[1]._text, 'B', 'tab title wrong');
assert.ok(!('disabled' in shadow.getElementById('bback').attributes), 'back should be enabled');
assert.ok('disabled' in shadow.getElementById('bfwd').attributes, 'forward should be disabled');

const expect = (wanted) => {
  const got = JSON.stringify(sent.shift());
  assert.strictEqual(got, JSON.stringify(wanted), `expected ${JSON.stringify(wanted)}, got ${got}`);
};
tz.children[2].dispatch('click', EV); expect({ t: 'ui', a: 'switch', i: 2 });
tz.children[1].children[2].dispatch('click', EV); expect({ t: 'ui', a: 'close', i: 1 });
tz.children[0].dispatch('auxclick', { button: 1 }); expect({ t: 'ui', a: 'close', i: 0 });
shadow.getElementById('plus').dispatch('click', EV); expect({ t: 'ui', a: 'new' });

// --- window controls and drag zone ---
shadow.getElementById('wmin').dispatch('click', EV); expect({ t: 'ui', a: 'wmin' });
shadow.getElementById('wmax').dispatch('click', EV); expect({ t: 'ui', a: 'wmaxtoggle' });
shadow.getElementById('wclose').dispatch('click', EV); expect({ t: 'ui', a: 'wclose' });
shadow.getElementById('drag').dispatch('mousedown', EV); expect({ t: 'ui', a: 'wdrag' });
shadow.getElementById('drag').dispatch('dblclick', EV); expect({ t: 'ui', a: 'wmaxtoggle' });

// maximize icon switches to the restore glyph
const wmax = shadow.getElementById('wmax');
assert.strictEqual((wmax._html.match(/<rect/g) || []).length, 1, 'max icon should be one rect');
assert.ok(!wmax._html.includes('<path'), 'max icon has no path');
win.__okBar({ tabs: [{ t: 'A' }], a: 0, u: '', b: false, f: false, m: true });
assert.ok(wmax._html.includes('<path'), 'restore icon adds the second window outline');

// --- the address bubble ---
const okb = shadow.getElementById('okb');
const input = shadow.getElementById('q');
win.__okBubbleFocus();
assert.ok(okb.className.includes('open'), 'focused address bubble opens');
input.blur();
// Start collapsed before testing the normal hover behavior.
okb.className = 'okb';

okb.dispatch('mouseenter', EV);
assert.ok(okb.className.includes('open'), 'hover opens the bubble');
okb.dispatch('mouseleave', EV);
assert.ok(!okb.className.includes('open'), 'leaving (unfocused) collapses the bubble');

shadow.getElementById('lens').dispatch('click', EV);
assert.ok(okb.className.includes('open'), 'clicking the lens opens the bubble');
assert.ok(input.focused && input.selected, 'lens click focuses and selects the input');

// typing + Enter navigates and collapses; Escape restores the URL
sent.length = 0;
input.value = 'example.com';
input.dispatch('keydown', { key: 'Enter', preventDefault() {} });
assert.strictEqual(sent[0].t, 'go', 'Enter should navigate');
assert.strictEqual(sent[1].a, 'refocus', 'Enter should refocus content');
input.dispatch('blur', EV);
assert.ok(!okb.className.includes('open'), 'bubble collapses after submit');

win.__okBubbleFocus();
assert.ok(okb.className.includes('open') && input.focused, 'Ctrl+L opens and focuses the bubble');
input.value = 'user is still typing';
win.__okBar({ tabs: [{ t: 'A' }], a: 0, u: 'https://stale-native-state.test/', b: false, f: false, m: false });
assert.strictEqual(input.value, 'user is still typing', 'state pushes must not overwrite address text while editing');
input.blur();
win.__okBar({ tabs: [{ t: 'A' }], a: 0, u: 'https://fresh-native-state.test/', b: false, f: false, m: false });
assert.strictEqual(input.value, 'https://fresh-native-state.test/', 'address updates after editing ends');

// --- keyed rendering: pills update in place, only genuinely new ones animate ---
const pill0 = tz.children[0];
win.__okBar({ tabs: [{ t: 'A2' }, { t: 'B' }], a: 0, u: '', b: false, f: false, m: false });
assert.strictEqual(tz.children[0], pill0, 'pills must be reused, not rebuilt');
assert.strictEqual(tz.children[0].children[1]._text, 'A2', 'title updates in place');
assert.strictEqual(tz.children.length, 2, 'removed pill is dropped');
win.__okBar({ tabs: [{ t: 'A2' }, { t: 'B' }, { t: 'C' }], a: 2, u: '', b: false, f: false, m: false });
assert.ok(tz.children[2].classList.contains('in'), 'new pill gets the subtle enter animation');
tz.children[2].dispatch('animationend', {});
assert.ok(!tz.children[2].classList.contains('in'), 'animation class removed after it ends');
assert.ok(tz.children[2].classList.contains('on'), 'active pill marked via classList');

// --- find in page ---
assert.strictEqual(typeof win.__okFind, 'function', 'find API not installed');
assert.strictEqual(typeof win.__okFindCycle, 'function', 'find-cycle API not installed');
win.__okFind();
const findEl = shadow.getElementById('find');
assert.ok(findEl.classList.contains('open'), '__okFind opens the find bar');
assert.ok(shadow.getElementById('fq').focused, 'find input focused');
shadow.getElementById('fq').dispatch('keydown', { key: 'Escape', preventDefault() {} });
assert.ok(!findEl.classList.contains('open'), 'Esc closes the find bar');

// --- deferred mounting: document-start before <html> exists ---
{
  const sent2 = [];
  let host2 = null;
  const listeners2 = {};
  const win2 = { __ok: o => sent2.push(o), __okBarInstalled: false };
  win2.top = win2;
  const doc2 = {
    activeElement: null,
    createElement: t => { const e = new FakeElement(t); if (t === 'div' && !host2) host2 = e; return e; },
    addEventListener(t, f) { (listeners2[t] = listeners2[t] || []).push(f); },
    removeEventListener(t, f) { listeners2[t] = (listeners2[t] || []).filter(g => g !== f); },
    body: null,
    documentElement: null, // <-- document-start: no root element yet
  };
  const prevWindow = global.window, prevDocument = global.document;
  global.window = win2; global.document = doc2;
  try {
    new Function(js)();
    assert.strictEqual(typeof win2.__okBar, 'function', 'APIs must install even before the document root exists');
    assert.ok(!host2.parentNode, 'host must not be mounted yet');
    // the page's <html> appears; the script mounts as soon as it does
    const html = new FakeElement('html');
    doc2.documentElement = html;
    (listeners2['readystatechange'] || []).forEach(f => f({}));
    assert.strictEqual(host2.parentNode, html, 'host must mount once documentElement exists');
  } finally {
    global.window = prevWindow; global.document = prevDocument;
  }
}

// --- menu (beside minimize), star, suggestions ---
sent.length = 0; // earlier sections leave consumed messages in the queue
const menu = shadow.getElementById('menu');
const wmenu = shadow.getElementById('wmenu');
assert.ok(menu && wmenu, 'menu button + popover exist');
wmenu.dispatch('click', { stopPropagation() {} });
assert.ok(menu.classList.contains('open'), 'menu opens');
shadow.getElementById('m-history').dispatch('click', EV);
expect({ t: 'menu', m: 'history' });
assert.ok(!menu.classList.contains('open'), 'menu closes after an action');

const bstar = shadow.getElementById('bstar');
win.__okBar({ tabs: [{ t: 'A' }], a: 0, u: 'https://x', b: false, f: false, m: false, k: true, e: 'Google' });
assert.ok(bstar._html.includes('fill="currentColor"'), 'star fills for a bookmarked page');
win.__okBar({ tabs: [{ t: 'A' }], a: 0, u: 'https://y', b: false, f: false, m: false, k: false, e: 'Google' });
assert.ok(!bstar._html.includes('fill="currentColor"'), 'star is hollow when not bookmarked');
bstar.dispatch('click', EV);
expect({ t: 'bm' });

assert.strictEqual(typeof win.__okSuggest, 'function', 'suggest API installed');
input.focus();
input.value = 'exam';
win.__okSuggest([{ u: 'https://example.com', t: 'Example', s: 'h' }]);
const sug = shadow.getElementById('sug');
assert.ok(sug.classList.contains('open'), 'suggestion list opens');
assert.strictEqual(sug.children.length, 2, 'search row + one suggestion row');
input.dispatch('keydown', { key: 'ArrowDown', preventDefault() {} });
input.dispatch('keydown', { key: 'Enter', preventDefault() {} });
expect({ t: 'go', u: 'https://example.com' }); // Enter on the selected suggestion

// --- favicons, auto-collapse, pin, drag reorder, context menu, loading line ---
sent.length = 0;
win.innerWidth = 1600;
win.__okBar({ tabs: [
  { t: 'Alpha', u: 'https://alpha.com/', f: 'https://alpha.com/icon.png' },
  { t: 'Beta', u: 'https://beta.com/', f: '' }
], a: 0, u: '', b: false, f: false, m: false });
assert.strictEqual(tz.children[0].children[0].children[0].tagName, 'img', 'favicon pill shows an img');
assert.strictEqual(tz.children[0].children[0].children[0].src, 'https://alpha.com/icon.png', 'favicon src set');
assert.strictEqual(tz.children[1].children[0]._text, 'B', 'no favicon -> letter avatar (host letter)');
assert.strictEqual(tz.children[1].children[1]._text, 'Beta', 'title renders next to the icon');
assert.ok(!tz.classList.contains('mini'), 'few tabs: full pills');

// auto-collapse when crowded
win.innerWidth = 700;
win.__okBar({ tabs: Array.from({ length: 10 }, (_, i) => ({ t: 'T' + i, u: 'https://t' + i + '.com/' })), a: 0, u: '', b: false, f: false, m: false });
assert.ok(tz.classList.contains('mini'), 'crowded: pills collapse to favicon-only');
win.innerWidth = 1600;
win.__okBar({ tabs: [{ t: 'A', u: 'https://a.com/' }, { t: 'B', u: 'https://b.com/' }], a: 1, u: '', b: false, f: false, m: false });
assert.ok(!tz.classList.contains('mini'), 'room again: pills expand');

// pinned pill is favicon-only
win.__okBar({ tabs: [{ t: 'A', u: 'https://a.com/', p: true }, { t: 'B', u: 'https://b.com/' }], a: 0, u: '', b: false, f: false, m: false });
assert.ok(tz.children[0].classList.contains('pin'), 'pinned pill marked');
assert.strictEqual(tz.children[0].title, 'A', 'pinned pill keeps the title as tooltip');

// drag reorder
sent.length = 0;
tz.children[1].dispatch('dragstart', { dataTransfer: { setData() {} } });
tz.children[0].dispatch('drop', { preventDefault() {} });
expect({ t: 'ui', a: 'reorder', i: 1, to: 0 });

// tab context menu
const ctx = shadow.getElementById('ctx');
tz.children[0].dispatch('contextmenu', { clientX: 60, clientY: 60, preventDefault() {} });
assert.ok(ctx.classList.contains('open'), 'right-click opens the tab menu');
assert.strictEqual(shadow.getElementById('c-pin').textContent, 'Unpin tab', 'pin label reflects the pinned state');
shadow.getElementById('c-dup').dispatch('click', EV);
expect({ t: 'ui', a: 'dup', i: 0 });
assert.ok(!ctx.classList.contains('open'), 'context menu closes after an action');

// loading hairline
assert.strictEqual(typeof win.__okLoad, 'function', 'loading API installed');
win.__okLoad(true);
assert.ok(shadow.getElementById('prog').classList.contains('on'), 'loading line appears');
win.__okLoad(false);
assert.ok(shadow.getElementById('prog').classList.contains('done'), 'loading line completes');

// menu: clicks retargeted to the shadow host (what document-level
// listeners see in a real browser for ALL shadow content) must NOT close
// the menu - this was the bug that made every menu item dead.
wmenu.dispatch('click', { stopProjection() {}, stopPropagation() {} });
assert.ok(menu.classList.contains('open'), 'menu opens');
(docListeners['mousedown'] || []).forEach(f => f({ clientX: 200, clientY: 110, target: createdHost }));
assert.ok(menu.classList.contains('open'), 'a click inside the menu (retargeted target, real coordinates) must not close it');
(docListeners['mousedown'] || []).forEach(f => f({ clientX: 10, clientY: 400, target: createdHost }));
assert.ok(!menu.classList.contains('open'), 'a click outside the menu closes it');

// menu carries the new entries
sent.length = 0;
wmenu.dispatch('click', { stopPropagation() {} });
shadow.getElementById('m-incognito').dispatch('click', EV);
expect({ t: 'menu', m: 'incognito' });
wmenu.dispatch('click', { stopPropagation() {} });
shadow.getElementById('m-downloads').dispatch('click', EV);
expect({ t: 'menu', m: 'downloads' });

// --- settings page: the REAL page script against a fake settings DOM ---
{
  const src2 = fs.readFileSync(path.join(__dirname, '..', 'cmd', 'okbrowser', 'pages.go'), 'utf8');
  const m2 = src2.match(/const settingsPageJS = `([\s\S]*?)`/);
  assert(m2, 'settingsPageJS not found in pages.go');

  const prevWindow2 = global.window, prevDocument2 = global.document;
  const sent2 = [];
  const toasts = [];
  const win2 = { __ok: o => sent2.push(o), __okToast: m => toasts.push(m) };
  win2.top = win2;
  global.window = win2;

  const doc2 = {
    activeElement: null,
    createElement: t => new FakeElement(t),
    addEventListener() {}, removeEventListener() {},
    getElementById(id) {
      let found = null;
      const walk = el => { if (found || !el) return; if (el.id === id) { found = el; return; } (el.children || []).forEach(walk); };
      walk(doc2.body);
      return found;
    },
    querySelectorAll(sel) {
      const cls = sel.replace(/^\./, '');
      const out = [];
      const walk = el => { if (!el) return; if (el.classList && el.classList.contains(cls)) out.push(el); (el.children || []).forEach(walk); };
      walk(doc2.body);
      return out;
    },
    body: new FakeElement('body'),
    documentElement: new FakeElement('html'),
  };
  global.document = doc2;

  const google = new FakeElement('div'); google.className = 'pill on'; google.setAttribute('data-v', 'Google');
  const bing = new FakeElement('div'); bing.className = 'pill'; bing.setAttribute('data-v', 'Bing');
  const ddg = new FakeElement('div'); ddg.className = 'pill'; ddg.setAttribute('data-v', 'DuckDuckGo');
  const eng = new FakeElement('div'); eng.className = 'card';
  eng.appendChild(google); eng.appendChild(bing); eng.appendChild(ddg);

  const restore = new FakeElement('div'); restore.id = 'restore'; restore.setAttribute('data-v', 'true');
  const knob = new FakeElement('div'); knob.style = { cssText: '', display: '', left: '20px' };
  restore.appendChild(knob);

  const rows = ['ch', 'cb', 'cs'].map(id => { const r = new FakeElement('div'); r.id = id; return r; });

  doc2.body.appendChild(eng);
  doc2.body.appendChild(restore);
  rows.forEach(r => doc2.body.appendChild(r));

  new Function(m2[1])(); // run the real settings page script

  // engine pill click
  bing.dispatch('click', EV);
  assert.deepStrictEqual(sent2.shift(), { t: 'set', m: 'engine', u: 'Bing' }, 'engine click posts set');
  assert.ok(bing.classList.contains('on'), 'clicked pill activates');
  assert.ok(!google.classList.contains('on'), 'previous pill deactivates');
  assert.ok(!ddg.classList.contains('on'), 'other pill stays off');
  assert.ok(toasts.some(x => x.includes('Bing')), 'engine change shows a toast');

  // restore toggle off / on
  restore.dispatch('click', EV);
  assert.deepStrictEqual(sent2.shift(), { t: 'set', m: 'restore', u: '0' }, 'toggle off posts 0');
  restore.dispatch('click', EV);
  assert.deepStrictEqual(sent2.shift(), { t: 'set', m: 'restore', u: '1' }, 'toggle on posts 1');

  // clear rows
  rows[0].dispatch('click', EV);
  assert.deepStrictEqual(sent2.shift(), { t: 'clear', m: 'history' }, 'clear history row');
  rows[1].dispatch('click', EV);
  assert.deepStrictEqual(sent2.shift(), { t: 'clear', m: 'bookmarks' }, 'clear bookmarks row');
  rows[2].dispatch('click', EV);
  assert.deepStrictEqual(sent2.shift(), { t: 'clear', m: 'session' }, 'clear session row');
  assert.ok(toasts.length >= 4, 'every action gives visible feedback');

  global.window = prevWindow2;
  global.document = prevDocument2;
}

// --- immersive auto-hide bar ---
// Auto-hide applies to both normal pages and New Tab.
win.__okBar({ tabs: [{ t: 'Example' }], a: 0, u: 'https://example.com', b: false, f: false, m: false, v: false });
const strip = shadow.getElementById('strip');
assert.ok(strip, 'strip element exists');
assert.ok(shadow.getElementById('edge'), 'top-edge tripwire exists');
assert.ok(shadow.getElementById('wcap'), 'window-control capsule exists');

// the bar may be open from the tests above; retire it first
input.blur();
strip.dispatch('mouseleave', {});
setTimeout(() => {
  assert.ok(!strip.classList.contains('open'), 'bar hides when the mouse leaves and nothing is focused');

  (docListeners['mousemove'] || []).forEach(f => f({ clientY: 2 }));
  assert.ok(strip.classList.contains('open'), 'mouse at the top edge reveals the bar');
  assert.ok(!shadow.getElementById('wcap').classList.contains('hid'), 'the capsule returns with the bar');
  strip.dispatch('mouseenter', {});
  win.__okBar({ tabs: [{ t: 'A' }, { t: 'B' }, { t: 'C' }, { t: 'D' }], a: 3, u: 'https://example.com/d', b: false, f: false, m: false });
  assert.ok(strip.classList.contains('open'), 'tab switch keeps the bar visible');
  assert.ok(tz.children[3].classList.contains('in'), 'the new tab pill animates in');

  win.__okBubbleFocus();
  assert.ok(strip.classList.contains('open'), 'Ctrl+L reveals the bar');
  input.blur();
  strip.dispatch('mouseleave', {});
  setTimeout(() => {
    assert.ok(!strip.classList.contains('open'), 'bar hides again after the mouse leaves');
    assert.ok(shadow.getElementById('wcap').classList.contains('hid'), 'the capsule hides with the bar');

    // the bar must never retire while the mouse is INSIDE the menu
    (docListeners['mousemove'] || []).forEach(f => f({ clientY: 2 })); // reveal
    wmenu.dispatch('click', { stopPropagation() {} });
    assert.ok(menu.classList.contains('open'), 'menu opens');
    strip.dispatch('mouseleave', {});   // heading down into the menu...
    menu.dispatch('mouseenter', {});    // ...and hovering it: pin!
    setTimeout(() => {
      assert.ok(strip.classList.contains('open'), 'bar stays while the mouse is over the menu');
      assert.ok(menu.classList.contains('open'), 'menu stays open while hovered');
      menu.dispatch('mouseleave', {});  // leaving the menu retires both
      setTimeout(() => {
        assert.ok(!menu.classList.contains('open'), 'menu closes once the bar retires');
        assert.ok(!strip.classList.contains('open'), 'bar retires after leaving the menu');

        // New Tab is immersive too: it opens ready for typing, then retires
        // after focus and the pointer leave. Hovering the top edge restores it.
        win.__okBar({ tabs: [{ t: 'New Tab' }], a: 0, u: '', b: false, f: false, m: false, v: false });
        win.__okBubbleFocus();
        assert.ok(strip.classList.contains('open'), 'New Tab controls can be revealed');
        input.blur();
        strip.dispatch('mouseleave', {});
        setTimeout(() => {
          assert.ok(!strip.classList.contains('open'), 'New Tab bar hides when no longer in use');
          (docListeners['mousemove'] || []).forEach(f => f({ clientY: 2 }));
          assert.ok(strip.classList.contains('open'), 'top-edge hover reveals New Tab bar again');
          console.log('shell UI logic tests: ALL PASSED');
        }, 550);
      }, 600);
    }, 600);
  }, 550);
}, 550);
