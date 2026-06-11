package workspace

import (
	"fmt"
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

	list, err := oldClient.ListAPIKeys()
	if err != nil {
		return nil, fmt.Errorf("list API keys: %w", err)
	}

	var own *client.APIKey
	for i := range list.Data {
		if list.Data[i].Prefix != "" && strings.HasPrefix(cred.APIKey, list.Data[i].Prefix) {
			own = &list.Data[i]
			break
		}
	}
	if own == nil {
		return nil, fmt.Errorf("could not identify the workspace API key among this environment's keys — rotate it via the dashboard instead")
	}
	if own.Permission != "write" {
		return nil, fmt.Errorf("cannot rotate a read-only key — write permission required to create keys")
	}

	newKey, err := oldClient.CreateAPIKey(client.CreateAPIKeyRequest{Name: own.Name, Permission: own.Permission})
	if err != nil {
		return nil, fmt.Errorf("create replacement key: %w", err)
	}

	oldAPIKey := cred.APIKey
	cred.APIKey = newKey.Key
	if err := SaveConfig(wsDir, wsCfg); err != nil {
		cred.APIKey = oldAPIKey
		if revokeErr := oldClient.RevokeAPIKey(newKey.ID); revokeErr != nil {
			return nil, fmt.Errorf("save config: %v — revoking the new key %s also failed (%v), revoke it manually via the dashboard", err, newKey.ID, revokeErr)
		}
		return nil, fmt.Errorf("save config: %w (new key revoked, old key still active)", err)
	}

	newClient := client.New(newKey.Key, cred.BaseURL, wsCfg.ProjectID, cred.EnvID)
	if _, err := newClient.GetProject(); err != nil {
		cred.APIKey = oldAPIKey
		saveErr := SaveConfig(wsDir, wsCfg)
		revokeErr := oldClient.RevokeAPIKey(newKey.ID)
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
	if err := newClient.RevokeAPIKey(own.ID); err != nil {
		result.RevokeErr = err
		return result, nil
	}
	result.Revoked = true
	return result, nil
}
