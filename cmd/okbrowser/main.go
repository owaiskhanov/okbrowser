//go:build windows

// OK Browser - a very small, very fast web browser for Windows 10 and later.
//
// It renders with the system's Microsoft Edge WebView2 engine (the same
// Chromium engine that powers Edge), wrapped in a tiny native Win32 chrome:
// one toolbar, one address bar, no framework, no bloat.
package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/jchv/go-webview2/webviewloader"
)

// localFileOrURL returns a file:// URL when the argument is an existing local
// file, otherwise the argument unchanged.
func localFileOrURL(arg string) string {
	st, err := os.Stat(arg)
	if err != nil || st.IsDir() {
		return arg
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return arg
	}
	u := url.URL{
		Scheme: "file",
		Path:   "/" + strings.ReplaceAll(abs, "\\", "/"),
	}
	return u.String()
}

// selfTestMode is set before the app is built so that even the earliest
// code paths (engine startup inside NewApp) know never to show a modal
// dialog and always to log instead - a dialog would hang a scripted test.
var selfTestMode bool

func main() {
	// The WebView2 Runtime ships with Windows 11 and up-to-date Windows 10.
	// If it is missing we offer to open the official download page.
	if v, err := webviewloader.GetInstalledVersion(); err != nil || v == "" {
		showRuntimeMissingDialog()
		os.Exit(1)
	}

	// An optional command line argument is the URL to open
	// (used when spawning new windows for target=_blank links).
	// Dragging a file onto the exe opens that local file.
	startURL := ""
	selfTest := false
	for _, arg := range os.Args[1:] {
		if arg == "--selftest" {
			selfTest = true
		} else if arg != "" && !strings.HasPrefix(arg, "-") && startURL == "" {
			startURL = localFileOrURL(arg)
		}
	}
	if selfTest {
		startURL = "https://example.com"
		selfTestMode = true
		// The scripted test has no user gesture, so Chromium's popup
		// blocker would suppress window.open before the engine's
		// NewWindowRequested event fires. Real clicks are never blocked.
		_ = os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--disable-popup-blocking")
		selfTestFileInit("[selftest] starting (pid " + fmt.Sprint(os.Getpid()) + ")\n")
		// Panics on the main goroutine must reach the log file, not just
		// the (uncaptured) console of a scripted run.
		defer func() {
			if r := recover(); r != nil {
				selfTestFileInit(fmt.Sprintf("[selftest] PANIC: %v\n%s\n", r, debug.Stack()))
				os.Exit(2)
			}
		}()
	}

	app, ok := NewApp(startURL)
	if !ok {
		if selfTest {
			selfTestFileInit("[selftest] FAIL: startup failed (NewApp returned false)\n")
		}
		os.Exit(1)
	}
	if selfTest {
		app.startSelfTest()
	}
	app.Run()
}
