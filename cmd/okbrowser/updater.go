//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/win"
)

const latestReleaseAPI = "https://api.github.com/repos/owaiskhanov/okbrowser/releases/latest"

type releaseInfo struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	fields := strings.Split(v, ".")
	out := make([]int, len(fields))
	for i, f := range fields {
		out[i], _ = strconv.Atoi(f)
	}
	return out
}
func newerVersion(candidate, current string) bool {
	a, b := versionParts(candidate), versionParts(current)
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av != bv {
			return av > bv
		}
	}
	return false
}

func (a *app) updateToast(text string) {
	b, _ := json.Marshal(text)
	a.execActive("window.__okToast&&window.__okToast(" + string(b) + ")")
}

func (a *app) checkForUpdates() {
	a.updateToast("Checking for updates…")
	go func() {
		client := &http.Client{Timeout: 20 * time.Second}
		req, _ := http.NewRequest("GET", latestReleaseAPI, nil)
		req.Header.Set("User-Agent", "OKBrowser/"+appVersion)
		resp, err := client.Do(req)
		if err != nil {
			a.postTask(func() { a.updateToast("Update check failed") })
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			a.postTask(func() { a.updateToast("Update service unavailable") })
			return
		}
		var rel releaseInfo
		if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&rel) != nil {
			a.postTask(func() { a.updateToast("Invalid update response") })
			return
		}
		if !newerVersion(rel.TagName, appVersion) {
			a.postTask(func() { a.updateToast("OK Browser is up to date") })
			return
		}
		a.postTask(func() { a.offerUpdate(rel) })
	}()
}

func (a *app) offerUpdate(rel releaseInfo) {
	text := fmt.Sprintf("OK Browser %s is available.\n\nThe download will be verified against its published SHA-256 checksum before installation.\n\nDownload and install now?", strings.TrimPrefix(rel.TagName, "v"))
	if win.MessageBox(a.hwnd, mustUTF16(text), mustUTF16("Verified update available"), win.MB_YESNO|win.MB_ICONINFORMATION) != win.IDYES {
		return
	}
	a.updateToast("Downloading verified update…")
	go a.downloadVerifiedUpdate(rel)
}

func (a *app) downloadVerifiedUpdate(rel releaseInfo) {
	var exeURL, sumURL string
	for _, asset := range rel.Assets {
		if asset.Name == "OKBrowser.exe" {
			exeURL = asset.BrowserDownloadURL
		}
		if asset.Name == "OKBrowser.exe.sha256" {
			sumURL = asset.BrowserDownloadURL
		}
	}
	if exeURL == "" || sumURL == "" {
		a.postTask(func() { a.updateToast("Release is missing verification files") })
		return
	}
	client := &http.Client{Timeout: 3 * time.Minute}
	fetch := func(raw string, max int64) ([]byte, error) {
		r, err := client.Get(raw)
		if err != nil {
			return nil, err
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return nil, fmt.Errorf("HTTP %d", r.StatusCode)
		}
		return io.ReadAll(io.LimitReader(r.Body, max))
	}
	sumText, err := fetch(sumURL, 4096)
	if err != nil {
		a.postTask(func() { a.updateToast("Could not download checksum") })
		return
	}
	want := strings.Fields(string(sumText))
	if len(want) == 0 || len(want[0]) != 64 {
		a.postTask(func() { a.updateToast("Invalid update checksum") })
		return
	}
	payload, err := fetch(exeURL, 32<<20)
	if err != nil || len(payload) < 2 || payload[0] != 'M' || payload[1] != 'Z' {
		a.postTask(func() { a.updateToast("Update download failed") })
		return
	}
	digest := sha256.Sum256(payload)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), want[0]) {
		a.postTask(func() { a.updateToast("Update blocked: checksum mismatch") })
		return
	}
	current, err := os.Executable()
	if err != nil {
		return
	}
	next := current + ".verified-update"
	if os.WriteFile(next, payload, 0700) != nil {
		a.postTask(func() { a.updateToast("Cannot write update beside this EXE") })
		return
	}
	script := filepath.Join(os.TempDir(), fmt.Sprintf("okbrowser-update-%d.cmd", os.Getpid()))
	body := fmt.Sprintf("@echo off\r\n:wait\r\ncopy /Y \"%s\" \"%s\" >nul 2>&1\r\nif errorlevel 1 (timeout /t 1 /nobreak >nul & goto wait)\r\nstart \"\" \"%s\"\r\ndel \"%s\"\r\ndel %%~f0\r\n", next, current, current, next)
	if os.WriteFile(script, []byte(body), 0600) != nil {
		return
	}
	a.postTask(func() {
		if win.MessageBox(a.hwnd, mustUTF16("The update was downloaded and its SHA-256 checksum is valid. Restart now?"), mustUTF16("Update verified"), win.MB_YESNO|win.MB_ICONINFORMATION) == win.IDYES {
			_ = exec.Command("cmd.exe", "/C", script).Start()
			win.DestroyWindow(a.hwnd)
		}
	})
}
