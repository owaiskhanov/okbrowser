//go:build windows

package main

import "testing"

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"v1.12.2.84", "1.12.2.83", true},
		{"v1.12.2.83", "1.12.2.83", false},
		{"v1.12.1.999", "1.12.2.1", false},
		{"v2.0.0", "1.99.99.99", true},
	}
	for _, tc := range cases {
		if got := newerVersion(tc.candidate, tc.current); got != tc.want {
			t.Errorf("newerVersion(%q,%q)=%v want %v", tc.candidate, tc.current, got, tc.want)
		}
	}
}
