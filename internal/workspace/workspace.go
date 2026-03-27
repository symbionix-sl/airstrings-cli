package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/symbionix/airstrings-cli/internal/client"
	"github.com/symbionix/airstrings-cli/internal/config"
)

const (
	DirName    = ".airstrings"
	ConfigFile = "config.json"
)

// WorkspaceConfig is the project-local config stored in .airstrings/config.json.
type WorkspaceConfig struct {
	Profile   string `json:"profile"`
	ProjectID string `json:"project_id"`
	EnvID     string `json:"env_id"`
	BaseURL   string `json:"base_url,omitempty"`
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
func LoadConfig(wsDir string) (*WorkspaceConfig, error) {
	data, err := os.ReadFile(filepath.Join(wsDir, ConfigFile))
	if err != nil {
		return nil, fmt.Errorf("read workspace config: %w", err)
	}

	var cfg WorkspaceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse workspace config: %w", err)
	}

	return &cfg, nil
}

// ResolveClient loads the global config, finds the referenced profile,
// and returns an API client configured for this workspace.
func ResolveClient(wsCfg *WorkspaceConfig) (*client.Client, error) {
	globalCfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load global config: %w", err)
	}

	profile, ok := globalCfg.Profiles[wsCfg.Profile]
	if !ok {
		return nil, fmt.Errorf("profile %q not found in ~/.airstrings/config.json", wsCfg.Profile)
	}

	baseURL := wsCfg.BaseURL
	if baseURL == "" {
		baseURL = profile.BaseURL
	}

	return client.New(profile.APIKey, baseURL, wsCfg.ProjectID, wsCfg.EnvID), nil
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
