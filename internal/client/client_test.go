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

func TestDashboardBase(t *testing.T) {
	cases := map[string]string{
		"https://api.airstrings.com":         "https://app.airstrings.com",
		"https://api-staging.airstrings.com": "https://app-staging.airstrings.com",
		"http://localhost:8080":              "https://app.airstrings.com",
	}
	for in, want := range cases {
		if got := DashboardBase(in); got != want {
			t.Errorf("DashboardBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAPIErrorExitCodes(t *testing.T) {
	cases := []struct {
		status int
		code   string
		want   int
	}{
		{403, "environment_protected", 7},
		{403, "quota_exceeded", 8},
		{403, "forbidden", 3},
		{401, "unauthorized", 3},
	}
	for _, tc := range cases {
		e := &APIError{StatusCode: tc.status, Body: ErrorResponse{Error: ErrorBody{Code: tc.code, Message: "m"}}}
		if got := e.ExitCode(); got != tc.want {
			t.Errorf("%d/%s → %d, want %d", tc.status, tc.code, got, tc.want)
		}
	}
}

func TestKeyType(t *testing.T) {
	cases := map[string]string{
		"as_org_abc":  "org",
		"as_proj_abc": "project",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef": "environment",
	}
	for key, want := range cases {
		if got := KeyType(key); got != want {
			t.Errorf("KeyType(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestDoOmitsEmptyAPIKeyHeader(t *testing.T) {
	var present bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header["X-Api-Key"]
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if _, err := New("", srv.URL, "", "").ListProjects(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if present {
		t.Error("X-API-Key header sent with an empty key")
	}
}
