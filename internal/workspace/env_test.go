package workspace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	c, _, err := Resolve(nil)
	ok := err == nil
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
		case "/v1/projects/proj_1/environments/env_a", "/v1/projects/proj_1/environments/env_b":
			id := r.URL.Path[len(r.URL.Path)-5:]
			json.NewEncoder(w).Encode(client.Environment{ID: id, IsDefault: id == "env_b"})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	t.Setenv("AIRSTRINGS_API_KEY", "key1")
	t.Setenv("AIRSTRINGS_BASE_URL", srv.URL)
	t.Setenv("AIRSTRINGS_PROJECT_ID", "")
	t.Setenv("AIRSTRINGS_ENV_ID", "")

	c, _, err := Resolve(nil)
	ok := err == nil
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

func TestPickEnv(t *testing.T) {
	envs := []client.Environment{
		{ID: "env_prod", Name: "production", IsDefault: true, IsSealed: true},
		{ID: "env_dev", Name: "dev"},
		{ID: "env_staging", Name: "staging"},
	}
	if got, err := pickEnv("as_proj_x", "", "p", envs); err != nil || got != "env_staging" {
		t.Errorf("project key: got %q, %v; want env_staging", got, err)
	}
	sealedOnly := []client.Environment{{ID: "env_a"}, {ID: "env_b", IsDefault: true, IsSealed: true}}
	sealedOnly[0].IsSealed = true
	if got, _ := pickEnv("as_org_x", "", "p", sealedOnly); got != "env_b" {
		t.Errorf("all sealed: got %q, want default env_b", got)
	}
	if _, err := pickEnv("as_proj_x", "", "p", nil); err == nil {
		t.Error("expected error for no environments")
	}
}

func probeServer(t *testing.T, bound map[string]bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			json.NewEncoder(w).Encode(client.Project{ID: "proj_1"})
		case "/v1/projects/proj_1/environments":
			json.NewEncoder(w).Encode(client.EnvironmentList{Data: []client.Environment{
				{ID: "env_prod", Name: "production", IsDefault: true, IsSealed: true},
				{ID: "env_staging", Name: "staging"},
			}})
		case "/v1/projects/proj_1/environments/env_prod", "/v1/projects/proj_1/environments/env_staging":
			id := strings.TrimPrefix(r.URL.Path, "/v1/projects/proj_1/environments/")
			if bound == nil {
				t.Errorf("typed key probed %s", r.URL.Path)
			}
			if !bound[id] {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":{"code":"not_found","message":"not found"}}`))
				return
			}
			json.NewEncoder(w).Encode(client.Environment{ID: id})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
}

func TestClientFromEnv_LegacyStagingKeyUsesBoundEnv(t *testing.T) {
	srv := probeServer(t, map[string]bool{"env_staging": true})
	defer srv.Close()
	t.Setenv("AIRSTRINGS_API_KEY", "0123456789abcdef")
	t.Setenv("AIRSTRINGS_BASE_URL", srv.URL)

	c, _, err := Resolve(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.EnvID() != "env_staging" {
		t.Errorf("EnvID = %q, want env_staging", c.EnvID())
	}
}

func TestClientFromEnv_ProjectKeyPicksStaging(t *testing.T) {
	srv := probeServer(t, nil)
	defer srv.Close()
	t.Setenv("AIRSTRINGS_API_KEY", "as_proj_x")
	t.Setenv("AIRSTRINGS_BASE_URL", srv.URL)

	c, _, err := Resolve(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.EnvID() != "env_staging" {
		t.Errorf("EnvID = %q, want env_staging", c.EnvID())
	}
}

func TestResolveSharedCredential_LegacyBoundEnv(t *testing.T) {
	srv := probeServer(t, map[string]bool{"env_staging": true})
	defer srv.Close()

	cred, err := ResolveSharedCredential("0123456789abcdef", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.EnvID != "env_staging" {
		t.Errorf("EnvID = %q, want env_staging", cred.EnvID)
	}
}

func TestResolveSharedCredential_ProjectKey(t *testing.T) {
	srv := probeServer(t, nil)
	defer srv.Close()

	cred, err := ResolveSharedCredential("as_proj_shared", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.ProjectID != "proj_1" || cred.EnvID != "env_staging" {
		t.Errorf("unexpected credential: %+v", cred)
	}
}
