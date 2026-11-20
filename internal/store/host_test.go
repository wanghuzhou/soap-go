package store

import (
	"errors"
	"testing"
)

// TestUpdateProjectHost stores a normalized host and rejects an invalid one,
// leaving the stored value alone.
func TestUpdateProjectHost(t *testing.T) {
	s, p := newNativeStore(t)

	if err := s.UpdateProjectHost(p.ID, "http://localhost/projectA/"); err != nil {
		t.Fatalf("UpdateProjectHost: %v", err)
	}
	got, _ := s.Get(p.ID)
	if got.Host != "http://localhost/projectA" {
		t.Fatalf("Host = %q, want the normalized %q", got.Host, "http://localhost/projectA")
	}

	if err := s.UpdateProjectHost(p.ID, "localhost/nope"); err == nil {
		t.Fatal("an invalid host should be rejected")
	}
	got, _ = s.Get(p.ID)
	if got.Host != "http://localhost/projectA" {
		t.Fatalf("a rejected host changed the stored value: %q", got.Host)
	}

	if err := s.UpdateProjectHost("missing", "http://x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing project: %v, want ErrNotFound", err)
	}
}
