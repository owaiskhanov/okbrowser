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

// Address-bar suggestions run on the UI thread on every keystroke (after a
// 120ms debounce), so their cost is felt directly as typing lag. The original
// implementation lowercased both fields of all 2000 history entries per call
// and aggregated matches into a map[string]*struct, allocating ~2000 objects
// and ~628KB each time. The folded forms are cached on the entry now and
// matches aggregate into a slice, cutting a realistic query to ~40 allocs.
//
// These tests pin the behaviour that rewrite must preserve.

// TestSuggestFindsMatchesCaseInsensitively guards the cached lowercase fields:
// if fold() is ever missed on an insertion path, the entry silently stops
// matching and the address bar just fails to suggest it.
func TestSuggestFindsMatchesCaseInsensitively(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	s := newStore()

	s.AddHistory("https://GitHub.com/OwaisKhanov/okbrowser", "OK Browser Repository")

	for _, q := range []string{"github", "GITHUB", "GitHub", "owaiskhanov", "REPOSITORY", "ok browser"} {
		got := s.Suggest(q, 6)
		if len(got) == 0 {
			t.Errorf("query %q matched nothing; the lowercase match cache is stale", q)
			continue
		}
		if got[0].URL != "https://GitHub.com/OwaisKhanov/okbrowser" {
			t.Errorf("query %q returned %q", q, got[0].URL)
		}
	}

	if got := s.Suggest("definitely-not-present", 6); len(got) != 0 {
		t.Errorf("non-matching query returned %d suggestions", len(got))
	}
}

// TestSuggestIsDeterministic is a real bug fix, not just a speed guard. The
// old code built its results by ranging over a Go map, whose iteration order
// is randomised, so equally-ranked suggestions could appear in a different
// order on each keystroke - the dropdown visibly reshuffled while typing, and
// the entry under the cursor could change between press and click.
func TestSuggestIsDeterministic(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	s := newStore()

	// Several distinct URLs that all match, all with identical rank.
	for i := 0; i < 40; i++ {
		s.AddHistory("https://site"+string(rune('a'+i%26))+string(rune('a'+i/26))+".example.com/p", "Shared Title")
	}

	first := s.Suggest("example", 6)
	if len(first) < 2 {
		t.Fatalf("expected several matches, got %d", len(first))
	}
	for i := 0; i < 200; i++ {
		got := s.Suggest("example", 6)
		if len(got) != len(first) {
			t.Fatalf("result count changed between identical calls: %d then %d", len(first), len(got))
		}
		for j := range got {
			if got[j].URL != first[j].URL {
				t.Fatalf("suggestion order changed between identical calls:\n  first[%d]=%s\n  later[%d]=%s",
					j, first[j].URL, j, got[j].URL)
			}
		}
	}
}

// TestSuggestRanksFrequentPagesFirst pins the ranking the rewrite had to keep:
// a page visited more often outranks one visited once.
func TestSuggestRanksFrequentPagesFirst(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	s := newStore()

	s.AddHistory("https://rare.example.com/page", "Rare Page")
	for i := 0; i < 5; i++ {
		// Interleave so consecutive-duplicate suppression does not drop these.
		s.AddHistory("https://common.example.com/page", "Common Page")
		s.AddHistory("https://filler.example.com/page", "Filler")
	}

	got := s.Suggest("example", 6)
	if len(got) == 0 {
		t.Fatal("no suggestions")
	}
	if got[0].URL != "https://common.example.com/page" {
		t.Errorf("most-visited page did not rank first; got %q", got[0].URL)
	}
}
