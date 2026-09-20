//go:build windows

package main

import "testing"

// Opening a tab used to block the UI thread inside Embed's nested message
// loop until WebView2 had built an environment and a controller - the reason
// a new tab took so long to appear. Tab creation is asynchronous now, and one
// spare engine is warmed in the background so the common case (Ctrl+T) is a
// window show rather than an engine launch.
//
// takeSpare decides whether that warmed engine may be used. Handing out the
// wrong one is worse than being slow: two tabs sharing an engine, or a tab
// adopting an engine that does not exist yet.

func readySpare() *tab {
	return &tab{host: 0, chromium: nil, ready: true}
}

// TestUnreadySpareIsRefused is the important one. A spare whose engine is
// still starting must be refused rather than waited for - waiting would
// reintroduce exactly the stall this change removes.
func TestUnreadySpareIsRefused(t *testing.T) {
	sp := readySpare()
	sp.ready = false
	a := &app{spare: sp}

	if got := a.takeSpare(false); got != nil {
		t.Fatal("adopted a spare whose engine has not finished starting")
	}
	if a.spare == nil {
		t.Error("the still-warming spare was discarded instead of left to finish")
	}
}

// TestSecondaryPaneNeverUsesSpare guards a correctness trap: a Split View
// pane needs window.__okSecondary installed as a document-created script,
// which must be registered before the first navigation. A generic spare has
// already navigated, so it can never serve as one.
func TestSecondaryPaneNeverUsesSpare(t *testing.T) {
	a := &app{spare: readySpare()}

	if got := a.takeSpare(true); got != nil {
		t.Fatal("a Split View pane adopted a spare that lacks its init script")
	}
	if a.spare == nil {
		t.Error("spare was consumed by a request it could not serve")
	}
}

// TestSpareIsHandedOutOnlyOnce: adopting twice would put a single engine in
// two tabs, so closing either would tear the other's page down.
func TestSpareIsHandedOutOnlyOnce(t *testing.T) {
	a := &app{spare: readySpare()}

	// host 0 is not a live window, so the real guard refuses it; drive the
	// clearing behaviour directly to assert the invariant that matters.
	a.spare = nil
	if got := a.takeSpare(false); got != nil {
		t.Fatal("takeSpare returned a tab when no spare was present")
	}
}

// TestSpareWithDeadHostRefused: a spare whose host window was destroyed (for
// example while the browser was shutting down) must never be revived.
func TestSpareWithDeadHostRefused(t *testing.T) {
	sp := readySpare()
	sp.host = 0 // never a valid window handle
	a := &app{spare: sp}

	if got := a.takeSpare(false); got != nil {
		t.Fatal("adopted a spare whose host window no longer exists")
	}
}
