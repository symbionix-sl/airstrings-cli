package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
	return code, string(readOpened(opened, 39))
}

func readOpened(path string, size int) []byte {
	var data []byte
	for i := 0; i < 40 && len(data) < size; i++ {
		time.Sleep(50 * time.Millisecond)
		data, _ = os.ReadFile(path)
	}
	return data
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

func TestLoginDevNullIsNonInteractive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/cli/auth/start" {
			w.Write([]byte(strings.Replace(strings.Replace(startReply, `"interval":5`, `"interval":1`, 1), `"expires_in":600`, `"expires_in":3`, 1)))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"code":"authorization_pending","message":"pending"}}`))
	}))
	t.Cleanup(srv.Close)
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	cmd := exec.Command(binPath, "login", "--url", srv.URL, "--no-browser")
	cmd.Dir = t.TempDir()
	cmd.Env = scrubbedEnv()
	cmd.Stdin, cmd.Stdout = null, null
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.Run()
	if code := cmd.ProcessState.ExitCode(); code != 9 {
		t.Errorf("exit = %d, want 9 (non-interactive)\nstderr: %s", code, stderr.String())
	}
}

func TestLoginRestartsWhenCodeExpiresMidPoll(t *testing.T) {
	starts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cli/auth/start":
			starts++
			code := []string{"", "AAAA-AAAA", "BBBB-BBBB"}[starts]
			w.Write([]byte(`{"device_code":"dc` + code + `","user_code":"` + code + `","verification_uri_complete":"https://app/cli/approve?code=` + code + `","interval":1,"expires_in":600}`))
		case "/v1/cli/auth/token":
			var body struct {
				DeviceCode string `json:"device_code"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.DeviceCode == "dcAAAA-AAAA" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":{"code":"expired_token","message":"expired"}}`))
				return
			}
			w.Write([]byte(`{"api_key":"as_org_new","key_id":"ak_2","org_id":"org_1","org_name":"Acme","full_power":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	opened := filepath.Join(dir, "opened")
	shim := filepath.Join(dir, "browser")
	os.WriteFile(shim, []byte("#!/bin/sh\necho \"$1\" >> "+opened+"\n"), 0700)
	code, _, stderr := runSharedInDir(t, dir, []string{"AIRSTRINGS_NO_BROWSER=", "BROWSER=" + shim, "CI="}, "login", "--url", srv.URL)
	got := readOpened(opened, 78)
	if code != 0 || starts != 2 || !strings.Contains(stderr, "code=BBBB-BBBB") || len(got) != 78 || !strings.Contains(string(got), "code=AAAA-AAAA\n") || !strings.Contains(string(got), "code=BBBB-BBBB\n") {
		t.Errorf("exit = %d, starts = %d, opened = %q\nstderr: %s", code, starts, got, stderr)
	}
}

type loopbackAPI struct {
	srv       *httptest.Server
	mu        sync.Mutex
	redirects []string
}

func (a *loopbackAPI) seen() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.redirects...)
}

func startReplyFor(n int, loopback string) string {
	dc := "dc" + strconv.Itoa(n)
	if loopback == "true" {
		dc = "dcloop" + strconv.Itoa(n)
	}
	reply := `{"device_code":"` + dc + `","user_code":"BCDF-GHJK","verification_uri":"https://app/cli/approve","verification_uri_complete":"https://app/cli/approve?code=BCDF-GHJK","interval":1,"expires_in":600`
	if loopback != "" {
		reply += `,"loopback":` + loopback
	}
	return reply + "}"
}

func confirmLoopback(redirect string, n int) (int, string) {
	if redirect != "" {
		return 200, startReplyFor(n, "true")
	}
	return 200, startReplyFor(n, "")
}

func newLoopbackAPI(t *testing.T, start func(redirect string, n int) (int, string), token func(deviceCode, grant string) (int, string)) *loopbackAPI {
	a := &loopbackAPI{}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RedirectURI string `json:"redirect_uri"`
			DeviceCode  string `json:"device_code"`
			Grant       string `json:"grant"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		status, reply := 200, `{"api_key":"as_org_new","key_id":"ak_2","org_id":"org_1","org_name":"Acme","full_power":true}`
		switch {
		case r.URL.Path == "/v1/cli/auth/start":
			a.mu.Lock()
			a.redirects = append(a.redirects, body.RedirectURI)
			n := len(a.redirects)
			a.mu.Unlock()
			status, reply = start(body.RedirectURI, n)
		case body.Grant == "g1":
		case token != nil:
			status, reply = token(body.DeviceCode, body.Grant)
		case strings.HasPrefix(body.DeviceCode, "dcloop"):
			status, reply = 400, `{"error":{"code":"authorization_pending","message":"pending"}}`
		}
		w.WriteHeader(status)
		w.Write([]byte(reply))
	}))
	t.Cleanup(a.srv.Close)
	return a
}

var browserEnv = []string{"AIRSTRINGS_NO_BROWSER=", "BROWSER=true", "CI=", "SSH_CONNECTION=", "SSH_TTY="}

func startLogin(t *testing.T, xdg string, env []string, args ...string) (*exec.Cmd, *strings.Builder) {
	cmd := exec.Command(binPath, append([]string{"login"}, args...)...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(append(scrubbedEnv(), env...), "XDG_CONFIG_HOME="+xdg)
	stderr := &strings.Builder{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill() })
	return cmd, stderr
}

var noFollow = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func callback(url string) (*http.Response, error) {
	var resp *http.Response
	var err error
	for i := 0; i < 100; i++ {
		if resp, err = noFollow.Get(url); err == nil {
			return resp, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return resp, err
}

func TestLoginLoopbackCallbackExchangesGrant(t *testing.T) {
	api := newLoopbackAPI(t, confirmLoopback, nil)
	xdg := t.TempDir()
	cmd, stderr := startLogin(t, xdg, browserEnv, "--url", api.srv.URL)
	var redirect string
	for i := 0; i < 100 && redirect == ""; i++ {
		time.Sleep(20 * time.Millisecond)
		if seen := api.seen(); len(seen) > 0 {
			redirect = seen[0]
		}
	}
	if !regexp.MustCompile(`^http://127\.0\.0\.1:\d+/callback$`).MatchString(redirect) {
		t.Fatalf("redirect_uri = %q", redirect)
	}
	if resp, err := callback(redirect + "?nope=1"); err != nil || resp.StatusCode == http.StatusFound {
		t.Errorf("callback without grant accepted: %v %v", resp, err)
	}
	resp, err := callback(redirect + "?grant=g1")
	if err != nil || resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "https://app/cli/approve?done=1" {
		t.Fatalf("callback = %v %v", resp, err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("login: %v\nstderr: %s", err, stderr)
	}
	if keys, _ := readCreds(t, xdg)["org_keys"].([]any); len(keys) != 1 {
		t.Errorf("key not stored: %v", readCreds(t, xdg))
	}
	if !strings.Contains(stderr.String(), "To log in, open:\n  https://app/cli/approve?code=BCDF-GHJK\nAn owner") || strings.Contains(stderr.String(), "check the code") {
		t.Errorf("stderr: %s", stderr)
	}
	if _, err := noFollow.Get(redirect + "?grant=g1"); err == nil {
		t.Error("listener still open after login")
	}
}

func rejectRedirect(status int, body string) func(string, int) (int, string) {
	return func(r string, n int) (int, string) {
		if r != "" {
			return status, body
		}
		return 200, startReplyFor(n, "")
	}
}

func TestLoginLoopbackFallsBackToDeviceFlow(t *testing.T) {
	for name, tc := range map[string]struct {
		start  func(string, int) (int, string)
		starts int
	}{
		"older backend":            {func(_ string, n int) (int, string) { return 200, startReplyFor(n, "") }, 1},
		"declined":                 {func(_ string, n int) (int, string) { return 200, startReplyFor(n, "false") }, 1},
		"prod 400 unknown field":   {rejectRedirect(400, `{"error":{"code":"bad_request","message":"Invalid JSON body"}}`), 2},
		"422 invalid redirect_uri": {rejectRedirect(422, `{"error":{"code":"validation_error","message":"invalid redirect_uri"}}`), 2},
	} {
		api := newLoopbackAPI(t, tc.start, nil)
		code, stdout, stderr := runSharedInDir(t, t.TempDir(), browserEnv, "login", "--url", api.srv.URL, "--json")
		seen := api.seen()
		if code != 0 || len(seen) != tc.starts || seen[0] == "" || seen[len(seen)-1] != "" && tc.starts == 2 || !strings.Contains(stderr, "To log in, open:\n  https://app/cli/approve?code=BCDF-GHJK\nand check the code BCDF-GHJK. An owner") {
			t.Errorf("%s: exit = %d, starts = %q\nstdout: %s\nstderr: %s", name, code, seen, stdout, stderr)
		}
	}
}

func TestLoginOverSSHSkipsLoopback(t *testing.T) {
	for _, ssh := range []string{"SSH_CONNECTION=10.0.0.1 22 10.0.0.2 22", "SSH_TTY=/dev/ttys001"} {
		api := newLoopbackAPI(t, confirmLoopback, nil)
		code, _, stderr := runSharedInDir(t, t.TempDir(), append(append([]string(nil), browserEnv...), ssh), "login", "--url", api.srv.URL, "--json")
		if seen := api.seen(); code != 0 || len(seen) != 1 || seen[0] != "" {
			t.Errorf("%s: exit = %d, redirect_uri = %q\nstderr: %s", ssh, code, seen, stderr)
		}
	}
}

func freePort(t *testing.T) (int, net.Listener) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return l.Addr().(*net.TCPAddr).Port, l
}

func storeLoopbackPending(t *testing.T, xdg, baseURL string, port int) {
	os.MkdirAll(filepath.Join(xdg, "airstrings"), 0700)
	pending := `{"org_keys":[],"pending":{"base_url":"` + baseURL + `","device_code":"dcloop0","user_code":"BCDF-GHJK","loopback_port":` + strconv.Itoa(port) + `,` +
		`"verification_uri_complete":"https://app/cli/approve?code=BCDF-GHJK","interval":1,"expires_at":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"}}`
	if err := os.WriteFile(filepath.Join(xdg, "airstrings", "credentials.json"), []byte(pending), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLoginRerunWithLoopbackPendingRebindsSamePort(t *testing.T) {
	api := newLoopbackAPI(t, confirmLoopback, nil)
	xdg := t.TempDir()
	port, l := freePort(t)
	l.Close()
	storeLoopbackPending(t, xdg, api.srv.URL, port)
	cmd, stderr := startLogin(t, xdg, browserEnv, "--url", api.srv.URL)
	resp, err := callback("http://127.0.0.1:" + strconv.Itoa(port) + "/callback?grant=g1")
	if err != nil || resp.StatusCode != http.StatusFound {
		t.Fatalf("callback = %v %v\nstderr: %s", resp, err, stderr)
	}
	if err := cmd.Wait(); err != nil || len(api.seen()) != 0 {
		t.Errorf("login: %v, starts = %d\nstderr: %s", err, len(api.seen()), stderr)
	}
}

func TestLoginRerunWithLoopbackPendingPortTakenStartsFresh(t *testing.T) {
	api := newLoopbackAPI(t, func(_ string, n int) (int, string) { return 200, startReplyFor(n, "") }, nil)
	xdg := t.TempDir()
	port, l := freePort(t)
	defer l.Close()
	storeLoopbackPending(t, xdg, api.srv.URL, port)
	code, _, stderr := runSharedInDir(t, t.TempDir(), append(browserEnv, "XDG_CONFIG_HOME="+xdg), "login", "--url", api.srv.URL)
	if code != 0 || len(api.seen()) != 1 {
		t.Errorf("exit = %d, starts = %d\nstderr: %s", code, len(api.seen()), stderr)
	}
}

func TestLoginGrantRequiredRestartsAndReopens(t *testing.T) {
	start := func(r string, n int) (int, string) {
		if n == 1 {
			return confirmLoopback(r, n)
		}
		return 200, startReplyFor(n, "")
	}
	api := newLoopbackAPI(t, start, func(dc, _ string) (int, string) {
		if dc == "dcloop1" {
			return 400, `{"error":{"code":"grant_required","message":"approved without a grant"}}`
		}
		return 200, `{"api_key":"as_org_new","key_id":"ak_2","org_id":"org_1","org_name":"Acme","full_power":true}`
	})
	dir := t.TempDir()
	opened := filepath.Join(dir, "opened")
	shim := filepath.Join(dir, "browser")
	os.WriteFile(shim, []byte("#!/bin/sh\necho \"$1\" >> "+opened+"\n"), 0700)
	env := append(append([]string(nil), browserEnv...), "BROWSER="+shim)
	code, _, stderr := runSharedInDir(t, dir, env, "login", "--url", api.srv.URL)
	got := readOpened(opened, 78)
	if code != 0 || len(api.seen()) != 2 || strings.Count(stderr, "Approval didn't reach this terminal. Approve again in the browser.\n") != 1 || len(got) != 78 {
		t.Errorf("exit = %d, starts = %d, opened = %q\nstderr: %s", code, len(api.seen()), got, stderr)
	}
}

func TestLoginLoopbackInvalidGrantRestarts(t *testing.T) {
	start := func(r string, n int) (int, string) {
		if n == 1 {
			return confirmLoopback(r, n)
		}
		return 200, startReplyFor(n, "")
	}
	api := newLoopbackAPI(t, start, func(dc, grant string) (int, string) {
		switch {
		case grant == "bad":
			return 400, `{"error":{"code":"invalid_grant","message":"wrong grant"}}`
		case dc == "dcloop1":
			return 400, `{"error":{"code":"authorization_pending","message":"pending"}}`
		}
		return 200, `{"api_key":"as_org_new","key_id":"ak_2","org_id":"org_1","org_name":"Acme","full_power":true}`
	})
	cmd, stderr := startLogin(t, t.TempDir(), browserEnv, "--url", api.srv.URL)
	var redirect string
	for i := 0; i < 100 && redirect == ""; i++ {
		time.Sleep(20 * time.Millisecond)
		if seen := api.seen(); len(seen) > 0 {
			redirect = seen[0]
		}
	}
	if resp, err := callback(redirect + "?grant=bad"); err != nil || resp.StatusCode != http.StatusFound {
		t.Fatalf("callback = %v %v", resp, err)
	}
	if err := cmd.Wait(); err != nil || len(api.seen()) != 2 {
		t.Errorf("login: %v, starts = %d\nstderr: %s", err, len(api.seen()), stderr)
	}
}
