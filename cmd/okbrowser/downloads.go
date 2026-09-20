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

type managedDownload struct {
	op                         *edge.ICoreWebView2DownloadOperation
	owner                      *tab
	path, source, mime         string
	received, total, lastBytes int64
	state, reason              uint32
	canResume                  bool
	speed                      int64
	lastPoll                   time.Time
	paused                     bool
}

func (a *app) onDownloadStarting(owner *tab, args *edge.ICoreWebView2DownloadStartingEventArgs) {
	op := args.GetDownloadOperation()
	if op == nil {
		return
	}
	op.AddRef()
	d := &managedDownload{op: op, owner: owner, path: args.GetResultFilePath(), source: op.GetURI(), mime: op.GetMimeType(), lastPoll: time.Now()}
	a.downloads = append(a.downloads, d)
	win.SetTimer(a.hwnd, 5, 500, 0)
}

func (a *app) pollDownloads() {
	active := false
	now := time.Now()
	for _, d := range a.downloads {
		if d.op == nil || d.state == 2 {
			continue
		}
		d.received, d.total = d.op.GetBytesReceived(), d.op.GetTotalBytesToReceive()
		d.state, d.reason, d.canResume = d.op.GetState(), d.op.GetInterruptReason(), d.op.GetCanResume()
		secs := now.Sub(d.lastPoll).Seconds()
		if secs > 0 {
			d.speed = int64(float64(d.received-d.lastBytes) / secs)
		}
		d.lastBytes, d.lastPoll = d.received, now
		if d.state == 0 {
			active = true
		}
		if d.state == 2 {
			d.op.Release()
			d.op = nil
		}
	}
	if !active {
		win.KillTimer(a.hwnd, 5)
	}
	if t := a.active(); t != nil && t.url == "okbrowser://downloads" {
		a.showInternal(t, "downloads")
	}
}

func (a *app) downloadFiles() []dlFile {
	files := listDownloads()
	byPath := make(map[string]int)
	for i := range files {
		byPath[strings.ToLower(files[i].Path)] = i
	}
	for _, d := range a.downloads {
		if d.path == "" {
			continue
		}
		f := dlFile{Name: filepath.Base(d.path), Path: d.path, Size: d.received, Mod: d.lastPoll.UnixMilli(), Source: d.source, Mime: d.mime, Speed: d.speed, Total: d.total, NativeState: d.state, Interrupt: d.reason, CanResume: d.canResume, Paused: d.paused, Partial: d.state == 0}
		if i, ok := byPath[strings.ToLower(d.path)]; ok {
			files[i] = f
		} else {
			files = append([]dlFile{f}, files...)
		}
	}
	return files
}

func (a *app) downloadAction(path, action string) bool {
	for _, d := range a.downloads {
		if !strings.EqualFold(d.path, path) || d.op == nil {
			continue
		}
		switch action {
		case "cancel":
			d.op.Cancel()
		case "pause":
			d.op.Pause()
			d.paused = true
		case "resume":
			d.op.Resume()
			d.paused = false
		}
		return true
	}
	return false
}

func sourceHost(raw string) string { u, _ := url.Parse(raw); return u.Hostname() }

func (a *app) hasActiveDownload(t *tab) bool {
	for _, d := range a.downloads {
		if d.owner == t && d.state == 0 {
			return true
		}
	}
	return false
}
