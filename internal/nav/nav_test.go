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
