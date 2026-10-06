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

const guidanceEnvs = `{"data":[` +
	`{"id":"env_prod","project_id":"proj_test","name":"production","is_default":true,"is_sealed":%s},` +
	`{"id":"env_staging","project_id":"proj_test","name":"staging","is_default":false,"is_sealed":false}]}`

func guidanceServer(t *testing.T, prodSealed bool, extra func(w http.ResponseWriter, r *http.Request) bool) *httptest.Server {
	sealed := "false"
	if prodSealed {
		sealed = "true"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if extra != nil && extra(w, r) {
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/environments"):
			w.Write([]byte(strings.Replace(guidanceEnvs, "%s", sealed, 1)))
		case strings.HasSuffix(r.URL.Path, "/api-keys"):
			w.Write([]byte(`{"data":[{"id":"ak_1","name":"agent","permission":"write","prefix":"stagingk"}]}`))
		case strings.HasSuffix(r.URL.Path, "/sdk-config"):
			w.Write([]byte(`{"org_id":"org_test","project_id":"proj_test","environments":[` +
				`{"id":"env_prod","name":"production","is_default":true,"is_sealed":true,"public_keys":[{"key_id":"UFJPRA==","public_key":"UFJPRA=="}]},` +
				`{"id":"env_staging","name":"staging","is_default":false,"is_sealed":false,"public_keys":[{"key_id":"U1RBRw==","public_key":"U1RBRw=="}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":{"code":"not_found","message":"Not found"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func stagingWorkspace(t *testing.T, srvURL string) string {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	if err := os.MkdirAll(wsDir, 0700); err != nil {
		t.Fatal(err)
	}
	seed := `{"project_id":"proj_test","project_name":"Test","active_env":"env_staging",` +
		`"credentials":[{"api_key":"stagingkey0000000000","base_url":"` + srvURL + `","env_id":"env_staging","env_name":"staging"}]}`
	if err := os.WriteFile(filepath.Join(wsDir, "config.json"), []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSDKConfigProductionFromStagingKey(t *testing.T) {
	srv := guidanceServer(t, true, nil)
	dir := stagingWorkspace(t, srv.URL)

	code, stdout, stderr := runSharedInDir(t, dir, nil, "sdk-config", "--env", "production", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\nstderr: %s", code, stderr)
	}
	var out struct {
		OrgID       string `json:"org_id"`
		Environment struct {
			ID string `json:"id"`
		} `json:"environment"`
		Protection string            `json:"protection"`
		Snippets   map[string]string `json:"snippets"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	if out.OrgID != "org_test" || out.Environment.ID != "env_prod" || out.Protection != "protected" {
		t.Errorf("unexpected config: %+v", out)
	}
	for _, p := range []string{"web", "react_native", "ios", "android", "go"} {
		if !strings.Contains(out.Snippets[p], "UFJPRA==") || !strings.Contains(out.Snippets[p], "env_prod") {
			t.Errorf("%s snippet missing prod key/env: %s", p, out.Snippets[p])
		}
	}
	if !strings.Contains(out.Snippets["go"], `OrganizationID: "org_test"`) || !strings.Contains(out.Snippets["go"], `PublicKeys:     []string{"UFJPRA=="}`) {
		t.Errorf("go snippet field names wrong:\n%s", out.Snippets["go"])
	}
	if !strings.Contains(out.Snippets["ios"], `organizationId: "org_test"`) || !strings.Contains(out.Snippets["android"], `publicKeys = listOf("UFJPRA==")`) {
		t.Errorf("snippet field names wrong:\n%s\n%s", out.Snippets["ios"], out.Snippets["android"])
	}
}

func TestEnvUseProductionWithoutKey(t *testing.T) {
	cases := []struct {
		name     string
		sealed   bool
		wantExit int
		want     []string
	}{
		{"protected", true, 7, []string{"Production is protected, so this workspace doesn't need a production key.", "airstrings sdk-config --env production", "/projects/proj_test/env/env_prod/promote"}},
		{"open", false, 3, []string{"Production accepts direct publishing, but this workspace has no production key.", "/projects/proj_test/env/env_prod/api-keys", "airstrings env add <key>"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := guidanceServer(t, tc.sealed, nil)
			dir := stagingWorkspace(t, srv.URL)
			code, _, stderr := runSharedInDir(t, dir, nil, "env", "use", "production")
			if code != tc.wantExit {
				t.Fatalf("exit = %d, want %d\nstderr: %s", code, tc.wantExit, stderr)
			}
			for _, w := range tc.want {
				if !strings.Contains(stderr, w) {
					t.Errorf("stderr missing %q\n%s", w, stderr)
				}
			}
			if strings.Contains(strings.ToLower(stderr), "not found") || strings.Contains(strings.ToLower(stderr), "sealed") {
				t.Errorf("forbidden vocabulary in: %s", stderr)
			}
		})
	}
}

func TestEnvListShowsKeyAndProtection(t *testing.T) {
	srv := guidanceServer(t, true, nil)
	dir := stagingWorkspace(t, srv.URL)
	code, stdout, stderr := runSharedInDir(t, dir, nil, "env", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\nstderr: %s", code, stderr)
	}
	var rows []struct {
		Name           string `json:"name"`
		Protection     string `json:"protection"`
		KeyInWorkspace bool   `json:"key_in_workspace"`
		Active         bool   `json:"active"`
	}
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	if len(rows) != 2 || rows[0].Protection != "protected" || rows[0].KeyInWorkspace || !rows[1].KeyInWorkspace || !rows[1].Active || rows[1].Protection != "open" {
		t.Errorf("unexpected rows: %+v", rows)
	}
}

func TestStatusShowsScopeAndHint(t *testing.T) {
	srv := guidanceServer(t, true, nil)
	dir := stagingWorkspace(t, srv.URL)
	code, stdout, stderr := runSharedInDir(t, dir, nil, "status")
	if code != 0 {
		t.Fatalf("exit = %d\nstderr: %s", code, stderr)
	}
	for _, w := range []string{"(write)", "Connected to staging. Production is protected.", "/projects/proj_test/env/env_prod/promote"} {
		if !strings.Contains(stdout, w) {
			t.Errorf("status missing %q\n%s", w, stdout)
		}
	}
}

func TestPublishProtectedExitCodeAndJSONError(t *testing.T) {
	srv := guidanceServer(t, true, func(w http.ResponseWriter, r *http.Request) bool {
		if !strings.HasSuffix(r.URL.Path, "/bundles/publish") {
			return false
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"code":"environment_protected","message":"Production is protected: changes reach it only by promotion.","next_step":"Publish to staging, then ask a human to promote: https://app/x"}}`))
		return true
	})
	code, stdout, stderr := runCLIAgainst(t, srv.URL, "publish", "--json")
	if code != 7 {
		t.Fatalf("exit = %d, want 7\nstderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on error, got %q", stdout)
	}
	var e struct {
		Error struct {
			Message  string `json:"message"`
			NextStep string `json:"next_step"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &e); err != nil {
		t.Fatalf("stderr not JSON: %v\n%s", err, stderr)
	}
	if !strings.Contains(e.Error.Message, "Production is protected") || e.Error.NextStep != "Publish to staging, then ask a human to promote: https://app/x" || e.Error.ExitCode != 7 {
		t.Errorf("unexpected error body: %+v", e.Error)
	}
}

func TestStringsSetPushFailureRestoresCSV(t *testing.T) {
	srv := guidanceServer(t, true, func(w http.ResponseWriter, r *http.Request) bool {
		if !strings.Contains(r.URL.Path, "/strings") {
			return false
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"code":"environment_protected","message":"Production is protected: changes reach it only by promotion."}}`))
		return true
	})
	dir := stagingWorkspace(t, srv.URL)
	csv := filepath.Join(dir, ".airstrings", "strings.csv")
	original := "key,locale,value,format\nhome.title,en,Hello,text\n"
	if err := os.WriteFile(csv, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runSharedInDir(t, dir, nil, "strings", "set", "home.title", "en=Changed", "--format", "text", "--push")
	if code != 7 {
		t.Fatalf("exit = %d, want 7\nstderr: %s", code, stderr)
	}
	got, _ := os.ReadFile(csv)
	if string(got) != original {
		t.Errorf("CSV modified after failed push:\n%s", got)
	}
	if !strings.Contains(stderr, "local CSV restored") {
		t.Errorf("stderr does not say CSV was restored: %s", stderr)
	}
}

func TestPromotePreviewPrintsApplyLink(t *testing.T) {
	srv := guidanceServer(t, true, func(w http.ResponseWriter, r *http.Request) bool {
		if !strings.HasSuffix(r.URL.Path, "/promotions/preview") {
			return false
		}
		w.Write([]byte(`{"source_env_id":"env_staging","target_env_id":"env_prod","summary":{"added":1},"entries":[]}`))
		return true
	})
	dir := stagingWorkspace(t, srv.URL)
	code, stdout, stderr := runSharedInDir(t, dir, nil, "promote", "preview")
	if code != 0 {
		t.Fatalf("exit = %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Apply in the dashboard: https://app.airstrings.com/projects/proj_test/env/env_prod/promote") {
		t.Errorf("missing apply link:\n%s", stdout)
	}
}

func workspaceWith(t *testing.T, srvURL, active string, creds ...string) string {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	if err := os.MkdirAll(wsDir, 0700); err != nil {
		t.Fatal(err)
	}
	var list []string
	for _, name := range creds {
		list = append(list, `{"api_key":"`+name+`key0000000000","base_url":"`+srvURL+`","env_id":"env_`+strings.TrimSuffix(name, "uction")+`","env_name":"`+name+`"}`)
	}
	seed := `{"project_id":"proj_test","project_name":"Test","active_env":"` + active + `","credentials":[` + strings.Join(list, ",") + `]}`
	if err := os.WriteFile(filepath.Join(wsDir, "config.json"), []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestStatusLabelsProtectionPerEnvironment(t *testing.T) {
	srv := guidanceServer(t, true, nil)
	dir := stagingWorkspace(t, srv.URL)
	code, stdout, stderr := runSharedInDir(t, dir, nil, "status")
	if code != 0 {
		t.Fatalf("exit = %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Protection: production: protected · staging: open\n") {
		t.Errorf("status protection line ambiguous:\n%s", stdout)
	}
	_, stdout, _ = runSharedInDir(t, dir, nil, "status", "--json")
	var out struct {
		ProtectionByEnv map[string]string `json:"protection_by_env"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil || out.ProtectionByEnv["staging"] != "open" || out.ProtectionByEnv["production"] != "protected" {
		t.Errorf("protection_by_env = %+v (%v)\n%s", out.ProtectionByEnv, err, stdout)
	}
}

func TestStatusProductionOnlyKeyNeedsStagingKey(t *testing.T) {
	srv := guidanceServer(t, true, nil)
	dir := workspaceWith(t, srv.URL, "env_prod", "production")
	code, stdout, stderr := runSharedInDir(t, dir, nil, "status")
	if code != 0 {
		t.Fatalf("exit = %d\nstderr: %s", code, stderr)
	}
	for _, w := range []string{"Production is protected: changes reach it only by promotion.", "Create a staging write key at ", "/projects/proj_test/env/env_staging/api-keys and run `airstrings env add <key>`, publish to staging, then ask a human to promote: ", "/projects/proj_test/env/env_prod/promote"} {
		if !strings.Contains(stdout, w) {
			t.Errorf("status missing %q\n%s", w, stdout)
		}
	}
}

func TestSDKConfigJSONSaysProtected(t *testing.T) {
	srv := guidanceServer(t, true, nil)
	dir := stagingWorkspace(t, srv.URL)
	_, stdout, stderr := runSharedInDir(t, dir, nil, "sdk-config", "--json")
	var out struct {
		Environment map[string]any `json:"environment"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("json: %v\n%s%s", err, stdout, stderr)
	}
	if out.Environment["protected"] != true {
		t.Errorf("environment.protected = %v", out.Environment["protected"])
	}
	if _, ok := out.Environment["is_sealed"]; ok {
		t.Errorf("environment still exposes is_sealed: %v", out.Environment)
	}
}

func TestShorthandEnvUseThenCommand(t *testing.T) {
	srv := guidanceServer(t, true, nil)
	dir := workspaceWith(t, srv.URL, "env_staging", "staging", "production")
	code, stdout, stderr := runSharedInDir(t, dir, nil, "-e", "-u", "production", "status", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"env_name": "production"`) {
		t.Errorf("-e -u production status did not switch:\n%s", stdout)
	}
	code, stdout, stderr = runSharedInDir(t, dir, nil, "-e", "-u", "staging")
	if code != 0 || !strings.Contains(stdout, "Env:      staging (env_staging)") {
		t.Errorf("-e -u staging: exit %d\n%s%s", code, stdout, stderr)
	}
}

func TestEnvIDOverrideNotice(t *testing.T) {
	srv := guidanceServer(t, true, nil)
	dir := workspaceWith(t, srv.URL, "env_staging", "staging", "production")
	env := []string{"AIRSTRINGS_API_KEY=prodkey", "AIRSTRINGS_PROJECT_ID=proj_test", "AIRSTRINGS_ENV_ID=env_prod", "AIRSTRINGS_BASE_URL=" + srv.URL}
	code, _, stderr := runSharedInDir(t, dir, env, "env")
	if code != 0 {
		t.Fatalf("exit = %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "Using environment production from AIRSTRINGS_ENV_ID") {
		t.Errorf("missing override notice:\n%s", stderr)
	}
	_, _, stderr = runSharedInDir(t, dir, []string{"AIRSTRINGS_API_KEY=stagingkey", "AIRSTRINGS_PROJECT_ID=proj_test", "AIRSTRINGS_ENV_ID=env_staging", "AIRSTRINGS_BASE_URL=" + srv.URL}, "env")
	if strings.Contains(stderr, "AIRSTRINGS_ENV_ID") {
		t.Errorf("notice printed without an override:\n%s", stderr)
	}
}

func TestSDKConfigSaysValuesArePublic(t *testing.T) {
	srv := guidanceServer(t, true, nil)
	dir := stagingWorkspace(t, srv.URL)

	_, stdout, stderr := runSharedInDir(t, dir, nil, "sdk-config")
	if first, _, _ := strings.Cut(stdout, "\n"); !strings.Contains(first, "not secrets") {
		t.Errorf("text output does not open with the public notice:\n%s%s", stdout, stderr)
	}

	_, stdout, stderr = runSharedInDir(t, dir, nil, "sdk-config", "--json")
	var out struct {
		Notice string `json:"notice"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("json: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(out.Notice, "not secrets") {
		t.Errorf("notice = %q", out.Notice)
	}
}
