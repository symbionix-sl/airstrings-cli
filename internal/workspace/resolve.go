package workspace

import (
	"fmt"
	"os"
	"strings"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

// ProjectFlag holds the global --project value (ID or name).
var ProjectFlag string

// Auth is the API key a command runs with and where it came from.
type Auth struct {
	Key     string
	BaseURL string
	Source  string
}

type UsageError struct {
	Message  string
	NextStep string
}

func (e *UsageError) Error() string { return e.Message }

func (a Auth) Type() string {
	if a.Key == "" {
		return "none"
	}
	return client.KeyType(a.Key)
}

func (a Auth) FullPower() any {
	if a.Type() != "org" {
		return nil
	}
	if k := storedOrgKey(a.BaseURL); a.Source == "login" && k != nil {
		return k.FullPower
	}
	return "unknown"
}

func (a Auth) HasKey(cfg *WorkspaceConfig) func(envID string) bool {
	typed := a.Type() == "org" || a.Type() == "project"
	return func(id string) bool { return typed || (cfg != nil && cfg.FindByEnvID(id) != nil) }
}

func baseURLFor(cfg *WorkspaceConfig) string {
	if u := os.Getenv("AIRSTRINGS_BASE_URL"); u != "" {
		return u
	}
	if cfg != nil {
		if cred, err := cfg.ActiveCredential(); err == nil && cred.BaseURL != "" {
			return cred.BaseURL
		}
	}
	return client.DefaultBaseURL
}

func storedOrgKey(baseURL string) *OrgKey {
	creds, err := LoadCreds()
	if err != nil {
		return nil
	}
	return creds.OrgKey(baseURL)
}

// ResolveAuth picks the key by precedence: AIRSTRINGS_ORG_API_KEY,
// AIRSTRINGS_API_KEY, the workspace key, then the stored login key.
func ResolveAuth(cfg *WorkspaceConfig) (Auth, bool) {
	if env, ok := EnvAuthFromEnv(); ok {
		return Auth{env.APIKey, env.BaseURL, env.Source}, true
	}
	if cfg != nil {
		if cred, err := cfg.ActiveCredential(); err == nil && cred.APIKey != "" {
			return Auth{cred.APIKey, cred.BaseURL, "workspace"}, true
		}
	}
	baseURL := baseURLFor(cfg)
	if k := storedOrgKey(baseURL); k != nil && (cfg == nil || cfg.OrgID == "" || cfg.OrgID == k.OrgID) {
		return Auth{k.APIKey, baseURL, "login"}, true
	}
	return Auth{}, false
}

// OrgAuth returns the org key for org-wide operations (project ls/create).
func OrgAuth(cfg *WorkspaceConfig) (Auth, bool) {
	if key := os.Getenv("AIRSTRINGS_ORG_API_KEY"); key != "" {
		return Auth{key, os.Getenv("AIRSTRINGS_BASE_URL"), "env:AIRSTRINGS_ORG_API_KEY"}, true
	}
	baseURL := baseURLFor(cfg)
	if k := storedOrgKey(baseURL); k != nil {
		return Auth{k.APIKey, baseURL, "login"}, true
	}
	return Auth{}, false
}

// Resolve builds the API client for a command: key per ResolveAuth, project
// from --project, AIRSTRINGS_PROJECT_ID, the workspace or the key itself, and
// environment from AIRSTRINGS_ENV_ID, the workspace or pickEnv.
func Resolve(cfg *WorkspaceConfig) (*client.Client, Auth, error) {
	auth, ok := ResolveAuth(cfg)
	if !ok {
		if cfg != nil {
			if _, err := cfg.ActiveCredential(); err != nil {
				return nil, auth, err
			}
		}
		return nil, auth, &UsageError{"no API key found", "Run: airstrings init (or set AIRSTRINGS_API_KEY)"}
	}
	if auth.Source == "env:AIRSTRINGS_API_KEY" {
		cfg = nil
	}
	envSource := strings.HasPrefix(auth.Source, "env:")

	projectID, lookup := ProjectFlag, ProjectFlag != "" && !strings.HasPrefix(ProjectFlag, "proj_")
	if projectID == "" {
		projectID = os.Getenv("AIRSTRINGS_PROJECT_ID")
	}
	if projectID == "" && cfg != nil {
		projectID = cfg.ProjectID
	}
	if projectID == "" || lookup {
		id, err := findProject(auth, projectID)
		if err != nil {
			return nil, auth, err
		}
		projectID = id
	}

	var envID string
	if envSource {
		envID = os.Getenv("AIRSTRINGS_ENV_ID")
	}
	if envID == "" && cfg != nil && cfg.ProjectID == projectID {
		envID = cfg.ActiveEnv
	}
	if envID == "" {
		envs, err := client.New(auth.Key, auth.BaseURL, projectID, "").ListEnvironments()
		if err != nil {
			return nil, auth, fmt.Errorf("resolve environment: %w", err)
		}
		if envID, err = pickEnv(auth.Key, auth.BaseURL, projectID, envs); err != nil {
			return nil, auth, fmt.Errorf("resolve environment: %w", err)
		}
	}
	return client.New(auth.Key, auth.BaseURL, projectID, envID), auth, nil
}

func findProject(auth Auth, want string) (string, error) {
	projects, err := client.New(auth.Key, auth.BaseURL, "", "").ListProjects()
	if err != nil {
		return "", fmt.Errorf("resolve project: %w", err)
	}
	if want == "" {
		if client.KeyType(auth.Key) != "org" && len(projects) == 1 {
			return projects[0].ID, nil
		}
		return "", &UsageError{"this org key covers every project in the organization; choose one", "Run: airstrings project ls, then pass --project <id> (or run airstrings init in the app's folder)"}
	}
	p, err := matchProject(projects, want)
	return p.ID, err
}
