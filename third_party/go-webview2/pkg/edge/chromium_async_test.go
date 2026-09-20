//go:build windows

package edge

import (
	"sync/atomic"
	"testing"
)

// Opening a tab used to call Embed, which runs a NESTED MESSAGE LOOP that
// spins until WebView2 has created an environment and a controller. On the UI
// thread that blocks everything - the whole browser was frozen for the entire
// engine startup, which is what made every new tab feel slow.
//
// EmbedAsync replaces the wait with a completion callback delivered from the
// application's own message loop. These tests pin the behaviour the async
// path depends on.

// TestEmbedAsyncReportsFailureThroughCallback checks that a failed engine
// creation still resolves. If it silently never called back, a tab would hang
// forever showing nothing, with no error - worse than the original stall.
func TestEmbedAsyncReportsFailureThroughCallback(t *testing.T) {
	var e Chromium
	var got, calls int32
	e.initDone = func(ok bool) {
		atomic.AddInt32(&calls, 1)
		if ok {
			atomic.StoreInt32(&got, 1)
		}
	}

	// Drive the real completion handler with a failing HRESULT
	// (0x8007139F) exactly as WebView2 would.
	e.EnvironmentCompleted(uintptr(0x8007139F), nil)

	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("callback ran %d times, want exactly 1", calls)
	}
	if atomic.LoadInt32(&got) != 0 {
		t.Error("engine creation failed but the callback reported success")
	}
	if atomic.LoadUintptr(&e.initFailed) == 0 {
		t.Error("initFailed not set, so the blocking Embed path would spin forever")
	}
	if e.initDone != nil {
		t.Error("initDone still set: a later event could invoke a stale callback")
	}
}

// TestControllerFailureReportsThroughCallback covers the second failure point.
// Environment creation can succeed and controller creation still fail (a
// profile locked by a dying engine process is the common real case).
func TestControllerFailureReportsThroughCallback(t *testing.T) {
	var e Chromium
	var calls, okSeen int32
	e.initDone = func(ok bool) {
		atomic.AddInt32(&calls, 1)
		if ok {
			atomic.StoreInt32(&okSeen, 1)
		}
	}

	e.CreateCoreWebView2ControllerCompleted(uintptr(0x8007139F), nil)

	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("callback ran %d times, want exactly 1", calls)
	}
	if atomic.LoadInt32(&okSeen) != 0 {
		t.Error("controller creation failed but the callback reported success")
	}
}

// TestEngineCallsAreSafeBeforeReady is the core safety property of going
// async. The tab now exists before its engine does, so application code can
// reach these methods with no controller and no webview. Every one of them
// dereferenced a nil pointer before this change.
func TestEngineCallsAreSafeBeforeReady(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("engine call before readiness crashed the browser: %v", r)
		}
	}()

	var e Chromium // no controller, no webview: mid-startup
	e.Navigate("https://example.com/")
	e.NavigateToString("<h1>hi</h1>")
	e.Init("var x=1;")
	e.Eval("window.x=1;")
	_ = e.Show()
	_ = e.Hide()
	e.Resize()
	e.Focus()
	e.Reload()
	e.GoBack()
	e.GoForward()
	if e.CanGoBack() || e.CanGoForward() {
		t.Error("a tab with no engine claims navigation history")
	}
	if _, err := e.GetSettings(); err == nil {
		t.Error("GetSettings returned no error while the engine was absent")
	}
}
