//go:build windows

package main

import (
	"strings"
	"sync"
	"sync/atomic"
)

// adblock.go implements OK Browser's lightweight ad and tracker blocker.
//
// It is intentionally a compact, dependency-free domain blocklist rather than
// a full EasyList/uBlock rule engine: it matches the request's host against a
// curated set of well-known advertising, analytics and tracking domains
// (suffix match, so "ads.doubleclick.net" is caught by "doubleclick.net").
// This blocks the overwhelming majority of ad/tracker traffic with essentially
// zero per-request cost and no external rule downloads - in keeping with the
// browser's "very light, fully offline" philosophy.
//
// Blocked requests are answered with an empty HTTP 204 so the page's own
// scripts see a clean, fast "no content" instead of a hanging socket.

// blockedDomains is the curated suffix blocklist. Every entry matches the
// domain itself and any subdomain of it.
var blockedDomains = []string{
	// Google advertising / analytics
	"doubleclick.net",
	"googlesyndication.com",
	"googleadservices.com",
	"google-analytics.com",
	"googletagmanager.com",
	"googletagservices.com",
	"adservice.google.com",
	"pagead2.googlesyndication.com",
	"partner.googleadservices.com",
	"analytics.google.com",
	// Amazon ads
	"amazon-adsystem.com",
	"assoc-amazon.com",
	// Facebook / Meta trackers
	"connect.facebook.net",
	"an.facebook.com",
	// Common ad networks / exchanges
	"adnxs.com",
	"adsrvr.org",
	"rubiconproject.com",
	"pubmatic.com",
	"openx.net",
	"criteo.com",
	"criteo.net",
	"casalemedia.com",
	"contextweb.com",
	"3lift.com",
	"sharethrough.com",
	"gumgum.com",
	"smartadserver.com",
	"adform.net",
	"taboola.com",
	"outbrain.com",
	"revcontent.com",
	"media.net",
	"bidswitch.net",
	"yieldmo.com",
	"teads.tv",
	"adcolony.com",
	"applovin.com",
	"inmobi.com",
	"mopub.com",
	"unityads.unity3d.com",
	"moatads.com",
	"adroll.com",
	"quantserve.com",
	"quantcount.com",
	"zedo.com",
	"exponential.com",
	"advertising.com",
	"servedbyadbutler.com",
	"adzerk.net",
	"scorecardresearch.com",
	// Analytics / tracking / telemetry
	"hotjar.com",
	"mixpanel.com",
	"segment.com",
	"segment.io",
	"amplitude.com",
	"fullstory.com",
	"mouseflow.com",
	"crazyegg.com",
	"chartbeat.com",
	"chartbeat.net",
	"newrelic.com",
	"nr-data.net",
	"branch.io",
	"appsflyer.com",
	"adjust.com",
	"kochava.com",
	"bugsnag.com",
	"optimizely.com",
	"clarity.ms",
	"bat.bing.com",
	"heapanalytics.com",
	"matomo.cloud",
	"snowplowanalytics.com",
	"yandex.ru/metrika",
	"mc.yandex.ru",
	"stats.wp.com",
	"pixel.wp.com",
	"loggly.com",
	"sentry.io",
	"track.hubspot.com",
	"cdn.mxpnl.com",
	// Social / consent widgets frequently used purely for tracking
	"onesignal.com",
	"pushcrew.com",
	"cootlogix.com",
}

// blockSet is the blocklist indexed for O(1) exact-host and suffix lookups.
// Built once, lazily, and never mutated afterwards.
var (
	blockOnce sync.Once
	blockSet  map[string]struct{}
	// blockCount is the running total of requests blocked this session.
	// It is read from the UI thread and written from engine callbacks, so
	// it is accessed atomically.
	blockCount int64
)

func buildBlockSet() {
	blockSet = make(map[string]struct{}, len(blockedDomains))
	for _, d := range blockedDomains {
		blockSet[strings.ToLower(d)] = struct{}{}
	}
}

// hostFromURL extracts the lowercase host (no port) from a request URL
// without the cost of a full url.Parse - these run on every subresource.
func hostFromURL(raw string) string {
	s := raw
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	// strip path/query/fragment
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	// strip userinfo
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	// strip port (but keep IPv6 brackets intact enough for suffix match)
	if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s, "]") {
		s = s[:i]
	}
	return strings.ToLower(strings.TrimSuffix(s, "."))
}

// shouldBlock reports whether a request to rawURL is an ad/tracker request.
// It matches the host and every parent domain against the blocklist, so a
// request to "a.b.doubleclick.net" is blocked by the "doubleclick.net" entry.
func shouldBlock(rawURL string) bool {
	blockOnce.Do(buildBlockSet)
	// A couple of blocklist entries include a path prefix (e.g. Yandex
	// Metrika lives under a path on an otherwise-legitimate host).
	low := strings.ToLower(rawURL)
	if strings.Contains(low, "yandex.ru/metrika") {
		return true
	}
	host := hostFromURL(rawURL)
	if host == "" {
		return false
	}
	if _, ok := blockSet[host]; ok {
		return true
	}
	// Walk parent domains: a.b.c.com -> b.c.com -> c.com
	for i := 0; i < len(host); i++ {
		if host[i] == '.' {
			if _, ok := blockSet[host[i+1:]]; ok {
				return true
			}
		}
	}
	return false
}

// noteBlocked increments the session block counter.
func noteBlocked() { atomic.AddInt64(&blockCount, 1) }

// blockedTotal returns the number of requests blocked this session.
func blockedTotal() int64 { return atomic.LoadInt64(&blockCount) }
