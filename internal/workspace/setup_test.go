package workspace

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

func TestProjectName_GitRemote(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:acme/mobile-app.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"pkg-name"}`), 0600)
	if got := ProjectName(dir); got != "mobile-app" {
		t.Errorf("ProjectName = %q, want mobile-app", got)
	}
}

func TestProjectName_PackageJSONStripsScope(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"@acme/web-shop"}`), 0600)
	if got := ProjectName(dir); got != "web-shop" {
		t.Errorf("ProjectName = %q, want web-shop", got)
	}
}

func TestProjectName_DirFallback(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-app")
	os.Mkdir(dir, 0700)
	if got := ProjectName(dir); got != "my-app" {
		t.Errorf("ProjectName = %q, want my-app", got)
	}
}

type setupAPI struct {
	t          *testing.T
	createCode int
	created    string
	probed     []string
}

const sdkConfigReply = `{"org_id":"org_1","project_id":"proj_new","environments":[` +
	`{"id":"env_prod","name":"production","is_default":true,"is_sealed":true,"public_keys":[{"key_id":"k1","public_key":"PK_PROD"}]},` +
	`{"id":"env_staging","name":"staging","is_default":false,"is_sealed":false,"public_keys":[{"key_id":"k2","public_key":"PK_STG"}]}]}`

func (a *setupAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == "GET" && r.URL.Path == "/v1/projects":
		if strings.HasPrefix(r.Header.Get("X-API-Key"), "as_org_") {
			w.Write([]byte(`{"data":[{"id":"proj_old","name":"Mobile-App"}]}`))
			return
		}
		w.Write([]byte(`{"id":"proj_new","name":"Bound"}`))
	case r.Method == "POST" && r.URL.Path == "/v1/projects":
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		a.created = body["name"]
		if a.createCode != 0 {
			w.WriteHeader(a.createCode)
			code := "conflict"
			if a.createCode == 403 {
				code = "quota_exceeded"
			}
			w.Write([]byte(`{"error":{"code":"` + code + `","message":"nope"}}`))
			return
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"id":"proj_new","name":"` + body["name"] + `"}`))
	case strings.HasSuffix(r.URL.Path, "/sdk-config"):
		w.Write([]byte(sdkConfigReply))
	case r.URL.Path == "/v1/projects/proj_new/environments":
		w.Write([]byte(`{"data":[{"id":"env_prod","name":"production","is_default":true,"is_sealed":true},{"id":"env_staging","name":"staging"}]}`))
	case strings.HasPrefix(r.URL.Path, "/v1/projects/proj_new/environments/") && !strings.HasSuffix(r.URL.Path, "/sections"):
		a.probed = append(a.probed, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/env_staging") {
			w.Write([]byte(`{"id":"env_staging","name":"staging","public_key":"PK_STG","organization_id":"org_1"}`))
			return
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"error":{"code":"not_found","message":"not found"}}`))
	case strings.HasSuffix(r.URL.Path, "/sections"):
		w.Write([]byte(`{"data":[{"name":"onboarding"}]}`))
	default:
		a.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}
}

func runSetup(t *testing.T, api *setupAPI, opts SetupOptions) (string, *SetupResult, error) {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	dir := filepath.Join(t.TempDir(), "my-app")
	os.Mkdir(dir, 0700)
	opts.BaseURL = srv.URL
	res, err := Setup(dir, opts)
	return dir, res, err
}

func TestSetup_OrgKeyCreatesProject(t *testing.T) {
	api := &setupAPI{t: t}
	dir, res, err := runSetup(t, api, SetupOptions{APIKey: "as_org_k"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if api.created != "my-app" || !res.Created || res.ProjectID != "proj_new" {
		t.Errorf("created %q, result %+v", api.created, res)
	}
	cfg, err := LoadConfig(filepath.Join(dir, DirName))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OrgID != "org_1" || len(cfg.Credentials) != 2 || cfg.Credentials[0].APIKey != "" || cfg.Credentials[0].PublicKey != "PK_PROD" {
		t.Errorf("config = %+v", cfg)
	}
	if _, err := os.Stat(filepath.Join(dir, DirName, "onboarding", "onboarding.csv")); err != nil {
		t.Errorf("section dir not created: %v", err)
	}
}

func TestSetup_NameCollisionExit2NamesProject(t *testing.T) {
	api := &setupAPI{t: t, createCode: 409}
	_, _, err := runSetup(t, api, SetupOptions{APIKey: "as_org_k", Name: "mobile-app"})
	var usage *UsageError
	if !errors.As(err, &usage) || !strings.Contains(usage.NextStep, "--project proj_old") || !strings.Contains(usage.NextStep, "--name") {
		t.Fatalf("err = %v, want UsageError naming proj_old", err)
	}
}

func TestSetup_ProjectFlagSelectsExisting(t *testing.T) {
	api := &setupAPI{t: t}
	_, res, err := runSetup(t, api, SetupOptions{APIKey: "as_org_k", Project: "mobile-app"})
	if err != nil || api.created != "" || res.Created || res.ProjectID != "proj_old" {
		t.Fatalf("err = %v, created = %q, result %+v", err, api.created, res)
	}
}

func TestSetup_ActiveEnvIsStaging(t *testing.T) {
	dir, res, err := runSetup(t, &setupAPI{t: t}, SetupOptions{APIKey: "as_org_k"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadConfig(filepath.Join(dir, DirName))
	if cfg.ActiveEnv != "env_staging" || res.ActiveEnvName != "staging" {
		t.Errorf("active env = %q (%q)", cfg.ActiveEnv, res.ActiveEnvName)
	}
}

func TestSetup_ProjectKeyNoProbe(t *testing.T) {
	api := &setupAPI{t: t}
	dir, res, err := runSetup(t, api, SetupOptions{APIKey: "as_proj_k"})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.probed) != 0 || api.created != "" || res.Created {
		t.Errorf("probed %v, created %q", api.probed, api.created)
	}
	cfg, _ := LoadConfig(filepath.Join(dir, DirName))
	if len(cfg.Credentials) != 2 || cfg.Credentials[1].APIKey != "as_proj_k" || cfg.ActiveEnv != "env_staging" {
		t.Errorf("config = %+v", cfg)
	}
}

func TestSetup_LegacyKeyFilesOnlyBoundEnv(t *testing.T) {
	dir, _, err := runSetup(t, &setupAPI{t: t}, SetupOptions{APIKey: "0123456789abcdef"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadConfig(filepath.Join(dir, DirName))
	if len(cfg.Credentials) != 1 || cfg.Credentials[0].EnvID != "env_staging" || cfg.ActiveEnv != "env_staging" || cfg.OrgID != "org_1" {
		t.Errorf("config = %+v", cfg)
	}
}

func TestSetup_QuotaExceededIsAPIError(t *testing.T) {
	_, _, err := runSetup(t, &setupAPI{t: t, createCode: 403}, SetupOptions{APIKey: "as_org_k"})
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) || apiErr.ExitCode() != 8 {
		t.Fatalf("err = %v, want quota APIError", err)
	}
}

func TestSetup_NoKeyIsErrNoKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, _, err := runSetup(t, &setupAPI{t: t}, SetupOptions{}); !errors.Is(err, ErrNoKey) {
		t.Fatalf("err = %v, want ErrNoKey", err)
	}
}
