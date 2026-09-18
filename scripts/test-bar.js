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
  addEventListener(t, f) { (this._listeners[t] = this._listeners[t] || []).push(f); }
  dispatch(t, ev) {
    ev = Object.assign({ stopPropagation() {}, preventDefault() {} }, ev);
    (this._listeners[t] || []).forEach(f => f(ev));
  }
  attachShadow() { const r = new FakeElement('#shadow'); this.shadowRoot = r; global.__shadow = r; return r; }
  getElementById(id) {
    let found = null;
    const walk = el => { if (found) return; if (el.id === id) { found = el; return; } (el.children || []).forEach(walk); };
    walk(this); return found;
  }
  toggleAttribute(n, v) { if (v) this.attributes[n] = ''; else delete this.attributes[n]; }
  focus() { this.focused = true; global.document.activeElement = this; }
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

// --- tabs in the frameless top bar ---
win.__okBar({ tabs: [{ t: 'A' }, { t: 'B' }, { t: 'C' }], a: 1, u: 'https://example.com/x', b: true, f: false, m: false });
const tz = shadow.getElementById('tz');
assert.strictEqual(tz.children.length, 3, 'tab pills not rendered');
assert.ok(tz.children[1].className.includes('on'), 'active tab not marked');
assert.strictEqual(tz.children[1].children[0]._text, 'B', 'tab title wrong');
assert.ok(!('disabled' in shadow.getElementById('bback').attributes), 'back should be enabled');
assert.ok('disabled' in shadow.getElementById('bfwd').attributes, 'forward should be disabled');

const expect = (wanted) => {
  const got = JSON.stringify(sent.shift());
  assert.strictEqual(got, JSON.stringify(wanted), `expected ${JSON.stringify(wanted)}, got ${got}`);
};
tz.children[2].dispatch('click', EV); expect({ t: 'ui', a: 'switch', i: 2 });
tz.children[1].children[1].dispatch('click', EV); expect({ t: 'ui', a: 'close', i: 1 });
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
assert.ok(!okb.className.includes('open'), 'bubble starts collapsed');

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

console.log('shell UI logic tests: ALL PASSED');
