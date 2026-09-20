//go:build windows

package main

import (
	"testing"

	"github.com/lxn/win"
)

// The main window is a borderless WS_POPUP, so DefWindowProc does not hit-test
// its resize frame - the browser must do it. The system frame metrics alone
// give a 4-8px band, which is narrower than what Windows really offers on a
// normal window and left users pixel-hunting for the edges.


// A 1000x700 window at (100,100). Frame metrics deliberately small (4px),
// so the minimum band is what decides - this is the real-world case that
// made the edges hard to grab.
var wr = win.RECT{Left: 100, Top: 100, Right: 1100, Bottom: 800}

// resizeBand() already applied its 8px minimum before this is called.
func ht(x, y int32) uintptr { return frameHitTest(x, y, wr, 8, 8) }

func TestEveryEdgeAndCornerIsReachable(t *testing.T) {
	midX, midY := int32(600), int32(450)
	cases := []struct {
		name string
		x, y int32
		want uintptr
	}{
		{"left edge", 102, midY, win.HTLEFT},
		{"right edge", 1098, midY, win.HTRIGHT},
		{"top edge", midX, 102, win.HTTOP},
		{"bottom edge", midX, 798, win.HTBOTTOM},
		{"top-left corner", 102, 102, win.HTTOPLEFT},
		{"top-right corner", 1098, 102, win.HTTOPRIGHT},
		{"bottom-left corner", 102, 798, win.HTBOTTOMLEFT},
		{"bottom-right corner", 1098, 798, win.HTBOTTOMRIGHT},
		{"centre is client", midX, midY, 0},
	}
	for _, c := range cases {
		if got := ht(c.x, c.y); got != c.want {
			t.Errorf("%s at (%d,%d): got %d want %d", c.name, c.x, c.y, got, c.want)
		}
	}
}

// The band the hit-test uses must be exactly the band WM_NCCALCSIZE reserved
// as non-client. Anything wider is dead: those pixels are client area, and
// the tab-host child window covers them, so the parent is never asked.
func TestBandMatchesTheReservedNonClientArea(t *testing.T) {
	for y := int32(100); y < 108; y++ {
		if got := frameHitTest(600, y, wr, 8, 8); got != win.HTTOP {
			t.Fatalf("y=%d should be within the top resize band, got %d", y, got)
		}
	}
	if got := frameHitTest(600, 108, wr, 8, 8); got != 0 {
		t.Errorf("y=108 is past the 8px band and should be client area, got %d", got)
	}
}

// Corners must be easier to hit than edges, but must not swallow them.
func TestCornersAreWiderButDoNotEatTheEdges(t *testing.T) {
	// 12px in from the left is past the 8px EDGE band, so mid-height it is
	// client area - the edge band is deliberately not widened.
	if got := ht(112, 450); got != 0 {
		t.Errorf("(112,450) is past the edge band and should be client, got %d", got)
	}
	// The same 12px inset IS inside the 16px corner square when it is also
	// 12px from the top: corners are the wider target.
	if got := ht(112, 112); got != win.HTTOPLEFT {
		t.Errorf("(112,112) should be the top-left corner, got %d", got)
	}
	// Just outside the corner square on both axes but still in the edge band.
	if got := ht(104, 200); got != win.HTLEFT {
		t.Errorf("(104,200) should be win.HTLEFT, got %d", got)
	}
}

// Guard the regression the old code had: no edge may report client area.
func TestNoEdgePixelFallsThroughToClient(t *testing.T) {
	for x := wr.Left; x < wr.Right; x += 7 {
		if ht(x, wr.Top+1) == 0 {
			t.Fatalf("top border at x=%d fell through to client area", x)
		}
		if ht(x, wr.Bottom-1) == 0 {
			t.Fatalf("bottom border at x=%d fell through to client area", x)
		}
	}
	for y := wr.Top; y < wr.Bottom; y += 7 {
		if ht(wr.Left+1, y) == 0 {
			t.Fatalf("left border at y=%d fell through to client area", y)
		}
		if ht(wr.Right-1, y) == 0 {
			t.Fatalf("right border at y=%d fell through to client area", y)
		}
	}
}

// This is the invariant the first attempt at this fix got wrong.
//
// WM_NCCALCSIZE decides how much of the window is non-client; WM_NCHITTEST is
// only ever consulted for those pixels. Everything else is client area, and
// the tab-host child window covers all of it - a child swallows the mouse, so
// the parent's hit-test never runs there.
//
// Widening only the hit-test band therefore changes nothing: the extra pixels
// belong to the child. Both must come from resizeBand().
func TestHitTestBandCannotExceedReservedFrame(t *testing.T) {
	// Simulate what WM_NCCALCSIZE reserves for a given band, then assert the
	// hit-test finds a border at every reserved pixel and none beyond it.
	for _, band := range []int32{4, 8, 12, 16} {
		client := win.RECT{
			Left:   wr.Left + band,
			Top:    wr.Top, // top stays flush: the bar sits there
			Right:  wr.Right - band,
			Bottom: wr.Bottom - band,
		}

		// Every pixel OUTSIDE the client rect on the left must hit-test.
		for x := wr.Left; x < client.Left; x++ {
			if got := frameHitTest(x, 450, wr, band, band); got == 0 {
				t.Fatalf("band=%d: reserved pixel x=%d reported client area", band, x)
			}
		}
		// The first client pixel must NOT be claimed, or the band is wider
		// than what was reserved and those hits never arrive.
		if got := frameHitTest(client.Left, 450, wr, band, band); got != 0 {
			t.Errorf("band=%d: x=%d is client area but hit-test claimed %d (band wider than reserved)",
				band, client.Left, got)
		}
		// Same on the bottom edge.
		for y := client.Bottom; y < wr.Bottom; y++ {
			if got := frameHitTest(600, y, wr, band, band); got == 0 {
				t.Fatalf("band=%d: reserved pixel y=%d reported client area", band, y)
			}
		}
		if got := frameHitTest(600, client.Bottom-1, wr, band, band); got != 0 {
			t.Errorf("band=%d: y=%d is client area but hit-test claimed %d",
				band, client.Bottom-1, got)
		}
	}
}
