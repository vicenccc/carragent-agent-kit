package carragent

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		value string
		want  time.Duration
	}{
		{"7", 7 * time.Second},
		{" 12 ", 12 * time.Second},
		{"0", 0},
		{"", 0},
		{"soon", 0},
		{"-5", 0},
		{"1e3", 0},
		{"1_0", 0},
		{"²", 0},
		{"9999999999", 0}, // more seconds than a Duration holds
		{"99999999999999999999999999", 0},
		{now.Add(30 * time.Second).Format(http.TimeFormat), 30 * time.Second},
		{now.Add(-30 * time.Second).Format(http.TimeFormat), 0},
		{"Wed, 21 Oct 2015 07:28:00 GMT", 0},
		{"2026-10-16T12:00:30Z", 0}, // RFC 3339 is not an HTTP date
	}
	for _, tc := range cases {
		if got := parseRetryAfter(tc.value, now); got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestCleanBaseURL(t *testing.T) {
	good := map[string]string{
		"https://carragent.com":             "https://carragent.com",
		"https://carragent.com/":            "https://carragent.com",
		"  https://carragent.com  ":         "https://carragent.com",
		"https://example.com/prefix//":      "https://example.com/prefix",
		"http://localhost:3000":             "http://localhost:3000",
		"http://127.0.0.1:8080/":            "http://127.0.0.1:8080",
		"http://[::1]:3000":                 "http://[::1]:3000",
		"http://127.1.2.3":                  "http://127.1.2.3",
		"https://xn--bcher-kva.example/api": "https://xn--bcher-kva.example/api",
	}
	for in, want := range good {
		got, err := cleanBaseURL(in)
		if err != nil || got != want {
			t.Errorf("cleanBaseURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"", "carragent.com", "//carragent.com", "ftp://carragent.com", "file:///etc/passwd", "https://",
		"http://carragent.com", "http://192.168.1.10", "http://localhost.example.com", "http://0.0.0.0:3000",
		"https://user:pass@carragent.com", "https://user@carragent.com", "https://carragent.com?x=1", "https://carragent.com?",
		"https://carragent.com#top", "https://carragent.com#", "https://carragent.com/%zz", "https://a b.example",
	} {
		if got, err := cleanBaseURL(in); err == nil {
			t.Errorf("cleanBaseURL(%q) = %q, want an error", in, got)
		}
	}
}

func TestEscape(t *testing.T) {
	cases := map[string]string{
		"collections":  "collections",
		"a/b c?d#e%f":  "a%2Fb%20c%3Fd%23e%25f",
		"facturación":  "facturaci%C3%B3n",
		"1+2=3&4":      "1%2B2%3D3%264",
		"A-Za-z0-9_.~": "A-Za-z0-9_.~",
		"cag_a:b%c":    "cag_a%3Ab%25c",
		"s e+cret/ü":   "s%20e%2Bcret%2F%C3%BC",
	}
	for in, want := range cases {
		if got := escape(in); got != want {
			t.Errorf("escape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPathSegment(t *testing.T) {
	for _, bad := range []string{"", ".", ".."} {
		if got, err := pathSegment(bad, "id"); err == nil {
			t.Errorf("pathSegment(%q) = %q, want an error", bad, got)
		}
	}
	if got, err := pathSegment("a.b", "id"); err != nil || got != "a.b" {
		t.Errorf("pathSegment(a.b) = %q, %v", got, err)
	}
}

func TestEncodeQuerySortsAndNeverUsesAPlusForASpace(t *testing.T) {
	query := map[string][]string{"q": {"late invoices+more"}, "lang": {"es"}}
	if got, want := encodeQuery(query), "lang=es&q=late%20invoices%2Bmore"; got != want {
		t.Errorf("encodeQuery = %q, want %q", got, want)
	}
}

func TestTheUserAgentNamesTheVersion(t *testing.T) {
	if userAgent != "carragent-go/"+Version {
		t.Errorf("userAgent = %q", userAgent)
	}
}
