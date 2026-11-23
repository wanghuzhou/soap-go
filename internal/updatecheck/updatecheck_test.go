package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.6.0", "v0.7.0", -1},
		{"v0.7.0", "v0.6.0", 1},
		{"v0.6.0", "v0.6.0", 0},
		{"0.6.0", "v0.6.0", 0},
		{"v0.6.10", "v0.6.9", 1},
		{"v0.7.0", "v0.7.0-rc1", 1},
		{"v0.7.0-rc1", "v0.7.0", -1},
		{"v1.0.0+build5", "v1.0.0", 0},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestEvaluate(t *testing.T) {
	res := Evaluate("v0.6.0", &Manifest{Version: "v0.7.0", Notes: "修复", Page: "https://x"})
	if !res.HasUpdate || res.Latest != "v0.7.0" || res.Notes != "修复" || res.Page != "https://x" {
		t.Fatalf("result = %+v", res)
	}
	if Evaluate("v0.7.0", &Manifest{Version: "v0.7.0"}).HasUpdate {
		t.Fatal("same version must not be an update")
	}
	if Evaluate("v0.8.0", &Manifest{Version: "v0.7.0"}).HasUpdate {
		t.Fatal("older release must not be an update")
	}
	if Evaluate("v0.6.0", &Manifest{}).HasUpdate {
		t.Fatal("empty manifest must not be an update")
	}
}

func TestFetchManifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request has no User-Agent")
		}
		_, _ = w.Write([]byte(`{"version":"v0.7.0","notes":"新增 .http 导入","page":"https://gh/release"}`))
	}))
	defer srv.Close()

	m, err := FetchManifest(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("FetchManifest: %v", err)
	}
	if m.Version != "v0.7.0" || m.Notes != "新增 .http 导入" || m.Page != "https://gh/release" {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestFetchManifestErrors(t *testing.T) {
	if _, err := FetchManifest(context.Background(), "", nil); err == nil {
		t.Fatal("empty URL accepted")
	}
	if _, err := FetchManifest(context.Background(), "ftp://x/y.json", nil); err == nil {
		t.Fatal("non-http URL accepted")
	}

	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer notFound.Close()
	if _, err := FetchManifest(context.Background(), notFound.URL, nil); err == nil {
		t.Fatal("404 accepted")
	}

	noVersion := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"notes":"x"}`))
	}))
	defer noVersion.Close()
	if _, err := FetchManifest(context.Background(), noVersion.URL, nil); err == nil ||
		!strings.Contains(err.Error(), "版本号") {
		t.Fatalf("missing version error = %v", err)
	}
}
