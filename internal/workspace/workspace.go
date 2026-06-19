package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

const (
	DirName    = ".airstrings"
	ConfigFile = "config.json"
)

// Credential holds an API key and the environment it belongs to.
type Credential struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url,omitempty"`
	EnvID   string `json:"env_id"`
	EnvName string `json:"env_name"`
}

// WorkspaceConfig is the project-local config stored in .airstrings/config.json.
type WorkspaceConfig struct {
	ProjectID   string       `json:"project_id"`
	ProjectName string       `json:"project_name"`
	ActiveEnv   string       `json:"active_env"`
	BundlesDir  string       `json:"bundles_dir,omitempty"`
	Credentials []Credential `json:"credentials"`
}

// ActiveCredential returns the credential matching the active environment.
func (c *WorkspaceConfig) ActiveCredential() (*Credential, error) {
	if c.ActiveEnv == "" {
		return nil, fmt.Errorf("no active environment — run: airstrings env add <api-key>")
	}
	for i := range c.Credentials {
		if c.Credentials[i].EnvID == c.ActiveEnv {
			return &c.Credentials[i], nil
		}
	}
	return nil, fmt.Errorf("no credentials for environment %s — run: airstrings env add <api-key>", c.ActiveEnv)
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
	tmp, err := os.CreateTemp(wsDir, ConfigFile+".tmp-*")
	if err != nil {
		return fmt.Errorf("write workspace config: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write workspace config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write workspace config: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(wsDir, ConfigFile)); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write workspace config: %w", err)
	}
	return nil
}

// ResolveClient returns an API client configured from workspace credentials.
func ResolveClient(wsCfg *WorkspaceConfig) (*client.Client, error) {
	cred, err := wsCfg.ActiveCredential()
	if err != nil {
		return nil, err
	}
	return client.New(cred.APIKey, cred.BaseURL, wsCfg.ProjectID, wsCfg.ActiveEnv), nil
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
