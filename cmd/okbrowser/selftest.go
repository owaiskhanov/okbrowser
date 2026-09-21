//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lxn/win"
)

// Built-in end-to-end navigation self test. Run with:
//
//	OKBrowser.exe --selftest
//
// It loads https://example.com and then walks every navigation path a real
// user hits, in order:
//
//  1. host-initiated load (typed URL)
//  2. renderer-initiated same-tab navigation (a plain link click)
//  3. a featureless window.open (native, so it reaches the engine's
//     NewWindowRequested event and opens a tab)
//  4. a trusted target=_blank click through the ENGINE's
//     NewWindowRequested event
//  5. a target=_blank link click through the bridge's capture listener
//  6. a sized window.open() -> a REAL popup window with a dedicated child
//     WebView2 (the Google sign-in path)
//
// Every phase must end with the page loaded in the right tab. Progress and
// the result are written to selftest.txt next to the executable, and the
// exit code is 0 only when every phase passes.

// selfTestPath returns the log location: next to the executable.
func selfTestPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "selftest.txt"
	}
	return filepath.Join(filepath.Dir(exe), "selftest.txt")
}

// selfTestFileInit writes (or appends) a line to the self-test log before
// the app object exists, so even the earliest failures leave evidence.
func selfTestFileInit(line string) {
	f, err := os.OpenFile(selfTestPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

// startSelfTest enables the self test and writes progress to selftest.txt.
func (a *app) startSelfTest() {
	f, err := os.OpenFile(selfTestPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	a.selfTestFile = f
	a.inSelfTest = true
	go func() {
		time.Sleep(150 * time.Second)
		fmt.Fprintf(f, "[selftest] FAIL: timed out in phase %d\n", a.selfPhase+1)
		f.Close()
		os.Exit(1)
	}()
}

// stlog appends a line to the self-test log.
func (a *app) stlog(format string, args ...interface{}) {
	if a.selfTestFile != nil {
		fmt.Fprintf(a.selfTestFile, format+"\n", args...)
		_ = a.selfTestFile.Sync()
	}
}

// selftestNavHook advances the self test after each successful navigation.
// Error pages never advance the phases: their about:blank navigation would
// re-deliver the stale t.url and could skip a phase.
func (a *app) selftestNavHook(t *tab) {
	if !a.inSelfTest || t.chromium == nil || t.errPage {
		return
	}
	switch {
	case a.selfPhase == 0 && strings.Contains(t.url, "example.com"):
		a.stlog("[selftest] phase 1 OK: host-initiated navigation loaded %s", t.url)
		a.selfPhase = 1
		// Click the page's own link (renderer-initiated same-tab navigation).
		t.chromium.Eval(`(function(){var a=document.querySelector('a');return a?(a.click(),'clicked'):'no-link'})()`)

	case a.selfPhase == 1 && strings.Contains(t.url, "iana.org"):
		a.stlog("[selftest] phase 2 OK: link-click navigation loaded %s", t.url)
		a.selfPhase = 2
		// A featureless window.open stays native and reaches the
		// engine's NewWindowRequested event, which opens a new tab.
		t.chromium.Eval(`window.open('https://www.iana.org/about')`)

	case a.selfPhase == 2 && strings.Contains(t.url, "iana.org/about") && len(a.tabs) > 1:
		a.stlog("[selftest] phase 3 OK: window.open opened a new tab (now %d tabs)", len(a.tabs))
		a.selfPhase = 3
		// Inject a target=_blank link marked data-ok-engine (the bridge
		// deliberately does not intercept it) and report its center; the
		// host then dispatches a TRUSTED CDP mouse click on it. This is
		// the exact Gmail scenario: a real user click on a _blank link
		// must reach the engine's NewWindowRequested event.
		t.chromium.Eval(`(function(){var a=document.createElement('a');a.href='https://www.iana.org/domains';a.target='_blank';a.setAttribute('data-ok-engine','1');a.style.cssText='position:fixed;left:30vw;top:35vh;width:40vw;height:20vh;background:#c22;color:#fff;font-size:20px';a.textContent='selftest engine click';document.body.appendChild(a);var r=a.getBoundingClientRect();window.__ok({t:'stclick',x:r.left+r.width/2,y:r.top+r.height/2});return 'injected'})()`)

	case a.selfPhase == 3 && strings.Contains(t.url, "iana.org/domains") && len(a.tabs) > 2:
		a.stlog("[selftest] phase 4 OK: engine NewWindowRequested opened a new tab (now %d tabs)", len(a.tabs))
		a.selfPhase = 4
		// A real target=_blank link, like the ones on google.com: the
		// bridge's capture listener forwards it as a new tab. The click is
		// deferred past the popup throttle window so the previous spawn
		// (phase 4) is never rate-limited by this one.
		t.chromium.Eval(`(function(){var a=document.createElement('a');a.href='https://www.iana.org/help';a.target='_blank';a.textContent='x';document.body.appendChild(a);setTimeout(function(){a.click()},500);return 'ok'})()`)

	case a.selfPhase == 4 && strings.Contains(t.url, "iana.org/help") && len(a.tabs) > 3:
		a.stlog("[selftest] phase 5 OK: _blank link opened a new tab (now %d tabs)", len(a.tabs))
		a.selfPhase = 5
		// The OAuth shape: window.open WITH features must produce a real
		// popup window (child WebView2 handed to the engine), not a tab.
		t.chromium.Eval(`setTimeout(function(){window.open('https://www.iana.org/about','okpopup','width=500,height=600')},500)`)
		win.SetTimer(a.hwnd, 6, 6000, 0)
	}
}

// selftestPopupTick checks the final phase: the sized window.open must have
// produced a real popup window rather than another tab.
func (a *app) selftestPopupTick() {
	if !a.inSelfTest || a.selfPhase != 5 {
		return
	}
	win.KillTimer(a.hwnd, 6)
	if len(a.popups) == 0 {
		a.stlog("[selftest] FAIL: sized window.open did not create a popup window")
		_ = a.selfTestFile.Close()
		os.Exit(1)
	}
	a.stlog("[selftest] phase 6 OK: sized window.open created a real popup window")
	a.stlog("[selftest] PASS: all navigation paths work")
	_ = a.selfTestFile.Close()
	os.Exit(0)
}
