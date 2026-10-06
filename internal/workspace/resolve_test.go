package workspace

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

func storeOrgKey(t *testing.T, k OrgKey) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	creds := &Creds{}
	creds.SetOrgKey(k)
	if err := SaveCreds(creds); err != nil {
		t.Fatal(err)
	}
}

func wsWithKey(key string) *WorkspaceConfig {
	return &WorkspaceConfig{
		ProjectID: "proj_ws", ActiveEnv: "env_s", OrgID: "org_1",
		Credentials: []Credential{{APIKey: key, EnvID: "env_s", EnvName: "staging"}},
	}
}

func TestResolveAuth_Precedence(t *testing.T) {
	cases := []struct {
		name       string
		orgEnv     string
		apiEnv     string
		ws         *WorkspaceConfig
		stored     bool
		wantKey    string
		wantSource string
	}{
		{"org env beats all", "as_org_env", "as_proj_env", wsWithKey("as_proj_ws"), true, "as_org_env", "env:AIRSTRINGS_ORG_API_KEY"},
		{"api env beats workspace", "", "as_proj_env", wsWithKey("as_proj_ws"), true, "as_proj_env", "env:AIRSTRINGS_API_KEY"},
		{"workspace beats stored", "", "", wsWithKey("as_proj_ws"), true, "as_proj_ws", "workspace"},
		{"stored when keyless workspace", "", "", wsWithKey(""), true, "as_org_stored", "login"},
		{"stored without workspace", "", "", nil, true, "as_org_stored", "login"},
		{"nothing", "", "", nil, false, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			if tc.stored {
				storeOrgKey(t, OrgKey{BaseURL: client.DefaultBaseURL, OrgID: "org_1", APIKey: "as_org_stored"})
			}
			t.Setenv("AIRSTRINGS_ORG_API_KEY", tc.orgEnv)
			t.Setenv("AIRSTRINGS_API_KEY", tc.apiEnv)
			auth, ok := ResolveAuth(tc.ws)
			if ok != (tc.wantKey != "") || auth.Key != tc.wantKey || auth.Source != tc.wantSource {
				t.Errorf("got %+v ok=%v, want key %q source %q", auth, ok, tc.wantKey, tc.wantSource)
			}
		})
	}
}

func TestResolveAuth_WorkspaceKeyBeatsStoredOrgKey(t *testing.T) {
	storeOrgKey(t, OrgKey{BaseURL: client.DefaultBaseURL, OrgID: "org_1", APIKey: "as_org_stored", FullPower: true})
	auth, _ := ResolveAuth(wsWithKey("0123456789abcdef"))
	if auth.Key != "0123456789abcdef" {
		t.Errorf("Key = %q, want the workspace key", auth.Key)
	}
}

func TestResolveAuth_StoredOrgKeySkippedOnOrgMismatch(t *testing.T) {
	storeOrgKey(t, OrgKey{BaseURL: client.DefaultBaseURL, OrgID: "org_other", APIKey: "as_org_stored"})
	if auth, ok := ResolveAuth(wsWithKey("")); ok {
		t.Errorf("got %+v, want no key for another org's workspace", auth)
	}
}

func TestResolveAuth_StoredOrgKeyMatchesBaseURL(t *testing.T) {
	storeOrgKey(t, OrgKey{BaseURL: "https://api-staging.example", APIKey: "as_org_stg"})
	if _, ok := ResolveAuth(nil); ok {
		t.Error("stored key for another API URL was used")
	}
	t.Setenv("AIRSTRINGS_BASE_URL", "https://api-staging.example")
	if auth, ok := ResolveAuth(nil); !ok || auth.Key != "as_org_stg" || auth.BaseURL != "https://api-staging.example" {
		t.Errorf("got %+v ok=%v", auth, ok)
	}
}

func TestResolve_OrgKeyWithoutProjectIsUsageError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"proj_a","name":"A"},{"id":"proj_b","name":"B"}]}`))
	}))
	defer srv.Close()
	t.Setenv("AIRSTRINGS_ORG_API_KEY", "as_org_env")
	t.Setenv("AIRSTRINGS_BASE_URL", srv.URL)

	_, _, err := Resolve(nil)
	var usage *UsageError
	if !errors.As(err, &usage) || usage.NextStep == "" {
		t.Fatalf("Resolve() error = %v, want UsageError with next step", err)
	}
}

func TestResolve_LegacyConfigUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP call to %s", r.URL.Path)
	}))
	defer srv.Close()
	storeOrgKey(t, OrgKey{BaseURL: srv.URL, OrgID: "org_1", APIKey: "as_org_stored", FullPower: true})

	dir := t.TempDir()
	wsDir := filepath.Join(dir, DirName)
	os.MkdirAll(wsDir, 0700)
	fixture := `{
  "project_id": "proj_1",
  "project_name": "App",
  "active_env": "env_staging",
  "org_id": "org_1",
  "credentials": [
    {"api_key": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "base_url": "` + srv.URL + `", "env_id": "env_prod", "env_name": "production"},
    {"api_key": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "base_url": "` + srv.URL + `", "env_id": "env_staging", "env_name": "staging"}
  ]
}`
	os.WriteFile(filepath.Join(wsDir, ConfigFile), []byte(fixture), 0600)
	cfg, err := LoadConfig(wsDir)
	if err != nil {
		t.Fatal(err)
	}

	c, auth, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if auth.Key[0] != 'b' || auth.Source != "workspace" || c.ProjectID() != "proj_1" || c.EnvID() != "env_staging" {
		t.Errorf("got %+v %s/%s", auth, c.ProjectID(), c.EnvID())
	}
}

func TestResolve_KeylessCredentialUsesOrgKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP call to %s", r.URL.Path)
	}))
	defer srv.Close()
	storeOrgKey(t, OrgKey{BaseURL: srv.URL, OrgID: "org_1", APIKey: "as_org_stored"})
	cfg := wsWithKey("")
	cfg.Credentials[0].BaseURL = srv.URL

	c, auth, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if auth.Key != "as_org_stored" || c.ProjectID() != "proj_ws" || c.EnvID() != "env_s" {
		t.Errorf("got %+v %s/%s", auth, c.ProjectID(), c.EnvID())
	}
}

func TestResolve_ProjectFlagByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			w.Write([]byte(`{"data":[{"id":"proj_a","name":"Alpha"},{"id":"proj_b","name":"Beta"}]}`))
		case "/v1/projects/proj_b/environments":
			json.NewEncoder(w).Encode(client.EnvironmentList{Data: []client.Environment{
				{ID: "env_p", Name: "production", IsDefault: true, IsSealed: true},
				{ID: "env_s", Name: "staging"},
			}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	t.Setenv("AIRSTRINGS_ORG_API_KEY", "as_org_env")
	t.Setenv("AIRSTRINGS_BASE_URL", srv.URL)
	ProjectFlag = "beta"
	t.Cleanup(func() { ProjectFlag = "" })

	c, _, err := Resolve(wsWithKey("as_proj_ws"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ProjectID() != "proj_b" || c.EnvID() != "env_s" {
		t.Errorf("got %s/%s, want proj_b/env_s", c.ProjectID(), c.EnvID())
	}
}

func TestResolve_OrgScopedOpUsesOrgKeyOverWorkspaceKey(t *testing.T) {
	storeOrgKey(t, OrgKey{BaseURL: client.DefaultBaseURL, OrgID: "org_1", APIKey: "as_org_stored"})
	auth, ok := OrgAuth(wsWithKey("as_proj_ws"))
	if !ok || auth.Key != "as_org_stored" {
		t.Errorf("OrgAuth() = %+v, %v", auth, ok)
	}
	t.Setenv("AIRSTRINGS_ORG_API_KEY", "as_org_env")
	if auth, _ := OrgAuth(nil); auth.Key != "as_org_env" {
		t.Errorf("OrgAuth() = %+v, want env org key", auth)
	}
}
