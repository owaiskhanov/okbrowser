//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/owaiskhanov/okbrowser/internal/nav"
)

// store is OK Browser's tiny local data vault: history, bookmarks,
// settings and the last session, each a small JSON file in
// %LOCALAPPDATA%\OKBrowser. Everything is kept in memory (reads are
// instant) and flushed to disk on a short debounce, plus on exit.
type store struct {
	mu        sync.Mutex
	dir       string
	history   []histEntry
	bookmarks []bmEntry
	settings  Settings
	permissions map[string]map[string]string // origin -> permission -> default/allow/deny
	favicons    map[string]string // origin -> last reported icon URL

	dirty      map[string]bool
	flushTimer *time.Timer
}

// histEntry is one visited page.
type histEntry struct {
	URL   string `json:"u"`
	Title string `json:"t"`
	TS    int64  `json:"ts"`
}

// Pages control the URL, title and icon they report through the bridge, and
// those strings are persisted. Cap them so a hostile or buggy page cannot
// grow the on-disk store without limit (a data: icon URL in particular can be
// megabytes). These bounds are far above anything a real site needs.
const (
	maxStoredURL     = 2048
	maxStoredTitle   = 512
	maxStoredIcon    = 2048
	maxStoredFavicons = 1000
	maxBookmarks     = 5000
)

// clip shortens s to at most n bytes without splitting a rune.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// bmEntry is one bookmark.
type bmEntry struct {
	URL   string `json:"u"`
	Title string `json:"t"`
	TS    int64  `json:"ts"`
}

// Settings are the user's choices (settings page / menu).
type Settings struct {
	Engine         string `json:"engine"`   // Google | Bing | DuckDuckGo
	RestoreSession bool   `json:"restore"`  // reopen tabs on startup
	Autofill       bool   `json:"autofill"` // WebView2 password/address autofill
	SleepMinutes   int    `json:"sleepMin"` // 0 disables sleeping tabs
	NeverSleep     map[string]bool `json:"neverSleep,omitempty"`
	LargeControls  bool `json:"largeControls"`
}

// sessionTab is one tab of a saved session.
type sessionTab struct {
	URL   string `json:"u"`
	Title string `json:"t"`
}

// sessionData is the full last-session snapshot, saved on exit and
// restored on the next start.
type sessionData struct {
	Tabs      []sessionTab `json:"tabs"`
	Active    int          `json:"a"`
	Maximized bool         `json:"max"`
	Rect      [4]int32     `json:"rect"` // normal position (workspace coords)
	Split     int          `json:"split"`
	SplitRatio float64     `json:"splitRatio"`
}

// suggestion is one address-bar suggestion row.
type suggestion struct {
	URL    string `json:"u"`
	Title  string `json:"t"`
	Source string `json:"s"` // "b" bookmark, "h" history
}

// engineList is the set of supported search engines.
var engineList = []string{"Google", "Bing", "DuckDuckGo"}

// validEngine reports whether e is a known search engine.
func validEngine(e string) bool {
	for _, n := range engineList {
		if n == e {
			return true
		}
	}
	return false
}

// newStore loads (or initializes) the data vault. It never fails hard:
// a missing or corrupt file just means defaults.
func newStore() *store {
	s := &store{
		dir: dataDir(),
		settings: Settings{
			Engine:         "Google",
			RestoreSession: true,
			Autofill:       true,
			SleepMinutes:   5,
		},
		permissions: make(map[string]map[string]string),
		favicons: make(map[string]string),
		dirty: map[string]bool{},
	}
	s.load("history.json", &s.history)
	s.load("bookmarks.json", &s.bookmarks)
	s.load("settings.json", &s.settings)
	s.load("permissions.json", &s.permissions)
	s.load("favicons.json", &s.favicons)
	if s.favicons == nil { s.favicons = make(map[string]string) }
	if s.permissions == nil { s.permissions = make(map[string]map[string]string) }
	if !validEngine(s.settings.Engine) { s.settings.Engine = "Google" }
	if s.settings.SleepMinutes < 0 || s.settings.SleepMinutes > 120 { s.settings.SleepMinutes = 5 }
	if s.settings.NeverSleep == nil { s.settings.NeverSleep = make(map[string]bool) }
	return s
}

// dataDir is %LOCALAPPDATA%\OKBrowser (same root as the WebView2 profile).
// Incognito windows keep their data in a throwaway temp folder instead.
func dataDir() string {
	if incognitoMode {
		return filepath.Join(os.TempDir(), fmt.Sprintf("OKBrowser-Incognito-%d", os.Getpid()))
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "OKBrowser")
}

func (s *store) load(name string, v interface{}) {
	p := filepath.Join(s.dir, name)
	b, err := os.ReadFile(p)
	if err == nil && json.Unmarshal(b, v) == nil { return }
	// A truncated/corrupt primary never destroys the user's last good data.
	if backup, e := os.ReadFile(p + ".bak"); e == nil { _ = json.Unmarshal(backup, v) }
}

// markDirty schedules a debounced flush (2s after the first change).
func (s *store) markDirty(name string) {
	s.dirty[name] = true
	if s.flushTimer == nil {
		s.flushTimer = time.AfterFunc(2*time.Second, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.flushLocked()
		})
	}
}

// Flush writes any pending changes now (also called on exit).
func (s *store) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushLocked()
}

// flushLocked writes the dirty files; caller holds s.mu.
func (s *store) flushLocked() {
	if s.flushTimer != nil {
		s.flushTimer.Stop()
		s.flushTimer = nil
	}
	for name := range s.dirty {
		var v interface{}
		switch name {
		case "history.json":
			v = s.history
		case "bookmarks.json":
			v = s.bookmarks
		case "settings.json":
			v = s.settings
		case "permissions.json":
			v = s.permissions
		case "favicons.json":
			v = s.favicons
		default:
			continue
		}
		s.write(name, v)
		delete(s.dirty, name)
	}
}

// write saves one file atomically (temp + rename).
// Caller holds s.mu (or is single-threaded startup).
func (s *store) write(name string, v interface{}) {
	_ = os.MkdirAll(s.dir, 0o755)
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	p := filepath.Join(s.dir, name)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil { return }
	if old, err := os.ReadFile(p); err == nil { _ = os.WriteFile(p+".bak", old, 0o644) }
	_ = os.Rename(tmp, p)
}

// ---- history ---------------------------------------------------------------

// AddHistory records a page visit. Search-result pages and consecutive
// duplicates are skipped; the log is capped at 2000 entries.
func (s *store) AddHistory(url, title string) {
	if url == "" || url == "about:blank" || strings.HasPrefix(url, "okbrowser://") {
		return
	}
	if nav.IsSearchURL(url) {
		return
	}
	// The page chooses these strings, so bound them before they are stored.
	url, title = clip(url, maxStoredURL), clip(title, maxStoredTitle)
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.history); n > 0 && s.history[n-1].URL == url {
		return
	}
	s.history = append(s.history, histEntry{URL: url, Title: title, TS: time.Now().UnixMilli()})
	if len(s.history) > 2000 {
		s.history = s.history[len(s.history)-2000:]
	}
	s.markDirty("history.json")
}

// SnapshotHistory returns a copy of the history, newest last.
func (s *store) SnapshotHistory() []histEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]histEntry, len(s.history))
	copy(out, s.history)
	return out
}

// RemoveHistory deletes the entry with the exact url and timestamp.
func (s *store) RemoveHistory(url string, ts int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.history {
		if e.URL == url && e.TS == ts {
			s.history = append(s.history[:i], s.history[i+1:]...)
			s.markDirty("history.json")
			return
		}
	}
}

// ClearHistory empties the history log.
func (s *store) ClearHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
	s.markDirty("history.json")
}

// ---- bookmarks ---------------------------------------------------------------

// IsBookmarked reports whether url is bookmarked.
func (s *store) IsBookmarked(url string) bool {
	if url == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.bookmarks {
		if b.URL == url {
			return true
		}
	}
	return false
}

// ToggleBookmark adds or removes a bookmark; returns true when added.
func (s *store) ToggleBookmark(url, title string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, b := range s.bookmarks {
		if b.URL == url {
			s.bookmarks = append(s.bookmarks[:i], s.bookmarks[i+1:]...)
			s.markDirty("bookmarks.json")
			return false
		}
	}
	if title == "" {
		title = url
	}
	if len(s.bookmarks) >= maxBookmarks {
		return false // refuse rather than grow without bound
	}
	s.bookmarks = append(s.bookmarks, bmEntry{URL: clip(url, maxStoredURL), Title: clip(title, maxStoredTitle), TS: time.Now().UnixMilli()})
	s.markDirty("bookmarks.json")
	return true
}

// RemoveBookmark deletes the bookmark for url.
func (s *store) RemoveBookmark(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, b := range s.bookmarks {
		if b.URL == url {
			s.bookmarks = append(s.bookmarks[:i], s.bookmarks[i+1:]...)
			s.markDirty("bookmarks.json")
			return
		}
	}
}

// SnapshotBookmarks returns a copy of the bookmarks.
func (s *store) SnapshotBookmarks() []bmEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]bmEntry, len(s.bookmarks))
	copy(out, s.bookmarks)
	return out
}

// ClearBookmarks empties the bookmark list.
func (s *store) ClearBookmarks() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bookmarks = nil
	s.markDirty("bookmarks.json")
}

func (s *store) SetFavicon(pageURL, icon string) {
	origin := permissionOrigin(pageURL)
	// A page may report a huge inline data: icon, so bound the value and the
	// number of origins we are willing to remember.
	if origin == "" || icon == "" || len(icon) > maxStoredIcon { return }
	s.mu.Lock(); defer s.mu.Unlock()
	if _, known := s.favicons[origin]; !known && len(s.favicons) >= maxStoredFavicons { return }
	if s.favicons[origin] != icon { s.favicons[origin] = icon; s.markDirty("favicons.json") }
}

// ---- start-page tiles + suggestions -----------------------------------------

// Tile is one most-visited start-page tile.
type Tile struct {
	URL     string
	Title   string
	Favicon string
}

// MostVisited aggregates history into the top n most-visited sites.
func (s *store) MostVisited(n int) []Tile {
	s.mu.Lock()
	defer s.mu.Unlock()
	type agg struct {
		title string
		count int64
		last  int64
	}
	m := map[string]*agg{}
	for _, e := range s.history {
		a := m[e.URL]
		if a == nil {
			a = &agg{title: e.Title}
			m[e.URL] = a
		}
		if e.Title != "" {
			a.title = e.Title
		}
		a.count++
		if e.TS > a.last {
			a.last = e.TS
		}
	}
	// Bookmarks always earn a place.
	for _, b := range s.bookmarks {
		if _, ok := m[b.URL]; !ok {
			m[b.URL] = &agg{title: b.Title, count: 1, last: b.TS}
		}
	}
	list := make([]*agg, 0, len(m))
	urls := make([]string, 0, len(m))
	for u, a := range m {
		list = append(list, a)
		urls = append(urls, u)
	}
	idx := make([]int, len(urls))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(x, y int) bool {
		ax, ay := list[idx[x]], list[idx[y]]
		if ax.count != ay.count {
			return ax.count > ay.count
		}
		return ax.last > ay.last
	})
	out := make([]Tile, 0, n)
	for _, i := range idx {
		if len(out) >= n {
			break
		}
		t := list[i].title
		if t == "" {
			t = urls[i]
		}
		out = append(out, Tile{URL: urls[i], Title: t, Favicon: s.favicons[permissionOrigin(urls[i])]})
	}
	return out
}

// Suggest returns up to n address suggestions for q: bookmarks first,
// then frequently/recently visited pages matching url or title.
func (s *store) Suggest(q string, n int) []suggestion {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" || n <= 0 {
		return nil
	}
	type cand struct {
		s    suggestion
		rank int64
	}
	var cands []cand
	s.mu.Lock()
	seen := map[string]bool{}
	for _, b := range s.bookmarks {
		if seen[b.URL] {
			continue
		}
		if strings.Contains(strings.ToLower(b.URL), q) || strings.Contains(strings.ToLower(b.Title), q) {
			seen[b.URL] = true
			t := b.Title
			if t == "" {
				t = b.URL
			}
			cands = append(cands, cand{suggestion{URL: b.URL, Title: t, Source: "b"}, 1_000_000 + b.TS/1_000_000})
		}
	}
	type hagg struct {
		title string
		count int64
		last  int64
	}
	m := map[string]*hagg{}
	for _, e := range s.history {
		if seen[e.URL] {
			continue
		}
		if !strings.Contains(strings.ToLower(e.URL), q) && !strings.Contains(strings.ToLower(e.Title), q) {
			continue
		}
		a := m[e.URL]
		if a == nil {
			a = &hagg{}
			m[e.URL] = a
		}
		if e.Title != "" {
			a.title = e.Title
		}
		a.count++
		if e.TS > a.last {
			a.last = e.TS
		}
	}
	for u, a := range m {
		t := a.title
		if t == "" {
			t = u
		}
		cands = append(cands, cand{suggestion{URL: u, Title: t, Source: "h"}, a.count*1000 + a.last/86_400_000})
	}
	s.mu.Unlock()
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].rank > cands[j].rank })
	out := make([]suggestion, 0, n)
	for _, c := range cands {
		if len(out) >= n {
			break
		}
		out = append(out, c.s)
	}
	return out
}

// ---- site permissions ------------------------------------------------------
func (s *store) Permission(origin, kind string) string {
	s.mu.Lock(); defer s.mu.Unlock()
	if byKind := s.permissions[origin]; byKind != nil { return byKind[kind] }
	return ""
}
func (s *store) SetPermission(origin, kind, state string) {
	if origin == "" { return }
	s.mu.Lock(); defer s.mu.Unlock()
	if s.permissions[origin] == nil { s.permissions[origin] = make(map[string]string) }
	if state == "default" || state == "" { delete(s.permissions[origin], kind) } else { s.permissions[origin][kind] = state }
	if len(s.permissions[origin]) == 0 { delete(s.permissions, origin) }
	s.markDirty("permissions.json")
}
func (s *store) ClearPermissions(origin string) {
	s.mu.Lock(); defer s.mu.Unlock(); delete(s.permissions, origin); s.markDirty("permissions.json")
}

// ---- settings + session -----------------------------------------------------

// Settings returns a copy of the current settings.
func (s *store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.settings
	out.NeverSleep = make(map[string]bool, len(s.settings.NeverSleep))
	for origin, value := range s.settings.NeverSleep { out.NeverSleep[origin] = value }
	return out
}

// SetSettings saves the settings.
func (s *store) SetSettings(v Settings) {
	if !validEngine(v.Engine) {
		v.Engine = "Google"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = v
	s.markDirty("settings.json")
}

// SaveSession persists the last session.
func (s *store) SaveSession(sd *sessionData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.write("session.json", sd)
}

// LoadSession returns the last session, or nil when there is none.
func (s *store) LoadSession() *sessionData {
	var sd sessionData
	s.load("session.json", &sd)
	if len(sd.Tabs) == 0 {
		return nil
	}
	return &sd
}

// ClearSession removes the saved session.
func (s *store) ClearSession() {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.Remove(filepath.Join(s.dir, "session.json"))
}
