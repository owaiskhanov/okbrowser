// Bar logic test - validates the Liquid Glass bar JavaScript that is
// embedded in cmd/okbrowser/bridge.go, without needing Windows.
//
//   node scripts/test-bar.js
//
// It extracts barJS from bridge.go, runs it against a minimal fake DOM and
// asserts rendering, state sync, click routing and keyboard handling.
const fs = require('fs');
const path = require('path');
const assert = require('assert');

const src = fs.readFileSync(path.join(__dirname, '..', 'cmd', 'okbrowser', 'bridge.go'), 'utf8');
const m = src.match(/const barJS = `\n([\s\S]*?)`\n/);
assert(m, 'barJS not found in bridge.go');
const js = m[1];

class FakeElement {
  constructor(tag) {
    this.tagName = tag; this.children = []; this.style = { cssText: '', display: '' };
    this._html = ''; this._text = ''; this.className = ''; this.disabled = false;
    this._listeners = {}; this.attributes = {};
  }
  set innerHTML(v) { this._html = v; this.children = []; this._parse(v); }
  get innerHTML() { return this._html; }
  _parse(html) {
    const re = /<(div|input)[^>]*id="([^"]+)"[^>]*>/g; let mm;
    while ((mm = re.exec(html))) { const e = new FakeElement(mm[1]); e.id = mm[2]; this.children.push(e); }
  }
  set textContent(v) { this._text = v; this.children = []; }
  get textContent() { return this._text; }
  appendChild(c) { this.children.push(c); return c; }
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
  focus() { this.focused = true; } select() { this.selected = true; } blur() {}
}

const EV = { stopPropagation() {}, preventDefault() {}, button: 0 };
const sent = [];
const win = { __ok: o => sent.push(o), __okBarInstalled: false };
win.top = win; // act as the top frame
global.window = win;
global.document = {
  activeElement: null,
  createElement: t => new FakeElement(t),
  addEventListener() {},
  body: { appendChild() {} },
  documentElement: { appendChild() {} },
};
global.CSSStyleSheet = class { replaceSync() {} };

new Function(js)();
const shadow = global.__shadow;
assert.strictEqual(typeof win.__okBar, 'function', 'bar API not installed');
assert.strictEqual(typeof win.__okBarFocus, 'function', 'focus API not installed');

win.__okBar({ tabs: [{ t: 'A' }, { t: 'B' }, { t: 'C' }], a: 1, u: 'https://example.com/x', b: true, f: false });
const tabsEl = shadow.getElementById('tabs');
assert.strictEqual(tabsEl.children.length, 3, 'tab pills not rendered');
assert.ok(tabsEl.children[1].className.includes('on'), 'active tab not marked');
assert.strictEqual(tabsEl.children[1].children[0]._text, 'B', 'tab title wrong');
assert.ok(!('disabled' in shadow.getElementById('back').attributes), 'back should be enabled');
assert.ok('disabled' in shadow.getElementById('fwd').attributes, 'forward should be disabled');

const expect = (wanted) => {
  const got = JSON.stringify(sent.shift());
  assert.strictEqual(got, JSON.stringify(wanted), `expected ${JSON.stringify(wanted)}, got ${got}`);
};
tabsEl.children[2].dispatch('click', EV); expect({ t: 'ui', a: 'switch', i: 2 });
tabsEl.children[1].children[1].dispatch('click', EV); expect({ t: 'ui', a: 'close', i: 1 });
tabsEl.children[0].dispatch('auxclick', { button: 1 }); expect({ t: 'ui', a: 'close', i: 0 });
shadow.getElementById('plus').dispatch('click', EV); expect({ t: 'ui', a: 'new' });
shadow.getElementById('rl').dispatch('click', EV); expect({ t: 'ui', a: 'reload' });
shadow.getElementById('back').dispatch('click', EV); expect({ t: 'ui', a: 'back' });
shadow.getElementById('fwd').dispatch('click', EV); expect({ t: 'ui', a: 'forward' });

const input = shadow.getElementById('a');
input.dispatch('keydown', { key: 'Enter', preventDefault() {} });
assert.strictEqual(sent[0].t, 'go', 'Enter should navigate');
assert.strictEqual(sent[1].a, 'refocus', 'Enter should refocus content');
input.dispatch('keydown', { key: 'Escape', preventDefault() {} });
assert.strictEqual(sent[sent.length - 1].a, 'refocus', 'Escape should refocus content');

win.__okBarFocus();
assert.ok(input.focused && input.selected, 'Ctrl+L should focus and select the address');

console.log('bar logic tests: ALL PASSED');
