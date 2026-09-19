//go:build windows

package main

import "testing"

func TestParseNetscapeBookmarks(t *testing.T) {
	html := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
    <DT><H3 ADD_DATE="1600000000">Bookmarks bar</H3>
    <DL><p>
        <DT><A HREF="https://example.com/" ADD_DATE="1600000123">Example Site</A>
        <DT><A HREF="https://news.ycombinator.com/">Hacker News</A>
        <DT><A HREF="https://github.com/owaiskhanov/okbrowser" ADD_DATE="1650000000">OK &amp; Browser</A>
        <DT><A HREF="javascript:void(0)">A bookmarklet</A>
        <DT><A HREF="place:type=6&sort=14">Firefox internal</A>
    </DL><p>
</DL><p>`
	entries := parseNetscapeBookmarks([]byte(html))
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(entries), entries)
	}
	if entries[0].URL != "https://example.com/" || entries[0].Title != "Example Site" {
		t.Errorf("entry0 = %+v", entries[0])
	}
	if entries[0].TS != 1600000123*1000 {
		t.Errorf("entry0 timestamp = %d, want %d", entries[0].TS, int64(1600000123*1000))
	}
	if entries[1].Title != "Hacker News" || entries[1].TS != 0 {
		t.Errorf("entry1 = %+v", entries[1])
	}
	// HTML entity in the title must be decoded.
	if entries[2].Title != "OK & Browser" {
		t.Errorf("entry2 title = %q, want %q", entries[2].Title, "OK & Browser")
	}
}

func TestParseNetscapeBookmarksEmpty(t *testing.T) {
	if got := parseNetscapeBookmarks([]byte("<html><body>nothing here</body></html>")); len(got) != 0 {
		t.Errorf("expected no bookmarks, got %+v", got)
	}
}

func TestImportBookmarksDeduplicates(t *testing.T) {
	s := &store{
		dir:       t.TempDir(),
		favicons:  map[string]string{},
		dirty:     map[string]bool{},
		bookmarks: []bmEntry{{URL: "https://example.com/", Title: "Existing"}},
	}
	added := s.ImportBookmarks([]bmEntry{
		{URL: "https://example.com/", Title: "Dup"},   // already present
		{URL: "https://new.example.org/", Title: "New"},
		{URL: "https://new.example.org/", Title: "New again"}, // dup within batch
		{URL: "", Title: "empty"},                              // skipped
	})
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	if len(s.bookmarks) != 2 {
		t.Fatalf("bookmark count = %d, want 2", len(s.bookmarks))
	}
}
