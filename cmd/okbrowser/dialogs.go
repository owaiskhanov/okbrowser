//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/lxn/win"
)

// runtimeDownloadURL is Microsoft's official "Evergreen" WebView2 Runtime
// bootstrapper (a small free installer).
const runtimeDownloadURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

func mustUTF16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	return p
}

// showRuntimeMissingDialog explains the one system requirement and offers to
// open the official download page.
func showRuntimeMissingDialog() {
	const text = "OK Browser needs the free Microsoft Edge WebView2 Runtime,\n" +
		"which is not installed on this PC.\n\n" +
		"It is a small download from Microsoft and takes under a minute\n" +
		"to install. Would you like to open the download page now?"
	const caption = "OK Browser"

	// Keep this on one OS thread; harmless if already locked.
	runtime.LockOSThread()

	r := win.MessageBox(0, mustUTF16(text), mustUTF16(caption),
		win.MB_YESNO|win.MB_ICONERROR)
	if r == win.IDYES {
		openExternal(runtimeDownloadURL)
	}
}

// openExternal opens a URL in the system default browser.
func openExternal(url string) {
	win.ShellExecute(0, mustUTF16("open"), mustUTF16(url), nil, nil, win.SW_SHOWNORMAL)
}

// dataPath returns the per-user WebView2 profile directory (cookies, cache,
// history - what makes logins persist between sessions). Incognito windows
// use a throwaway folder instead.
func dataPath() string {
	if incognitoMode {
		return filepath.Join(os.TempDir(), fmt.Sprintf("OKBrowser-Incognito-%d", os.Getpid()))
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "OKBrowser", "WebView2")
}

// openPath opens a file with its default application.
func openPath(path string) {
	if path == "" {
		return
	}
	win.ShellExecute(0, mustUTF16("open"), mustUTF16(path), nil, nil, win.SW_SHOWNORMAL)
}

// showInFolder reveals a file in Windows Explorer.
func showInFolder(path string) {
	if path == "" {
		return
	}
	_ = exec.Command("explorer", "/select,"+path).Start()
}

// spawnIncognito starts a private OK Browser window: a separate process
// with a throwaway profile - nothing is written to disk history.
func spawnIncognito() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = exec.Command(exe, "--incognito").Start()
}

// spawnNewWindow starts a new OK Browser process, optionally opening the
// given URL. One window per process keeps each window feather-light.
func spawnNewWindow(url string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	var cmd *exec.Cmd
	if url == "" {
		cmd = exec.Command(exe)
	} else {
		cmd = exec.Command(exe, url)
	}
	_ = cmd.Start()
}
