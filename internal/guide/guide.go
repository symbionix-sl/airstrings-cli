// Package guide holds the environment-guidance wording shared by the CLI and
// MCP server (docs/contracts/environment-guidance.md in the parent repo).
package guide

import (
	"fmt"
	"strings"
)

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
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func PromoteURL(dashboard, projectID, envID string) string {
	return fmt.Sprintf("%s/projects/%s/env/%s/promote", dashboard, projectID, envID)
}

func APIKeysURL(dashboard, projectID, envID string) string {
	return fmt.Sprintf("%s/projects/%s/env/%s/api-keys", dashboard, projectID, envID)
}

// ProtectedNoKey is state 4: switching to a protected env the workspace has no key for.
func ProtectedNoKey(envName, promoteURL string) Hint {
	return Hint{
		Message:  fmt.Sprintf("%s is protected, so this workspace doesn't need a %s key.", Title(envName), envName),
		NextStep: fmt.Sprintf("SDK setup: `airstrings sdk-config --env %s`. To ship: publish to staging, then ask a human to promote: %s", envName, promoteURL),
	}
}

// OpenNoKey is state 5: the env accepts direct publishing but the workspace has no key for it.
func OpenNoKey(envName, apiKeysURL string) Hint {
	return Hint{
		Message:  fmt.Sprintf("%s accepts direct publishing, but this workspace has no %s key.", Title(envName), envName),
		NextStep: fmt.Sprintf("Create one at %s (%s, write), then `airstrings env add <key>`", apiKeysURL, envName),
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
		return Hint{Message: fmt.Sprintf("Publishing directly to %s.", defName)}
	case defSealed:
		return Hint{
			Message:  fmt.Sprintf("Connected to %s. %s is protected.", activeName, Title(defName)),
			NextStep: fmt.Sprintf("Publish to %s, then ask a human to promote: %s", activeName, promoteURL),
		}
	case !defKeyInWorkspace:
		return OpenNoKey(defName, defAPIKeysURL)
	default:
		return Hint{
			Message:  fmt.Sprintf("Connected to %s. %s accepts direct publishing.", activeName, Title(defName)),
			NextStep: fmt.Sprintf("`airstrings env use %s` to publish there directly", defName),
		}
	}
}

// Snippets returns ready-to-paste SDK initialisers per platform, using the
// configuration field names from each SDK README.
func Snippets(orgID, projectID, envID string, publicKeys []string) map[string]string {
	quoted := func(q string) string {
		parts := make([]string, len(publicKeys))
		for i, k := range publicKeys {
			parts[i] = q + k + q
		}
		return strings.Join(parts, ", ")
	}
	js := func(pkg string) string {
		return fmt.Sprintf(`import { AirStrings } from '%s'

const airstrings = new AirStrings({
  organizationId: '%s',
  projectId: '%s',
  environmentId: '%s',
  publicKeys: [%s],
})`, pkg, orgID, projectID, envID, quoted("'"))
	}
	return map[string]string{
		"web":          js("@airstrings/web"),
		"react_native": js("@airstrings/react-native"),
		"ios": fmt.Sprintf(`import AirStrings

let airStrings = AirStrings(configuration: .init(
    organizationId: "%s",
    projectId: "%s",
    environmentId: "%s",
    publicKeys: [%s]
))`, orgID, projectID, envID, quoted(`"`)),
		"android": fmt.Sprintf(`val config = AirStringsConfiguration(
    organizationId = "%s",
    projectId = "%s",
    environmentId = "%s",
    publicKeys = listOf(%s),
)

val airStrings = AirStrings.create(context, config)`, orgID, projectID, envID, quoted(`"`)),
	}
}

var SnippetOrder = []struct{ Key, Label string }{
	{"web", "Web"},
	{"react_native", "React Native"},
	{"ios", "iOS (Swift)"},
	{"android", "Android (Kotlin)"},
}
