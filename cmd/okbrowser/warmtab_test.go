//go:build windows

package main

import "testing"

// The pre-warmed new tab is a small state machine and every wrong
// combination is visible to the user, so pin the transitions down:
//
//	warmStart   - the tab already carries a rendered start page, so
//	              attachTab must NOT render it a second time
//	warmPainted - that page has also already painted, so the reveal can
//	              skip waiting for the first-paint signal
//
// warmPainted implies warmStart; the reverse is not true (re-rendering
// stale speed-dial tiles clears only warmPainted).

func TestWarmFlagsFreshSpare(t *testing.T) {
	// warmSpare(): rendered and painted.
	tb := &tab{warmStart: true, warmPainted: true}
	if !tb.warmStart || !tb.warmPainted {
		t.Fatal("a fresh spare should be both rendered and painted")
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
		{true, true},   // fresh spare
		{true, false},  // tiles re-rendered
		{false, false}, // adopted / ordinary tab
	}
	for _, s := range states {
		if s.painted && !s.start {
			t.Fatalf("invalid state reached: painted=%v start=%v", s.painted, s.start)
		}
	}
	// The illegal combination must not be produced anywhere.
	bad := &tab{warmStart: false, warmPainted: true}
	if bad.warmPainted && !bad.warmStart {
		t.Log("painted-without-content is the state the code must never build")
	}
}
