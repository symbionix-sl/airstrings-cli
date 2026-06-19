package workspace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

func TestEnvAuthFromEnv_Unset(t *testing.T) {
	t.Setenv("AIRSTRINGS_API_KEY", "")
	_, ok := EnvAuthFromEnv()
	if ok {
		t.Fatal("expected ok=false when AIRSTRINGS_API_KEY unset")
	}
}

func TestEnvAuthFromEnv_Set(t *testing.T) {
	t.Setenv("AIRSTRINGS_API_KEY", "key1")
	t.Setenv("AIRSTRINGS_BASE_URL", "https://api.example.com")
	t.Setenv("AIRSTRINGS_PROJECT_ID", "proj_1")
	t.Setenv("AIRSTRINGS_ENV_ID", "env_1")

	env, ok := EnvAuthFromEnv()
	if !ok {
		t.Fatal("expected ok=true when AIRSTRINGS_API_KEY set")
	}
	if env.APIKey != "key1" || env.BaseURL != "https://api.example.com" || env.ProjectID != "proj_1" || env.EnvID != "env_1" {
		t.Errorf("unexpected EnvAuth: %+v", env)
	}
}

func TestClientFromEnv_ModeA_NoHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP call to %s", r.URL.Path)
	}))
	defer srv.Close()

	t.Setenv("AIRSTRINGS_API_KEY", "key1")
	t.Setenv("AIRSTRINGS_BASE_URL", srv.URL)
	t.Setenv("AIRSTRINGS_PROJECT_ID", "proj_1")
	t.Setenv("AIRSTRINGS_ENV_ID", "env_1")

	c, ok, err := ClientFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if c.ProjectID() != "proj_1" || c.EnvID() != "env_1" {
		t.Errorf("expected proj_1/env_1, got %s/%s", c.ProjectID(), c.EnvID())
	}
}

func TestClientFromEnv_ModeB_Resolves(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			json.NewEncoder(w).Encode(client.Project{ID: "proj_1", Name: "Test"})
		case "/v1/projects/proj_1/environments":
			json.NewEncoder(w).Encode(client.EnvironmentList{
				Data: []client.Environment{
					{ID: "env_a", Name: "dev", IsDefault: false},
					{ID: "env_b", Name: "prod", IsDefault: true},
				},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	t.Setenv("AIRSTRINGS_API_KEY", "key1")
	t.Setenv("AIRSTRINGS_BASE_URL", srv.URL)
	t.Setenv("AIRSTRINGS_PROJECT_ID", "")
	t.Setenv("AIRSTRINGS_ENV_ID", "")

	c, ok, err := ClientFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if c.ProjectID() != "proj_1" {
		t.Errorf("expected ProjectID proj_1, got %s", c.ProjectID())
	}
	if c.EnvID() != "env_b" {
		t.Errorf("expected EnvID env_b (default), got %s", c.EnvID())
	}
}

func TestDefaultEnvID(t *testing.T) {
	withDefault := []client.Environment{
		{ID: "env_a", IsDefault: false},
		{ID: "env_b", IsDefault: true},
	}
	if got := defaultEnvID(withDefault); got != "env_b" {
		t.Errorf("expected env_b, got %s", got)
	}

	noDefault := []client.Environment{
		{ID: "env_a", IsDefault: false},
		{ID: "env_c", IsDefault: false},
	}
	if got := defaultEnvID(noDefault); got != "env_a" {
		t.Errorf("expected first env_a, got %s", got)
	}

	if got := defaultEnvID(nil); got != "" {
		t.Errorf("expected empty for no envs, got %s", got)
	}
}
