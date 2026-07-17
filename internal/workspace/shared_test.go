package workspace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

func TestSharedCredentialRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := WorkspaceConfig{
		ProjectID:   "proj_main",
		ProjectName: "Main",
		ActiveEnv:   "env_main",
		Credentials: []Credential{{APIKey: "key_main", EnvID: "env_main", EnvName: "prod"}},
		Shared: &SharedCredential{
			APIKey:    "ask_shared_key",
			BaseURL:   "https://api.example.com",
			ProjectID: "proj_shared",
			EnvID:     "env_shared",
		},
	}
	if err := Init(dir, cfg); err != nil {
		t.Fatalf("init: %v", err)
	}
	wsDir := filepath.Join(dir, ".airstrings")

	loaded, err := LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Shared == nil {
		t.Fatal("shared credential lost on round-trip")
	}
	if loaded.Shared.APIKey != "ask_shared_key" || loaded.Shared.ProjectID != "proj_shared" ||
		loaded.Shared.EnvID != "env_shared" || loaded.Shared.BaseURL != "https://api.example.com" {
		t.Errorf("unexpected shared: %+v", loaded.Shared)
	}
	if loaded.ProjectID != "proj_main" || len(loaded.Credentials) != 1 || loaded.Credentials[0].APIKey != "key_main" {
		t.Errorf("project fields not preserved alongside shared: %+v", loaded)
	}

	loaded.Shared.APIKey = "ask_rotated"
	if err := SaveConfig(wsDir, loaded); err != nil {
		t.Fatalf("save: %v", err)
	}
	again, err := LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if again.Shared == nil || again.Shared.APIKey != "ask_rotated" {
		t.Errorf("shared not persisted after save: %+v", again.Shared)
	}
}

func TestSharedOmittedWhenNil(t *testing.T) {
	data, err := json.Marshal(WorkspaceConfig{ProjectID: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "shared") {
		t.Errorf("nil shared should be omitted from JSON, got %s", data)
	}
}

func TestResolveSharedCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			if got := r.Header.Get("X-API-Key"); got != "ask_shared" {
				t.Errorf("X-API-Key = %q, want ask_shared", got)
			}
			json.NewEncoder(w).Encode(client.Project{ID: "proj_x", Name: "Shared"})
		case "/v1/projects/proj_x/environments":
			json.NewEncoder(w).Encode(client.EnvironmentList{
				Data: []client.Environment{
					{ID: "env_a", IsDefault: false},
					{ID: "env_b", IsDefault: true},
				},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cred, err := ResolveSharedCredential("ask_shared", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.APIKey != "ask_shared" || cred.BaseURL != srv.URL {
		t.Errorf("unexpected key/url: %+v", cred)
	}
	if cred.ProjectID != "proj_x" {
		t.Errorf("ProjectID = %q, want proj_x", cred.ProjectID)
	}
	if cred.EnvID != "env_b" {
		t.Errorf("EnvID = %q, want env_b (default)", cred.EnvID)
	}
}

func TestResolveSharedCredential_NoEnvsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			json.NewEncoder(w).Encode(client.Project{ID: "proj_x"})
		case "/v1/projects/proj_x/environments":
			json.NewEncoder(w).Encode(client.EnvironmentList{Data: []client.Environment{}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	if _, err := ResolveSharedCredential("ask_shared", srv.URL); err == nil {
		t.Fatal("expected error when no environments exist")
	}
}

func TestSharedClient_EnvWins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			json.NewEncoder(w).Encode(client.Project{ID: "proj_env"})
		case "/v1/projects/proj_env/environments":
			json.NewEncoder(w).Encode(client.EnvironmentList{
				Data: []client.Environment{{ID: "env_env", IsDefault: true}},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := Init(dir, WorkspaceConfig{
		Shared: &SharedCredential{APIKey: "ask_config", BaseURL: srv.URL, ProjectID: "proj_config", EnvID: "env_config"},
	}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("AIRSTRINGS_SHARED_API_KEY", "ask_env")
	t.Setenv("AIRSTRINGS_BASE_URL", srv.URL)

	c, err := SharedClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ProjectID() != "proj_env" || c.EnvID() != "env_env" {
		t.Errorf("env did not win: got %s/%s, want proj_env/env_env", c.ProjectID(), c.EnvID())
	}
}

func TestSharedClient_ConfigWhenEnvUnset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP call to %s — config credential is cached", r.URL.Path)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := Init(dir, WorkspaceConfig{
		Shared: &SharedCredential{APIKey: "ask_config", BaseURL: srv.URL, ProjectID: "proj_config", EnvID: "env_config"},
	}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("AIRSTRINGS_SHARED_API_KEY", "")
	t.Setenv("AIRSTRINGS_BASE_URL", "")

	c, err := SharedClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ProjectID() != "proj_config" || c.EnvID() != "env_config" {
		t.Errorf("got %s/%s, want proj_config/env_config", c.ProjectID(), c.EnvID())
	}
}

func TestSharedClient_NoKeyErrorsNamingInit(t *testing.T) {
	t.Setenv("AIRSTRINGS_SHARED_API_KEY", "")

	t.Run("no workspace", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := SharedClient()
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "shared init") {
			t.Errorf("error %q missing 'shared init'", err)
		}
	})

	t.Run("workspace without shared", func(t *testing.T) {
		dir := t.TempDir()
		if err := Init(dir, WorkspaceConfig{ProjectID: "p1", ActiveEnv: "e1"}); err != nil {
			t.Fatal(err)
		}
		t.Chdir(dir)
		_, err := SharedClient()
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "shared init") {
			t.Errorf("error %q missing 'shared init'", err)
		}
	})
}
