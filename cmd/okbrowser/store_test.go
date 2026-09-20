//go:build windows

package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestClipNeverSplitsRune checks the helper that bounds page-supplied strings
// before they are written to disk. Truncating in the middle of a multi-byte
// rune would corrupt the JSON store, so clip must always cut on a boundary.
func TestClipNeverSplitsRune(t *testing.T) {
	if got := clip("hello", 10); got != "hello" {
		t.Errorf("short string altered: %q", got)
	}
	if got := clip("hello", 3); got != "hel" {
		t.Errorf("got %q, want %q", got, "hel")
	}
	for _, s := range []string{strings.Repeat("漢", 10), strings.Repeat("🙂", 5)} {
		for n := 0; n <= len(s); n++ {
			got := clip(s, n)
			if !utf8.ValidString(got) {
				t.Fatalf("clip(%q, %d) produced invalid UTF-8: %q", s, n, got)
			}
			if len(got) > n {
				t.Fatalf("clip(_, %d) returned %d bytes", n, len(got))
			}
		}
	}
}

// TestStoreBoundsPageSuppliedStrings verifies that a hostile page cannot grow
// the on-disk store without limit through the values it reports over the
// bridge (URL, title and favicon are all page-controlled).
func TestStoreBoundsPageSuppliedStrings(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	s := newStore()

	huge := "https://example.com/" + strings.Repeat("a", 64*1024)
	s.AddHistory(huge, strings.Repeat("t", 64*1024))
	hist := s.SnapshotHistory()
	if len(hist) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(hist))
	}
	if len(hist[0].URL) > maxStoredURL {
		t.Errorf("history URL not bounded: %d bytes", len(hist[0].URL))
	}
	if len(hist[0].Title) > maxStoredTitle {
		t.Errorf("history title not bounded: %d bytes", len(hist[0].Title))
	}

	// An oversized inline data: icon must be refused outright.
	s.SetFavicon("https://example.com/", "data:image/png;base64,"+strings.Repeat("A", 64*1024))
	if got := s.favicons["https://example.com"]; got != "" {
		t.Errorf("oversized favicon stored (%d bytes)", len(got))
	}
	s.SetFavicon("https://example.com/", "https://example.com/i.png")
	if s.favicons["https://example.com"] == "" {
		t.Error("a normal favicon should still be stored")
	}
}
