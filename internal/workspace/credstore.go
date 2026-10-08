package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	LoopbackPort            int       `json:"loopback_port,omitempty"`
	Org                     string    `json:"-"`
}

// Creds is the user-level credential store (org keys from `airstrings login`).
type Creds struct {
	OrgKeys []OrgKey          `json:"org_keys"`
	Active  map[string]string `json:"active,omitempty"`
	Pending *PendingLogin     `json:"pending,omitempty"`
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

// OrgKey returns the key for orgID at baseURL, or with orgID "" the active
// org's key, else the most recently created one.
func (c *Creds) OrgKey(baseURL, orgID string) *OrgKey {
	if orgID == "" && c.Active[baseURL] != "" {
		if k := c.OrgKey(baseURL, c.Active[baseURL]); k != nil {
			return k
		}
	}
	var best *OrgKey
	for i := range c.OrgKeys {
		k := &c.OrgKeys[i]
		if k.BaseURL == baseURL && (orgID == "" || k.OrgID == orgID) && (best == nil || k.CreatedAt.After(best.CreatedAt)) {
			best = k
		}
	}
	return best
}

func (c *Creds) SetOrgKey(k OrgKey) {
	if old := c.OrgKey(k.BaseURL, k.OrgID); old != nil && old.OrgID == k.OrgID {
		*old = k
		return
	}
	c.OrgKeys = append(c.OrgKeys, k)
}

// UseOrg makes the stored org matching ref (ID or name) active for baseURL.
func (c *Creds) UseOrg(baseURL, ref string) (*OrgKey, error) {
	var known []string
	for i := range c.OrgKeys {
		k := &c.OrgKeys[i]
		if k.BaseURL != baseURL {
			continue
		}
		if k.OrgID == ref || strings.EqualFold(k.OrgName, ref) {
			if c.Active == nil {
				c.Active = map[string]string{}
			}
			c.Active[baseURL] = k.OrgID
			return k, nil
		}
		known = append(known, fmt.Sprintf("%s (%s)", k.OrgName, k.OrgID))
	}
	next := "Run: airstrings login"
	if len(known) > 0 {
		next = "Use one of: " + strings.Join(known, ", ") + ", or run airstrings login"
	}
	return nil, &UsageError{fmt.Sprintf("no stored sign-in for org %q", ref), next}
}

func (c *Creds) DeleteOrgKey(baseURL, orgID string) bool {
	for i := range c.OrgKeys {
		if c.OrgKeys[i].BaseURL == baseURL && c.OrgKeys[i].OrgID == orgID {
			c.OrgKeys = append(c.OrgKeys[:i], c.OrgKeys[i+1:]...)
			if c.Active[baseURL] == orgID {
				delete(c.Active, baseURL)
			}
			return true
		}
	}
	return false
}

// OrgName returns the stored name of orgID, or orgID when none is stored.
func OrgName(orgID string) string {
	if creds, err := LoadCreds(); err == nil {
		for _, k := range creds.OrgKeys {
			if k.OrgID == orgID && k.OrgName != "" {
				return k.OrgName
			}
		}
	}
	return orgID
}

func WrongOrgMessage(k *OrgKey, org string) string {
	return fmt.Sprintf("Approved for %s (%s), but this setup is for %s. Sign in to the dashboard of %s's organization and approve again.", k.OrgName, k.OrgID, org, org)
}

// PendingFor returns the unexpired pending login for baseURL, or nil.
func (c *Creds) PendingFor(baseURL string) *PendingLogin {
	if c.Pending == nil || c.Pending.BaseURL != baseURL || !time.Now().Before(c.Pending.ExpiresAt) {
		return nil
	}
	return c.Pending
}
