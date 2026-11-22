package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"soapgo/internal/model"
)

// TestExactlyMaxBodyNotTruncated checks a body of exactly the cap is not
// mislabeled as truncated; only content beyond the cap is.
func TestExactlyMaxBodyNotTruncated(t *testing.T) {
	const chunk = 1 << 20
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		buf := make([]byte, chunk)
		for i := range buf {
			buf[i] = 'A'
		}
		for i := 0; i < maxBodyBytes/chunk; i++ {
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	resp := Send(model.SendRequest{URL: srv.URL, Method: "GET", TimeoutMS: 10000}, nil)
	if resp.Error != "" {
		t.Fatalf("request failed: %s", resp.Error)
	}
	if resp.Truncated {
		t.Fatal("an exactly 16 MB body was marked truncated")
	}
	if resp.Size != maxBodyBytes {
		t.Fatalf("Size = %d, want %d", resp.Size, maxBodyBytes)
	}
}
