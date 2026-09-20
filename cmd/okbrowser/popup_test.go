//go:build windows

package main

import (
	"testing"

	"github.com/lxn/win"
)

// Sign-in providers call window.open with an explicit width/height. Those
// numbers come straight from a web page, so they are untrusted: a page can
// ask for a 1x1 window, a 30000px one, or nothing at all.

func TestPopupSizeClampsHostilePageRequests(t *testing.T) {
	cases := []struct {
		name         string
		w, h         uint32
		hasSize      bool
		wantW, wantH int32
	}{
		{"typical Google consent window", 500, 600, true, 500, 600},
		{"no size requested falls back", 0, 0, false, 500, 640},
		{"zero size is treated as absent", 0, 0, true, 500, 640},
		{"a sliver is widened to the minimum", 1, 1, true, popupMinW, popupMinH},
		{"absurd size is capped", 30000, 30000, true, popupMaxW, popupMaxH},
		{"narrow but tall is clamped per axis", 10, 800, true, popupMinW, 800},
		{"wide but short is clamped per axis", 900, 10, true, 900, popupMinH},
	}
	for _, c := range cases {
		gotW, gotH := popupSize(c.w, c.h, c.hasSize)
		if gotW != c.wantW || gotH != c.wantH {
			t.Errorf("%s: popupSize(%d,%d,%v) = %dx%d, want %dx%d",
				c.name, c.w, c.h, c.hasSize, gotW, gotH, c.wantW, c.wantH)
		}
	}
}

// A popup with no requested position must be centred over the browser, the
// way Chrome does it - not pinned to the top-left of the screen.
func TestPopupCentresOverOwnerWhenNoPositionRequested(t *testing.T) {
	owner := win.RECT{Left: 100, Top: 100, Right: 1100, Bottom: 900} // 1000x800

	x, y := popupOrigin(owner, 0, 0, false, 500, 600)
	wantX := int32(100 + (1000-500)/2) // 350
	wantY := int32(100 + (800-600)/2)  // 200
	if x != wantX || y != wantY {
		t.Errorf("centred popup at (%d,%d), want (%d,%d)", x, y, wantX, wantY)
	}
}

// An explicitly requested position must be honoured.
func TestPopupHonoursRequestedPosition(t *testing.T) {
	owner := win.RECT{Left: 0, Top: 0, Right: 1920, Bottom: 1080}
	x, y := popupOrigin(owner, 640, 480, true, 500, 600)
	if x != 640 || y != 480 {
		t.Errorf("requested position ignored: got (%d,%d), want (640,480)", x, y)
	}
}
