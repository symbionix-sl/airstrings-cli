package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const sharedTestKey = "ask_shared_test"

func runSharedCLIAgainst(t *testing.T, srvURL string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(scrubbedEnv(),
		"AIRSTRINGS_SHARED_API_KEY="+sharedTestKey,
		"AIRSTRINGS_BASE_URL="+srvURL,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

// sharedResolveHandler answers the two discovery calls a scoped key makes:
// GetProject (GET /v1/projects) and ListEnvironments. Returns true if handled.
func sharedResolveHandler(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.URL.Path == "/v1/projects" && r.Method == http.MethodGet:
		w.Write([]byte(`{"id":"proj_shared","name":"shared","default_locale":"en"}`))
		return true
	case r.URL.Path == "/v1/projects/proj_shared/environments" && r.Method == http.MethodGet:
		w.Write([]byte(`{"data":[{"id":"env_shared","project_id":"proj_shared","name":"production","is_default":true}]}`))
		return true
	}
	return false
}

func TestSharedListHitsBucketWithSharedKey(t *testing.T) {
	var gotKey, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sharedResolveHandler(w, r) {
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/strings") {
			gotKey = r.Header.Get("X-API-Key")
			gotPath = r.URL.Path
			w.Write([]byte(`{"data":[{"key":"welcome.title","format":"text","values":{"en":"Hi"}}],"pagination":{"has_more":false}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"nope","message":"unexpected ` + r.Method + " " + r.URL.Path + `"}}`))
	}))
	defer srv.Close()

	code, stdout, stderr := runSharedCLIAgainst(t, srv.URL, "shared", "ls")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if gotKey != sharedTestKey {
		t.Errorf("X-API-Key = %q, want %q", gotKey, sharedTestKey)
	}
	if want := "/v1/projects/proj_shared/environments/env_shared/strings"; gotPath != want {
		t.Errorf("list path = %q, want %q", gotPath, want)
	}
	if !strings.Contains(stdout, "welcome.title") {
		t.Errorf("stdout missing key\nstdout: %s", stdout)
	}
}

func TestSharedAddUpsertsWithSharedKey(t *testing.T) {
	var gotKey, gotPath string
	var putBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sharedResolveHandler(w, r) {
			return
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/strings/") {
			gotKey = r.Header.Get("X-API-Key")
			gotPath = r.URL.Path
			putBody, _ = io.ReadAll(r.Body)
			w.Write([]byte(`{"key":"welcome.title","format":"text","values":{"en":"Hi"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"nope","message":"unexpected ` + r.Method + " " + r.URL.Path + `"}}`))
	}))
	defer srv.Close()

	code, stdout, stderr := runSharedCLIAgainst(t, srv.URL, "shared", "add", "welcome.title", "en=Hi", "--format", "text")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if gotKey != sharedTestKey {
		t.Errorf("X-API-Key = %q, want %q", gotKey, sharedTestKey)
	}
	if want := "/v1/projects/proj_shared/environments/env_shared/strings/welcome.title"; gotPath != want {
		t.Errorf("upsert path = %q, want %q", gotPath, want)
	}
	if putBody == nil {
		t.Fatal("no PUT request captured")
	}
	var req struct {
		Format string             `json:"format"`
		Values map[string]*string `json:"values"`
	}
	if err := json.Unmarshal(putBody, &req); err != nil {
		t.Fatalf("PUT body not valid JSON: %v\nbody: %s", err, putBody)
	}
	if req.Format != "text" {
		t.Errorf("format = %q, want text", req.Format)
	}
	if v := req.Values["en"]; v == nil || *v != "Hi" {
		t.Errorf("values[en] = %v, want %q", req.Values["en"], "Hi")
	}
}

func TestSharedMissingKeyIsUsageError(t *testing.T) {
	code, _, stderr := runCLI(t, "shared", "ls")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "AIRSTRINGS_SHARED_API_KEY") {
		t.Errorf("stderr missing AIRSTRINGS_SHARED_API_KEY\nstderr: %s", stderr)
	}
}

func TestSharedHelpMakesNoNetworkCall(t *testing.T) {
	code, stdout, _ := runCLI(t, "shared", "--help")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Usage: airstrings shared <init|ls|add|rm|import>") {
		t.Errorf("stdout missing shared usage\nstdout: %s", stdout)
	}
}

func TestSharedUnknownSubcommand(t *testing.T) {
	code, _, stderr := runCLI(t, "shared", "bogus")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown shared command") {
		t.Errorf("stderr missing unknown-subcommand message\nstderr: %s", stderr)
	}
}

// runSharedInDir runs the CLI in a fixed dir (so the workspace it writes/reads
// can be inspected) with AIRSTRINGS_* scrubbed unless supplied via extraEnv.
func runSharedInDir(t *testing.T, dir string, extraEnv []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = dir
	cmd.Env = append(scrubbedEnv(), extraEnv...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

type sharedConfigOnDisk struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Credentials []struct {
		APIKey string `json:"api_key"`
		EnvID  string `json:"env_id"`
	} `json:"credentials"`
	Shared *struct {
		APIKey    string `json:"api_key"`
		BaseURL   string `json:"base_url"`
		ProjectID string `json:"project_id"`
		EnvID     string `json:"env_id"`
	} `json:"shared"`
}

func readSharedConfig(t *testing.T, dir string) sharedConfigOnDisk {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".airstrings", "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	var cfg sharedConfigOnDisk
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config.json: %v", err)
	}
	return cfg
}

func TestSharedInitSavesCredentialAndCreatesWorkspace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sharedResolveHandler(w, r) {
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"nope","message":"unexpected ` + r.Method + " " + r.URL.Path + `"}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	key := "ask_live_secret_1234"
	code, stdout, stderr := runSharedInDir(t, dir, nil, "shared", "init", key, "--url", srv.URL)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "ask_live...1234") {
		t.Errorf("stdout missing masked key\nstdout: %s", stdout)
	}
	if strings.Contains(stdout, key) {
		t.Errorf("stdout leaked full key\nstdout: %s", stdout)
	}

	cfg := readSharedConfig(t, dir)
	if cfg.Shared == nil {
		t.Fatal("config.json has no shared object")
	}
	if cfg.Shared.APIKey != key {
		t.Errorf("shared.api_key = %q, want %q", cfg.Shared.APIKey, key)
	}
	if cfg.Shared.ProjectID != "proj_shared" || cfg.Shared.EnvID != "env_shared" {
		t.Errorf("shared project/env = %s/%s, want proj_shared/env_shared", cfg.Shared.ProjectID, cfg.Shared.EnvID)
	}
	if cfg.Shared.BaseURL != srv.URL {
		t.Errorf("shared.base_url = %q, want %q", cfg.Shared.BaseURL, srv.URL)
	}
	if cfg.ProjectID != "" || len(cfg.Credentials) != 0 {
		t.Errorf("fresh workspace should carry no project credentials: %+v", cfg)
	}
}

func TestSharedInitPreservesExistingWorkspace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sharedResolveHandler(w, r) {
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	if err := os.MkdirAll(wsDir, 0700); err != nil {
		t.Fatal(err)
	}
	seed := `{"project_id":"proj_main","project_name":"Main","active_env":"env_main",` +
		`"credentials":[{"api_key":"key_main","base_url":"https://api.example.com","env_id":"env_main","env_name":"prod"}]}`
	if err := os.WriteFile(filepath.Join(wsDir, "config.json"), []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}

	key := "ask_shared_write_9999"
	code, stdout, stderr := runSharedInDir(t, dir, nil, "shared", "init", key, "--url", srv.URL)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	cfg := readSharedConfig(t, dir)
	if cfg.ProjectID != "proj_main" {
		t.Errorf("existing project_id dropped: %q", cfg.ProjectID)
	}
	if len(cfg.Credentials) != 1 || cfg.Credentials[0].APIKey != "key_main" {
		t.Errorf("existing credentials dropped: %+v", cfg.Credentials)
	}
	if cfg.Shared == nil || cfg.Shared.APIKey != key {
		t.Errorf("shared not added: %+v", cfg.Shared)
	}
}

func TestSharedLsUsesSavedCredential(t *testing.T) {
	var gotKey, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sharedResolveHandler(w, r) {
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/strings") {
			gotKey = r.Header.Get("X-API-Key")
			gotPath = r.URL.Path
			w.Write([]byte(`{"data":[{"key":"welcome.title","format":"text","values":{"en":"Hi"}}],"pagination":{"has_more":false}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"nope","message":"unexpected"}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	key := "ask_saved_key_5678"
	if code, so, se := runSharedInDir(t, dir, nil, "shared", "init", key, "--url", srv.URL); code != 0 {
		t.Fatalf("init exit = %d\nstdout: %s\nstderr: %s", code, so, se)
	}

	code, stdout, stderr := runSharedInDir(t, dir, nil, "shared", "ls")
	if code != 0 {
		t.Fatalf("ls exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if gotKey != key {
		t.Errorf("X-API-Key = %q, want saved %q", gotKey, key)
	}
	if want := "/v1/projects/proj_shared/environments/env_shared/strings"; gotPath != want {
		t.Errorf("ls path = %q, want %q", gotPath, want)
	}
	if !strings.Contains(stdout, "welcome.title") {
		t.Errorf("stdout missing key\nstdout: %s", stdout)
	}
}

func TestSharedAddUsesSavedCredential(t *testing.T) {
	var gotKey, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sharedResolveHandler(w, r) {
			return
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/strings/") {
			gotKey = r.Header.Get("X-API-Key")
			gotPath = r.URL.Path
			w.Write([]byte(`{"key":"welcome.title","format":"text","values":{"en":"Hi"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"nope","message":"unexpected"}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	key := "ask_saved_add_4321"
	if code, so, se := runSharedInDir(t, dir, nil, "shared", "init", key, "--url", srv.URL); code != 0 {
		t.Fatalf("init exit = %d\nstdout: %s\nstderr: %s", code, so, se)
	}

	code, stdout, stderr := runSharedInDir(t, dir, nil, "shared", "add", "welcome.title", "en=Hi", "--format", "text")
	if code != 0 {
		t.Fatalf("add exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if gotKey != key {
		t.Errorf("X-API-Key = %q, want saved %q", gotKey, key)
	}
	if want := "/v1/projects/proj_shared/environments/env_shared/strings/welcome.title"; gotPath != want {
		t.Errorf("upsert path = %q, want %q", gotPath, want)
	}
}

func TestSharedEnvKeyOverridesSavedCredential(t *testing.T) {
	var gotKey, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sharedResolveHandler(w, r) {
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/strings") {
			gotKey = r.Header.Get("X-API-Key")
			gotPath = r.URL.Path
			w.Write([]byte(`{"data":[],"pagination":{"has_more":false}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"nope","message":"unexpected"}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	if err := os.MkdirAll(wsDir, 0700); err != nil {
		t.Fatal(err)
	}
	saved := "ask_saved_config_key"
	seed := `{"shared":{"api_key":"` + saved + `","base_url":"` + srv.URL + `","project_id":"proj_config","env_id":"env_config"}}`
	if err := os.WriteFile(filepath.Join(wsDir, "config.json"), []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}

	envKey := "ask_env_override_key"
	extra := []string{"AIRSTRINGS_SHARED_API_KEY=" + envKey, "AIRSTRINGS_BASE_URL=" + srv.URL}
	code, stdout, stderr := runSharedInDir(t, dir, extra, "shared", "ls")
	if code != 0 {
		t.Fatalf("ls exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if gotKey != envKey {
		t.Errorf("X-API-Key = %q, want env key %q (env must override saved)", gotKey, envKey)
	}
	if gotKey == saved {
		t.Errorf("used saved key %q; env key must win", saved)
	}
	if want := "/v1/projects/proj_shared/environments/env_shared/strings"; gotPath != want {
		t.Errorf("path = %q, want env-resolved %q", gotPath, want)
	}
}

func TestSharedNoKeyMentionsSharedInit(t *testing.T) {
	t.Run("no workspace", func(t *testing.T) {
		code, _, stderr := runSharedInDir(t, t.TempDir(), nil, "shared", "ls")
		if code != 2 {
			t.Fatalf("exit = %d, want 2\nstderr: %s", code, stderr)
		}
		if !strings.Contains(stderr, "shared init") {
			t.Errorf("stderr missing 'shared init'\nstderr: %s", stderr)
		}
	})

	t.Run("workspace without shared", func(t *testing.T) {
		dir := t.TempDir()
		wsDir := filepath.Join(dir, ".airstrings")
		if err := os.MkdirAll(wsDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(wsDir, "config.json"), []byte(`{"project_id":"p1","active_env":"e1"}`), 0600); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runSharedInDir(t, dir, nil, "shared", "ls")
		if code != 2 {
			t.Fatalf("exit = %d, want 2\nstderr: %s", code, stderr)
		}
		if !strings.Contains(stderr, "shared init") {
			t.Errorf("stderr missing 'shared init'\nstderr: %s", stderr)
		}
	})
}

func TestSharedInitMissingKeyIsUsageError(t *testing.T) {
	code, _, stderr := runCLI(t, "shared", "init")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "shared init") {
		t.Errorf("stderr missing usage naming 'shared init'\nstderr: %s", stderr)
	}
}
