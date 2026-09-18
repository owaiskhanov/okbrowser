//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
//  3. window.open through the page bridge (the overridden JS entry point)
//  4. window.open through the ENGINE's NewWindowRequested event (the
//     safety net - reached by restoring the native window.open)
//  5. a target=_blank link click through the bridge's capture listener
//
// Every phase must end with the page loaded in the right tab. Progress and
// the result are written to selftest.txt next to the executable, and the
// exit code is 0 only when every phase passes.

// startSelfTest enables the self test and writes progress to selftest.txt.
func (a *app) startSelfTest() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	f, err := os.Create(filepath.Join(filepath.Dir(exe), "selftest.txt"))
	if err != nil {
		return
	}
	a.selfTestFile = f
	a.inSelfTest = true
	go func() {
		time.Sleep(90 * time.Second)
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
		// window.open is overridden by the bridge: this goes through the
		// 'open' host message and must open a new tab.
		t.chromium.Eval(`window.open('https://www.iana.org/about')`)

	case a.selfPhase == 2 && strings.Contains(t.url, "iana.org/about") && len(a.tabs) > 1:
		a.stlog("[selftest] phase 3 OK: window.open opened a new tab (now %d tabs)", len(a.tabs))
		a.selfPhase = 3
		// Restore the NATIVE window.open so the request reaches the engine:
		// the NewWindowRequested event must mark it handled and open a tab.
		t.chromium.Eval(`delete window.open; window.open('https://www.iana.org/domains')`)

	case a.selfPhase == 3 && strings.Contains(t.url, "iana.org/domains") && len(a.tabs) > 2:
		a.stlog("[selftest] phase 4 OK: engine NewWindowRequested opened a new tab (now %d tabs)", len(a.tabs))
		a.selfPhase = 4
		// A real target=_blank link, like the ones on google.com: the
		// bridge's capture listener forwards it as a new tab.
		t.chromium.Eval(`(function(){var a=document.createElement('a');a.href='https://www.iana.org/help';a.target='_blank';a.textContent='x';document.body.appendChild(a);a.click();return 'ok'})()`)

	case a.selfPhase == 4 && strings.Contains(t.url, "iana.org/help") && len(a.tabs) > 3:
		a.stlog("[selftest] phase 5 OK: _blank link opened a new tab (now %d tabs)", len(a.tabs))
		a.stlog("[selftest] PASS: all navigation paths work")
		_ = a.selfTestFile.Close()
		os.Exit(0)
	}
}
