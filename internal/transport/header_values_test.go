package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"soapgo/internal/model"
)

// TestSendKeepsEveryResponseHeaderValue checks repeated headers such as
// Set-Cookie keep all their values rather than only the first.
func TestSendKeepsEveryResponseHeaderValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "a=1; Path=/")
		w.Header().Add("Set-Cookie", "b=2; Path=/")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp := Send(model.SendRequest{URL: srv.URL, Method: "GET", TimeoutMS: 5000}, &model.ProxySettings{Mode: "direct"})
	if resp.Error != "" {
		t.Fatalf("Send: %s", resp.Error)
	}
	if got, want := resp.Headers["Set-Cookie"], "a=1; Path=/\nb=2; Path=/"; got != want {
		t.Fatalf("Set-Cookie = %q, want %q", got, want)
	}
}
