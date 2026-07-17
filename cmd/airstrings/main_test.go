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
	"sync/atomic"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "airstrings-cli-test")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "airstrings")
	out, err := exec.Command("go", "build", "-o", binPath, ".").CombinedOutput()
	if err != nil {
		os.RemoveAll(dir)
		panic("build failed: " + err.Error() + "\n" + string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func scrubbedEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "AIRSTRINGS_") {
			continue
		}
		env = append(env, e)
	}
	return env
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = t.TempDir()
	cmd.Env = scrubbedEnv()
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

func TestCLISubprocess(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{"push help", []string{"push", "--help"}, 0, "Usage: airstrings push [--section <name>]", ""},
		{"publish help", []string{"publish", "--help"}, 0, "Usage: airstrings publish [locale...]", ""},
		{"pull help", []string{"pull", "--help"}, 0, "Usage: airstrings pull [--section <name>]", ""},
		{"apikey help", []string{"apikey", "--help"}, 0, "Usage: airstrings apikey rotate [--env <name>]", ""},
		{"status help", []string{"status", "--help"}, 0, "Usage: airstrings status", ""},
		{"strings help", []string{"strings", "--help"}, 0, "Usage: airstrings strings <ls|get|set|rm>", ""},
		{"init help", []string{"init", "--help"}, 0, "Usage: airstrings init <api-key> [--url <base-url>] [--purge]", ""},
		{"env help", []string{"env", "--help"}, 0, "Usage: airstrings env [use|add|rm|create]", ""},
		{"push short help", []string{"push", "-h"}, 0, "Usage: airstrings push [--section <name>]", ""},
		{"status unknown flag", []string{"status", "--totallyfake"}, 2, "", "unknown flag: --totallyfake"},
		{"push unknown flag", []string{"push", "--totallyfake"}, 2, "", "unknown flag: --totallyfake"},
		{"publish unknown flag", []string{"publish", "--totallyfake"}, 2, "", "unknown flag: --totallyfake"},
		{"pull unknown flag", []string{"pull", "--totallyfake"}, 2, "", "unknown flag: --totallyfake"},
		{"strings ls unknown flag", []string{"strings", "ls", "--totallyfake"}, 2, "", "unknown flag: --totallyfake"},
		{"strings set unknown flag", []string{"strings", "set", "k", "en=v", "--format", "text", "--bogus"}, 2, "", "unknown flag: --bogus"},
		{"variants help", []string{"variants", "--help"}, 0, "Usage: airstrings variants", ""},
		{"variants unknown flag", []string{"variants", "status", "home.title", "--totallyfake"}, 2, "", "unknown flag: --totallyfake"},
		{"variants bad allocation", []string{"variants", "allocation", "home.title", "control=abc"}, 2, "", "invalid percentage"},
		{"top-level help flag", []string{"--help"}, 0, "Usage: airstrings <command> [options]", ""},
		{"help command", []string{"help"}, 0, "Usage: airstrings <command> [options]", ""},
		{"version", []string{"version"}, 0, "airstrings", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, tc.args...)
			if code != tc.wantExit {
				t.Fatalf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tc.wantExit, stdout, stderr)
			}
			if tc.wantStdout != "" && !strings.Contains(stdout, tc.wantStdout) {
				t.Errorf("stdout missing %q\nstdout: %s", tc.wantStdout, stdout)
			}
			if tc.wantStderr != "" && !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr missing %q\nstderr: %s", tc.wantStderr, stderr)
			}
		})
	}
}

func TestStatusReportsProtection(t *testing.T) {
	envList := func(defaultSealed bool) string {
		sealed := "false"
		if defaultSealed {
			sealed = "true"
		}
		return `{"data":[` +
			`{"id":"env_prod","project_id":"proj_test","name":"production","is_default":true,"is_sealed":` + sealed + `},` +
			`{"id":"env_dev","project_id":"proj_test","name":"dev","is_default":false,"is_sealed":false}` +
			`]}`
	}

	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name: "sealed default env is protected",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(envList(true)))
			},
			want: `"protection": "protected"`,
		},
		{
			name: "unsealed default env is yolo",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(envList(false)))
			},
			want: `"protection": "yolo"`,
		},
		{
			name: "api error is unknown",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"boom"}`))
			},
			want: `"protection": "unknown"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			cmd := exec.Command(binPath, "status", "--json")
			cmd.Dir = t.TempDir()
			cmd.Env = append(scrubbedEnv(),
				"AIRSTRINGS_API_KEY=ak_test_protection",
				"AIRSTRINGS_PROJECT_ID=proj_test",
				"AIRSTRINGS_ENV_ID=env_prod",
				"AIRSTRINGS_BASE_URL="+srv.URL,
			)
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()

			code := 0
			if err != nil {
				exitErr, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("run: %v", err)
				}
				code = exitErr.ExitCode()
			}
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (status must never fail)\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Errorf("stdout missing %q\nstdout: %s", tc.want, stdout.String())
			}
		})
	}
}

func TestPublishHelpMakesNoNetworkCall(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer srv.Close()

	cmd := exec.Command(binPath, "publish", "--help")
	cmd.Dir = t.TempDir()
	cmd.Env = append(scrubbedEnv(),
		"AIRSTRINGS_API_KEY=ak_test_regression",
		"AIRSTRINGS_PROJECT_ID=proj_test",
		"AIRSTRINGS_ENV_ID=env_test",
		"AIRSTRINGS_BASE_URL="+srv.URL,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run: %v", err)
		}
		code = exitErr.ExitCode()
	}

	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage: airstrings publish [locale...]") {
		t.Errorf("stdout missing publish usage\nstdout: %s", stdout.String())
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Errorf("publish --help made %d network request(s); want 0", n)
	}
}

func runCLIAgainst(t *testing.T, srvURL string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(scrubbedEnv(),
		"AIRSTRINGS_API_KEY=ak_test_variants",
		"AIRSTRINGS_PROJECT_ID=proj_test",
		"AIRSTRINGS_ENV_ID=env_test",
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

func TestVariantsHelpMakesNoNetworkCall(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte("{}"))
	}))
	defer srv.Close()

	code, stdout, stderr := runCLIAgainst(t, srv.URL, "variants", "--help")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Usage: airstrings variants") {
		t.Errorf("stdout missing variants usage\nstdout: %s", stdout)
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Errorf("variants --help made %d network request(s); want 0", n)
	}
}

func TestVariantsExitCodes(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   int
	}{
		{"forbidden is exit 3", http.StatusForbidden, 3},
		{"not found is exit 4", http.StatusNotFound, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(`{"error":{"code":"nope","message":"denied"}}`))
			}))
			defer srv.Close()

			code, stdout, stderr := runCLIAgainst(t, srv.URL, "variants", "status", "home.title")
			if code != tc.want {
				t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, tc.want, stdout, stderr)
			}
		})
	}
}

func TestVariantsStatusJSONIsParseable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"experiment_id":"exp_1","key":"home.title","env_id":"env_test","status":"running","allocation":{"control":70,"blue":30},"variants":{"control":{"en":"Hello"},"blue":{"en":"Hi"}}}`))
	}))
	defer srv.Close()

	code, stdout, stderr := runCLIAgainst(t, srv.URL, "variants", "status", "home.title", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if got["key"] != "home.title" {
		t.Errorf("key = %v, want home.title", got["key"])
	}
}

func TestVariantsSetMergesNewVariantAtZero(t *testing.T) {
	var putBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(`{"experiment_id":"exp_1","key":"home.title","env_id":"env_test","status":"draft","allocation":{"control":100},"variants":{"control":{"en":"Hello"}}}`))
		case http.MethodPut:
			putBody, _ = io.ReadAll(r.Body)
			w.Write([]byte(`{"experiment_id":"exp_1","key":"home.title","status":"draft","allocation":{"control":100,"blue":0},"variants":{}}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	code, stdout, stderr := runCLIAgainst(t, srv.URL, "variants", "set", "home.title", "blue", "en=Hi")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if putBody == nil {
		t.Fatal("no PUT request captured")
	}

	var req struct {
		Allocation map[string]int               `json:"allocation"`
		Variants   map[string]map[string]string `json:"variants"`
	}
	if err := json.Unmarshal(putBody, &req); err != nil {
		t.Fatalf("PUT body not valid JSON: %v\nbody: %s", err, putBody)
	}
	if pct, ok := req.Allocation["blue"]; !ok || pct != 0 {
		t.Errorf("allocation[blue] = %d (present=%v), want 0", pct, ok)
	}
	sum := 0
	for _, p := range req.Allocation {
		sum += p
	}
	if sum != 100 {
		t.Errorf("allocation sums to %d, want 100 (allocation=%v)", sum, req.Allocation)
	}
	if req.Variants["blue"]["en"] != "Hi" {
		t.Errorf("variants[blue][en] = %q, want %q", req.Variants["blue"]["en"], "Hi")
	}
}

func TestInitWritesSDKConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/projects":
			w.Write([]byte(`{"id":"proj_test","name":"Test Project","default_locale":"en"}`))
		case strings.HasSuffix(r.URL.Path, "/sections"):
			w.Write([]byte(`{"data":[]}`))
		case strings.HasSuffix(r.URL.Path, "/environments"):
			w.Write([]byte(`{"data":[{"id":"env_staging","project_id":"proj_test","name":"staging","is_default":true,"is_sealed":false}]}`))
		case strings.Contains(r.URL.Path, "/environments/"):
			w.Write([]byte(`{"id":"env_staging","project_id":"proj_test","name":"staging","is_default":true,"is_sealed":false,"public_key":"pk_test_abc","organization_id":"org_test_123"}`))
		default:
			w.Write([]byte(`{"data":[]}`))
		}
	}))
	defer srv.Close()

	workDir := t.TempDir()
	cmd := exec.Command(binPath, "init", "ak_test_key", "--url", srv.URL, "--json")
	cmd.Dir = workDir
	cmd.Env = scrubbedEnv()
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("init failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(workDir, ".airstrings", "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}

	var cfg struct {
		OrgID       string `json:"org_id"`
		Credentials []struct {
			EnvID     string `json:"env_id"`
			PublicKey string `json:"public_key"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config.json not valid JSON: %v\nbody: %s", err, data)
	}
	if cfg.OrgID != "org_test_123" {
		t.Errorf("org_id = %q, want org_test_123", cfg.OrgID)
	}
	var pk string
	for _, c := range cfg.Credentials {
		if c.EnvID == "env_staging" {
			pk = c.PublicKey
		}
	}
	if pk != "pk_test_abc" {
		t.Errorf("active credential public_key = %q, want pk_test_abc", pk)
	}
}
