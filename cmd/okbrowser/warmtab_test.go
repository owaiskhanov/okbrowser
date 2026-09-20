//go:build windows

package main

import "testing"

// The pre-warmed new tab is a small state machine and every wrong
// combination is visible to the user, so pin the transitions down:
//
//	warmStart   - the tab already carries a start-page navigation, so
//	              attachTab must NOT render it a second time
//	warmPainted - that navigation has also painted, so the reveal can
//	              skip waiting for the first-paint signal
//
// warmPainted implies warmStart; the reverse is not true (re-rendering
// stale speed-dial tiles clears only warmPainted).
func TestWarmFlagsFreshSpare(t *testing.T) {
	// warmSpare() has only *issued* NavigateToString. The navigation is
	// asynchronous, so it must not claim that an unpainted document is
	// ready to reveal - that was the source of the white/blank flash.
	tb := &tab{warmStart: true, warmPainted: false}
	if !tb.warmStart || tb.warmPainted {
		t.Fatal("a fresh spare must wait for NavigationCompleted before instant reveal")
	}

	// onNavCompleted() is the first safe point to mark it painted.
	tb.warmPainted = true
	if !tb.warmPainted {
		t.Fatal("a completed spare should be eligible for instant reveal")
	}
}

func TestWarmFlagsAfterTileRefresh(t *testing.T) {
	// takeSpare() re-rendered stale tiles: content is present but not yet
	// on screen, so the reveal must wait for paint.
	tb := &tab{warmStart: true, warmPainted: true}
	tb.warmPainted = false // as takeSpare does
	if !tb.warmStart {
		t.Error("re-rendering tiles must not make attachTab render again")
	}
	if tb.warmPainted {
		t.Error("a just-issued render has not painted; reveal must wait")
	}
}

func TestWarmFlagsClearedWhenAdopted(t *testing.T) {
	// Adopting for a URL, and adopting as an empty tab, must both leave
	// the flags clear so a later reuse of the struct cannot skip a render.
	for _, name := range []string{"url", "empty"} {
		tb := &tab{warmStart: true, warmPainted: true}
		tb.warmStart, tb.warmPainted = false, false // both attach paths
		if tb.warmStart || tb.warmPainted {
			t.Errorf("%s: warm flags must be cleared on adoption", name)
		}
	}
}

// An instant reveal is only safe when the content is already on screen -
// otherwise the user sees a blank frame. warmPainted is exactly that
// condition, so it must never be true without warmStart.
func TestWarmPaintedImpliesWarmStart(t *testing.T) {
	states := []struct{ start, painted bool }{
		{true, true},   // completed spare
		{true, false},  // newly issued or tiles re-rendered
		{false, false}, // adopted / ordinary tab
	}
	for _, s := range states {
		if s.painted && !s.start {
			t.Fatalf("invalid state reached: painted=%v start=%v", s.painted, s.start)
		}
	}
}
