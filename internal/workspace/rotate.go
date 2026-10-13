package workspace

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

// RotateResult summarizes the outcome of an API key rotation.
type RotateResult struct {
	OldKeyID     string `json:"old_key_id"`
	NewKeyID     string `json:"new_key_id"`
	NewKeyPrefix string `json:"new_key_prefix"`
	Revoked      bool   `json:"revoked"`
	RevokeErr    error  `json:"-"`
}

// RotateKey creates a replacement API key for the credential, persists it,
// verifies it, and revokes the old key. The credential must point into wsCfg.
func RotateKey(wsDir string, wsCfg *WorkspaceConfig, cred *Credential) (*RotateResult, error) {
	oldClient := client.New(cred.APIKey, cred.BaseURL, wsCfg.ProjectID, cred.EnvID)
	if client.KeyType(cred.APIKey) != "environment" {
		return rotateProjectKey(wsDir, wsCfg, oldClient, cred.APIKey)
	}

	own, err := findOwnKey(oldClient, cred.APIKey)
	if err != nil {
		return nil, err
	}
	if own.Permission != "write" {
		return nil, fmt.Errorf("cannot rotate a read-only key — write permission required to create keys")
	}

	newKey, err := oldClient.CreateAPIKey(client.CreateAPIKeyRequest{Name: own.Name, Permission: own.Permission})
	if err != nil {
		return nil, fmt.Errorf("create replacement key: %w", err)
	}

	newClient := client.New(newKey.Key, cred.BaseURL, wsCfg.ProjectID, cred.EnvID)
	oldAPIKey := cred.APIKey
	cred.APIKey = newKey.Key
	if err := SaveConfig(wsDir, wsCfg); err != nil {
		cred.APIKey = oldAPIKey
		if revokeErr := newClient.RevokeAPIKey(newKey.ID); revokeErr != nil {
			return nil, fmt.Errorf("save config: %v — revoking the new key %s also failed (%v), revoke it manually via the dashboard", err, newKey.ID, revokeErr)
		}
		return nil, fmt.Errorf("save config: %w (new key revoked, old key still active)", err)
	}

	if _, err := newClient.GetProject(); err != nil {
		cred.APIKey = oldAPIKey
		saveErr := SaveConfig(wsDir, wsCfg)
		revokeErr := newClient.RevokeAPIKey(newKey.ID)
		msg := fmt.Sprintf("verify new key: %v", err)
		if saveErr != nil {
			msg += fmt.Sprintf(" — restoring the old key in config failed (%v)", saveErr)
		}
		if revokeErr != nil {
			msg += fmt.Sprintf(" — revoking the new key %s also failed (%v), revoke it manually via the dashboard", newKey.ID, revokeErr)
		}
		return nil, fmt.Errorf("%s", msg)
	}

	result := &RotateResult{
		OldKeyID:     own.ID,
		NewKeyID:     newKey.ID,
		NewKeyPrefix: newKey.Prefix,
	}
	if err := oldClient.RevokeAPIKey(own.ID); err != nil {
		result.RevokeErr = err
		return result, nil
	}
	result.Revoked = true
	return result, nil
}

// rotateProjectKey swaps the project key server-side (the old key dies in the
// same call) and replaces it in every credential.
func rotateProjectKey(wsDir string, wsCfg *WorkspaceConfig, c *client.Client, oldKey string) (*RotateResult, error) {
	newKey, err := c.RotateProjectKey()
	if err != nil {
		return nil, fmt.Errorf("rotate project key: %w", err)
	}
	for i := range wsCfg.Credentials {
		if wsCfg.Credentials[i].APIKey == oldKey {
			wsCfg.Credentials[i].APIKey = newKey.Key
		}
	}
	if err := SaveConfig(wsDir, wsCfg); err != nil {
		return nil, fmt.Errorf("save config: %w — the old key is already revoked; create a new project key in the dashboard", err)
	}
	return &RotateResult{NewKeyID: newKey.ID, NewKeyPrefix: newKey.Prefix, Revoked: true}, nil
}

func findOwnKey(c *client.Client, apiKey string) (*client.APIKey, error) {
	list, err := c.ListAPIKeys()
	if err != nil {
		return nil, fmt.Errorf("list API keys: %w", err)
	}
	for i := range list.Data {
		if list.Data[i].Prefix != "" && strings.HasPrefix(apiKey, list.Data[i].Prefix) {
			return &list.Data[i], nil
		}
	}
	return nil, fmt.Errorf("could not identify the workspace API key among the project's keys — revoke it via the dashboard instead")
}

// RevokeStoredKeys revokes every key stored in config.json (environment
// credentials and the shared-bucket key), clearing each one as it goes so the
// workspace falls back to the login key. A key the API already rejects (401)
// is only cleared. It returns the revoked key IDs and the number of dead keys.
func RevokeStoredKeys(wsDir string, wsCfg *WorkspaceConfig) ([]string, int, error) {
	var revoked []string
	dead := 0
	revoke := func(c *client.Client, key string, clear func()) error {
		own, err := findOwnKey(c, key)
		var apiErr *client.APIError
		switch {
		case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized:
			dead++
		case err != nil:
			return err
		default:
			if err := c.RevokeAPIKey(own.ID); err != nil {
				return err
			}
			revoked = append(revoked, own.ID)
		}
		clear()
		if err := SaveConfig(wsDir, wsCfg); err != nil {
			return fmt.Errorf("save config: %w — the key no longer works; remove it from config.json by hand", err)
		}
		return nil
	}
	for i := range wsCfg.Credentials {
		cred := wsCfg.Credentials[i]
		if cred.APIKey == "" {
			continue
		}
		err := revoke(client.New(cred.APIKey, cred.BaseURL, wsCfg.ProjectID, cred.EnvID), cred.APIKey, func() {
			for j := range wsCfg.Credentials {
				if wsCfg.Credentials[j].APIKey == cred.APIKey {
					wsCfg.Credentials[j].APIKey = ""
				}
			}
		})
		if err != nil {
			return revoked, dead, err
		}
	}
	if s := wsCfg.Shared; s != nil && s.APIKey != "" {
		if err := revoke(client.New(s.APIKey, s.BaseURL, s.ProjectID, s.EnvID), s.APIKey, func() { wsCfg.Shared = nil }); err != nil {
			return revoked, dead, err
		}
	}
	return revoked, dead, nil
}
