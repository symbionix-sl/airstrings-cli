package guide

import (
	"strings"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

type Details struct {
	Protection      string            `json:"protection"`
	ProtectionByEnv map[string]string `json:"protection_by_env,omitempty"`
	KeyScope        string            `json:"key_scope"`
	Hint            *Hint             `json:"hint"`
	Summary         string            `json:"-"`
}

type EnvRow struct {
	client.Environment
	Protection     string `json:"protection"`
	KeyInWorkspace bool   `json:"key_in_workspace"`
	Active         bool   `json:"active"`
}

func EnvRows(envs []client.Environment, hasKey func(envID string) bool, activeID string) []EnvRow {
	rows := make([]EnvRow, len(envs))
	for i, e := range envs {
		rows[i] = EnvRow{e, Protection(e.IsSealed), hasKey(e.ID), e.ID == activeID}
	}
	return rows
}

func ProtectionSummary(envs []client.Environment) string {
	parts := make([]string, len(envs))
	for i, e := range envs {
		parts[i] = client.StripControl(e.Name) + ": " + Protection(e.IsSealed)
	}
	return strings.Join(parts, " · ")
}

func SDKEnvironment(e client.SDKEnvironment) map[string]any {
	return map[string]any{
		"id":          e.ID,
		"name":        e.Name,
		"is_default":  e.IsDefault,
		"protected":   e.IsSealed,
		"public_keys": e.PublicKeys,
	}
}

// Inspect derives key scope, per-environment protection and the state hint.
// Best-effort: a nil client or failed call degrades to "unknown".
func Inspect(c *client.Client, apiKey string, hasKey func(envID string) bool) Details {
	d := Details{Protection: "unknown", KeyScope: "unknown"}
	if c == nil {
		return d
	}
	if keys, err := c.ListAPIKeys(); err == nil && len(apiKey) >= 8 {
		for _, k := range keys.Data {
			if k.Prefix == apiKey[:8] {
				d.KeyScope = k.Permission
			}
		}
	}
	envs, err := c.ListEnvironments()
	if err != nil {
		return d
	}
	var active, def *client.Environment
	d.ProtectionByEnv = map[string]string{}
	for i := range envs {
		d.ProtectionByEnv[envs[i].Name] = Protection(envs[i].IsSealed)
		if envs[i].ID == c.EnvID() {
			active = &envs[i]
		}
		if envs[i].IsDefault {
			def = &envs[i]
		}
	}
	d.Summary = ProtectionSummary(envs)
	if def == nil || active == nil {
		return d
	}
	d.Protection = Protection(def.IsSealed)
	has := func(id string) bool { return id == active.ID || (hasKey != nil && hasKey(id)) }
	dash := c.DashboardURL("")
	promote := PromoteURL(dash, c.ProjectID(), def.ID)
	h := Status(active.Name, active.IsDefault, def.Name, def.IsSealed, has(def.ID), promote, APIKeysURL(dash, c.ProjectID(), def.ID))
	if active.IsDefault && def.IsSealed && !otherKey(envs, def.ID, has) {
		if st := stagingEnv(envs, def.ID); st != nil {
			h = NeedStagingKey(def.Name, st.Name, APIKeysURL(dash, c.ProjectID(), st.ID), promote)
		}
	}
	d.Hint = &h
	return d
}

func otherKey(envs []client.Environment, defID string, has func(string) bool) bool {
	for _, e := range envs {
		if e.ID != defID && has(e.ID) {
			return true
		}
	}
	return false
}

func stagingEnv(envs []client.Environment, exclude string) *client.Environment {
	var found *client.Environment
	for i := range envs {
		e := &envs[i]
		if e.ID == exclude || e.IsSealed {
			continue
		}
		if e.Name == "staging" {
			return e
		}
		if found == nil {
			found = e
		}
	}
	return found
}
