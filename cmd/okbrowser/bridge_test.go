//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsDownloadPathConfinement checks the gate that guards dl-open, dl-show
// and dl-remove. Every web page gets the bridge, so a page can post any path
// it likes; only files genuinely inside the user's Downloads folder may be
// opened, revealed or deleted.
func TestIsDownloadPathConfinement(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)

	downloads := filepath.Join(home, "Downloads")
	secrets := filepath.Join(home, "Secrets")
	for _, d := range []string{downloads, secrets} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	inside := filepath.Join(downloads, "setup.exe")
	if err := os.WriteFile(inside, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(secrets, "passwords.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	allow := []struct{ name, path string }{
		{"plain file", inside},
		{"not-yet-created download target", filepath.Join(downloads, "pending.crdownload")},
		{"nested subfolder", filepath.Join(downloads, "sub", "a.zip")},
	}
	for _, tc := range allow {
		if !isDownloadPath(tc.path) {
			t.Errorf("%s: %q should be allowed", tc.name, tc.path)
		}
	}

	deny := []struct{ name, path string }{
		{"empty", ""},
		{"the folder itself", downloads},
		{"sibling directory", outside},
		{"parent traversal", filepath.Join(downloads, "..", "Secrets", "passwords.txt")},
		{"prefix look-alike", home + string(filepath.Separator) + "DownloadsEvil" + string(filepath.Separator) + "x"},
		{"windows system file", `C:\Windows\System32\drivers\etc\hosts`},
	}
	for _, tc := range deny {
		if isDownloadPath(tc.path) {
			t.Errorf("%s: %q must be rejected", tc.name, tc.path)
		}
	}
}

// TestIsDownloadPathRejectsLinkEscape is the regression guard for the real
// hole: a symlink or NTFS junction placed inside Downloads used to pass the
// lexical filepath.Rel check while actually pointing somewhere else, so a
// page could delete or open arbitrary files by asking for a path "inside"
// Downloads that redirects out of it.
func TestIsDownloadPathRejectsLinkEscape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)

	downloads := filepath.Join(home, "Downloads")
	secrets := filepath.Join(home, "Secrets")
	for _, d := range []string{downloads, secrets} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(secrets, "passwords.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(downloads, "escape")
	if err := os.Symlink(secrets, link); err != nil {
		// Creating links needs privilege or Developer Mode on Windows.
		t.Skipf("cannot create symlink: %v", err)
	}

	escape := filepath.Join(link, "passwords.txt")
	if isDownloadPath(escape) {
		t.Errorf("path escaping Downloads through a link must be rejected: %q", escape)
	}
}
