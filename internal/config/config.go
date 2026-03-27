package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Profile holds credentials and settings for one project context.
type Profile struct {
	Name      string `json:"name"`
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	EnvID     string `json:"env_id,omitempty"`
}

// Config holds all CLI configuration.
type Config struct {
	ActiveProfile string             `json:"active_profile"`
	Profiles      map[string]Profile `json:"profiles"`
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

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Profiles: make(map[string]Profile)}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]Profile)
	}
	return &cfg, nil
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

func (c *Config) Active() (*Profile, error) {
	if c.ActiveProfile == "" {
		return nil, fmt.Errorf("no active profile — run: airstrings profile add <name>")
	}
	p, ok := c.Profiles[c.ActiveProfile]
	if !ok {
		return nil, fmt.Errorf("active profile %q not found in config", c.ActiveProfile)
	}
	return &p, nil
}
