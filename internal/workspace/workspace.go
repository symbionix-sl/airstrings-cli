package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
	"github.com/symbionix-sl/airstrings-cli/internal/guide"
)

const (
	DirName    = ".airstrings"
	ConfigFile = "config.json"
)

// Credential holds an API key and the environment it belongs to.
type Credential struct {
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url,omitempty"`
	EnvID     string `json:"env_id"`
	EnvName   string `json:"env_name"`
	PublicKey string `json:"public_key,omitempty"`
}

// SharedCredential holds the org shared-key bucket's scoped write key plus the
// project/environment it resolves to (cached at `shared init` time).
type SharedCredential struct {
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url,omitempty"`
	ProjectID string `json:"project_id"`
	EnvID     string `json:"env_id"`
}

// WorkspaceConfig is the project-local config stored in .airstrings/config.json.
type WorkspaceConfig struct {
	ProjectID   string            `json:"project_id"`
	ProjectName string            `json:"project_name"`
	ActiveEnv   string            `json:"active_env"`
	OrgID       string            `json:"org_id,omitempty"`
	BundlesDir  string            `json:"bundles_dir,omitempty"`
	Credentials []Credential      `json:"credentials"`
	Shared      *SharedCredential `json:"shared,omitempty"`
}

// ActiveCredential returns the credential matching the active environment.
func (c *WorkspaceConfig) ActiveCredential() (*Credential, error) {
	if c.ActiveEnv == "" {
		return nil, fmt.Errorf("no active environment — run: airstrings init <project-key>")
	}
	for i := range c.Credentials {
		if c.Credentials[i].EnvID == c.ActiveEnv {
			return &c.Credentials[i], nil
		}
	}
	return nil, fmt.Errorf("no credentials for environment %s — run: airstrings init <project-key>", c.ActiveEnv)
}

// FindByEnvID returns the credential for a given environment ID, or nil.
func (c *WorkspaceConfig) FindByEnvID(envID string) *Credential {
	for i := range c.Credentials {
		if c.Credentials[i].EnvID == envID {
			return &c.Credentials[i]
		}
	}
	return nil
}

// AddOrUpdate upserts a credential, keyed by env_id.
func (c *WorkspaceConfig) AddOrUpdate(cred Credential) {
	for i := range c.Credentials {
		if c.Credentials[i].EnvID == cred.EnvID {
			c.Credentials[i] = cred
			return
		}
	}
	c.Credentials = append(c.Credentials, cred)
}

// Remove deletes a credential by env_id. Returns true if found.
func (c *WorkspaceConfig) Remove(envID string) bool {
	for i := range c.Credentials {
		if c.Credentials[i].EnvID == envID {
			c.Credentials = append(c.Credentials[:i], c.Credentials[i+1:]...)
			return true
		}
	}
	return false
}

// Init creates a .airstrings/ workspace in the given directory.
func Init(dir string, cfg WorkspaceConfig) error {
	wsDir := filepath.Join(dir, DirName)
	if err := os.MkdirAll(wsDir, 0700); err != nil {
		return fmt.Errorf("create workspace dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	cfgPath := filepath.Join(wsDir, ConfigFile)
	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	gitignorePath := filepath.Join(wsDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		if err := os.WriteFile(gitignorePath, []byte("config.json\ndoctor.json\n"), 0600); err != nil {
			return fmt.Errorf("write gitignore: %w", err)
		}
	}

	return nil
}

// FindFrom walks up from the given directory looking for a .airstrings/config.json.
// Returns the .airstrings directory path, or an error if not found.
func FindFrom(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		wsDir := filepath.Join(dir, DirName)
		cfgPath := filepath.Join(wsDir, ConfigFile)
		if _, err := os.Stat(cfgPath); err == nil {
			return wsDir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}

	return "", fmt.Errorf("no .airstrings workspace found (searched up from %s)", startDir)
}

// Find walks up from the current working directory to find a workspace.
func Find() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return FindFrom(cwd)
}

// LoadConfig reads and parses the workspace config.json from a .airstrings directory.
// Handles migration from old format (env_id field → active_env).
func LoadConfig(wsDir string) (*WorkspaceConfig, error) {
	data, err := os.ReadFile(filepath.Join(wsDir, ConfigFile))
	if err != nil {
		return nil, fmt.Errorf("read workspace config: %w", err)
	}

	var cfg WorkspaceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse workspace config: %w", err)
	}

	// Migrate old format: if active_env is empty, check for legacy env_id field
	if cfg.ActiveEnv == "" {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err == nil {
			if envIDRaw, ok := raw["env_id"]; ok {
				var envID string
				if json.Unmarshal(envIDRaw, &envID) == nil {
					cfg.ActiveEnv = envID
				}
			}
		}
	}

	return &cfg, nil
}

// SaveConfig writes the workspace config back to .airstrings/config.json.
func SaveConfig(wsDir string, cfg *WorkspaceConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace config: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(wsDir, ConfigFile), data); err != nil {
		return fmt.Errorf("write workspace config: %w", err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// ResolveClient returns an API client for the workspace; see Resolve.
func ResolveClient(wsCfg *WorkspaceConfig) (*client.Client, error) {
	c, _, err := Resolve(wsCfg)
	return c, err
}

// EnvAuth holds credentials sourced from environment variables.
type EnvAuth struct {
	Source    string
	APIKey    string
	BaseURL   string
	ProjectID string
	EnvID     string
}

// EnvAuthFromEnv reads AIRSTRINGS_* variables. AIRSTRINGS_ORG_API_KEY wins
// over AIRSTRINGS_API_KEY; the bool is false when neither is set.
func EnvAuthFromEnv() (EnvAuth, bool) {
	source := "AIRSTRINGS_ORG_API_KEY"
	key := os.Getenv(source)
	if key == "" {
		source = "AIRSTRINGS_API_KEY"
		key = os.Getenv(source)
	}
	if key == "" {
		return EnvAuth{}, false
	}
	return EnvAuth{
		Source:    "env:" + source,
		APIKey:    key,
		BaseURL:   os.Getenv("AIRSTRINGS_BASE_URL"),
		ProjectID: os.Getenv("AIRSTRINGS_PROJECT_ID"),
		EnvID:     os.Getenv("AIRSTRINGS_ENV_ID"),
	}, true
}

// EnvOverride reports the environment AIRSTRINGS_ENV_ID selects when it
// differs from the active environment of the workspace in the current directory.
func EnvOverride() (string, bool) {
	env, ok := EnvAuthFromEnv()
	if !ok || env.EnvID == "" {
		return "", false
	}
	dir, err := Find()
	if err != nil {
		return "", false
	}
	cfg, err := LoadConfig(dir)
	if err != nil || cfg.ActiveEnv == env.EnvID {
		return "", false
	}
	if cred := cfg.FindByEnvID(env.EnvID); cred != nil {
		return cred.EnvName, true
	}
	return env.EnvID, true
}

// SharedClientFromEnv builds an API client for the org shared-key bucket from
// AIRSTRINGS_SHARED_API_KEY (+ optional AIRSTRINGS_BASE_URL). The scoped key
// self-identifies its project and default environment, resolved server-side in
// one or two API calls — no project/env IDs are read. The second return is
// false when AIRSTRINGS_SHARED_API_KEY is unset.
func SharedClientFromEnv() (*client.Client, bool, error) {
	key := os.Getenv("AIRSTRINGS_SHARED_API_KEY")
	if key == "" {
		return nil, false, nil
	}
	baseURL := os.Getenv("AIRSTRINGS_BASE_URL")

	proj, err := client.New(key, baseURL, "", "").GetProject()
	if err != nil {
		return nil, true, fmt.Errorf("resolve project from AIRSTRINGS_SHARED_API_KEY: %w", err)
	}

	envs, err := client.New(key, baseURL, proj.ID, "").ListEnvironments()
	if err != nil {
		return nil, true, fmt.Errorf("resolve environment from AIRSTRINGS_SHARED_API_KEY: %w", err)
	}
	envID, err := pickEnv(key, baseURL, proj.ID, envs)
	if err != nil {
		return nil, true, fmt.Errorf("resolve environment from AIRSTRINGS_SHARED_API_KEY: %w", err)
	}

	return client.New(key, baseURL, proj.ID, envID), true, nil
}

// ResolveSharedCredential resolves the shared bucket's project and default
// environment from a scoped write key, in one or two API calls.
func ResolveSharedCredential(apiKey, baseURL string) (*SharedCredential, error) {
	proj, err := client.New(apiKey, baseURL, "", "").GetProject()
	if err != nil {
		return nil, fmt.Errorf("resolve project from shared key: %w", err)
	}
	envs, err := client.New(apiKey, baseURL, proj.ID, "").ListEnvironments()
	if err != nil {
		return nil, fmt.Errorf("resolve environment from shared key: %w", err)
	}
	envID, err := pickEnv(apiKey, baseURL, proj.ID, envs)
	if err != nil {
		return nil, fmt.Errorf("resolve environment from shared key: %w", err)
	}
	return &SharedCredential{APIKey: apiKey, BaseURL: baseURL, ProjectID: proj.ID, EnvID: envID}, nil
}

// SharedClient builds a client for the org shared-key bucket. AIRSTRINGS_SHARED_API_KEY
// (env) wins; otherwise the stored `shared` credential in the nearest workspace
// config is used. Returns an error explaining `shared init` when neither is set.
func SharedClient() (*client.Client, error) {
	if c, ok, err := SharedClientFromEnv(); ok || err != nil {
		return c, err
	}
	wsDir, err := Find()
	if err != nil {
		return nil, fmt.Errorf("no shared-bucket key — run: airstrings shared init <api-key> (or set AIRSTRINGS_SHARED_API_KEY)")
	}
	cfg, err := LoadConfig(wsDir)
	if err != nil {
		return nil, err
	}
	if cfg.Shared == nil || cfg.Shared.APIKey == "" {
		return nil, fmt.Errorf("no shared-bucket key — run: airstrings shared init <api-key>")
	}
	s := cfg.Shared
	return client.New(s.APIKey, s.BaseURL, s.ProjectID, s.EnvID), nil
}

// BoundEnvs returns the environments a legacy environment key authenticates to.
// The error is set only when no environment matched and a probe failed for a
// reason other than 403/404.
func BoundEnvs(apiKey, baseURL, projectID string, envs []client.Environment) ([]client.Environment, error) {
	probe := client.New(apiKey, baseURL, projectID, "")
	var bound []client.Environment
	var probeErr error
	for _, env := range envs {
		full, err := probe.GetEnvironment(env.ID)
		if err != nil {
			var apiErr *client.APIError
			if !errors.As(err, &apiErr) || (apiErr.StatusCode != 404 && apiErr.StatusCode != 403) {
				probeErr = err
			}
			continue
		}
		bound = append(bound, *full)
	}
	if len(bound) == 0 {
		return nil, probeErr
	}
	return bound, nil
}

// pickEnv selects the environment a key works against: a legacy key's own
// environment, otherwise the open staging environment, else the default.
func pickEnv(apiKey, baseURL, projectID string, envs []client.Environment) (string, error) {
	if len(envs) == 0 {
		return "", fmt.Errorf("no environments found")
	}
	if client.KeyType(apiKey) == "environment" {
		bound, err := BoundEnvs(apiKey, baseURL, projectID, envs)
		if err != nil {
			return "", err
		}
		if len(bound) == 0 {
			return "", fmt.Errorf("API key does not authenticate to any environment in this project")
		}
		envs = bound
	} else if e := guide.OpenEnv(envs, ""); e != nil {
		return e.ID, nil
	}
	return defaultEnvID(envs), nil
}

func defaultEnvID(envs []client.Environment) string {
	for _, e := range envs {
		if e.IsDefault {
			return e.ID
		}
	}
	if len(envs) > 0 {
		return envs[0].ID
	}
	return ""
}

// DetectMode returns the workspace mode based on what files exist:
// "sections" if any section subdirectories with CSVs exist,
// "flat" if only strings.csv exists,
// "empty" if no CSVs are found.
func DetectMode(wsDir string) string {
	entries, err := os.ReadDir(wsDir)
	if err != nil {
		return "empty"
	}

	hasFlat := false
	for _, e := range entries {
		if e.IsDir() {
			csvPath := filepath.Join(wsDir, e.Name(), e.Name()+".csv")
			if _, err := os.Stat(csvPath); err == nil {
				return "sections"
			}
		}
		if e.Name() == "strings.csv" {
			hasFlat = true
		}
	}

	if hasFlat {
		return "flat"
	}
	return "empty"
}

// CreateSectionDir creates a section directory with an empty CSV file.
func CreateSectionDir(wsDir, name string) error {
	if err := ValidateSectionName(name); err != nil {
		return err
	}
	return WriteCSV(CSVPath(wsDir, name), nil)
}

// ValidateSectionName checks that a section name is safe for filesystem use.
func ValidateSectionName(name string) error {
	if name == "" {
		return fmt.Errorf("section name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("section name %q is not allowed", name)
	}
	if name == ConfigFile || name == "strings.csv" {
		return fmt.Errorf("section name %q conflicts with reserved file", name)
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("section name %q contains invalid characters", name)
	}
	if strings.HasPrefix(name, "..") {
		return fmt.Errorf("section name %q is not allowed", name)
	}
	return nil
}

// ValidateFormat checks that a string format is one of the supported values.
func ValidateFormat(format string) error {
	switch format {
	case "text", "icu":
		return nil
	default:
		return fmt.Errorf("invalid format %q — must be 'text' or 'icu'", format)
	}
}

// LooksLikeICU reports whether a value contains an ICU-style {…} placeholder.
func LooksLikeICU(value string) bool {
	open := strings.IndexByte(value, '{')
	if open < 0 {
		return false
	}
	return strings.IndexByte(value[open+1:], '}') >= 0
}

// FlagICUInText returns the sorted locales whose value looks like ICU but is
// declared as text format. Returns nil when format is not text or none match.
func FlagICUInText(format string, values map[string]string) []string {
	if format != "text" {
		return nil
	}
	var flagged []string
	for loc, val := range values {
		if LooksLikeICU(val) {
			flagged = append(flagged, loc)
		}
	}
	sort.Strings(flagged)
	return flagged
}
