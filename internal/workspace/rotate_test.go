package workspace_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
	"github.com/symbionix-sl/airstrings-cli/internal/workspace"
)

const (
	rotateOldKey = "oldkey12_secret_tail_0001"
	rotateNewKey = "newkey99_secret_tail_0002"
)

type revocation struct {
	id      string
	usedKey string
}

type apiKeyAPI struct {
	mu          sync.Mutex
	keys        []client.APIKey
	verifyFail  map[string]bool
	revokeFail  map[string]bool
	created     []client.CreateAPIKeyRequest
	revoked     []revocation
	verifyCalls []string
	mux         *http.ServeMux
}

func newAPIKeyAPI(ownPermission string) *apiKeyAPI {
	api := &apiKeyAPI{
		keys: []client.APIKey{
			{ID: "ak_other", Name: "Other", Permission: "read", Prefix: "readonly"},
			{ID: "ak_old", Name: "CLI key", Permission: ownPermission, Prefix: rotateOldKey[:8]},
		},
		verifyFail: make(map[string]bool),
		revokeFail: make(map[string]bool),
		mux:        http.NewServeMux(),
	}
	api.mux.HandleFunc("/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		defer api.mu.Unlock()
		key := r.Header.Get("X-API-Key")
		api.verifyCalls = append(api.verifyCalls, key)
		if api.verifyFail[key] {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(client.ErrorResponse{
				Error: client.ErrorBody{Code: "unauthorized", Message: "Invalid API key"},
			})
			return
		}
		json.NewEncoder(w).Encode(client.Project{ID: "p1", Name: "Rotation Test"})
	})
	api.mux.HandleFunc("/v1/projects/p1/environments/e1/api-keys", func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		defer api.mu.Unlock()
		switch r.Method {
		case "GET":
			json.NewEncoder(w).Encode(client.APIKeyList{Data: api.keys})
		case "POST":
			var req client.CreateAPIKeyRequest
			json.NewDecoder(r.Body).Decode(&req)
			api.created = append(api.created, req)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(client.APIKeyCreated{
				ID:         "ak_new",
				Name:       req.Name,
				Permission: req.Permission,
				Key:        rotateNewKey,
				Prefix:     rotateNewKey[:8],
			})
		}
	})
	api.mux.HandleFunc("/v1/projects/p1/environments/e1/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		defer api.mu.Unlock()
		if r.Method != "DELETE" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := filepath.Base(r.URL.Path)
		if api.revokeFail[id] {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(client.ErrorResponse{
				Error: client.ErrorBody{Code: "internal", Message: "revoke failed"},
			})
			return
		}
		api.revoked = append(api.revoked, revocation{id: id, usedKey: r.Header.Get("X-API-Key")})
		w.WriteHeader(http.StatusNoContent)
	})
	return api
}

func setupRotateWorkspace(t *testing.T, baseURL string) (string, *workspace.WorkspaceConfig, *workspace.Credential) {
	t.Helper()
	dir := t.TempDir()
	cfg := workspace.WorkspaceConfig{
		ProjectID:   "p1",
		ProjectName: "Rotation Test",
		ActiveEnv:   "e1",
		Credentials: []workspace.Credential{
			{APIKey: rotateOldKey, BaseURL: baseURL, EnvID: "e1", EnvName: "production"},
		},
	}
	if err := workspace.Init(dir, cfg); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	wsDir := filepath.Join(dir, ".airstrings")
	wsCfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cred, err := wsCfg.ActiveCredential()
	if err != nil {
		t.Fatalf("active credential: %v", err)
	}
	return wsDir, wsCfg, cred
}

func TestRotateKey_HappyPath(t *testing.T) {
	api := newAPIKeyAPI("write")
	srv := httptest.NewServer(api.mux)
	defer srv.Close()

	wsDir, wsCfg, cred := setupRotateWorkspace(t, srv.URL)

	result, err := workspace.RotateKey(wsDir, wsCfg, cred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OldKeyID != "ak_old" || result.NewKeyID != "ak_new" {
		t.Errorf("unexpected result: %+v", result)
	}
	if result.NewKeyPrefix != rotateNewKey[:8] {
		t.Errorf("expected prefix %q, got %q", rotateNewKey[:8], result.NewKeyPrefix)
	}
	if !result.Revoked {
		t.Error("expected old key to be revoked")
	}

	if len(api.created) != 1 || api.created[0].Name != "CLI key" || api.created[0].Permission != "write" {
		t.Errorf("unexpected create requests: %+v", api.created)
	}
	if len(api.revoked) != 1 || api.revoked[0].id != "ak_old" {
		t.Fatalf("unexpected revocations: %+v", api.revoked)
	}
	if api.revoked[0].usedKey != rotateNewKey {
		t.Errorf("expected old key revoked with new key, used %q", api.revoked[0].usedKey)
	}
	if len(api.verifyCalls) != 1 || api.verifyCalls[0] != rotateNewKey {
		t.Errorf("expected one verify call with new key, got %+v", api.verifyCalls)
	}

	loaded, err := workspace.LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Credentials[0].APIKey != rotateNewKey {
		t.Errorf("expected config to hold new key, got %q", loaded.Credentials[0].APIKey)
	}
}

func TestRotateKey_ReadOnlyRejected(t *testing.T) {
	api := newAPIKeyAPI("read")
	srv := httptest.NewServer(api.mux)
	defer srv.Close()

	wsDir, wsCfg, cred := setupRotateWorkspace(t, srv.URL)

	_, err := workspace.RotateKey(wsDir, wsCfg, cred)
	if err == nil {
		t.Fatal("expected error for read-only key")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("expected read-only error, got %q", err.Error())
	}
	if len(api.created) != 0 {
		t.Errorf("expected no key creation, got %+v", api.created)
	}

	loaded, _ := workspace.LoadConfig(wsDir)
	if loaded.Credentials[0].APIKey != rotateOldKey {
		t.Errorf("expected config unchanged, got %q", loaded.Credentials[0].APIKey)
	}
}

func TestRotateKey_KeyNotIdentified(t *testing.T) {
	api := newAPIKeyAPI("write")
	api.keys = []client.APIKey{
		{ID: "ak_other", Name: "Other", Permission: "write", Prefix: "mismatch"},
	}
	srv := httptest.NewServer(api.mux)
	defer srv.Close()

	wsDir, wsCfg, cred := setupRotateWorkspace(t, srv.URL)

	_, err := workspace.RotateKey(wsDir, wsCfg, cred)
	if err == nil {
		t.Fatal("expected error when key cannot be identified")
	}
	if !strings.Contains(err.Error(), "dashboard") {
		t.Errorf("expected dashboard hint, got %q", err.Error())
	}
}

func TestRotateKey_VerifyFailureRollsBack(t *testing.T) {
	api := newAPIKeyAPI("write")
	api.verifyFail[rotateNewKey] = true
	srv := httptest.NewServer(api.mux)
	defer srv.Close()

	wsDir, wsCfg, cred := setupRotateWorkspace(t, srv.URL)

	_, err := workspace.RotateKey(wsDir, wsCfg, cred)
	if err == nil {
		t.Fatal("expected error when verification fails")
	}

	loaded, loadErr := workspace.LoadConfig(wsDir)
	if loadErr != nil {
		t.Fatalf("load config: %v", loadErr)
	}
	if loaded.Credentials[0].APIKey != rotateOldKey {
		t.Errorf("expected old key restored, got %q", loaded.Credentials[0].APIKey)
	}
	if len(api.revoked) != 1 || api.revoked[0].id != "ak_new" {
		t.Fatalf("expected new key revoked, got %+v", api.revoked)
	}
	if api.revoked[0].usedKey != rotateOldKey {
		t.Errorf("expected new key revoked with old key, used %q", api.revoked[0].usedKey)
	}
}

func TestRotateKey_OldKeyRevokeFails(t *testing.T) {
	api := newAPIKeyAPI("write")
	api.revokeFail["ak_old"] = true
	srv := httptest.NewServer(api.mux)
	defer srv.Close()

	wsDir, wsCfg, cred := setupRotateWorkspace(t, srv.URL)

	result, err := workspace.RotateKey(wsDir, wsCfg, cred)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Revoked {
		t.Error("expected Revoked=false when old key revocation fails")
	}
	if result.RevokeErr == nil {
		t.Error("expected RevokeErr to be set")
	}

	loaded, _ := workspace.LoadConfig(wsDir)
	if loaded.Credentials[0].APIKey != rotateNewKey {
		t.Errorf("expected config to hold new key, got %q", loaded.Credentials[0].APIKey)
	}
}
