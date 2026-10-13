package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWorkspace(t *testing.T, dir, key, baseURL string) {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, ".airstrings"), 0700)
	cfg := `{"project_id":"proj_test","project_name":"T","active_env":"env_s","credentials":[` +
		`{"api_key":"` + key + `","base_url":"` + baseURL + `","env_id":"env_p","env_name":"production"},` +
		`{"api_key":"` + key + `","base_url":"` + baseURL + `","env_id":"env_s","env_name":"staging"}]}`
	if err := os.WriteFile(filepath.Join(dir, ".airstrings", "config.json"), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAPIKeyLsUsesProjectPathShowsScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/projects/proj_test/environments" {
			w.Write([]byte(`{"data":[{"id":"env_s","name":"staging"}]}`))
			return
		}
		if r.URL.Path != "/v1/projects/proj_test/api-keys" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":[{"id":"ak_1","name":"CI","permission":"write","scope":"project","prefix":"as_proj_1234abcd"},` +
			`{"id":"ak_2","name":"Old","permission":"read","scope":"environment","env_id":"env_s","prefix":"1234abcd"}]}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	writeWorkspace(t, dir, "as_proj_k", srv.URL)

	code, stdout, stderr := runSharedInDir(t, dir, nil, "apikey", "ls")
	if code != 0 || !strings.Contains(stdout, "SCOPE") || !strings.Contains(stdout, "project") || !strings.Contains(stdout, "environment (staging)") {
		t.Errorf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

func TestAPIKeyRotateProjectKeyUsesRotateEndpoint(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"ak_new","key":"as_proj_newkey_0002","prefix":"as_proj_newkey_0","scope":"project","permission":"write"}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	writeWorkspace(t, dir, "as_proj_oldkey_0001", srv.URL)

	code, stdout, stderr := runSharedInDir(t, dir, nil, "apikey", "rotate")
	if code != 0 || path != "POST /v1/projects/proj_test/api-keys/rotate" {
		t.Fatalf("exit = %d, path %q\nstdout: %s\nstderr: %s", code, path, stdout, stderr)
	}
	if cfg := readSharedConfig(t, dir); cfg.Credentials[0].APIKey != "as_proj_newkey_0002" || cfg.Credentials[1].APIKey != "as_proj_newkey_0002" {
		t.Errorf("credentials = %+v", cfg.Credentials)
	}
}

func TestAPIKeyRotateOrgModeIsUsageError(t *testing.T) {
	dir := t.TempDir()
	writeWorkspace(t, dir, "", "https://api.example.com")
	code, _, stderr := runSharedInDir(t, dir, nil, "apikey", "rotate")
	if code != 2 || !strings.Contains(stderr, "project key") {
		t.Errorf("exit = %d\nstderr: %s", code, stderr)
	}
}

func TestAPIKeyUnknownSubcommandExit2(t *testing.T) {
	if code, _, _ := runCLI(t, "apikey", "create"); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestAPIKeyRevokeProjectKeyRevokesSelfAndClearsConfig(t *testing.T) {
	var deleted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/projects/proj_test/api-keys":
			w.Write([]byte(`{"data":[{"id":"ak_other","prefix":"as_proj_otherkey"},{"id":"ak_self","prefix":"as_proj_oldkey_0"}]}`))
		case r.Method == "DELETE":
			deleted = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	writeWorkspace(t, dir, "as_proj_oldkey_0001", srv.URL)

	code, stdout, stderr := runSharedInDir(t, dir, nil, "apikey", "revoke")
	if code != 0 || deleted != "/v1/projects/proj_test/api-keys/ak_self" || strings.Count(stdout, "Revoked") != 1 {
		t.Fatalf("exit = %d, deleted %q\nstdout: %s\nstderr: %s", code, deleted, stdout, stderr)
	}
	if cfg := readSharedConfig(t, dir); cfg.Credentials[0].APIKey != "" || cfg.Credentials[1].APIKey != "" {
		t.Errorf("credentials = %+v", cfg.Credentials)
	}
}

func TestAPIKeyRevokeLegacyKeysRevokesEveryStoredKey(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/projects/proj_test/environments/env_p/api-keys":
			w.Write([]byte(`{"data":[{"id":"ak_prod","prefix":"prodkey1"}]}`))
		case r.Method == "GET" && r.URL.Path == "/v1/projects/proj_test/environments/env_s/api-keys":
			w.Write([]byte(`{"data":[{"id":"ak_stg","prefix":"stgkey12"}]}`))
		case r.Method == "DELETE":
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".airstrings"), 0700)
	cfg := `{"project_id":"proj_test","project_name":"T","active_env":"env_s","credentials":[` +
		`{"api_key":"prodkey1_secret_0001","base_url":"` + srv.URL + `","env_id":"env_p","env_name":"production"},` +
		`{"api_key":"stgkey12_secret_0002","base_url":"` + srv.URL + `","env_id":"env_s","env_name":"staging"}]}`
	os.WriteFile(filepath.Join(dir, ".airstrings", "config.json"), []byte(cfg), 0600)

	code, stdout, stderr := runSharedInDir(t, dir, nil, "apikey", "revoke")
	want := []string{"/v1/projects/proj_test/environments/env_p/api-keys/ak_prod", "/v1/projects/proj_test/environments/env_s/api-keys/ak_stg"}
	if code != 0 || strings.Join(deleted, ",") != strings.Join(want, ",") {
		t.Fatalf("exit = %d, deleted %v\nstdout: %s\nstderr: %s", code, deleted, stdout, stderr)
	}
	if c := readSharedConfig(t, dir); c.Credentials[0].APIKey != "" || c.Credentials[1].APIKey != "" {
		t.Errorf("credentials = %+v", c.Credentials)
	}
}

func TestAPIKeyRevokeLoginModeIsUsageError(t *testing.T) {
	dir := t.TempDir()
	writeWorkspace(t, dir, "", "https://api.example.com")
	code, _, stderr := runSharedInDir(t, dir, nil, "apikey", "revoke")
	if code != 2 || !strings.Contains(stderr, "logout") {
		t.Errorf("exit = %d\nstderr: %s", code, stderr)
	}
}

func TestAPIKeyRevokeAlsoRevokesSharedBucketKey(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/projects/proj_test/api-keys":
			w.Write([]byte(`{"data":[{"id":"ak_self","prefix":"as_proj_oldkey_0"}]}`))
		case r.Method == "GET" && r.URL.Path == "/v1/projects/proj_shared/api-keys":
			w.Write([]byte(`{"data":[{"id":"ak_shared","prefix":"as_proj_shared_0"}]}`))
		case r.Method == "DELETE":
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".airstrings"), 0700)
	cfg := `{"project_id":"proj_test","project_name":"T","active_env":"env_p","credentials":[` +
		`{"api_key":"as_proj_oldkey_0001","base_url":"` + srv.URL + `","env_id":"env_p","env_name":"production"}],` +
		`"shared":{"api_key":"as_proj_shared_0002","base_url":"` + srv.URL + `","project_id":"proj_shared","env_id":"env_sh"}}`
	os.WriteFile(filepath.Join(dir, ".airstrings", "config.json"), []byte(cfg), 0600)

	code, stdout, stderr := runSharedInDir(t, dir, nil, "apikey", "revoke")
	want := "/v1/projects/proj_test/api-keys/ak_self,/v1/projects/proj_shared/api-keys/ak_shared"
	if code != 0 || strings.Join(deleted, ",") != want || !strings.Contains(stdout, "ak_self, ak_shared") {
		t.Fatalf("exit = %d, deleted %v\nstdout: %s\nstderr: %s", code, deleted, stdout, stderr)
	}
	if c := readSharedConfig(t, dir); c.Credentials[0].APIKey != "" || c.Shared != nil {
		t.Errorf("config = %+v", c)
	}
}

func TestAPIKeyLsMarksWorkspaceKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/projects/proj_test/environments" {
			w.Write([]byte(`{"data":[]}`))
			return
		}
		w.Write([]byte(`{"data":[{"id":"ak_mine","name":"Mine","scope":"project","prefix":"as_proj_mine_000"},` +
			`{"id":"ak_ci","name":"CI","scope":"project","prefix":"as_proj_ci_00000"}]}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	writeWorkspace(t, dir, "as_proj_mine_0001_secret", srv.URL)

	code, stdout, stderr := runSharedInDir(t, dir, nil, "apikey", "ls")
	lines := strings.Split(stdout, "\n")
	if code != 0 || !strings.Contains(stdout, "IN WORKSPACE") || len(lines) < 4 ||
		!strings.Contains(lines[2], "ak_mine") || !strings.HasSuffix(strings.TrimSpace(lines[2]), "✓") ||
		strings.Contains(lines[3], "✓") {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	_, stdout, _ = runSharedInDir(t, dir, nil, "apikey", "ls", "--json")
	if !strings.Contains(stdout, `"in_workspace": true`) || !strings.Contains(stdout, `"in_workspace": false`) {
		t.Errorf("json: %s", stdout)
	}
}

func TestStatusShowsStoredKeys(t *testing.T) {
	dir := t.TempDir()
	writeWorkspace(t, dir, "as_proj_mine_0001_secret", "http://127.0.0.1:1")
	_, stdout, stderr := runSharedInDir(t, dir, nil, "status")
	if !strings.Contains(stdout, "Stored:   as_proj_mine...cret (apikey revoke removes them)") {
		t.Errorf("stdout: %s\nstderr: %s", stdout, stderr)
	}
	_, stdout, _ = runSharedInDir(t, dir, nil, "status", "--json")
	if !strings.Contains(stdout, `"stored_keys": [`) || !strings.Contains(stdout, `"as_proj_mine...cret"`) {
		t.Errorf("json: %s", stdout)
	}

	login := t.TempDir()
	writeWorkspace(t, login, "", "http://127.0.0.1:1")
	_, stdout, _ = runSharedInDir(t, login, nil, "status", "--json")
	if !strings.Contains(stdout, `"stored_keys": []`) {
		t.Errorf("login json: %s", stdout)
	}
}
