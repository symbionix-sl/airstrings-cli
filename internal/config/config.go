package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Credential holds an API key and the project+environment it belongs to.
type Credential struct {
	APIKey      string `json:"api_key"`
	BaseURL     string `json:"base_url,omitempty"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	EnvID       string `json:"env_id"`
	EnvName     string `json:"env_name"`
}

// Config holds all CLI configuration.
type Config struct {
	ActiveProject string       `json:"active_project"`
	ActiveEnv     string       `json:"active_env"`
	Credentials   []Credential `json:"credentials"`
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".airstrings"), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// legacy types for migration from profile-based config
type legacyProfile struct {
	Name      string `json:"name"`
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	EnvID     string `json:"env_id,omitempty"`
}

type legacyConfig struct {
	ActiveProfile string                    `json:"active_profile"`
	Profiles      map[string]legacyProfile  `json:"profiles"`
}

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Detect format: legacy has "profiles" key, new has "credentials" key
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if _, hasProfiles := probe["profiles"]; hasProfiles {
		return migrateFromLegacy(data)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func migrateFromLegacy(data []byte) (*Config, error) {
	var legacy legacyConfig
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parse legacy config: %w", err)
	}

	cfg := &Config{}
	for _, prof := range legacy.Profiles {
		cred := Credential{
			APIKey:    prof.APIKey,
			BaseURL:   prof.BaseURL,
			ProjectID: prof.ProjectID,
			EnvID:     prof.EnvID,
		}
		cfg.Credentials = append(cfg.Credentials, cred)
	}

	// Set active from the legacy active profile
	if active, ok := legacy.Profiles[legacy.ActiveProfile]; ok {
		cfg.ActiveProject = active.ProjectID
		cfg.ActiveEnv = active.EnvID
	}

	// Write migrated config
	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("save migrated config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(p, data, 0600)
}

// ActiveCredential returns the credential matching the active project + env.
func (c *Config) ActiveCredential() (*Credential, error) {
	if c.ActiveProject == "" || c.ActiveEnv == "" {
		return nil, fmt.Errorf("not logged in — run: airstrings login <api-key>")
	}
	for i := range c.Credentials {
		if c.Credentials[i].ProjectID == c.ActiveProject && c.Credentials[i].EnvID == c.ActiveEnv {
			return &c.Credentials[i], nil
		}
	}
	return nil, fmt.Errorf("no credentials for project %s env %s — run: airstrings login <api-key>", c.ActiveProject, c.ActiveEnv)
}

// FindByEnvID returns the credential for a given environment ID, or nil.
func (c *Config) FindByEnvID(envID string) *Credential {
	for i := range c.Credentials {
		if c.Credentials[i].EnvID == envID {
			return &c.Credentials[i]
		}
	}
	return nil
}

// FindByProjectID returns all credentials for a given project.
func (c *Config) FindByProjectID(projectID string) []Credential {
	var result []Credential
	for _, cred := range c.Credentials {
		if cred.ProjectID == projectID {
			result = append(result, cred)
		}
	}
	return result
}

// AddOrUpdate upserts a credential, keyed by env_id.
func (c *Config) AddOrUpdate(cred Credential) {
	for i := range c.Credentials {
		if c.Credentials[i].EnvID == cred.EnvID {
			c.Credentials[i] = cred
			return
		}
	}
	c.Credentials = append(c.Credentials, cred)
}

// Remove deletes a credential by env_id. Returns true if found.
func (c *Config) Remove(envID string) bool {
	for i := range c.Credentials {
		if c.Credentials[i].EnvID == envID {
			c.Credentials = append(c.Credentials[:i], c.Credentials[i+1:]...)
			return true
		}
	}
	return false
}

// Projects returns unique project IDs with their names from stored credentials.
func (c *Config) Projects() map[string]string {
	projects := make(map[string]string)
	for _, cred := range c.Credentials {
		if _, ok := projects[cred.ProjectID]; !ok {
			projects[cred.ProjectID] = cred.ProjectName
		}
	}
	return projects
}
