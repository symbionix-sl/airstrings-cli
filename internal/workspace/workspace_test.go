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
		ProjectID:   "proj-123",
		ProjectName: "Test Project",
		ActiveEnv:   "env-456",
		Credentials: []Credential{
			{APIKey: "key1", EnvID: "env-456", EnvName: "production"},
		},
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
	if loaded.ProjectID != "proj-123" || loaded.ActiveEnv != "env-456" {
		t.Errorf("unexpected config: %+v", loaded)
	}
	if len(loaded.Credentials) != 1 || loaded.Credentials[0].APIKey != "key1" {
		t.Errorf("unexpected credentials: %+v", loaded.Credentials)
	}
}

func TestInit_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	cfg := WorkspaceConfig{ProjectID: "p1", ActiveEnv: "e1"}

	Init(dir, cfg)
	// Init again should overwrite without error
	cfg.ActiveEnv = "e2"
	if err := Init(dir, cfg); err != nil {
		t.Fatalf("unexpected error on re-init: %v", err)
	}

	loaded, _ := LoadConfig(filepath.Join(dir, ".airstrings"))
	if loaded.ActiveEnv != "e2" {
		t.Errorf("expected updated env ID e2, got %q", loaded.ActiveEnv)
	}
}

func TestInit_GitignoreProtectsSecrets(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, WorkspaceConfig{ProjectID: "p1", ActiveEnv: "e1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	giPath := filepath.Join(dir, ".airstrings", ".gitignore")

	got, err := os.ReadFile(giPath)
	if err != nil {
		t.Fatalf("gitignore missing: %v", err)
	}
	if string(got) != "config.json\ndoctor.json\n" {
		t.Errorf("unexpected gitignore content: %q", got)
	}

	// Re-init must not clobber a user-customized gitignore.
	custom := []byte("config.json\ndoctor.json\ncustom\n")
	if err := os.WriteFile(giPath, custom, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir, WorkspaceConfig{ProjectID: "p1", ActiveEnv: "e2"}); err != nil {
		t.Fatalf("unexpected error on re-init: %v", err)
	}
	got, _ = os.ReadFile(giPath)
	if string(got) != string(custom) {
		t.Errorf("re-init clobbered gitignore: %q", got)
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	cfg := WorkspaceConfig{
		ProjectID: "p1",
		ActiveEnv: "e1",
		Credentials: []Credential{
			{APIKey: "key1", BaseURL: "https://custom.api", EnvID: "e1", EnvName: "prod"},
		},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(filepath.Join(wsDir, "config.json"), data, 0600)

	loaded, err := LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.ProjectID != "p1" || loaded.Credentials[0].BaseURL != "https://custom.api" {
		t.Errorf("unexpected config: %+v", loaded)
	}
}

func TestLoadConfig_OldFormat(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	// Old format with env_id instead of active_env
	old := `{"project_id":"p1","env_id":"e1","base_url":"https://api.example.com"}`
	os.WriteFile(filepath.Join(wsDir, "config.json"), []byte(old), 0600)

	loaded, err := LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.ProjectID != "p1" {
		t.Errorf("expected project_id p1, got %q", loaded.ProjectID)
	}
	if loaded.ActiveEnv != "e1" {
		t.Errorf("expected active_env migrated from env_id to e1, got %q", loaded.ActiveEnv)
	}
	if len(loaded.Credentials) != 0 {
		t.Errorf("expected empty credentials for old format, got %d", len(loaded.Credentials))
	}
}

func TestSaveConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := WorkspaceConfig{
		ProjectID: "p1",
		ActiveEnv: "e1",
		Credentials: []Credential{
			{APIKey: "key1", EnvID: "e1", EnvName: "prod"},
		},
	}
	if err := Init(dir, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wsDir := filepath.Join(dir, ".airstrings")

	cfg.Credentials[0].APIKey = "key2"
	if err := SaveConfig(wsDir, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if loaded.Credentials[0].APIKey != "key2" {
		t.Errorf("expected key2, got %q", loaded.Credentials[0].APIKey)
	}
}

func TestSaveConfig_AtomicAndPermissions(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, WorkspaceConfig{ProjectID: "p1", ActiveEnv: "e1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wsDir := filepath.Join(dir, ".airstrings")

	cfg := &WorkspaceConfig{ProjectID: "p1", ActiveEnv: "e2"}
	if err := SaveConfig(wsDir, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(filepath.Join(wsDir, "config.json"))
	if err != nil {
		t.Fatalf("config.json missing: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected 0600 file permissions, got %o", perm)
	}

	entries, err := os.ReadDir(wsDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "config.json" && e.Name() != ".gitignore" {
			t.Errorf("unexpected leftover file: %s", e.Name())
		}
	}

	loaded, err := LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if loaded.ActiveEnv != "e2" {
		t.Errorf("expected e2, got %q", loaded.ActiveEnv)
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

func TestValidateFormat(t *testing.T) {
	for _, f := range []string{"text", "icu"} {
		if err := ValidateFormat(f); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", f, err)
		}
	}

	for _, f := range []string{"", "xml", "TEXT", "ICU", "json", "plain"} {
		if err := ValidateFormat(f); err == nil {
			t.Errorf("expected %q to be invalid", f)
		}
	}
}

func TestLooksLikeICU(t *testing.T) {
	icu := []string{"Hi {name}", "{count, plural, one {# item} other {# items}}", "a {0} b", "{}"}
	for _, v := range icu {
		if !LooksLikeICU(v) {
			t.Errorf("expected %q to look like ICU", v)
		}
	}

	plain := []string{"Hello", "100% sure", "a { b", "c } d", "", "no braces here"}
	for _, v := range plain {
		if LooksLikeICU(v) {
			t.Errorf("expected %q to not look like ICU", v)
		}
	}
}

func TestFlagICUInText(t *testing.T) {
	values := map[string]string{"en": "Hi {name}", "it": "Ciao", "de": "Hallo {name}"}

	if got := FlagICUInText("icu", values); got != nil {
		t.Errorf("expected nil for icu format, got %v", got)
	}

	got := FlagICUInText("text", values)
	want := []string{"de", "en"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("expected %v, got %v", want, got)
	}

	if got := FlagICUInText("text", map[string]string{"en": "Hello"}); got != nil {
		t.Errorf("expected nil when no braces, got %v", got)
	}
}

// --- Credential method tests ---

func TestActiveCredential(t *testing.T) {
	cfg := &WorkspaceConfig{
		ActiveEnv: "env_1",
		Credentials: []Credential{
			{APIKey: "key1", EnvID: "env_1"},
			{APIKey: "key2", EnvID: "env_2"},
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
	cfg := &WorkspaceConfig{ActiveEnv: "env_missing"}
	_, err := cfg.ActiveCredential()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestActiveCredential_NotLoggedIn(t *testing.T) {
	cfg := &WorkspaceConfig{}
	_, err := cfg.ActiveCredential()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAddOrUpdate_New(t *testing.T) {
	cfg := &WorkspaceConfig{}
	cfg.AddOrUpdate(Credential{APIKey: "key1", EnvID: "env_1"})
	if len(cfg.Credentials) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(cfg.Credentials))
	}
}

func TestAddOrUpdate_Existing(t *testing.T) {
	cfg := &WorkspaceConfig{
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
	cfg := &WorkspaceConfig{
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
	cfg := &WorkspaceConfig{}
	if cfg.Remove("env_missing") {
		t.Error("expected Remove to return false")
	}
}

func TestFindByEnvID(t *testing.T) {
	cfg := &WorkspaceConfig{
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
