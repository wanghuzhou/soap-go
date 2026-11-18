package model

import (
	"encoding/base64"
	"testing"
)

func TestAuthSettingsAuthorization(t *testing.T) {
	var none *AuthSettings
	if got := none.Authorization(); got != "" {
		t.Fatalf("nil Authorization = %q", got)
	}

	basic := &AuthSettings{Type: "basic", Enabled: true, Username: "u", Password: "p"}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p"))
	if got := basic.Authorization(); got != want {
		t.Fatalf("basic = %q, want %q", got, want)
	}
	basic.Enabled = false
	if got := basic.Authorization(); got != "" {
		t.Fatalf("disabled basic = %q", got)
	}

	bearer := &AuthSettings{Type: "bearer", Enabled: true, Token: "t"}
	if got := bearer.Authorization(); got != "Bearer t" {
		t.Fatalf("bearer default prefix = %q", got)
	}
	bearer.Prefix = "Token"
	if got := bearer.Authorization(); got != "Token t" {
		t.Fatalf("bearer custom prefix = %q", got)
	}
	bearer.Token = ""
	if got := bearer.Authorization(); got != "" {
		t.Fatalf("bearer without token = %q", got)
	}
}
