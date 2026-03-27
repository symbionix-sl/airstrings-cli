package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	dir := t.TempDir()

	cfg := WorkspaceConfig{
		Profile:   "test",
		ProjectID: "proj-123",
		EnvID:     "env-456",
	}
	if err := Init(dir, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify .airstrings dir exists with correct permissions
	wsDir := filepath.Join(dir, ".airstrings")
	info, err := os.Stat(wsDir)
	if err != nil {
		t.Fatalf(".airstrings dir not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("expected 0700 dir permissions, got %o", perm)
	}

	// Verify config.json exists with correct permissions
	cfgPath := filepath.Join(wsDir, "config.json")
	cfgInfo, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("config.json not created: %v", err)
	}
	if perm := cfgInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("expected 0600 file permissions, got %o", perm)
	}

	// Verify config content
	loaded, err := LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if loaded.Profile != "test" || loaded.ProjectID != "proj-123" || loaded.EnvID != "env-456" {
		t.Errorf("unexpected config: %+v", loaded)
	}
}

func TestInit_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	cfg := WorkspaceConfig{Profile: "test", ProjectID: "p1", EnvID: "e1"}

	Init(dir, cfg)
	// Init again should overwrite without error
	cfg.EnvID = "e2"
	if err := Init(dir, cfg); err != nil {
		t.Fatalf("unexpected error on re-init: %v", err)
	}

	loaded, _ := LoadConfig(filepath.Join(dir, ".airstrings"))
	if loaded.EnvID != "e2" {
		t.Errorf("expected updated env ID e2, got %q", loaded.EnvID)
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	cfg := WorkspaceConfig{Profile: "prod", ProjectID: "p1", EnvID: "e1", BaseURL: "https://custom.api"}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(filepath.Join(wsDir, "config.json"), data, 0600)

	loaded, err := LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.Profile != "prod" || loaded.BaseURL != "https://custom.api" {
		t.Errorf("unexpected config: %+v", loaded)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/.airstrings")
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadConfig_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)
	os.WriteFile(filepath.Join(wsDir, "config.json"), []byte("{bad json"), 0600)

	_, err := LoadConfig(wsDir)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestFind_InCurrentDir(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)
	os.WriteFile(filepath.Join(wsDir, "config.json"), []byte("{}"), 0600)

	found, err := FindFrom(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != wsDir {
		t.Errorf("expected %q, got %q", wsDir, found)
	}
}

func TestFind_InParentDir(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)
	os.WriteFile(filepath.Join(wsDir, "config.json"), []byte("{}"), 0600)

	// Create a subdirectory and search from there
	subDir := filepath.Join(dir, "src", "components")
	os.MkdirAll(subDir, 0700)

	found, err := FindFrom(subDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != wsDir {
		t.Errorf("expected %q, got %q", wsDir, found)
	}
}

func TestFind_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := FindFrom(dir)
	if err == nil {
		t.Fatal("expected error when no workspace found")
	}
}

func TestDetectMode_Empty(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	mode := DetectMode(wsDir)
	if mode != "empty" {
		t.Errorf("expected 'empty', got %q", mode)
	}
}

func TestDetectMode_Flat(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)
	os.WriteFile(filepath.Join(wsDir, "strings.csv"), []byte("key,locale,value,format\n"), 0600)

	mode := DetectMode(wsDir)
	if mode != "flat" {
		t.Errorf("expected 'flat', got %q", mode)
	}
}

func TestDetectMode_Sections(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	secDir := filepath.Join(wsDir, "home")
	os.MkdirAll(secDir, 0700)
	os.WriteFile(filepath.Join(secDir, "home.csv"), []byte("key,locale,value,format\n"), 0600)

	mode := DetectMode(wsDir)
	if mode != "sections" {
		t.Errorf("expected 'sections', got %q", mode)
	}
}

func TestCreateSectionDir(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	if err := CreateSectionDir(wsDir, "home"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify dir exists
	secDir := filepath.Join(wsDir, "home")
	info, err := os.Stat(secDir)
	if err != nil {
		t.Fatalf("section dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}

	// Verify CSV with header exists
	csvPath := filepath.Join(secDir, "home.csv")
	rows, err := ReadCSV(csvPath)
	if err != nil {
		t.Fatalf("failed to read CSV: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty CSV, got %d rows", len(rows))
	}
}

func TestValidateSectionName(t *testing.T) {
	valid := []string{"home", "login", "my-section", "section_1", "évolution"}
	for _, name := range valid {
		if err := ValidateSectionName(name); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", name, err)
		}
	}

	invalid := []string{"", ".", "..", "../escape", "a/b", "a\\b", "config.json", "strings.csv"}
	for _, name := range invalid {
		if err := ValidateSectionName(name); err == nil {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}
