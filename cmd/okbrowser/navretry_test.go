//go:build windows

package main

import (
	"testing"

	"github.com/owaiskhanov/okbrowser/internal/nav"
)

// TestInternalNavigateToStringBoundary prevents a subtle but destructive
// navigation race: WebView2 represents NavigateToString as a base64 data URL.
// If a user follows a link before it finishes, its canceled completion is
// ConnectionAborted. That completion belongs to browser chrome, never to the
// newly clicked link, and must be consumed independently.
func TestInternalNavigateToStringBoundary(t *testing.T) {
	if !isNavigateToStringURI("DATA:text/html;charset=utf-8;base64,PGgxPk9LPC9oMT4=") {
		t.Fatal("NavigateToString data URI not recognized")
	}
	for _, raw := range []string{"https://example.com/", "data:text/plain,hello", "okbrowser://start"} {
		if isNavigateToStringURI(raw) {
			t.Errorf("%q must not be treated as an internal HTML navigation", raw)
		}
	}

	tb := &tab{internalNavs: 1}
	if !tb.preserveInternalDocumentState("data:text/html;base64,PGgxPk9LPC9oMT4=") {
		t.Fatal("a pending New Tab document must not replace the clicked URL")
	}
	tb.markInternalNavigation(41)
	// The freshly clicked link is navigation 42. Its completion must not
	// consume New Tab's marker merely because it completed first.
	tb.activeNavID, tb.activeNavKnown = 42, true
	if tb.completeInternalNavigation(42, true) {
		t.Fatal("a real completion must not consume an internal-page marker")
	}
	if tb.isStaleNavigation(41, true) == false {
		t.Fatal("the older internal completion must be identified as stale")
	}
	if !tb.completeInternalNavigation(41, true) || tb.internalNavs != 0 {
		t.Fatal("the matching internal completion must be consumed exactly once")
	}
	if tb.completeInternalNavigation(42, true) {
		t.Fatal("a real completion must not be consumed after the internal marker is gone")
	}

	// Error pages intentionally have no pending marker: their generated
	// document still must preserve the failed target URL and error state.
	tb.errPage = true
	if !tb.preserveInternalDocumentState("data:text/html;charset=utf-8;base64,PGgxPkVycm9yPC9oMT4=") {
		t.Fatal("an error page must never expose its generated data URI")
	}
}

// TestHostErrorIsRetryable pins down which navigation failures earn an
// apex/www retry. Host-level failures do; page-level ones must not, so a
// genuinely broken page still reaches the error page immediately.
func TestHostErrorIsRetryable(t *testing.T) {
	retry := map[uint32]string{
		1:  "CertificateCommonNameIsIncorrect",
		2:  "CertificateExpired",
		4:  "CertificateRevoked",
		5:  "CertificateIsInvalid",
		6:  "ServerUnreachable",
		7:  "Timeout",
		9:  "ConnectionAborted",
		10: "ConnectionReset",
		12: "CannotConnect",
		13: "HostNameNotResolved",
	}
	for code, name := range retry {
		if !hostErrorIsRetryable(code) {
			t.Errorf("code %d (%s) should be retryable", code, name)
		}
	}

	noRetry := map[uint32]string{
		0:  "Unknown/success",
		3:  "ClientCertificateContainsErrors",
		8:  "ErrorHTTPInvalidServerResponse",
		11: "Disconnected",
		14: "OperationCanceled",
		15: "RedirectFailed",
		16: "UnexpectedError",
	}
	for code, name := range noRetry {
		if hostErrorIsRetryable(code) {
			t.Errorf("code %d (%s) must not be retryable", code, name)
		}
	}
}

// TestAltRetryTarget exercises the full retry policy without needing a
// live web engine: the reported bug retries, and every guard holds.
func TestAltRetryTarget(t *testing.T) {
	// The exact reported case: typing the bare domain fails to connect,
	// so the www name is tried.
	tb := &tab{url: "https://hthecofounder.com/"}
	if got := altRetryTarget(tb, 12); got != "https://www.hthecofounder.com/" {
		t.Errorf("apex retry = %q, want the www alternate", got)
	}

	// One shot only: once armed, the same chain never retries again, so
	// a site that is down on both names cannot loop.
	spent := &tab{url: "https://hthecofounder.com/", altTried: true}
	if got := altRetryTarget(spent, 12); got != "" {
		t.Errorf("second retry = %q, want none", got)
	}

	// Page-level failures go straight to the error page.
	if got := altRetryTarget(&tab{url: "https://example.com/"}, 16); got != "" {
		t.Errorf("non-host error retried with %q", got)
	}

	// A user-cancelled navigation is never retried.
	if got := altRetryTarget(&tab{url: "https://example.com/"}, 14); got != "" {
		t.Errorf("cancelled navigation retried with %q", got)
	}

	// Local development addresses are never rewritten.
	if got := altRetryTarget(&tab{url: "http://localhost:3000/app"}, 12); got != "" {
		t.Errorf("localhost retried with %q", got)
	}

	// A nil tab must not panic.
	if got := altRetryTarget(nil, 12); got != "" {
		t.Errorf("nil tab returned %q", got)
	}
}

// TestRetryAltHostNeedsAnEngine makes sure a tab with no engine is refused
// without consuming its one-shot retry budget or panicking.
func TestRetryAltHostNeedsAnEngine(t *testing.T) {
	a := &app{}
	tb := &tab{url: "https://hthecofounder.com/"}
	if a.retryAltHost(tb, 12) {
		t.Fatal("retry must not fire for a tab with no engine")
	}
	if tb.altTried {
		t.Fatal("a refused retry must not consume the one-shot budget")
	}
}

// TestRetryTerminatesAcrossRedirects is a regression test for an infinite
// navigation loop.
//
// An apex that 301s to www is extremely common. WebView2 raises a fresh
// NavigationStarting for every redirect hop, so re-arming the retry budget
// when a navigation *starts* let this happen forever:
//
//	apex -> (301) www -> fails -> retry apex -> (301) www -> fails -> ...
//
// The budget is therefore cleared only when a page actually loads or the
// user navigates somewhere new. This test drives that exact sequence and
// asserts the chain terminates after a single retry.
func TestRetryTerminatesAcrossRedirects(t *testing.T) {
	tb := &tab{url: "https://example.com/"} // user typed the apex

	// Hop 1: the apex redirects to www. A redirect must NOT refill the
	// budget, so simulate the hop as the engine does - URL changes only.
	tb.url = "https://www.example.com/"

	// The redirect target fails to connect: one retry is allowed.
	first := altRetryTarget(tb, 12)
	if first != "https://example.com/" {
		t.Fatalf("first retry = %q, want the apex", first)
	}
	tb.altTried = true // as retryAltHost does
	tb.url = first

	// The retry lands on the apex, which redirects to www again...
	tb.url = "https://www.example.com/"
	// ...and fails again. This MUST now stop rather than loop.
	if again := altRetryTarget(tb, 12); again != "" {
		t.Fatalf("redirect hop refilled the retry budget (got %q) - infinite loop", again)
	}
}

// TestRetryBudgetRefillsOnlyOnRealBoundaries documents when a tab is
// allowed to retry again: after a successful load, or a new user
// navigation - never merely because another navigation started.
func TestRetryBudgetRefillsOnlyOnRealBoundaries(t *testing.T) {
	tb := &tab{url: "https://example.com/", altTried: true}
	if altRetryTarget(tb, 12) != "" {
		t.Fatal("a spent budget must stay spent")
	}
	// onNavCompleted clears it on a successful load; navigateTab clears
	// it for a new user-initiated address.
	tb.altTried = false
	if altRetryTarget(tb, 12) == "" {
		t.Fatal("a refilled budget should allow one retry again")
	}
}

// TestRetryAltHostSkipsHostsWithoutAlternates makes sure local development
// addresses never get rewritten.
func TestRetryAltHostSkipsHostsWithoutAlternates(t *testing.T) {
	for _, u := range []string{
		"http://localhost:3000/app",
		"http://127.0.0.1:8080/",
		"okbrowser://settings",
	} {
		if got := nav.AltHostURL(u); got != "" {
			t.Errorf("AltHostURL(%q) = %q, want no alternate", u, got)
		}
	}
}
