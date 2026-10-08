package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestStartCLIAuth(t *testing.T) {
	name := ClientName("0.18.0")
	host, _ := os.Hostname()
	if !strings.HasPrefix(name, "airstrings-cli 0.18.0 on ") || !strings.Contains("airstrings-cli 0.18.0 on "+host, name) {
		t.Errorf("unexpected client name %q", name)
	}
	if long := ClientName(strings.Repeat("9", 200)); len([]rune(long)) > 90 {
		t.Errorf("client name not truncated: %d runes", len([]rune(long)))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/cli/auth/start" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if _, ok := r.Header["X-Api-Key"]; ok {
			t.Error("X-API-Key sent on a public endpoint")
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["client_name"] != name {
			t.Errorf("client_name = %q, want %q", body["client_name"], name)
		}
		w.Write([]byte(`{"device_code":"dc","user_code":"BCDF-GHJK","verification_uri":"https://app/cli/approve","verification_uri_complete":"https://app/cli/approve?code=BCDF-GHJK","interval":5,"expires_in":600}`))
	}))
	defer srv.Close()

	s, err := New("", srv.URL, "", "").StartCLIAuth(name, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.DeviceCode != "dc" || s.UserCode != "BCDF-GHJK" || s.VerificationURIComplete == "" || s.Interval != 5 || s.ExpiresIn != 600 {
		t.Errorf("unexpected start: %+v", s)
	}
}

func pollServer(t *testing.T, status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/cli/auth/token" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		if req["device_code"] != "dc" {
			t.Errorf("device_code = %q", req["device_code"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
}

func TestPollCLIAuth_PendingIsAPIErrorCode(t *testing.T) {
	srv := pollServer(t, 400, `{"error":{"code":"authorization_pending","message":"pending"}}`)
	defer srv.Close()

	_, err := New("", srv.URL, "", "").PollCLIAuth("dc", "")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Body.Error.Code != "authorization_pending" {
		t.Fatalf("expected authorization_pending APIError, got %v", err)
	}
}

func TestPollCLIAuth_AccessDenied(t *testing.T) {
	srv := pollServer(t, 400, `{"error":{"code":"access_denied","message":"denied","next_step":"Ask an owner of Acme"}}`)
	defer srv.Close()

	_, err := New("", srv.URL, "", "").PollCLIAuth("dc", "")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Body.Error.Code != "access_denied" || apiErr.Body.Error.NextStep != "Ask an owner of Acme" {
		t.Fatalf("expected access_denied APIError with next_step, got %v", err)
	}
}

func TestPollCLIAuth_Approved(t *testing.T) {
	srv := pollServer(t, 200, `{"api_key":"as_org_k","key_id":"ak_1","org_id":"org_1","org_name":"Acme","full_power":true}`)
	defer srv.Close()

	tok, err := New("", srv.URL, "", "").PollCLIAuth("dc", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.APIKey != "as_org_k" || tok.KeyID != "ak_1" || tok.OrgID != "org_1" || tok.OrgName != "Acme" || !tok.FullPower {
		t.Errorf("unexpected token: %+v", tok)
	}
}

func TestRevokeOrgKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/v1/org/api-keys/ak_1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "as_org_k" {
			t.Errorf("unexpected key %q", r.Header.Get("X-API-Key"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := New("as_org_k", srv.URL, "", "").RevokeOrgKey("ak_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUserAgentOnEveryRequest(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.UserAgent())
		w.Write([]byte(`{"device_code":"d","user_code":"U","verification_uri_complete":"x","interval":5,"expires_in":600}`))
	}))
	defer srv.Close()
	old := UserAgent
	UserAgent = "airstrings-cli/9.9.9"
	defer func() { UserAgent = old }()
	New("", srv.URL, "", "").StartCLIAuth("n", "")
	New("k", srv.URL, "p", "e").CreateImport([]byte("key,locale,value,format\n"), nil)
	if len(got) != 2 || got[0] != "airstrings-cli/9.9.9" || got[1] != "airstrings-cli/9.9.9" {
		t.Errorf("User-Agent = %v", got)
	}
}
