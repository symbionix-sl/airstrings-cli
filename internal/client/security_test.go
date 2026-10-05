package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateBaseURL(t *testing.T) {
	for _, ok := range []string{"https://api.airstrings.com", "https://api.example.com:8443/base", "http://localhost:8080", "http://127.0.0.1:9000", "http://[::1]:8080"} {
		if err := ValidateBaseURL(ok); err != nil {
			t.Errorf("%s: unexpected error %v", ok, err)
		}
	}
	for _, bad := range []string{"http://api.airstrings.com", "http://evil.example", "ftp://localhost", "file:///etc/passwd", "api.airstrings.com", "https://", "http://localhost.evil.com", "http://127.0.0.2"} {
		if err := ValidateBaseURL(bad); err == nil {
			t.Errorf("%s: expected rejection", bad)
		}
	}
}

func TestClientRefusesInvalidBaseURL(t *testing.T) {
	c := New("secret-key", "http://evil.example", "p1", "e1")
	if _, err := c.GetProject(); err == nil || !strings.Contains(err.Error(), "invalid API URL") {
		t.Fatalf("expected invalid API URL error, got %v", err)
	}
}

func TestClientDoesNotFollowCrossHostRedirect(t *testing.T) {
	var leaked string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("X-API-Key")
		w.Write([]byte(`{}`))
	}))
	defer other.Close()
	otherURL := strings.Replace(other.URL, "127.0.0.1", "localhost", 1)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, otherURL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer api.Close()

	_, err := New("secret-key", api.URL, "p1", "e1").GetProject()
	if err == nil {
		t.Fatal("expected redirect refusal")
	}
	if leaked != "" {
		t.Fatalf("API key forwarded to redirect target: %q", leaked)
	}
}

func TestAPIErrorStripsControlCharacters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"code":"forbidden","message":"bad\u001b[2J\u001b]0;pwn\u0007 thing","next_step":"run\u001b[31m this\u009b","details":[{"field":"n\u001bame","reason":"r\u0000"}]}}`))
	}))
	defer srv.Close()

	_, err := New("k", srv.URL, "p1", "e1").GetProject()
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	for _, s := range []string{apiErr.Error(), apiErr.Body.Error.NextStep} {
		if strings.ContainsAny(s, "\x1b\x07\x00\u009b") {
			t.Errorf("control characters survived: %q", s)
		}
	}
	if apiErr.Body.Error.Message != "bad[2J]0;pwn thing" {
		t.Errorf("unexpected message %q", apiErr.Body.Error.Message)
	}
}
