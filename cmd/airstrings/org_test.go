package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func orgAPI(t *testing.T, tokenReply *string) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path+" "+key)
		mu.Unlock()
		org := "org_" + strings.TrimPrefix(key, "as_org_")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/cli/auth/start":
			w.Write([]byte(startReply))
		case r.URL.Path == "/v1/cli/auth/token":
			if strings.Contains(*tokenReply, "error") {
				w.WriteHeader(400)
			}
			w.Write([]byte(*tokenReply))
		case r.Method == "DELETE":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == "POST" && r.URL.Path == "/v1/projects":
			w.WriteHeader(201)
			w.Write([]byte(`{"id":"proj_` + org + `","name":"app"}`))
		case strings.HasSuffix(r.URL.Path, "/sdk-config"):
			w.Write([]byte(`{"org_id":"` + org + `","project_id":"proj_` + org + `","environments":[{"id":"env_prod","name":"production","is_default":true,"public_keys":[]}]}`))
		case strings.HasSuffix(r.URL.Path, "/environments"):
			w.Write([]byte(`{"data":[{"id":"env_prod","name":"production","is_default":true}]}`))
		case strings.HasSuffix(r.URL.Path, "/sections"):
			w.Write([]byte(`{"data":[]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), seen...) }
}

func storeOrgKeys(t *testing.T, xdg, baseURL string, orgs ...string) {
	t.Helper()
	var keys []string
	for i, o := range orgs {
		keys = append(keys, `{"base_url":"`+baseURL+`","org_id":"org_`+o+`","org_name":"`+strings.ToUpper(o)+`","key_id":"ak_`+o+`","api_key":"as_org_`+o+`","created_at":"2026-10-0`+string(rune('1'+i))+`T00:00:00Z"}`)
	}
	os.MkdirAll(filepath.Join(xdg, "airstrings"), 0700)
	if err := os.WriteFile(filepath.Join(xdg, "airstrings", "credentials.json"), []byte(`{"org_keys":[`+strings.Join(keys, ",")+`]}`), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInitOrgWithoutKeyForThatOrgStartsLogin(t *testing.T) {
	reply := `{"error":{"code":"authorization_pending","message":"pending"}}`
	srv, seen := orgAPI(t, &reply)
	xdg := t.TempDir()
	storeOrgKeys(t, xdg, srv.URL, "a")
	code, stdout, stderr := runSharedInDir(t, appDir(t), []string{"XDG_CONFIG_HOME=" + xdg}, "init", "--org", "org_b", "--url", srv.URL, "--json")
	if code != 9 || !strings.Contains(stdout, "cli/approve?code=BCDF-GHJK") {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, s := range seen() {
		if strings.HasSuffix(s, "as_org_a") {
			t.Errorf("used org A's key: %s", s)
		}
	}
}

func TestInitOrgApprovedForAnotherOrgExits3(t *testing.T) {
	reply := `{"error":{"code":"authorization_pending","message":"pending"}}`
	srv, _ := orgAPI(t, &reply)
	xdg := t.TempDir()
	dir := appDir(t)
	env := []string{"XDG_CONFIG_HOME=" + xdg}
	if code, _, _ := runSharedInDir(t, dir, env, "init", "--org", "org_b", "--url", srv.URL); code != 9 {
		t.Fatalf("first run exit = %d, want 9", code)
	}
	reply = `{"api_key":"as_org_a","key_id":"ak_a","org_id":"org_a","org_name":"A","full_power":false}`
	code, stdout, stderr := runSharedInDir(t, dir, env, "init", "--org", "org_b", "--url", srv.URL)
	want := "Approved for A (org_a), but this setup is for org_b. Sign in to the dashboard of org_b's organization and approve again."
	if code != 3 || !strings.Contains(stderr, want) {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".airstrings")); err == nil {
		t.Error("workspace written for the wrong org")
	}
	if active, _ := readCreds(t, xdg)["active"].(map[string]any); active[srv.URL] == "org_a" {
		t.Errorf("wrong-org approval became active: %v", active)
	}
}

func TestInitOrgRelinksWorkspaceFromAnotherOrg(t *testing.T) {
	reply := ""
	srv, seen := orgAPI(t, &reply)
	xdg := t.TempDir()
	storeOrgKeys(t, xdg, srv.URL, "a", "b")
	dir := appDir(t)
	env := []string{"XDG_CONFIG_HOME=" + xdg}
	if code, _, stderr := runSharedInDir(t, dir, env, "init", "--org", "org_a", "--url", srv.URL); code != 0 || readSharedConfig(t, dir).ProjectID != "proj_org_a" {
		t.Fatalf("init org_a exit = %d\nstderr: %s", code, stderr)
	}
	if code, stdout, _ := runSharedInDir(t, dir, env, "init", "--org", "org_a", "--url", srv.URL); code != 0 || !strings.Contains(stdout, "already initialized") {
		t.Fatalf("same-org rerun exit = %d\nstdout: %s", code, stdout)
	}
	if err := os.WriteFile(filepath.Join(dir, ".airstrings", "strings.csv"), []byte("key,locale,value,format\nk,en,unpushed,text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := len(seen())
	code, stdout, stderr := runSharedInDir(t, dir, env, "init", "--org", "org_b", "--url", srv.URL)
	if code != 0 || !strings.Contains(stdout, "This folder was linked to A; re-linked to B. Previous local files moved to .airstrings.org_a") {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if cfg := readSharedConfig(t, dir); cfg.ProjectID != "proj_org_b" {
		t.Errorf("workspace = %+v", cfg)
	}
	if data, err := os.ReadFile(filepath.Join(dir, ".airstrings.org_a", "strings.csv")); err != nil || !strings.Contains(string(data), "unpushed") {
		t.Errorf("old strings not kept: %v %q", err, data)
	}
	for _, s := range seen()[before:] {
		if strings.HasSuffix(s, "as_org_a") {
			t.Errorf("touched org A: %s", s)
		}
	}
}

func revokedKeys(seen []string) string {
	var out []string
	for _, s := range seen {
		if strings.HasPrefix(s, "DELETE ") {
			out = append(out, s[strings.LastIndex(s, "/")+1:])
		}
	}
	return strings.Join(out, ",")
}

func TestLogoutRevokesOnlyTheActiveOrg(t *testing.T) {
	reply := ""
	srv, seen := orgAPI(t, &reply)
	xdg := t.TempDir()
	storeOrgKeys(t, xdg, srv.URL, "a", "b")
	if code, _, stderr := runSharedInDir(t, t.TempDir(), []string{"XDG_CONFIG_HOME=" + xdg}, "logout", "--url", srv.URL); code != 0 {
		t.Fatalf("exit = %d\nstderr: %s", code, stderr)
	}
	if got := revokedKeys(seen()); got != "ak_b as_org_b" {
		t.Errorf("revoked %q, want only the active (most recent) org b", got)
	}
	if keys, _ := readCreds(t, xdg)["org_keys"].([]any); len(keys) != 1 || keys[0].(map[string]any)["org_id"] != "org_a" {
		t.Errorf("keys left: %v", keys)
	}
}

func TestLogoutOrgRevokesOnlyThatOrg(t *testing.T) {
	reply := ""
	srv, seen := orgAPI(t, &reply)
	xdg := t.TempDir()
	storeOrgKeys(t, xdg, srv.URL, "a", "b")
	if code, stdout, stderr := runSharedInDir(t, t.TempDir(), []string{"XDG_CONFIG_HOME=" + xdg}, "logout", "--org", "org_a", "--url", srv.URL); code != 0 || !strings.Contains(stdout, "Logged out of A") {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if got := revokedKeys(seen()); got != "ak_a as_org_a" {
		t.Errorf("revoked %q, want only org a", got)
	}
	keys, _ := readCreds(t, xdg)["org_keys"].([]any)
	if len(keys) != 1 || keys[0].(map[string]any)["org_id"] != "org_b" {
		t.Errorf("keys left: %v", keys)
	}
}

func TestOrgListsSignInsAndUseSwitchesActive(t *testing.T) {
	xdg := t.TempDir()
	base := "https://api.example.test"
	storeOrgKeys(t, xdg, base, "a", "b")
	env := []string{"XDG_CONFIG_HOME=" + xdg, "AIRSTRINGS_BASE_URL=" + base}
	type row struct {
		OrgID     string `json:"org_id"`
		OrgName   string `json:"org_name"`
		FullPower bool   `json:"full_power"`
		Active    bool   `json:"active"`
	}
	list := func() []row {
		code, stdout, stderr := runSharedInDir(t, t.TempDir(), env, "org", "--json")
		if code != 0 {
			t.Fatalf("org exit = %d\nstderr: %s", code, stderr)
		}
		var rows []row
		if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, stdout)
		}
		return rows
	}
	if rows := list(); len(rows) != 2 || rows[0].Active || !rows[1].Active || rows[0].OrgName != "A" {
		t.Fatalf("rows = %+v, want b active", rows)
	}
	if code, stdout, stderr := runSharedInDir(t, t.TempDir(), env, "org", "use", "A"); code != 0 || !strings.Contains(stdout, "A") {
		t.Fatalf("org use exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if rows := list(); !rows[0].Active || rows[1].Active {
		t.Errorf("rows = %+v, want a active", rows)
	}
	code, _, stderr := runSharedInDir(t, t.TempDir(), env, "org", "use", "nope")
	if code != 2 || !strings.Contains(stderr, "org_a") || !strings.Contains(stderr, "org_b") || !strings.Contains(stderr, "airstrings login") {
		t.Errorf("unknown org exit = %d\nstderr: %s", code, stderr)
	}
}

func TestStatusShowsWorkspaceOrgName(t *testing.T) {
	reply := ""
	srv, _ := orgAPI(t, &reply)
	xdg := t.TempDir()
	storeOrgKeys(t, xdg, srv.URL, "a")
	dir := appDir(t)
	env := []string{"XDG_CONFIG_HOME=" + xdg}
	if code, _, stderr := runSharedInDir(t, dir, env, "init", "--org", "org_a", "--url", srv.URL); code != 0 {
		t.Fatalf("init exit = %d\nstderr: %s", code, stderr)
	}
	if code, stdout, _ := runSharedInDir(t, dir, env, "status"); code != 0 || !strings.Contains(stdout, "Org:      A (org_a)") {
		t.Errorf("status exit = %d\nstdout: %s", code, stdout)
	}
}
