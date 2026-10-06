package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrCredStore = errors.New("credential store is not writable")

const CredStoreNextStep = "Set XDG_CONFIG_HOME to a writable directory, or set AIRSTRINGS_ORG_API_KEY"

type OrgKey struct {
	BaseURL   string    `json:"base_url"`
	OrgID     string    `json:"org_id"`
	OrgName   string    `json:"org_name"`
	KeyID     string    `json:"key_id"`
	APIKey    string    `json:"api_key"`
	FullPower bool      `json:"full_power"`
	CreatedAt time.Time `json:"created_at"`
}

type PendingLogin struct {
	BaseURL                 string    `json:"base_url"`
	DeviceCode              string    `json:"device_code"`
	UserCode                string    `json:"user_code"`
	VerificationURIComplete string    `json:"verification_uri_complete"`
	Interval                int       `json:"interval"`
	ExpiresAt               time.Time `json:"expires_at"`
}

// Creds is the user-level credential store (org keys from `airstrings login`).
type Creds struct {
	OrgKeys []OrgKey      `json:"org_keys"`
	Pending *PendingLogin `json:"pending,omitempty"`
}

func CredentialsPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "airstrings", "credentials.json")
}

func LoadCreds() (*Creds, error) {
	var creds Creds
	data, err := os.ReadFile(CredentialsPath())
	if errors.Is(err, os.ErrNotExist) {
		return &creds, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse %s: %w", CredentialsPath(), err)
	}
	return &creds, nil
}

func SaveCreds(creds *Creds) error {
	path := CredentialsPath()
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("%w: %v", ErrCredStore, err)
	}
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("%w: %v", ErrCredStore, err)
	}
	return nil
}

func (c *Creds) OrgKey(baseURL string) *OrgKey {
	for i := range c.OrgKeys {
		if c.OrgKeys[i].BaseURL == baseURL {
			return &c.OrgKeys[i]
		}
	}
	return nil
}

func (c *Creds) SetOrgKey(k OrgKey) {
	if old := c.OrgKey(k.BaseURL); old != nil {
		*old = k
		return
	}
	c.OrgKeys = append(c.OrgKeys, k)
}

func (c *Creds) DeleteOrgKey(baseURL string) bool {
	for i := range c.OrgKeys {
		if c.OrgKeys[i].BaseURL == baseURL {
			c.OrgKeys = append(c.OrgKeys[:i], c.OrgKeys[i+1:]...)
			return true
		}
	}
	return false
}

// PendingFor returns the unexpired pending login for baseURL, or nil.
func (c *Creds) PendingFor(baseURL string) *PendingLogin {
	if c.Pending == nil || c.Pending.BaseURL != baseURL || !time.Now().Before(c.Pending.ExpiresAt) {
		return nil
	}
	return c.Pending
}
