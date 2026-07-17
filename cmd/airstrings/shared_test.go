package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
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
	if !strings.Contains(stdout, "Usage: airstrings shared <ls|add|rm|import>") {
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
