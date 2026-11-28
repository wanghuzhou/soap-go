package model

import "testing"

func TestParseHTTPURL(t *testing.T) {
	ok := []string{"http://example.test/a", "HTTPS://EXAMPLE.test", "http://localhost:8080"}
	for _, raw := range ok {
		u, err := ParseHTTPURL(raw)
		if err != nil {
			t.Fatalf("ParseHTTPURL(%q) = %v", raw, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			t.Fatalf("ParseHTTPURL(%q) scheme = %q", raw, u.Scheme)
		}
	}
	bad := []string{"", "ftp://example.test", "example.test", "http://", "://bad"}
	for _, raw := range bad {
		if _, err := ParseHTTPURL(raw); err == nil {
			t.Fatalf("ParseHTTPURL(%q) accepted a non-http URL", raw)
		}
	}
}
