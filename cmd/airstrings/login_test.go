package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const startReply = `{"device_code":"dc","user_code":"BCDF-GHJK","verification_uri":"https://app/cli/approve","verification_uri_complete":"https://app/cli/approve?code=BCDF-GHJK","interval":5,"expires_in":600}`

func loginServer(t *testing.T, token string, tokenStatus int) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/cli/auth/start":
			w.Write([]byte(startReply))
		case "/v1/cli/auth/token":
			w.WriteHeader(tokenStatus)
			w.Write([]byte(token))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func readCreds(t *testing.T, xdg string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(xdg, "airstrings", "credentials.json"))
	if err != nil {
		t.Fatalf("read creds: %v", err)
	}
	var m map[string]any
	json.Unmarshal(data, &m)
	return m
}

func TestLoginNonTTYPrintsURLAndExits9(t *testing.T) {
	srv := loginServer(t, `{"error":{"code":"authorization_pending","message":"pending"}}`, 400)
	xdg := t.TempDir()
	code, stdout, stderr := runSharedInDir(t, t.TempDir(), []string{"XDG_CONFIG_HOME=" + xdg}, "login", "--url", srv.URL, "--json")
	if code != 9 {
		t.Fatalf("exit = %d, want 9\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if out["status"] != "pending" || out["verification_uri_complete"] != "https://app/cli/approve?code=BCDF-GHJK" || out["user_code"] != "BCDF-GHJK" || out["next_step"] == nil {
		t.Errorf("unexpected payload: %v", out)
	}
	if p, _ := readCreds(t, xdg)["pending"].(map[string]any); p == nil || p["device_code"] != "dc" {
		t.Errorf("pending not saved: %v", readCreds(t, xdg))
	}
}

func TestLoginInCIPrintsPendingWithEnvHint(t *testing.T) {
	srv := loginServer(t, `{"error":{"code":"authorization_pending","message":"pending"}}`, 400)
	code, stdout, stderr := runSharedInDir(t, t.TempDir(), []string{"CI=true"}, "login", "--url", srv.URL, "--json")
	if code != 9 || !strings.Contains(stdout, "AIRSTRINGS_API_KEY") {
		t.Errorf("exit = %d, want 9 with env hint\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

func TestLoginRerunResumesPendingAndStoresKey0600(t *testing.T) {
	pending := `{"error":{"code":"authorization_pending","message":"pending"}}`
	approved := `{"api_key":"as_org_new","key_id":"ak_new","org_id":"org_1","org_name":"Acme","full_power":true}`
	reply, status := pending, 400
	starts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/cli/auth/start" {
			starts++
			w.Write([]byte(startReply))
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(reply))
	}))
	defer srv.Close()
	xdg := t.TempDir()
	env := []string{"XDG_CONFIG_HOME=" + xdg}

	if code, _, _ := runSharedInDir(t, t.TempDir(), env, "login", "--url", srv.URL, "--json"); code != 9 {
		t.Fatalf("first run exit = %d, want 9", code)
	}
	reply, status = approved, 200
	code, stdout, stderr := runSharedInDir(t, t.TempDir(), env, "login", "--url", srv.URL, "--json")
	if code != 0 || starts != 1 {
		t.Fatalf("rerun exit = %d, starts = %d\nstdout: %s\nstderr: %s", code, starts, stdout, stderr)
	}
	if !strings.Contains(stdout, `"org_name": "Acme"`) || strings.Contains(stdout, "as_org_new") {
		t.Errorf("unexpected output (must name org, never print the key): %s", stdout)
	}
	fi, _ := os.Stat(filepath.Join(xdg, "airstrings", "credentials.json"))
	if fi.Mode().Perm() != 0600 {
		t.Errorf("credentials mode = %o, want 600", fi.Mode().Perm())
	}
	keys, _ := readCreds(t, xdg)["org_keys"].([]any)
	if len(keys) != 1 || keys[0].(map[string]any)["api_key"] != "as_org_new" {
		t.Errorf("org key not stored: %v", readCreds(t, xdg))
	}
}

func TestLoginAccessDeniedExit3NextStep(t *testing.T) {
	srv := loginServer(t, `{"error":{"code":"access_denied","message":"A member cannot approve","next_step":"Ask an owner of Acme to approve"}}`, 400)
	env := []string{"XDG_CONFIG_HOME=" + t.TempDir()}
	runSharedInDir(t, t.TempDir(), env, "login", "--url", srv.URL)
	code, _, stderr := runSharedInDir(t, t.TempDir(), env, "login", "--url", srv.URL)
	if code != 3 || !strings.Contains(stderr, "Ask an owner of Acme") {
		t.Errorf("exit = %d, want 3 with next step\nstderr: %s", code, stderr)
	}
}

func TestLoginOldBackend404SaysUseInitWithKey(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	code, _, stderr := runCLI(t, "login", "--url", srv.URL)
	if code != 3 || !strings.Contains(stderr, "airstrings init <api-key>") {
		t.Errorf("exit = %d\nstderr: %s", code, stderr)
	}
}

func storeKey(t *testing.T, xdg, baseURL string) {
	t.Helper()
	os.MkdirAll(filepath.Join(xdg, "airstrings"), 0700)
	data := `{"org_keys":[{"base_url":"` + baseURL + `","key_id":"ak_1","api_key":"as_org_k","org_name":"Acme"}]}`
	if err := os.WriteFile(filepath.Join(xdg, "airstrings", "credentials.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLogoutRevokesSelfAndDeletes(t *testing.T) {
	var revoked, with string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		revoked, with = r.Method+" "+r.URL.Path, r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	xdg := t.TempDir()
	storeKey(t, xdg, srv.URL)

	code, stdout, stderr := runSharedInDir(t, t.TempDir(), []string{"XDG_CONFIG_HOME=" + xdg}, "logout", "--url", srv.URL)
	if code != 0 || revoked != "DELETE /v1/org/api-keys/ak_1" || with != "as_org_k" {
		t.Fatalf("exit %d, revoked %q with %q\nstdout: %s\nstderr: %s", code, revoked, with, stdout, stderr)
	}
	if keys, _ := readCreds(t, xdg)["org_keys"].([]any); len(keys) != 0 {
		t.Errorf("key not deleted: %v", keys)
	}
}

func TestLogoutRevokeFailureStillDeletesAndWarns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	xdg := t.TempDir()
	storeKey(t, xdg, srv.URL)

	code, _, stderr := runSharedInDir(t, t.TempDir(), []string{"XDG_CONFIG_HOME=" + xdg}, "logout", "--url", srv.URL)
	if code != 0 || !strings.Contains(stderr, "Warning") {
		t.Errorf("exit %d\nstderr: %s", code, stderr)
	}
	if keys, _ := readCreds(t, xdg)["org_keys"].([]any); len(keys) != 0 {
		t.Errorf("key not deleted: %v", keys)
	}
}

func TestUsageListsLoginAndOrgKeyEnv(t *testing.T) {
	code, stdout, _ := runCLI(t, "help")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, w := range []string{"login", "logout", "project ls", "apikey ls", "promote --to", "--project <id|name>", "AIRSTRINGS_ORG_API_KEY", "AIRSTRINGS_NO_BROWSER", "9 login pending"} {
		if !strings.Contains(stdout, w) {
			t.Errorf("usage missing %q", w)
		}
	}
	if strings.Contains(stdout, "\n  init <api-key>") {
		t.Error("usage still says init requires a key")
	}
}

func TestInitHelpNoKeyRequired(t *testing.T) {
	_, stdout, _ := runCLI(t, "init", "--help")
	for _, w := range []string{"Usage: airstrings init [<api-key>]", "airstrings login", "exits 9"} {
		if !strings.Contains(stdout, w) {
			t.Errorf("init help missing %q\n%s", w, stdout)
		}
	}
	_, stdout, _ = runCLI(t, "login", "--help")
	if !strings.Contains(stdout, "Usage: airstrings login") {
		t.Errorf("login help = %s", stdout)
	}
}

func TestCLISendsUserAgent(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.UserAgent()
		w.Write([]byte(startReply))
	}))
	t.Cleanup(srv.Close)
	runCLI(t, "login", "--url", srv.URL)
	if !strings.HasPrefix(ua, "airstrings-cli/") {
		t.Errorf("User-Agent = %q", ua)
	}
}

func TestLoginPendingNextStepHasURL(t *testing.T) {
	srv := loginServer(t, `{"error":{"code":"authorization_pending","message":"pending"}}`, http.StatusBadRequest)
	code, _, stderr := runCLI(t, "login", "--url", srv.URL)
	if code != 9 || !strings.Contains(stderr, "Next step: Approve in the browser at https://app/cli/approve?code=BCDF-GHJK, then run the same command again") || strings.Contains(stderr, "open verification_uri_complete") {
		t.Errorf("exit = %d\nstderr: %s", code, stderr)
	}
}

func approveOnSecondPoll(t *testing.T) (*httptest.Server, *int) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cli/auth/start":
			w.Write([]byte(strings.Replace(startReply, `"interval":5`, `"interval":1`, 1)))
		case "/v1/cli/auth/token":
			polls++
			if polls == 1 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":{"code":"authorization_pending","message":"pending"}}`))
				return
			}
			w.Write([]byte(`{"api_key":"as_org_new","key_id":"ak_2","org_id":"org_1","org_name":"Acme","full_power":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &polls
}

func TestLoginNonTTYWithBrowserPollsUntilApproved(t *testing.T) {
	srv, polls := approveOnSecondPoll(t)
	code, stdout, stderr := runSharedInDir(t, t.TempDir(), []string{"AIRSTRINGS_NO_BROWSER=", "BROWSER=true", "CI="}, "login", "--url", srv.URL, "--json")
	if code != 0 || *polls != 2 || !strings.Contains(stdout, `"status": "logged_in"`) {
		t.Errorf("exit = %d, polls = %d\nstdout: %s\nstderr: %s", code, *polls, stdout, stderr)
	}
}

func TestLoginNonTTYNoBrowserExits9WithoutPolling(t *testing.T) {
	srv, polls := approveOnSecondPoll(t)
	code, stdout, _ := runSharedInDir(t, t.TempDir(), []string{"AIRSTRINGS_NO_BROWSER=", "BROWSER=true", "CI="}, "login", "--url", srv.URL, "--no-browser", "--json")
	if code != 9 || *polls != 0 || !strings.Contains(stdout, "Approve in the browser at https://app/cli/approve?code=BCDF-GHJK, then run the same command again") {
		t.Errorf("exit = %d, polls = %d\nstdout: %s", code, *polls, stdout)
	}
}

func pendingRerun(t *testing.T, args ...string) (int, string) {
	srv, _ := approveOnSecondPoll(t)
	dir := t.TempDir()
	xdg := filepath.Join(dir, "xdg")
	os.MkdirAll(filepath.Join(xdg, "airstrings"), 0700)
	pending := `{"org_keys":[],"pending":{"base_url":"` + srv.URL + `","device_code":"dc","user_code":"BCDF-GHJK",` +
		`"verification_uri_complete":"https://app/cli/approve?code=BCDF-GHJK","interval":1,"expires_at":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"}}`
	os.WriteFile(filepath.Join(xdg, "airstrings", "credentials.json"), []byte(pending), 0600)
	opened := filepath.Join(dir, "opened")
	shim := filepath.Join(dir, "browser")
	os.WriteFile(shim, []byte("#!/bin/sh\necho \"$1\" >> "+opened+"\n"), 0700)
	code, _, stderr := runSharedInDir(t, dir, []string{"XDG_CONFIG_HOME=" + xdg, "AIRSTRINGS_NO_BROWSER=", "BROWSER=" + shim, "CI="}, append([]string{"login", "--url", srv.URL}, args...)...)
	if code != 0 {
		t.Fatalf("exit = %d\nstderr: %s", code, stderr)
	}
	time.Sleep(200 * time.Millisecond)
	data, _ := os.ReadFile(opened)
	return code, string(data)
}

func TestLoginRerunReopensBrowser(t *testing.T) {
	if _, opened := pendingRerun(t); opened != "https://app/cli/approve?code=BCDF-GHJK\n" {
		t.Errorf("browser opened with %q", opened)
	}
}

func TestLoginRerunNoBrowserDoesNotOpen(t *testing.T) {
	if _, opened := pendingRerun(t, "--no-browser"); opened != "" {
		t.Errorf("browser opened with %q", opened)
	}
}
