// Package nav implements OK Browser's address-bar parsing and the built-in
// start page. It is pure Go with no platform dependencies so it can be unit
// tested on any operating system.
package nav

import (
	"net"
	"net/url"
	"strings"
)

// Engines maps the supported search engines to their query URL prefixes.
var Engines = map[string]string{
	"Google":     "https://www.google.com/search?q=",
	"Bing":       "https://www.bing.com/search?q=",
	"DuckDuckGo": "https://duckduckgo.com/?q=",
}

// Parse converts raw address-bar input into a URL to navigate to.
//
//	""                     -> ""            (caller shows the start page)
//	"example.com"          -> https://example.com
//	"example.com/x?y=1"    -> https://example.com/x?y=1
//	"http://example.com"   -> unchanged
//	"localhost:3000"       -> http://localhost:3000
//	"192.168.1.5:8080"     -> http://192.168.1.5:8080
//	"[::1]:8080"           -> http://[::1]:8080
//	"how to boil rice"     -> https://www.google.com/search?q=how+to+boil+rice
//	"giraffe"              -> https://www.google.com/search?q=giraffe
//
// The search provider is chosen with ParseWithEngine; Parse uses Google.
func Parse(input string) string { return ParseWithEngine(input, "Google") }

// ParseWithEngine converts raw address-bar input into a URL, searching
// with the named engine (see Engines; unknown names fall back to Google).
func ParseWithEngine(input, engine string) string {
	searchURL := Engines["Google"]
	if u, ok := Engines[engine]; ok {
		searchURL = u
	}
	return parse(input, searchURL)
}

// IsSearchURL reports whether u is a search-engine results page - such
// pages are excluded from history tiles and most-visited rankings.
func IsSearchURL(u string) bool {
	for _, prefix := range Engines {
		if strings.HasPrefix(u, prefix) {
			return true
		}
	}
	return false
}

func parse(input, searchURL string) string {
	s := strings.TrimSpace(input)
	s = strings.Trim(s, "\"'")
	if s == "" {
		return ""
	}

	// Already has a known scheme: use as-is.
	low := strings.ToLower(s)
	for _, prefix := range []string{"http://", "https://", "file://", "about:", "data:", "edge://", "chrome://"} {
		if strings.HasPrefix(low, prefix) {
			return s
		}
	}

	// Protocol-relative URLs.
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}

	// Anything with a space is treated as a search query.
	if strings.Contains(s, " ") {
		return searchURL + url.QueryEscape(s)
	}

	// Isolate the host part (strip path, query and fragment).
	host := s
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}

	// Local addresses get plain HTTP.
	if host == "localhost" || strings.HasPrefix(host, "localhost:") {
		return "http://" + s
	}
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh // "192.168.1.5:8080" -> "192.168.1.5"
	}
	if net.ParseIP(h) != nil || isIPv6Host(host) {
		return "http://" + s
	}

	// No dot in the host: not a domain, so search for it.
	if !strings.Contains(host, ".") {
		return searchURL + url.QueryEscape(s)
	}

	// Looks like a domain name: default to HTTPS.
	return "https://" + s
}

// AltHostURL returns the "www." alternate of rawURL - the address a real
// browser silently retries when the first attempt fails at the network
// level. It returns "" when no sensible alternate exists.
//
//	https://example.com/x      -> https://www.example.com/x
//	https://www.example.com/x  -> https://example.com/x
//
// Plenty of domains only publish a working server on one of the two names:
// the apex may have stale A records, refuse connections, or serve a
// certificate that is only valid for the "www" name. Browsers hide that
// misconfiguration by retrying the other name; WebView2 has no address bar
// of its own, so OK Browser has to do it.
//
// Scheme, credentials, port, path, query and fragment are all preserved.
// Hosts that cannot meaningfully gain or lose a "www" label - IP literals,
// localhost, single-label names and non-HTTP schemes - yield "".
func AltHostURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u == nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https": // only the web schemes have a www convention
	default:
		return ""
	}

	// Hostname() drops any port and the brackets around IPv6 literals.
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return ""
	}
	if net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return ""
	}

	var alt string
	if rest, ok := strings.CutPrefix(host, "www."); ok {
		// "www.example.com" -> "example.com", but never strip the label
		// down to a bare TLD such as "www.com" -> "com".
		if !strings.Contains(rest, ".") {
			return ""
		}
		alt = rest
	} else {
		alt = "www." + host
	}

	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(alt, port)
	} else {
		u.Host = alt
	}
	return u.String()
}

// isIPv6Host reports whether host is a bracketed IPv6 literal,
// optionally with a port, e.g. "[::1]" or "[fe80::1]:8080".
func isIPv6Host(host string) bool {
	if !strings.HasPrefix(host, "[") {
		return false
	}
	end := strings.LastIndex(host, "]")
	if end < 0 {
		return false
	}
	return net.ParseIP(host[1:end]) != nil
}
