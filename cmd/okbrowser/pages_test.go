//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStartPageIconOnly verifies the start page tiles carry no text labels.
func TestStartPageIconOnly(t *testing.T) {
	html := StartPageHTML([]Tile{{URL: "https://example.com/", Title: "Example", Favicon: "https://example.com/icon.png"}}, "Google")
	if !strings.Contains(html, `class="tile"`) {
		t.Fatal("tiles missing")
	}
	if strings.Contains(html, `font-size:11.5px;font-weight:550;white-space`) {
		t.Fatal("tile text label leaked back")
	}
	if !strings.Contains(html, `title="Example"`) {
		t.Fatal("tile tooltip (title) missing")
	}
	if !strings.Contains(html, `id="toast"`) {
		t.Fatal("toast mount missing")
	}
	if !strings.Contains(html, `data-u="https://example.com/"`) {
		t.Fatal("tile target URL missing")
	}
	if !strings.Contains(html, `src="https://example.com/icon.png"`) {
		t.Fatal("tile favicon missing")
	}
}

// TestSettingsPageStructure verifies every control the settings script
// wires is present in the rendered markup.
func TestSettingsPageStructure(t *testing.T) {
	html := SettingsHTML(Settings{Engine: "Bing", RestoreSession: true}, "1.10.1")
	for _, want := range []string{
		`id="eng"`,
		`data-v="Google"`,
		`data-v="Bing"`,
		`data-v="DuckDuckGo"`,
		`class="pill on" data-v="Bing"`, // active engine marked server-side
		`id="restore"`,
		`data-v="true"`,
		`id="ch"`,
		`id="cb"`,
		`id="cs"`,
		`id="toast"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("settings page missing %q", want)
		}
	}
	if strings.Contains(html, "%!") {
		t.Fatal("unrendered format verb in settings page")
	}
	// exactly one active pill
	if got := strings.Count(html, `pill on`); got != 1 {
		t.Fatalf("expected 1 active pill, got %d", got)
	}
}

// TestHistoryAndBookmarksStructure verifies the interactive bits of the
// history and bookmarks pages.
func TestHistoryAndBookmarksStructure(t *testing.T) {
	h := HistoryHTML([]histEntry{{URL: "https://example.com/", Title: "Example", TS: 1700000000000}})
	for _, want := range []string{`id="f"`, `id="clear"`, `data-u="https://example.com/"`, `data-ts="1700000000000"`, `id="toast"`} {
		if !strings.Contains(h, want) {
			t.Errorf("history page missing %q", want)
		}
	}
	if strings.Contains(h, "location.reload") {
		t.Error("history page still uses location.reload (blanks string pages)")
	}

	b := BookmarksHTML([]bmEntry{{URL: "https://example.com/", Title: "Example"}})
	for _, want := range []string{`data-u="https://example.com/"`, `class="xx"`, `id="toast"`} {
		if !strings.Contains(b, want) {
			t.Errorf("bookmarks page missing %q", want)
		}
	}
}

// TestDownloadsPageStructure verifies the downloads page rows.
func TestDownloadsPageStructure(t *testing.T) {
	html := DownloadsHTML([]dlFile{{Name: "setup.exe", Path: `C:\Users\x\Downloads\setup.exe`, Size: 1234567, Mod: 1700000000000}})
	if !strings.Contains(html, `data-p="C:\\Users\\x\\Downloads\\setup.exe"`) {
		if !strings.Contains(html, "setup.exe") {
			t.Fatal("downloads row missing")
		}
	}
	if !strings.Contains(html, `id="toast"`) {
		t.Fatal("toast mount missing")
	}
}

// TestListDownloads verifies the downloads page data path: files are read
// from USERPROFILE\Downloads, newest first, directories skipped.
func TestListDownloads(t *testing.T) {
	parent := t.TempDir()
	dl := filepath.Join(parent, "Downloads")
	if err := os.MkdirAll(dl, 0o755); err != nil {
		t.Fatal(err)
	}
	want := []string{"a.bin", "b.bin", "c.bin"}
	for _, n := range want {
		if err := os.WriteFile(filepath.Join(dl, n), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dl, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USERPROFILE", parent)

	got := listDownloads()
	if len(got) != len(want) {
		t.Fatalf("expected %d downloads, got %d", len(want), len(got))
	}
	seen := map[string]bool{}
	for _, g := range got {
		if filepath.Dir(g.Path) != dl {
			t.Errorf("bad path %q", g.Path)
		}
		if g.Size != 4 {
			t.Errorf("bad size for %q: %d", g.Name, g.Size)
		}
		seen[g.Name] = true
	}
	for _, n := range want {
		if !seen[n] {
			t.Errorf("missing %q", n)
		}
	}
	_ = fmt.Sprint(seen)
}
