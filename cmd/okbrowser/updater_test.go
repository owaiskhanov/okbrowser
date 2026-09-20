//go:build windows

package main

import "testing"

func TestNewerVersion(t *testing.T) {
	cases := []struct{ candidate, current string; want bool }{
		{"v1.12.2.84", "1.12.2.83", true},
		{"v1.12.2.83", "1.12.2.83", false},
		{"v1.12.1.999", "1.12.2.1", false},
		{"v2.0.0", "1.99.99.99", true},

		// A release whose base version is behind an already-published one
		// is invisible to the updater even though its build number is
		// higher. v1.12.2.111 shipped after 1.17.0.110 and stranded every
		// user on 1.17.x, so VERSION_BASE must never move backwards.
		{"v1.12.2.111", "1.17.0.110", false},
		{"v1.17.1.112", "1.17.0.110", true},
	}
	for _, tc := range cases {
		if got := newerVersion(tc.candidate, tc.current); got != tc.want {
			t.Errorf("newerVersion(%q,%q)=%v want %v", tc.candidate, tc.current, got, tc.want)
		}
	}
}
