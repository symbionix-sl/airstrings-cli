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
