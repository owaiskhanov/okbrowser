//go:build windows

package main

import (
	"html"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

// import.go imports bookmarks from other browsers. Chrome, Edge, Firefox,
// Brave, Opera and Safari all export the same "Netscape Bookmark File"
// (bookmarks.html), so a single parser covers every mainstream browser.
//
// The user picks the exported HTML file with the standard Windows Open dialog;
// we parse out the <A HREF="..."> anchors and merge them into the store,
// skipping duplicates.

// bookmarkAnchor matches one <A HREF="url" ...>title</A> entry in a Netscape
// bookmark export. HREF is required; ADD_DATE (unix seconds) is optional.
var bookmarkAnchor = regexp.MustCompile(`(?is)<a\s+[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)

// addDateAttr pulls the optional ADD_DATE="unixseconds" attribute from an
// anchor's attribute list.
var addDateAttr = regexp.MustCompile(`(?i)add_date="(\d+)"`)

// parseNetscapeBookmarks extracts bookmark entries from Netscape bookmark
// HTML. javascript: and place: (Firefox internal) URLs are skipped.
func parseNetscapeBookmarks(data []byte) []bmEntry {
	content := string(data)
	matches := bookmarkAnchor.FindAllStringSubmatch(content, -1)
	out := make([]bmEntry, 0, len(matches))
	for _, m := range matches {
		rawURL := html.UnescapeString(strings.TrimSpace(m[1]))
		if rawURL == "" {
			continue
		}
		low := strings.ToLower(rawURL)
		if strings.HasPrefix(low, "javascript:") || strings.HasPrefix(low, "place:") || strings.HasPrefix(low, "about:") {
			continue
		}
		// The title is the anchor's inner text with any nested tags stripped.
		title := html.UnescapeString(stripTags(m[2]))
		title = strings.TrimSpace(title)
		var ts int64
		if d := addDateAttr.FindStringSubmatch(m[0]); d != nil {
			if secs, err := strconv.ParseInt(d[1], 10, 64); err == nil && secs > 0 {
				ts = secs * 1000 // seconds -> milliseconds
			}
		}
		out = append(out, bmEntry{URL: rawURL, Title: title, TS: ts})
	}
	return out
}

// stripTags returns inner with any HTML tags removed (the inner text of an
// anchor may contain markup on some exports).
func stripTags(inner string) string {
	var b strings.Builder
	depth := 0
	for _, r := range inner {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// importBookmarksFlow shows a file picker, parses the chosen bookmark export,
// merges it into the store and reports the result with a toast.
func (a *app) importBookmarksFlow() {
	path := pickBookmarkFile(a.hwnd)
	if path == "" {
		return // user cancelled
	}
	data, err := os.ReadFile(path)
	if err != nil {
		a.updateToast("Could not read that file")
		return
	}
	entries := parseNetscapeBookmarks(data)
	if len(entries) == 0 {
		a.updateToast("No bookmarks found in that file")
		return
	}
	added := a.store.ImportBookmarks(entries)
	if added == 0 {
		a.updateToast("All bookmarks were already saved")
	} else if added == 1 {
		a.updateToast("Imported 1 bookmark")
	} else {
		a.updateToast("Imported " + strconv.Itoa(added) + " bookmarks")
	}
	// If the bookmarks page is showing, refresh it to reveal the imports.
	if t := a.active(); t != nil && t.url == "okbrowser://bookmarks" {
		a.showInternal(t, "bookmarks")
	}
}

// pickBookmarkFile shows the standard Windows Open dialog filtered to HTML
// bookmark exports and returns the chosen path ("" if cancelled).
func pickBookmarkFile(owner win.HWND) string {
	buf := make([]uint16, win.MAX_PATH)
	// Filter: "Bookmark files (*.html;*.htm)\0*.html;*.htm\0All files\0*.*\0\0"
	filter := utf16WithNuls("Bookmark files (*.html;*.htm)", "*.html;*.htm", "All files", "*.*")
	title, _ := syscall.UTF16PtrFromString("Import bookmarks from another browser")
	ofn := win.OPENFILENAME{
		LStructSize: uint32(unsafe.Sizeof(win.OPENFILENAME{})),
		HwndOwner:   owner,
		LpstrFilter: &filter[0],
		LpstrFile:   &buf[0],
		NMaxFile:    uint32(len(buf)),
		LpstrTitle:  title,
		Flags:       win.OFN_FILEMUSTEXIST | win.OFN_PATHMUSTEXIST | win.OFN_HIDEREADONLY,
	}
	if !win.GetOpenFileName(&ofn) {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// utf16WithNuls builds a double-NUL-terminated UTF-16 filter string from a
// flat list of pairs (label, pattern, label, pattern, ...).
func utf16WithNuls(parts ...string) []uint16 {
	var out []uint16
	for _, p := range parts {
		enc, _ := syscall.UTF16FromString(p)
		out = append(out, enc...) // UTF16FromString already appends a NUL
	}
	out = append(out, 0) // final terminating NUL
	return out
}
