//go:build windows

package main

import (
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/lxn/win"
)

// WebView2 download states (COREWEBVIEW2_DOWNLOAD_STATE).
const (
	dlInProgress  uint32 = 0
	dlInterrupted uint32 = 1
	dlCompleted   uint32 = 2
)

// maxTrackedDownloads caps the live list. Finished entries still render from
// the Downloads folder listing, so dropping the oldest costs nothing but
// stops a long session from growing this slice forever.
const maxTrackedDownloads = 200

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

// release drops our reference to the native operation exactly once.
func (d *managedDownload) release() {
	if d.op != nil {
		d.op.Release()
		d.op = nil
	}
}

func (a *app) onDownloadStarting(owner *tab, args *edge.ICoreWebView2DownloadStartingEventArgs) {
	if args == nil { return }
	op := args.GetDownloadOperation()
	if op == nil { return }

	// The event args own the operation; keep it alive for as long as we poll
	// it. Released in pollDownloads once the download reaches a final state.
	op.AddRef()

	// Read the path from the event args, falling back to the operation: the
	// args value is authoritative because the host may have redirected it.
	path := args.GetResultFilePath()
	if path == "" { path = op.GetResultFilePath() }

	d := &managedDownload{
		op: op,
		owner: owner,
		path: path,
		source: op.GetURI(),
		mime: op.GetMimeType(),
		total: op.GetTotalBytesToReceive(),
		lastPoll: time.Now(),
	}

	// Mark the tab so the navigation that spawned this download does not get
	// replaced by the error page (a download completes the navigation with
	// IsSuccess=FALSE).
	if owner != nil { owner.downloadAt = time.Now() }

	a.downloads = append(a.downloads, d)
	if n := len(a.downloads) - maxTrackedDownloads; n > 0 {
		for _, old := range a.downloads[:n] { old.release() }
		a.downloads = append([]*managedDownload(nil), a.downloads[n:]...)
	}
	win.SetTimer(a.hwnd, 5, 500, 0)
}

func (a *app) pollDownloads() {
	active := false
	now := time.Now()
	for _, d := range a.downloads {
		if d.op == nil { continue }
		d.received, d.total = d.op.GetBytesReceived(), d.op.GetTotalBytesToReceive()
		d.state, d.reason, d.canResume = d.op.GetState(), d.op.GetInterruptReason(), d.op.GetCanResume()
		if secs := now.Sub(d.lastPoll).Seconds(); secs > 0 {
			if delta := d.received - d.lastBytes; delta > 0 {
				d.speed = int64(float64(delta) / secs)
			} else {
				d.speed = 0
			}
		}
		d.lastBytes, d.lastPoll = d.received, now
		if d.state == dlInProgress && !d.paused { active = true }
		// Release the native object once it can no longer change. Interrupted
		// downloads stay referenced: they can still be resumed.
		if d.state == dlCompleted { d.release() }
	}
	if !active { win.KillTimer(a.hwnd, 5) }
	if t := a.active(); t != nil && t.url == "okbrowser://downloads" { a.showInternal(t, "downloads") }
}

func (a *app) downloadFiles() []dlFile {
	files := listDownloads()
	byPath := make(map[string]int)
	for i := range files { byPath[strings.ToLower(files[i].Path)] = i }
	for _, d := range a.downloads {
		if d.path == "" { continue }
		f := dlFile{Name:filepath.Base(d.path), Path:d.path, Size:d.received, Mod:d.lastPoll.UnixMilli(), Source:d.source, Mime:d.mime, Speed:d.speed, Total:d.total, NativeState:d.state, Interrupt:d.reason, CanResume:d.canResume, Paused:d.paused, Partial:d.state==dlInProgress}
		if i, ok := byPath[strings.ToLower(d.path)]; ok {
			// Keep the on-disk size/mtime once the file is finished.
			if d.state == dlCompleted { f.Size, f.Mod = files[i].Size, files[i].Mod }
			files[i] = f
		} else {
			files = append([]dlFile{f}, files...)
		}
	}
	return files
}

func (a *app) downloadAction(path, action string) bool {
	for _, d := range a.downloads {
		if !strings.EqualFold(d.path, path) || d.op == nil { continue }
		switch action {
		case "cancel":
			d.op.Cancel()
			d.paused = false
		case "pause":
			d.op.Pause()
			d.paused = true
		case "resume":
			d.op.Resume()
			d.paused = false
			// Resuming needs the progress timer running again.
			win.SetTimer(a.hwnd, 5, 500, 0)
		default:
			return false
		}
		return true
	}
	return false
}

func sourceHost(raw string) string { u, err := url.Parse(raw); if err != nil || u == nil { return "" }; return u.Hostname() }

// detachDownloads clears the owner of every download belonging to t, which is
// about to be destroyed. The transfers themselves continue.
func (a *app) detachDownloads(t *tab) {
	for _, d := range a.downloads {
		if d.owner == t { d.owner = nil }
	}
}

func (a *app) hasActiveDownload(t *tab) bool {
	for _, d := range a.downloads { if d.owner == t && d.state == dlInProgress { return true } }
	return false
}
