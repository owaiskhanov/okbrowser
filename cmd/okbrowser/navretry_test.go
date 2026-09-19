//go:build windows

package main

import (
	"testing"

	"github.com/owaiskhanov/okbrowser/internal/nav"
)

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
	if tb.altTried || tb.altPending {
		t.Fatal("a refused retry must not consume the one-shot budget")
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
