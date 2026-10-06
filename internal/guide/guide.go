// Package guide holds the environment-guidance wording shared by the CLI and
// MCP server (docs/contracts/environment-guidance.md in the parent repo).
package guide

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ShellArg makes a server-provided name safe to paste into a shell command.
func ShellArg(s string) string {
	s = client.StripControl(s)
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

type Hint struct {
	Message  string `json:"message"`
	NextStep string `json:"next_step,omitempty"`
}

func Protection(sealed bool) string {
	if sealed {
		return "protected"
	}
	return "open"
}

func Title(name string) string {
	name = client.StripControl(name)
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func PromoteURL(dashboard, projectID, envID string) string {
	return fmt.Sprintf("%s/projects/%s/env/%s/promote", dashboard, projectID, envID)
}

func APIKeysURL(dashboard, projectID string) string {
	return fmt.Sprintf("%s/projects/%s/api-keys", dashboard, projectID)
}

// ProtectedNoKey is state 4: switching to a protected env the workspace has no key for.
func ProtectedNoKey(envName, promoteURL string) Hint {
	return Hint{
		Message:  fmt.Sprintf("%s is protected, so this workspace doesn't need a %s key.", Title(envName), client.StripControl(envName)),
		NextStep: fmt.Sprintf("SDK setup: `airstrings sdk-config --env %s`. To ship: publish to staging, then ask a human to promote: %s", ShellArg(envName), promoteURL),
	}
}

// OpenNoKey is state 5: the env accepts direct publishing but the workspace has no key for it.
func OpenNoKey(envName, apiKeysURL string) Hint {
	return Hint{
		Message:  fmt.Sprintf("%s accepts direct publishing, but this workspace has no %s key.", Title(envName), client.StripControl(envName)),
		NextStep: fmt.Sprintf("Create a project key at %s, then `airstrings init <key>`", apiKeysURL),
	}
}

// NeedStagingKey is state 2 for a workspace whose only key is for the protected env.
func NeedStagingKey(defName, stagingName, apiKeysURL, promoteURL string) Hint {
	return Hint{
		Message: fmt.Sprintf("%s is protected: changes reach it only by promotion.", Title(defName)),
		NextStep: fmt.Sprintf("Create a project key at %s and run `airstrings init <key>`, publish to %s, then ask a human to promote: %s",
			apiKeysURL, client.StripControl(stagingName), promoteURL),
	}
}

// Status picks the one-line hint for the active environment (states 1, 2, 5, 6).
func Status(activeName string, activeIsDefault bool, defName string, defSealed, defKeyInWorkspace bool, promoteURL, defAPIKeysURL string) Hint {
	switch {
	case activeIsDefault && defSealed:
		return Hint{
			Message:  fmt.Sprintf("%s is protected: changes reach it only by promotion.", Title(defName)),
			NextStep: "Publish to staging, then ask a human to promote: " + promoteURL,
		}
	case activeIsDefault:
		return Hint{Message: fmt.Sprintf("Publishing directly to %s.", client.StripControl(defName))}
	case defSealed:
		return Hint{
			Message:  fmt.Sprintf("Connected to %s. %s is protected.", client.StripControl(activeName), Title(defName)),
			NextStep: fmt.Sprintf("Publish to %s, then ask a human to promote: %s", client.StripControl(activeName), promoteURL),
		}
	case !defKeyInWorkspace:
		return OpenNoKey(defName, defAPIKeysURL)
	default:
		return Hint{
			Message:  fmt.Sprintf("Connected to %s. %s accepts direct publishing.", client.StripControl(activeName), Title(defName)),
			NextStep: fmt.Sprintf("`airstrings env use %s` to publish there directly", ShellArg(defName)),
		}
	}
}

// PublicNotice states that SDK config is safe to show: agents otherwise mistake
// the Ed25519 public keys for credentials.
const PublicNotice = "All values are public, not secrets: safe to show, commit and embed in app code. They contain no API key."

// SDKConfig is the sdk-config payload shared by the CLI --json output and the MCP tool.
func SDKConfig(cfg *client.SDKConfig, env client.SDKEnvironment, apiBaseURL string) map[string]any {
	keys := make([]string, len(env.PublicKeys))
	for i, k := range env.PublicKeys {
		keys[i] = k.PublicKey
	}
	return map[string]any{
		"notice":      PublicNotice,
		"org_id":      cfg.OrgID,
		"project_id":  cfg.ProjectID,
		"environment": SDKEnvironment(env),
		"protection":  Protection(env.IsSealed),
		"snippets":    Snippets(cfg.OrgID, cfg.ProjectID, env.ID, keys, apiBaseURL),
	}
}

// Snippets returns ready-to-paste SDK initialisers per platform, using the
// configuration field names from each SDK README.
func Snippets(orgID, projectID, envID string, publicKeys []string, apiBaseURL string) map[string]string {
	quoted := func(q string) string {
		parts := make([]string, len(publicKeys))
		for i, k := range publicKeys {
			parts[i] = q + k + q
		}
		return strings.Join(parts, ", ")
	}
	var jsBase, iosBase, ktBase, goBase string
	if apiBaseURL != "" && apiBaseURL != client.DefaultBaseURL {
		jsBase = fmt.Sprintf("\n  apiBaseURL: '%s',", apiBaseURL)
		iosBase = fmt.Sprintf(",\n    apiBaseURL: URL(string: %q)!", apiBaseURL)
		ktBase = fmt.Sprintf("\n    apiBaseURL = %q,", apiBaseURL)
		goBase = fmt.Sprintf("\n\tAPIBaseURL:     %q,", apiBaseURL)
	}
	js := func(pkg string) string {
		return fmt.Sprintf(`import { AirStrings } from '%s'

const airstrings = new AirStrings({
  organizationId: '%s',
  projectId: '%s',
  environmentId: '%s',
  publicKeys: [%s],
  locale: 'en',%s
})`, pkg, orgID, projectID, envID, quoted("'"), jsBase)
	}
	return map[string]string{
		"web":          js("@airstrings/web"),
		"react_native": js("@airstrings/react-native"),
		"ios": fmt.Sprintf(`import AirStrings

let airStrings = AirStrings(configuration: .init(
    organizationId: "%s",
    projectId: "%s",
    environmentId: "%s",
    publicKeys: [%s]%s
))`, orgID, projectID, envID, quoted(`"`), iosBase),
		"android": fmt.Sprintf(`val config = AirStringsConfiguration(
    organizationId = "%s",
    projectId = "%s",
    environmentId = "%s",
    publicKeys = listOf(%s),%s
)

val airStrings = AirStrings.create(context, config)`, orgID, projectID, envID, quoted(`"`), ktBase),
		"go": fmt.Sprintf(`import airstrings "github.com/symbionix-sl/airstrings-sdk-go"

client, err := airstrings.New(airstrings.Config{
	OrganizationID: "%s",
	ProjectID:      "%s",
	EnvironmentID:  "%s",
	PublicKeys:     []string{%s},
	Locales:        []string{"en"},%s
})`, orgID, projectID, envID, quoted(`"`), goBase),
	}
}

var SnippetOrder = []struct{ Key, Label string }{
	{"web", "Web"},
	{"react_native", "React Native"},
	{"ios", "iOS (Swift)"},
	{"android", "Android (Kotlin)"},
	{"go", "Go"},
}
