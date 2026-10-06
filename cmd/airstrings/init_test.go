package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func initAPI(t *testing.T, tokenReply *string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/cli/auth/start":
			w.Write([]byte(startReply))
		case r.URL.Path == "/v1/cli/auth/token":
			if strings.Contains(*tokenReply, "error") {
				w.WriteHeader(400)
			}
			w.Write([]byte(*tokenReply))
		case r.Method == "GET" && r.URL.Path == "/v1/projects":
			w.Write([]byte(`{"data":[{"id":"proj_old","name":"Existing"}]}`))
		case r.Method == "POST" && r.URL.Path == "/v1/projects":
			if !strings.HasPrefix(r.Header.Get("X-API-Key"), "as_org_") {
				t.Errorf("project created with key %q", r.Header.Get("X-API-Key"))
			}
			w.WriteHeader(201)
			w.Write([]byte(`{"id":"proj_new","name":"app"}`))
		case strings.HasSuffix(r.URL.Path, "/sdk-config"):
			id := strings.Split(r.URL.Path, "/")[3]
			w.Write([]byte(`{"org_id":"org_1","project_id":"` + id + `","environments":[` +
				`{"id":"env_prod","name":"production","is_default":true,"is_sealed":true,"public_keys":[{"key_id":"k","public_key":"PK_PROD"}]},` +
				`{"id":"env_staging","name":"staging","is_default":false,"is_sealed":false,"public_keys":[]}]}`))
		case strings.HasSuffix(r.URL.Path, "/sections"):
			w.Write([]byte(`{"data":[]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func appDir(t *testing.T) string {
	dir := filepath.Join(t.TempDir(), "app")
	os.Mkdir(dir, 0700)
	return dir
}

func TestInitNoArgsWithOrgKeyCreatesProjectAndPrintsSnippet(t *testing.T) {
	reply := ""
	srv := initAPI(t, &reply)
	dir := appDir(t)
	code, stdout, stderr := runSharedInDir(t, dir, []string{"AIRSTRINGS_ORG_API_KEY=as_org_env", "AIRSTRINGS_BASE_URL=" + srv.URL}, "init", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var out struct {
		ProjectID   string `json:"project_id"`
		Created     bool   `json:"created"`
		Environment string `json:"environment"`
		SDKConfig   struct {
			Environment struct {
				ID string `json:"id"`
			} `json:"environment"`
			Snippets map[string]string `json:"snippets"`
		} `json:"sdk_config"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if out.ProjectID != "proj_new" || !out.Created || out.Environment != "staging" || out.SDKConfig.Environment.ID != "env_prod" || !strings.Contains(out.SDKConfig.Snippets["web"], "PK_PROD") {
		t.Errorf("unexpected output: %+v", out)
	}
	if strings.Contains(stdout, "as_org_env") {
		t.Error("output contains the org key")
	}
	if cfg := readSharedConfig(t, dir); cfg.ProjectID != "proj_new" || cfg.Credentials[0].APIKey != "" {
		t.Errorf("workspace = %+v", cfg)
	}
}

func TestInitNoCredsNonTTYExits9WithURL(t *testing.T) {
	reply := `{"error":{"code":"authorization_pending","message":"pending"}}`
	srv := initAPI(t, &reply)
	xdg := t.TempDir()
	dir := appDir(t)
	code, stdout, stderr := runSharedInDir(t, dir, []string{"XDG_CONFIG_HOME=" + xdg}, "init", "--url", srv.URL, "--json")
	if code != 9 || !strings.Contains(stdout, "cli/approve?code=BCDF-GHJK") {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if p, _ := readCreds(t, xdg)["pending"].(map[string]any); p == nil {
		t.Error("pending login not saved")
	}
	if _, err := os.Stat(filepath.Join(dir, ".airstrings")); err == nil {
		t.Error("workspace written before login completed")
	}
}

func TestInitRerunAfterApprovalCompletes(t *testing.T) {
	reply := `{"error":{"code":"authorization_pending","message":"pending"}}`
	srv := initAPI(t, &reply)
	env := []string{"XDG_CONFIG_HOME=" + t.TempDir()}
	dir := appDir(t)
	if code, _, _ := runSharedInDir(t, dir, env, "init", "--url", srv.URL); code != 9 {
		t.Fatalf("first run exit = %d, want 9", code)
	}
	reply = `{"api_key":"as_org_new","key_id":"ak_1","org_id":"org_1","org_name":"Acme","full_power":true}`
	code, stdout, stderr := runSharedInDir(t, dir, env, "init", "--url", srv.URL)
	if code != 0 || !strings.Contains(stdout, "PK_PROD") {
		t.Fatalf("rerun exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if cfg := readSharedConfig(t, dir); cfg.ProjectID != "proj_new" {
		t.Errorf("workspace = %+v", cfg)
	}
}

func TestInitRerunInInitializedDirIsIdempotentExit0(t *testing.T) {
	reply := ""
	srv := initAPI(t, &reply)
	dir := appDir(t)
	env := []string{"AIRSTRINGS_ORG_API_KEY=as_org_env", "AIRSTRINGS_BASE_URL=" + srv.URL}
	runSharedInDir(t, dir, env, "init")
	code, stdout, stderr := runSharedInDir(t, dir, env, "init")
	if code != 0 || !strings.Contains(stdout, "already initialized") {
		t.Errorf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

func TestInitProjectFlagSelectsExisting(t *testing.T) {
	reply := ""
	srv := initAPI(t, &reply)
	dir := appDir(t)
	code, stdout, stderr := runSharedInDir(t, dir, []string{"AIRSTRINGS_ORG_API_KEY=as_org_env", "AIRSTRINGS_BASE_URL=" + srv.URL}, "init", "--project", "existing", "--json")
	if code != 0 || !strings.Contains(stdout, `"created": false`) {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if cfg := readSharedConfig(t, dir); cfg.ProjectID != "proj_old" {
		t.Errorf("workspace project = %q, want proj_old", cfg.ProjectID)
	}
}
