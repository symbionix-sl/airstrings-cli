package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Credentials) != 0 {
		t.Errorf("expected empty credentials, got %d", len(cfg.Credentials))
	}
}

func TestLoad_NewFormat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(cfgDir, 0700)

	data := `{
		"active_project": "proj_1",
		"active_env": "env_1",
		"credentials": [
			{"api_key": "key1", "project_id": "proj_1", "env_id": "env_1", "env_name": "production"}
		]
	}`
	os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(data), 0600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Credentials) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(cfg.Credentials))
	}
	if cfg.Credentials[0].APIKey != "key1" {
		t.Errorf("expected key1, got %s", cfg.Credentials[0].APIKey)
	}
	if cfg.ActiveProject != "proj_1" {
		t.Errorf("expected proj_1, got %s", cfg.ActiveProject)
	}
}

func TestLoad_LegacyMigration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(cfgDir, 0700)

	legacy := `{
		"active_profile": "staging",
		"profiles": {
			"staging": {
				"name": "staging",
				"api_key": "old_key",
				"base_url": "https://api-staging.example.com",
				"project_id": "proj_old",
				"env_id": "env_old"
			}
		}
	}`
	cfgPath := filepath.Join(cfgDir, "config.json")
	os.WriteFile(cfgPath, []byte(legacy), 0600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Credentials) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(cfg.Credentials))
	}
	if cfg.Credentials[0].APIKey != "old_key" {
		t.Errorf("expected old_key, got %s", cfg.Credentials[0].APIKey)
	}
	if cfg.ActiveProject != "proj_old" {
		t.Errorf("expected proj_old, got %s", cfg.ActiveProject)
	}
	if cfg.ActiveEnv != "env_old" {
		t.Errorf("expected env_old, got %s", cfg.ActiveEnv)
	}

	// Verify file was rewritten in new format
	data, _ := os.ReadFile(cfgPath)
	var rewritten map[string]json.RawMessage
	json.Unmarshal(data, &rewritten)
	if _, ok := rewritten["credentials"]; !ok {
		t.Error("migrated file should have 'credentials' key")
	}
	if _, ok := rewritten["profiles"]; ok {
		t.Error("migrated file should not have 'profiles' key")
	}
}

func TestActiveCredential(t *testing.T) {
	cfg := &Config{
		ActiveProject: "proj_1",
		ActiveEnv:     "env_1",
		Credentials: []Credential{
			{APIKey: "key1", ProjectID: "proj_1", EnvID: "env_1"},
			{APIKey: "key2", ProjectID: "proj_1", EnvID: "env_2"},
		},
	}

	cred, err := cfg.ActiveCredential()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.APIKey != "key1" {
		t.Errorf("expected key1, got %s", cred.APIKey)
	}
}

func TestActiveCredential_NotFound(t *testing.T) {
	cfg := &Config{ActiveProject: "proj_1", ActiveEnv: "env_missing"}
	_, err := cfg.ActiveCredential()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestActiveCredential_NotLoggedIn(t *testing.T) {
	cfg := &Config{}
	_, err := cfg.ActiveCredential()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAddOrUpdate_New(t *testing.T) {
	cfg := &Config{}
	cfg.AddOrUpdate(Credential{APIKey: "key1", EnvID: "env_1"})
	if len(cfg.Credentials) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(cfg.Credentials))
	}
}

func TestAddOrUpdate_Existing(t *testing.T) {
	cfg := &Config{
		Credentials: []Credential{
			{APIKey: "old_key", EnvID: "env_1"},
		},
	}
	cfg.AddOrUpdate(Credential{APIKey: "new_key", EnvID: "env_1"})
	if len(cfg.Credentials) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(cfg.Credentials))
	}
	if cfg.Credentials[0].APIKey != "new_key" {
		t.Errorf("expected new_key, got %s", cfg.Credentials[0].APIKey)
	}
}

func TestRemove(t *testing.T) {
	cfg := &Config{
		Credentials: []Credential{
			{APIKey: "key1", EnvID: "env_1"},
			{APIKey: "key2", EnvID: "env_2"},
		},
	}
	if !cfg.Remove("env_1") {
		t.Error("expected Remove to return true")
	}
	if len(cfg.Credentials) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(cfg.Credentials))
	}
	if cfg.Credentials[0].EnvID != "env_2" {
		t.Errorf("expected env_2 remaining, got %s", cfg.Credentials[0].EnvID)
	}
}

func TestRemove_NotFound(t *testing.T) {
	cfg := &Config{}
	if cfg.Remove("env_missing") {
		t.Error("expected Remove to return false")
	}
}

func TestFindByEnvID(t *testing.T) {
	cfg := &Config{
		Credentials: []Credential{
			{APIKey: "key1", EnvID: "env_1"},
		},
	}
	if cred := cfg.FindByEnvID("env_1"); cred == nil {
		t.Error("expected to find credential")
	}
	if cred := cfg.FindByEnvID("env_missing"); cred != nil {
		t.Error("expected nil for missing env")
	}
}

func TestFindByProjectID(t *testing.T) {
	cfg := &Config{
		Credentials: []Credential{
			{APIKey: "key1", ProjectID: "proj_1", EnvID: "env_1"},
			{APIKey: "key2", ProjectID: "proj_1", EnvID: "env_2"},
			{APIKey: "key3", ProjectID: "proj_2", EnvID: "env_3"},
		},
	}
	creds := cfg.FindByProjectID("proj_1")
	if len(creds) != 2 {
		t.Errorf("expected 2 credentials for proj_1, got %d", len(creds))
	}
}

func TestProjects(t *testing.T) {
	cfg := &Config{
		Credentials: []Credential{
			{ProjectID: "proj_1", ProjectName: "App1", EnvID: "env_1"},
			{ProjectID: "proj_1", ProjectName: "App1", EnvID: "env_2"},
			{ProjectID: "proj_2", ProjectName: "App2", EnvID: "env_3"},
		},
	}
	projects := cfg.Projects()
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
	if projects["proj_1"] != "App1" {
		t.Errorf("expected App1, got %s", projects["proj_1"])
	}
}
