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
// It loads https://example.com, then (1) verifies the host-initiated load,
// (2) clicks the page's link (renderer-initiated same-tab navigation), and
// (3) calls window.open (renderer-initiated new tab). Progress and the
// result are written to selftest.txt next to the executable, and the exit
// code is 0 only when every phase passes.

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
		fmt.Fprintln(f, "[selftest] FAIL: timed out")
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
func (a *app) selftestNavHook(t *tab) {
	if !a.inSelfTest || t.chromium == nil {
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
		// window.open goes through the bridge (renderer-initiated new tab).
		t.chromium.Eval(`window.open('https://www.iana.org/about')`)

	case a.selfPhase == 2 && strings.Contains(t.url, "iana.org/about") && len(a.tabs) > 1:
		a.stlog("[selftest] phase 3 OK: window.open created a new tab (now %d tabs)", len(a.tabs))
		a.stlog("[selftest] PASS: all navigation paths work")
		_ = a.selfTestFile.Close()
		os.Exit(0)
	}
}
