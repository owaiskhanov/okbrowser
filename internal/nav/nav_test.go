package nav

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Empty input -> start page.
		{"", ""},
		{"   ", ""},
		{" \"\" ", ""},

		// Schemes are preserved.
		{"http://example.com", "http://example.com"},
		{"https://example.com", "https://example.com"},
		{"HTTPS://Example.COM", "HTTPS://Example.COM"},
		{"file:///C:/Users/me/page.html", "file:///C:/Users/me/page.html"},
		{"about:blank", "about:blank"},

		// Bare domains gain https.
		{"example.com", "https://example.com"},
		{"example.com/", "https://example.com/"},
		{"example.com/page?x=1#frag", "https://example.com/page?x=1#frag"},
		{"sub.domain.example.co.uk", "https://sub.domain.example.co.uk"},
		{"example.com:8080/x", "https://example.com:8080/x"},
		{"münchen.de", "https://münchen.de"},

		// Local addresses gain http.
		{"localhost", "http://localhost"},
		{"localhost:3000", "http://localhost:3000"},
		{"localhost:3000/admin", "http://localhost:3000/admin"},
		{"127.0.0.1", "http://127.0.0.1"},
		{"192.168.1.5:8080", "http://192.168.1.5:8080"},
		{"[::1]:8080", "http://[::1]:8080"},

		// Protocol-relative.
		{"//example.com/x", "https://example.com/x"},

		// Search queries.
		{"how to boil rice", "https://www.google.com/search?q=how+to+boil+rice"},
		{"giraffe", "https://www.google.com/search?q=giraffe"},
		{"what is 2 + 2?", "https://www.google.com/search?q=what+is+2+%2B+2%3F"},
		{"c# generics", "https://www.google.com/search?q=c%23+generics"},
	}
	for _, c := range cases {
		if got := Parse(c.in); got != c.want {
			t.Errorf("Parse(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseIsDeterministicAndTrimmed(t *testing.T) {
	if got := Parse("  example.com  "); got != "https://example.com" {
		t.Errorf("trailing spaces not trimmed: %q", got)
	}
	if got := Parse("'example.com'"); got != "https://example.com" {
		t.Errorf("quotes not trimmed: %q", got)
	}
}


// TestAltHostURL covers the www <-> apex retry used when a navigation
// fails at the network level (the hthecofounder.com class of bug: the
// apex has A records that refuse connections while www works).
func TestAltHostURL(t *testing.T) {
	cases := []struct{ in, want string }{
		// Apex gains www.
		{"https://hthecofounder.com/", "https://www.hthecofounder.com/"},
		{"https://example.com", "https://www.example.com"},
		{"http://example.com/a/b?x=1#f", "http://www.example.com/a/b?x=1#f"},
		{"https://example.co.uk/x", "https://www.example.co.uk/x"},
		{"https://example.com:8443/x", "https://www.example.com:8443/x"},

		// www is dropped, so a www-only failure retries the apex.
		{"https://www.example.com/x", "https://example.com/x"},
		{"http://www.example.com:8080/", "http://example.com:8080/"},

		// Deeper subdomains are left alone apart from the www label.
		{"https://api.example.com/x", "https://www.api.example.com/x"},

		// No sensible alternate exists.
		{"https://localhost:3000/x", ""},
		{"http://127.0.0.1:8080/", ""},
		{"http://[::1]:8080/", ""},
		{"https://intranet/x", ""},
		{"https://www.com/", ""},
		{"file:///C:/tmp/page.html", ""},
		{"about:blank", ""},
		{"okbrowser://settings", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := AltHostURL(c.in); got != c.want {
			t.Errorf("AltHostURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestAltHostURLIsAnInvolution checks the retry cannot ping-pong: applying
// it twice must return the original URL, so one retry is always terminal.
func TestAltHostURLIsAnInvolution(t *testing.T) {
	for _, in := range []string{
		"https://hthecofounder.com/", "https://example.com/x?y=1",
		"https://www.example.com/x", "http://example.com:8080/",
	} {
		alt := AltHostURL(in)
		if alt == "" {
			t.Fatalf("AltHostURL(%q) unexpectedly empty", in)
		}
		if back := AltHostURL(alt); back != in {
			t.Errorf("AltHostURL(AltHostURL(%q)) = %q, want %q", in, back, in)
		}
	}
}

// TestAltHostURLPreservesCredentialsAndCase keeps the retry faithful to
// the original request.
func TestAltHostURLPreservesCredentialsAndCase(t *testing.T) {
	if got := AltHostURL("https://user:pw@example.com/x"); got != "https://user:pw@www.example.com/x" {
		t.Errorf("credentials lost: %q", got)
	}
	if got := AltHostURL("https://EXAMPLE.com/Path"); got != "https://www.example.com/Path" {
		t.Errorf("path case or host normalization wrong: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestParseWithEngine(t *testing.T) {
	if got := ParseWithEngine("giraffe", "Bing"); got != "https://www.bing.com/search?q=giraffe" {
		t.Errorf("ParseWithEngine(bing) = %q", got)
	}
	if got := ParseWithEngine("giraffe", "DuckDuckGo"); got != "https://duckduckgo.com/?q=giraffe" {
		t.Errorf("ParseWithEngine(ddg) = %q", got)
	}
	if got := ParseWithEngine("giraffe", "Nope"); got != "https://www.google.com/search?q=giraffe" {
		t.Errorf("ParseWithEngine(unknown) = %q", got)
	}
	if !IsSearchURL("https://www.google.com/search?q=x") || IsSearchURL("https://example.com/") {
		t.Error("IsSearchURL misclassifies")
	}
}
