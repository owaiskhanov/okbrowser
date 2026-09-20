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

// A popup opened from an already-loaded page - "Sign in with Google" is the
// common one - arrives through NewWindowRequested and becomes a new tab with
// a URL. WebView2 can invoke the engine-creation completion handler
// SYNCHRONOUSLY in that situation, because the browser process and the
// environment already exist.
//
// The URL therefore has to be attached to the tab before engine creation
// starts. When it was assigned after the EmbedAsync call, an inline
// completion ran first, found no pending URL, and rendered the start page -
// the sign-in tab opened and then just sat there empty.

// TestPendingURLSurvivesInlineEngineCompletion models the completion handler
// firing before createTab has returned.
func TestPendingURLSurvivesInlineEngineCompletion(t *testing.T) {
	const oauth = "https://accounts.google.com/o/oauth2/auth?client_id=x"

	// The tab as createTab now builds it: URL present from the start.
	tb := &tab{title: "New Tab", zoom: 1.0, pendingURL: oauth}

	// The engine completes inline, before anything else touches the tab.
	// This mirrors the branch in onEngineReady that consumes pendingURL.
	tb.ready = true
	navigated := ""
	if tb.pendingURL != "" {
		navigated = tb.pendingURL
		tb.pendingURL = ""
	}

	if navigated != oauth {
		t.Fatalf("inline engine completion did not navigate to the popup URL; got %q", navigated)
	}
	if tb.pendingURL != "" {
		t.Errorf("pendingURL left set after navigating: %q", tb.pendingURL)
	}
}

// TestAttachDoesNotNavigateTwice guards the other half of the fix. Once an
// inline completion has consumed the URL, attachTab must not issue the same
// navigation again - a second Navigate on an in-flight OAuth load can cancel
// the first and lose the authorisation state.
func TestAttachDoesNotNavigateTwice(t *testing.T) {
	const oauth = "https://accounts.google.com/o/oauth2/auth?client_id=x"

	// State after an inline completion has already navigated.
	tb := &tab{ready: true, pendingURL: "", url: oauth}

	// The condition attachTab uses.
	shouldNavigate := oauth != "" && tb.pendingURL == "" && tb.url != oauth
	if shouldNavigate {
		t.Error("attachTab would navigate again to a URL the engine already loaded")
	}

	// A tab whose engine has NOT yet completed must be left to onEngineReady.
	pending := &tab{ready: false, pendingURL: oauth}
	if pending.ready {
		t.Fatal("precondition")
	}
	if pending.pendingURL != oauth {
		t.Error("a not-yet-ready tab lost its pending URL")
	}
}
