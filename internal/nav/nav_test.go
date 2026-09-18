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
		{"how to boil rice", "https://duckduckgo.com/?q=how+to+boil+rice"},
		{"giraffe", "https://duckduckgo.com/?q=giraffe"},
		{"what is 2 + 2?", "https://duckduckgo.com/?q=what+is+2+%2B+2%3F"},
		{"c# generics", "https://duckduckgo.com/?q=c%23+generics"},
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

func TestStartHTML(t *testing.T) {
	if len(StartHTML) == 0 {
		t.Fatal("StartHTML is empty")
	}
	// Sanity: the page posts through the host bridge, not directly.
	if !contains(StartHTML, `window.__ok({ t: "go", u: q })`) {
		t.Error("start page must route searches through the host bridge")
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
