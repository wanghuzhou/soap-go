package transport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"soapgo/internal/model"
)

// TestSendReadsBodyFromFile checks Send loads a binary body named by FilePath
// inside the request, keeping the read off the UI thread.
func TestSendReadsBodyFromFile(t *testing.T) {
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(path, []byte("file-body"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	resp := Send(model.SendRequest{
		URL: srv.URL, Method: "POST", Headers: map[string]string{},
		FilePath: path, TimeoutMS: 5000,
	}, &model.ProxySettings{Mode: "direct"})
	if resp.Error != "" {
		t.Fatalf("Send: %s", resp.Error)
	}
	if string(got) != "file-body" {
		t.Fatalf("server body = %q, want file-body", got)
	}
}

// TestSendMissingFileBodyFails checks a bad FilePath is a clear error, not a
// request without a body.
func TestSendMissingFileBodyFails(t *testing.T) {
	resp := Send(model.SendRequest{
		URL: "http://127.0.0.1:1/", Method: "POST", Headers: map[string]string{},
		FilePath: filepath.Join(t.TempDir(), "missing.bin"), TimeoutMS: 1000,
	}, &model.ProxySettings{Mode: "direct"})
	if resp.Error == "" {
		t.Fatal("a missing file body should fail")
	}
}
