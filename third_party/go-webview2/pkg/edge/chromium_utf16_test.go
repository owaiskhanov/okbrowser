//go:build windows

package edge

import "testing"

// TestUTF16PtrRejectsNUL is the regression guard for a browser-wide crash.
//
// Navigate, NavigateToString, Init and Eval used to convert their argument
// with windows.StringToUTF16Ptr, which PANICS on an embedded NUL, and Eval
// used log.Fatal. All four carry attacker-controlled text: a page can post
// any URL through the bridge and can set any document.title, so
//
//	window.__ok({t:"go", u:"https://example.com/\u0000"})
//
// was enough for any website to terminate the whole browser. The conversion
// now fails closed and the call is skipped.
func TestUTF16PtrRejectsNUL(t *testing.T) {
	bad := []string{
		"\x00",
		"https://example.com/\x00evil",
		"trailing\x00",
		"window.__okBar(\"a\x00b\")",
	}
	for _, s := range bad {
		if _, ok := utf16Ptr(s); ok {
			t.Errorf("utf16Ptr(%q) accepted a string containing NUL", s)
		}
	}

	good := []string{
		"",
		"https://example.com/",
		"window.__okBar({\"t\":\"x\"})",
		"unicode: \u2028\u2029 ünïcødé 漢字",
	}
	for _, s := range good {
		if _, ok := utf16Ptr(s); !ok {
			t.Errorf("utf16Ptr(%q) rejected a valid string", s)
		}
	}
}

// TestNavigateSurvivesNUL calls the real entry points with a NUL-bearing
// string. Before the fix these panicked; now they must return quietly.
// A nil *Chromium receiver is fine because the guard returns before any
// field is touched.
func TestNavigateSurvivesNUL(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a NUL in page-controlled text panicked the browser: %v", r)
		}
	}()
	var e Chromium
	e.Navigate("https://example.com/\x00")
	e.NavigateToString("<h1>\x00</h1>")
	e.Init("var x=1;\x00")
	e.Eval("window.x=1;\x00")
}
