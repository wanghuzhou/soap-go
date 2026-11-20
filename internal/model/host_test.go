package model

import "testing"

func TestJoinProjectURL(t *testing.T) {
	cases := []struct {
		host, endpoint, want string
	}{
		// A relative path is appended to the host, with exactly one slash.
		{"http://localhost/projectA", "/a/b/list", "http://localhost/projectA/a/b/list"},
		{"http://localhost/projectA/", "a/b/list", "http://localhost/projectA/a/b/list"},
		{"http://localhost/projectA//", "/a", "http://localhost/projectA/a"},
		// A full endpoint wins over the host.
		{"http://localhost/projectA", "http://other.test/x", "http://other.test/x"},
		{"http://localhost/projectA", "https://other.test/x?a=1", "https://other.test/x?a=1"},
		// No host: the endpoint goes as it is.
		{"", "/a/b", "/a/b"},
		{"   ", "/a/b", "/a/b"},
		// The query string of a relative path is kept.
		{"http://localhost/projectA", "/a/b?x=1", "http://localhost/projectA/a/b?x=1"},
		// An empty endpoint is the host itself.
		{"http://localhost/projectA", "", "http://localhost/projectA"},
	}
	for _, c := range cases {
		if got := JoinProjectURL(c.host, c.endpoint); got != c.want {
			t.Errorf("JoinProjectURL(%q, %q) = %q, want %q", c.host, c.endpoint, got, c.want)
		}
	}
}

func TestNormalizeHost(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"  ", ""},
		{"http://localhost/projectA/", "http://localhost/projectA"},
		{"http://localhost/projectA/?q=1#frag", "http://localhost/projectA"},
		{"http://localhost/", "http://localhost"},
		// Not an absolute URL: only trimmed.
		{"localhost/projectA/", "localhost/projectA"},
	}
	for _, c := range cases {
		if got := NormalizeHost(c.in); got != c.want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidateHost(t *testing.T) {
	ok := []string{"", "http://localhost/projectA", "https://api.example.com/v1/"}
	for _, h := range ok {
		if err := ValidateHost(h); err != nil {
			t.Errorf("ValidateHost(%q) = %v, want nil", h, err)
		}
	}
	bad := []string{
		"localhost/projectA",       // no scheme
		"ftp://localhost/projectA", // wrong scheme
		"http://",                  // no host
		"http://localhost/p?q=1",   // query
		"http://localhost/p#frag",  // fragment
	}
	for _, h := range bad {
		if err := ValidateHost(h); err == nil {
			t.Errorf("ValidateHost(%q) = nil, want an error", h)
		}
	}
}

func TestEndpointAbsolute(t *testing.T) {
	abs := []string{"http://x/y", "https://x", "HTTP://X/y"}
	for _, e := range abs {
		if !EndpointAbsolute(e) {
			t.Errorf("EndpointAbsolute(%q) = false, want true", e)
		}
	}
	rel := []string{"", "/a/b", "a/b", "example.test/x", "//example.test/x"}
	for _, e := range rel {
		if EndpointAbsolute(e) {
			t.Errorf("EndpointAbsolute(%q) = true, want false", e)
		}
	}
}
