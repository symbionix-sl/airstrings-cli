package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListAPIKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/projects/proj/environments/env/api-keys" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("unexpected X-API-Key header: %q", r.Header.Get("X-API-Key"))
		}
		json.NewEncoder(w).Encode(APIKeyList{
			Data: []APIKey{
				{ID: "ak_1", Name: "CI", Permission: "write", Prefix: "airstrin"},
				{ID: "ak_2", Name: "Readonly", Permission: "read", Prefix: "airstrra"},
			},
		})
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	list, err := c.ListAPIKeys()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list.Data) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(list.Data))
	}
	if list.Data[0].ID != "ak_1" || list.Data[0].Permission != "write" {
		t.Errorf("unexpected first key: %+v", list.Data[0])
	}
	if list.Data[0].LastUsedAt != nil {
		t.Errorf("expected nil last_used_at, got %v", list.Data[0].LastUsedAt)
	}
}

func TestCreateAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/projects/proj/environments/env/api-keys" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("unexpected X-API-Key header: %q", r.Header.Get("X-API-Key"))
		}
		var req CreateAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Name != "CI" || req.Permission != "write" {
			t.Errorf("unexpected request body: %+v", req)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(APIKeyCreated{
			ID:         "ak_new",
			Name:       req.Name,
			Permission: req.Permission,
			Key:        "airstrings_v1_proj_new_w_secret",
			Prefix:     "airstrin",
		})
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	created, err := c.CreateAPIKey(CreateAPIKeyRequest{Name: "CI", Permission: "write"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.ID != "ak_new" || created.Key != "airstrings_v1_proj_new_w_secret" {
		t.Errorf("unexpected response: %+v", created)
	}
}

func TestRevokeAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/v1/projects/proj/environments/env/api-keys/ak_1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("unexpected X-API-Key header: %q", r.Header.Get("X-API-Key"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	if err := c.RevokeAPIKey("ak_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListAPIKeys_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: ErrorBody{Code: "unauthorized", Message: "Invalid API key"},
		})
	}))
	defer srv.Close()

	c := New("bad-key", srv.URL, "proj", "env")
	_, err := c.ListAPIKeys()
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", apiErr.StatusCode)
	}
}
