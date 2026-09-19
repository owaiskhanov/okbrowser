//go:build windows

package main

import "testing"

func TestShouldBlock(t *testing.T) {
	blocked := []string{
		"https://doubleclick.net/ad",
		"https://ads.doubleclick.net/x.js",
		"https://a.b.doubleclick.net/pixel",
		"http://www.google-analytics.com/collect",
		"https://www.googletagmanager.com/gtm.js",
		"https://securepubads.g.doubleclick.net/tag/js/gpt.js",
		"https://cdn.taboola.com/libtrc/x.js",
		"https://s.amazon-adsystem.com/x",
		"https://connect.facebook.net/en_US/fbevents.js",
		"https://mc.yandex.ru/metrika/tag.js",
	}
	for _, u := range blocked {
		if !shouldBlock(u) {
			t.Errorf("shouldBlock(%q) = false, want true", u)
		}
	}
	allowed := []string{
		"https://example.com/page",
		"https://github.com/user/repo",
		"https://cdn.jsdelivr.net/npm/x.js",
		"https://notdoubleclick.net.evil.example.com/x", // suffix must be a full label
		"https://mygoogle-analytics.com.example.org/x",
		"https://news.ycombinator.com",
		"",
	}
	for _, u := range allowed {
		if shouldBlock(u) {
			t.Errorf("shouldBlock(%q) = true, want false", u)
		}
	}
}

func TestHostFromURL(t *testing.T) {
	cases := map[string]string{
		"https://Example.com/path?q=1":   "example.com",
		"http://user:pass@host.com:8080": "host.com",
		"https://sub.domain.co.uk/x#f":   "sub.domain.co.uk",
		"ftp://files.example.com":        "files.example.com",
		"host.only":                      "host.only",
	}
	for in, want := range cases {
		if got := hostFromURL(in); got != want {
			t.Errorf("hostFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}
