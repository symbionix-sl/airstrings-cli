package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
	"github.com/symbionix-sl/airstrings-cli/internal/guide"
)

var (
	ErrNoKey    = errors.New("no API key: log in first")
	ErrWrongOrg = errors.New("key belongs to another organization")
)

type SetupOptions struct {
	APIKey  string
	BaseURL string
	Name    string
	Project string
	Org     string
}

type SetupResult struct {
	ProjectID     string
	ProjectName   string
	Created       bool
	ActiveEnvName string
	Environments  int
	Sections      int
	RelinkedFrom  string
	SDK           *client.SDKConfig // nil for legacy environment keys
}

// ProjectName derives a project name from the git remote, package.json, or the directory name.
func ProjectName(dir string) string {
	if out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output(); err == nil {
		url := strings.TrimSuffix(strings.TrimSpace(string(out)), ".git")
		if name := url[strings.LastIndexAny(url, "/:")+1:]; name != "" {
			return name
		}
	}
	var pkg struct{ Name string }
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil && json.Unmarshal(data, &pkg) == nil && pkg.Name != "" {
		return pkg.Name[strings.LastIndex(pkg.Name, "/")+1:]
	}
	return filepath.Base(dir)
}

func setupAuth(opts SetupOptions) (Auth, error) {
	if opts.APIKey != "" {
		return Auth{opts.APIKey, opts.BaseURL, "arg"}, nil
	}
	if env, ok := EnvAuthFromEnv(); ok {
		return Auth{env.APIKey, opts.BaseURL, env.Source}, nil
	}
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = client.DefaultBaseURL
	}
	if k := storedOrgKey(baseURL, opts.Org); k != nil {
		return Auth{k.APIKey, opts.BaseURL, "login"}, nil
	}
	return Auth{}, ErrNoKey
}

// Setup binds dir to a project: an org key selects (--project) or creates one
// named after the folder; a project or legacy key uses its own project.
func Setup(dir string, opts SetupOptions) (*SetupResult, error) {
	auth, err := setupAuth(opts)
	if err != nil {
		return nil, err
	}
	keyType := client.KeyType(auth.Key)
	res := &SetupResult{}
	switch {
	case keyType == "org" && opts.Project == "":
		if res.ProjectID, res.ProjectName, err = createProject(auth, opts.Name, dir); err != nil {
			return nil, err
		}
		res.Created = true
	default:
		projects, err := client.New(auth.Key, auth.BaseURL, "", "").ListProjects()
		if err != nil {
			return nil, err
		}
		p := projects[0]
		if opts.Project != "" {
			if p, err = matchProject(projects, opts.Project); err != nil {
				return nil, err
			}
		}
		res.ProjectID, res.ProjectName = p.ID, p.Name
	}

	c := client.New(auth.Key, auth.BaseURL, res.ProjectID, "")
	cfg := WorkspaceConfig{ProjectID: res.ProjectID, ProjectName: res.ProjectName}
	var envs []client.Environment
	if keyType == "environment" {
		all, err := c.ListEnvironments()
		if err != nil {
			return nil, err
		}
		if envs, err = BoundEnvs(auth.Key, auth.BaseURL, res.ProjectID, all); err != nil {
			return nil, err
		}
		if len(envs) == 0 {
			return nil, fmt.Errorf("API key does not authenticate to any environment in this project")
		}
		for _, e := range envs {
			cfg.AddOrUpdate(Credential{APIKey: auth.Key, BaseURL: opts.BaseURL, EnvID: e.ID, EnvName: e.Name, PublicKey: e.PublicKey})
		}
		cfg.OrgID = envs[0].OrganizationID
		cfg.ActiveEnv = defaultEnvID(envs)
	} else {
		if res.SDK, err = c.GetSDKConfig(); err != nil {
			return nil, err
		}
		cfg.OrgID = res.SDK.OrgID
		stored := ""
		if keyType == "project" && auth.Source == "arg" {
			stored = auth.Key
		}
		for _, e := range res.SDK.Environments {
			cred := Credential{APIKey: stored, BaseURL: opts.BaseURL, EnvID: e.ID, EnvName: e.Name}
			if len(e.PublicKeys) > 0 {
				cred.PublicKey = e.PublicKeys[0].PublicKey
			}
			cfg.AddOrUpdate(cred)
			envs = append(envs, client.Environment{ID: e.ID, Name: e.Name, IsDefault: e.IsDefault, IsSealed: e.IsSealed})
		}
		cfg.ActiveEnv = defaultEnvID(envs)
		if e := guide.OpenEnv(envs, ""); e != nil {
			cfg.ActiveEnv = e.ID
		}
	}
	res.Environments = len(envs)
	for _, e := range envs {
		if e.ID == cfg.ActiveEnv {
			res.ActiveEnvName = e.Name
		}
	}

	if opts.Org != "" && cfg.OrgID != opts.Org {
		return nil, fmt.Errorf("%w: the key is for %s, but this setup is for %s", ErrWrongOrg, cfg.OrgID, opts.Org)
	}
	if old, err := LoadConfig(filepath.Join(dir, DirName)); err == nil && old.ProjectID == cfg.ProjectID {
		cfg.BundlesDir, cfg.Shared = old.BundlesDir, old.Shared
	} else if err == nil && old.OrgID != "" && old.OrgID != cfg.OrgID {
		if err := os.RemoveAll(filepath.Join(dir, DirName)); err != nil {
			return nil, fmt.Errorf("remove workspace: %w", err)
		}
		res.RelinkedFrom = old.OrgID
	}
	if err := Init(dir, cfg); err != nil {
		return nil, fmt.Errorf("init workspace: %w", err)
	}
	if sections, err := client.New(auth.Key, auth.BaseURL, res.ProjectID, cfg.ActiveEnv).ListSections(); err == nil {
		for _, sec := range sections.Data {
			CreateSectionDir(filepath.Join(dir, DirName), sec.Name)
		}
		res.Sections = len(sections.Data)
	}
	return res, nil
}

func createProject(auth Auth, name, dir string) (string, string, error) {
	if name == "" {
		name = ProjectName(dir)
	}
	c := client.New(auth.Key, auth.BaseURL, "", "")
	p, err := c.CreateProject(client.CreateProjectRequest{Name: name})
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 409 {
		usage := &UsageError{fmt.Sprintf("a project named %q already exists", name), "Run: airstrings init --name <other-name>"}
		if projects, lerr := c.ListProjects(); lerr == nil {
			if existing, merr := matchProject(projects, name); merr == nil {
				usage.NextStep = fmt.Sprintf("Run: airstrings init --project %s (use the existing project) or airstrings init --name <other-name>", existing.ID)
			}
		}
		return "", "", usage
	}
	if err != nil {
		return "", "", err
	}
	return p.ID, p.Name, nil
}

func matchProject(projects []client.Project, want string) (client.Project, error) {
	for _, p := range projects {
		if p.ID == want || strings.EqualFold(p.Name, want) {
			return p, nil
		}
	}
	return client.Project{}, &UsageError{fmt.Sprintf("project %q not found", want), "Run: airstrings project ls"}
}
