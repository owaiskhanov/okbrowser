//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/lxn/win"
)

type managedDownload struct {
	op *edge.ICoreWebView2DownloadOperation
	owner *tab
	path, source, mime string
	received, total, lastBytes int64
	state, reason uint32
	canResume bool
	speed int64
	lastPoll time.Time
	paused bool
}

func (a *app) onDownloadStarting(owner *tab, args *edge.ICoreWebView2DownloadStartingEventArgs) {
	op := args.GetDownloadOperation()
	if op == nil { return }
	op.AddRef()
	d := &managedDownload{op:op, owner:owner, path:args.GetResultFilePath(), source:op.GetURI(), mime:op.GetMimeType(), lastPoll:time.Now()}
	a.downloads = append(a.downloads, d)
	win.SetTimer(a.hwnd, 5, 500, 0)

	// This callback originates inside WebView2. Build the one new row back in
	// the window-proc context, but only once for the new download - never on
	// every progress tick (which used to re-navigate the entire page twice a
	// second and visibly reset its scroll/fade).
	a.postTask(func() {
		if t := a.active(); t != nil && t.url == "okbrowser://downloads" {
			a.showInternal(t, "downloads")
		}
	})
}

// downloadProgress is the tiny live payload sent to an already-open downloads
// page. It deliberately contains only text that is changing (bytes, speed,
// percentage); the page and its event handlers stay mounted.
type downloadProgress struct {
	Path   string `json:"p"`
	Status string `json:"s"`
}

func downloadProgressText(d *managedDownload) string {
	if d.paused {
		return "Paused · " + humanSize(d.received)
	}
	if d.state == 1 {
		text := "Interrupted"
		if d.canResume { text += " · can resume" }
		return text
	}
	text := "Downloading"
	if d.total > 0 {
		text += fmt.Sprintf(" · %.0f%%", 100*float64(d.received)/float64(d.total))
	}
	text += " · " + humanSize(d.received)
	if d.speed > 0 { text += " · " + humanSize(d.speed) + "/s" }
	if host := sourceHost(d.source); host != "" { text += " · " + host }
	return text
}

// pushDownloadProgress updates an already-rendered Downloads page without
// navigating it. Re-navigation discarded scroll position and made the list
// flicker during every active download.
func (a *app) pushDownloadProgress(t *tab) {
	if t == nil || t.chromium == nil || t.url != "okbrowser://downloads" {
		return
	}
	items := make([]downloadProgress, 0, len(a.downloads))
	for _, d := range a.downloads {
		if d.op == nil || d.state == 2 || d.path == "" { continue }
		items = append(items, downloadProgress{Path: d.path, Status: downloadProgressText(d)})
	}
	b, err := json.Marshal(items)
	if err == nil {
		t.chromium.Eval("window.__okDownloadProgress&&window.__okDownloadProgress(" + string(b) + ")")
	}
}

func (a *app) pollDownloads() {
	active, needsPageRefresh := false, false
	now := time.Now()
	for _, d := range a.downloads {
		if d.op == nil || d.state == 2 { continue }
		previousState := d.state
		d.received, d.total = d.op.GetBytesReceived(), d.op.GetTotalBytesToReceive()
		d.state, d.reason, d.canResume = d.op.GetState(), d.op.GetInterruptReason(), d.op.GetCanResume()
		secs := now.Sub(d.lastPoll).Seconds()
		if secs > 0 { d.speed = int64(float64(d.received-d.lastBytes)/secs) }
		d.lastBytes, d.lastPoll = d.received, now
		if d.state == 0 { active = true }
		if d.state != previousState && d.state != 0 {
			// Completion/interruption changes the page's controls, so refresh
			// once. Byte/speed changes stay on the zero-flicker path above.
			needsPageRefresh = true
		}
		if d.state == 2 {
			d.op.Release(); d.op = nil
		}
	}
	if !active { win.KillTimer(a.hwnd, 5) }
	if t := a.active(); t != nil && t.url == "okbrowser://downloads" {
		if needsPageRefresh {
			a.showInternal(t, "downloads")
		} else {
			a.pushDownloadProgress(t)
		}
	}
}

func (a *app) downloadFiles() []dlFile {
	files := listDownloads()
	byPath := make(map[string]int)
	for i := range files { byPath[strings.ToLower(files[i].Path)] = i }
	for _, d := range a.downloads {
		if d.path == "" { continue }
		f := dlFile{Name:filepath.Base(d.path), Path:d.path, Size:d.received, Mod:d.lastPoll.UnixMilli(), Source:d.source, Mime:d.mime, Speed:d.speed, Total:d.total, NativeState:d.state, Interrupt:d.reason, CanResume:d.canResume, Paused:d.paused, Partial:d.state==0}
		if i, ok := byPath[strings.ToLower(d.path)]; ok { files[i] = f } else { files = append([]dlFile{f}, files...) }
	}
	return files
}

func (a *app) downloadAction(path, action string) bool {
	for _, d := range a.downloads {
		if !strings.EqualFold(d.path, path) || d.op == nil { continue }
		switch action {
		case "cancel":
			d.op.Cancel()
		case "pause":
			d.op.Pause()
			d.paused = true
		case "resume":
			d.op.Resume()
			d.paused = false
			// An interrupted download stops the poll timer. Restart it so a
			// resumed transfer immediately receives the same smooth in-place
			// progress updates as a newly started one.
			win.SetTimer(a.hwnd, 5, 500, 0)
		}
		return true
	}
	return false
}

func sourceHost(raw string) string { u, _ := url.Parse(raw); return u.Hostname() }

func (a *app) hasActiveDownload(t *tab) bool {
	for _, d := range a.downloads { if d.owner == t && d.state == 0 { return true } }
	return false
}
