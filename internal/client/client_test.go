package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIError_ExitCode(t *testing.T) {
	cases := map[int]int{
		401: 3,
		403: 3,
		404: 4,
		429: 6,
		500: 1,
		400: 1,
	}
	for status, want := range cases {
		err := &APIError{StatusCode: status}
		if got := err.ExitCode(); got != want {
			t.Errorf("status %d: expected exit code %d, got %d", status, want, got)
		}
	}
}

func TestNetworkError_FromUnreachableServer(t *testing.T) {
	srv := httptest.NewServer(nil)
	url := srv.URL
	srv.Close()

	c := New("test-key", url, "proj", "env")
	_, err := c.GetProject()
	if err == nil {
		t.Fatal("expected error from closed server")
	}
	if !errors.As(err, new(*NetworkError)) {
		t.Errorf("expected *NetworkError, got %T: %v", err, err)
	}
}

func TestListBundles_DecodesFallbackURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"locale":"en","cdn_url":"https://cdn.example/b.json","fallback_url":"https://api.example/v1/bundles/b.json"}]}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	bundles, err := c.ListBundles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bundles) != 1 {
		t.Fatalf("got %d bundles, want 1", len(bundles))
	}
	if got, want := bundles[0].FallbackURL, "https://api.example/v1/bundles/b.json"; got != want {
		t.Errorf("FallbackURL = %q, want %q", got, want)
	}
}
