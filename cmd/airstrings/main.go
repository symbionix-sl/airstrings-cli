package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/symbionix-sl/airstrings-cli/internal/bundlepull"
	"github.com/symbionix-sl/airstrings-cli/internal/client"
	"github.com/symbionix-sl/airstrings-cli/internal/doctor"
	"github.com/symbionix-sl/airstrings-cli/internal/output"
	"github.com/symbionix-sl/airstrings-cli/internal/workspace"
)

var version = "dev"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	// Check for --json flag anywhere
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--json" {
			output.JSONMode = true
		} else {
			filtered = append(filtered, a)
		}
	}
	args = filtered

	// Handle shorthand flags: -p -u <project>, -e -u <env>, chainable
	if handleShorthandFlags(args) {
		return
	}

	cmd := args[0]
	args = args[1:]

	switch cmd {
	case "login":
		output.Errorf("'login' has been replaced by 'init'. Use: airstrings init <api-key>")
	case "logout":
		output.Errorf("'logout' has been replaced. Use: airstrings env rm <name>")
	case "status":
		handleStatus(args)
	case "project":
		handleProject(args)
	case "env":
		handleEnv(args)
	case "envs": // backward compat
		handleEnv(args)
	case "apikey":
		handleAPIKey(args)
	case "strings":
		handleStrings(args)
	case "sections":
		handleSections(args)
	case "bundles":
		handleBundles(args)
	case "doctor":
		handleDoctor(args)
	case "publish":
		handlePublish(args)
	case "locales":
		handleLocales(args)
	case "import":
		handleImport(args)
	case "init":
		handleInit(args)
	case "local":
		handleLocal(args)
	case "push":
		handlePush(args)
	case "pull":
		handlePull(args)
	case "mcp":
		handleMCP(args)
	case "profile":
		output.Errorf("'profile' commands have been replaced. Use: login, logout, status, project use, env use")
	case "help", "--help", "-h":
		printUsage()
	case "version", "--version":
		fmt.Printf("airstrings %s\n", version)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`airstrings — AirStrings CLI

Usage: airstrings <command> [options]

Setup:
  init <api-key> [--url <base-url>]     Initialize workspace and authenticate
                  [--purge]             Re-init and remove local strings
  status                                Show active project, environment, and key

Navigation:
  project                               Show current project info
  env                                   List environments (✓ = active)
  env use <name>                        Switch active environment
  env add <api-key> [--url <base-url>]  Add environment credentials
  env rm <name>                         Remove environment credentials
  -e -u <env-name>                      Switch environment (shorthand)
  locales                               List locales with string counts

API keys:
  apikey rotate [--env <name>]          Rotate the workspace API key

Strings:
  strings list [--locale <loc>] [--section <id>] [--limit <n>]
  strings get <key>
  strings set <key> <locale>=<value> [<locale>=<value>...]
  strings create <key> <locale>=<value> [--format text|icu] [--section <id>]
  strings delete <key>

Sections:
  sections list
  sections create <name> [--description <desc>]
  sections delete <id>

Bundles:
  bundles                 List published bundles
  bundles pull [dir] [--locale <bcp47>]
                          Pull published, signed bundles into a committable
                          seed folder (default airstrings/bundles/) for
                          offline fallback. Distinct from 'pull', which
                          fetches draft strings as editable CSVs
  publish [locale...]     Publish bundles (all locales if none specified)
  doctor [dir]            Verify bundled-fallback integration in this project

Import:
  import csv <file>       Import strings from CSV file
  import status <id>      Check import status

Workspace:
  local set <key> <locale>=<value> [--format text|icu] [--section <name>]
  local rm <key> [--locale <loc>] [--section <name>]
  local ls [--section <name>]                      List local strings
  push [--section <name>]                          Push local strings to API
  pull [--section <name>]                          Pull remote draft strings to local
                                                   CSVs (published bundles: bundles pull)

MCP:
  mcp install                  Install MCP server for Claude Code
  mcp install --claude-desktop Install MCP server for Claude Desktop
  mcp uninstall                Remove MCP server from Claude Code
  mcp status                   Check MCP installation status

Flags:
  --json                  Output as JSON (works with any command)

`)
}

// mustWorkspace finds and loads the workspace config, or exits with an error.
func mustWorkspace() (string, *workspace.WorkspaceConfig) {
	wsDir, err := workspace.Find()
	if err != nil {
		output.Errorf("no workspace found — run: airstrings init <api-key>")
	}
	wsCfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		output.Errorf("load workspace: %s", err)
	}
	return wsDir, wsCfg
}

// mustClient loads workspace config and returns a ready API client.
func mustClient() *client.Client {
	_, wsCfg := mustWorkspace()
	c, err := workspace.ResolveClient(wsCfg)
	if err != nil {
		output.Errorf("%s", err)
	}
	return c
}

// handleShorthandFlags processes -e -u <name> flags.
// Returns true if flags were handled (no further command processing needed).
func handleShorthandFlags(args []string) bool {
	if len(args) == 0 || args[0][0] != '-' || args[0] == "--json" || args[0] == "--help" || args[0] == "--version" || args[0] == "-h" {
		return false
	}

	wsDir, wsCfg := mustWorkspace()

	changed := false
	i := 0
	for i < len(args) {
		if (args[i] == "-e" || args[i] == "--env") && i+2 < len(args) && (args[i+1] == "-u" || args[i+1] == "--use") {
			name := args[i+2]
			switchEnv(wsCfg, name)
			changed = true
			i += 3
		} else {
			break
		}
	}

	if !changed {
		return false
	}

	if err := workspace.SaveConfig(wsDir, wsCfg); err != nil {
		output.Errorf("save workspace: %s", err)
	}

	// If there are remaining args after flags, they're a command — don't handle here
	if i < len(args) {
		return false
	}

	// No command after flags — print status
	printStatus(wsCfg)
	return true
}

func switchEnv(wsCfg *workspace.WorkspaceConfig, name string) {
	// Find env by name (case-insensitive)
	for _, cred := range wsCfg.Credentials {
		if strings.EqualFold(cred.EnvName, name) {
			wsCfg.ActiveEnv = cred.EnvID
			return
		}
	}
	// Try by ID
	for _, cred := range wsCfg.Credentials {
		if cred.EnvID == name {
			wsCfg.ActiveEnv = cred.EnvID
			return
		}
	}

	// List available envs
	var names []string
	for _, c := range wsCfg.Credentials {
		names = append(names, c.EnvName)
	}
	output.Errorf("environment %q not found. Available: %s", name, strings.Join(names, ", "))
}

func printStatus(wsCfg *workspace.WorkspaceConfig) {
	cred, err := wsCfg.ActiveCredential()
	if err != nil {
		output.Errorf("%s", err)
	}

	if output.JSONMode {
		output.JSON(map[string]string{
			"project_id":   wsCfg.ProjectID,
			"project_name": wsCfg.ProjectName,
			"env_id":       cred.EnvID,
			"env_name":     cred.EnvName,
			"base_url":     cred.BaseURL,
		})
		return
	}

	fmt.Printf("Project:  %s (%s)\n", wsCfg.ProjectName, wsCfg.ProjectID)
	fmt.Printf("Env:      %s (%s)\n", cred.EnvName, cred.EnvID)
	url := cred.BaseURL
	if url == "" {
		url = "https://api.airstrings.com"
	}
	fmt.Printf("API URL:  %s\n", url)
	fmt.Printf("Key:      %s...%s\n", cred.APIKey[:8], cred.APIKey[len(cred.APIKey)-4:])
}

// --- Auth commands ---

// parseKeyAndURL extracts an API key and optional --url/--base-url from args.
func parseKeyAndURL(args []string) (string, string) {
	if len(args) < 1 {
		output.Errorf("usage: provide an API key")
	}
	apiKey := args[0]
	var baseURL string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--url", "--base-url":
			i++
			if i < len(args) {
				baseURL = args[i]
			}
		}
	}
	return apiKey, baseURL
}

// validateAndDiscover validates an API key and returns the project and environments.
func validateAndDiscover(apiKey, baseURL string) (*client.Project, []client.Environment) {
	c := client.New(apiKey, baseURL, "", "")
	proj, err := c.GetProject()
	if err != nil {
		output.Errorf("invalid API key: %s", err)
	}

	c2 := client.New(apiKey, baseURL, proj.ID, "")
	envs, err := c2.ListEnvironments()
	if err != nil {
		output.Errorf("list environments: %s", err)
	}
	return proj, envs
}

// addCredentials adds environment credentials to a workspace config and returns the active env name.
func addCredentials(wsCfg *workspace.WorkspaceConfig, apiKey, baseURL string, envs []client.Environment) string {
	var activeEnvID, activeEnvName string
	for _, env := range envs {
		cred := workspace.Credential{
			APIKey:  apiKey,
			BaseURL: baseURL,
			EnvID:   env.ID,
			EnvName: env.Name,
		}
		wsCfg.AddOrUpdate(cred)

		if env.IsDefault || activeEnvID == "" {
			activeEnvID = env.ID
			activeEnvName = env.Name
		}
	}

	// Only set active env if not already set
	if wsCfg.ActiveEnv == "" && activeEnvID != "" {
		wsCfg.ActiveEnv = activeEnvID
	}
	return activeEnvName
}

func handleStatus(args []string) {
	_, wsCfg := mustWorkspace()
	printStatus(wsCfg)
}

// --- Project commands ---

func handleProject(args []string) {
	if len(args) > 0 && args[0] == "use" {
		output.Errorf("workspace is bound to one project — init a new workspace for a different project")
	}

	c := mustClient()
	proj, err := c.GetProject()
	if err != nil {
		output.Errorf("get project: %s", err)
	}

	if output.JSONMode {
		output.JSON(proj)
		return
	}

	fmt.Printf("Project: %s\n", proj.Name)
	fmt.Printf("  ID:       %s\n", proj.ID)
	fmt.Printf("  Locale:   %s\n", proj.DefaultLocale)
	fmt.Printf("  Strings:  %d\n", proj.StringCount)
	fmt.Printf("  Locales:  %d\n", proj.LocaleCount)
	if proj.Description != "" {
		fmt.Printf("  Desc:     %s\n", proj.Description)
	}
}

// --- Env commands ---

func handleEnv(args []string) {
	if len(args) > 0 && args[0] == "use" {
		if len(args) < 2 {
			output.Errorf("usage: airstrings env use <name>")
		}
		wsDir, wsCfg := mustWorkspace()
		switchEnv(wsCfg, args[1])
		if err := workspace.SaveConfig(wsDir, wsCfg); err != nil {
			output.Errorf("save workspace: %s", err)
		}
		cred, _ := wsCfg.ActiveCredential()
		output.Success(fmt.Sprintf("Switched to %s / %s", wsCfg.ProjectName, cred.EnvName))
		return
	}

	if len(args) > 0 && args[0] == "add" {
		if len(args) < 2 {
			output.Errorf("usage: airstrings env add <api-key> [--url <base-url>]")
		}
		apiKey, baseURL := parseKeyAndURL(args[1:])
		wsDir, wsCfg := mustWorkspace()

		// Validate and discover environments for this key
		_, envs := validateAndDiscover(apiKey, baseURL)
		activeEnvName := addCredentials(wsCfg, apiKey, baseURL, envs)

		if err := workspace.SaveConfig(wsDir, wsCfg); err != nil {
			output.Errorf("save workspace: %s", err)
		}

		output.Success(fmt.Sprintf("Added %d environment(s): %s", len(envs), activeEnvName))
		for _, env := range envs {
			marker := "  "
			if env.ID == wsCfg.ActiveEnv {
				marker = "✓ "
			}
			fmt.Printf("  %s%s\n", marker, env.Name)
		}
		return
	}

	if len(args) > 0 && args[0] == "rm" {
		if len(args) < 2 {
			output.Errorf("usage: airstrings env rm <name>")
		}
		wsDir, wsCfg := mustWorkspace()

		// Find by name (case-insensitive) or ID
		var target *workspace.Credential
		for _, cred := range wsCfg.Credentials {
			if strings.EqualFold(cred.EnvName, args[1]) || cred.EnvID == args[1] {
				target = &cred
				break
			}
		}
		if target == nil {
			output.Errorf("environment %q not found", args[1])
		}

		name := target.EnvName
		wsCfg.Remove(target.EnvID)

		// Pick new active if we removed the active one
		if wsCfg.ActiveEnv == target.EnvID {
			if len(wsCfg.Credentials) > 0 {
				wsCfg.ActiveEnv = wsCfg.Credentials[0].EnvID
			} else {
				wsCfg.ActiveEnv = ""
			}
		}

		if err := workspace.SaveConfig(wsDir, wsCfg); err != nil {
			output.Errorf("save workspace: %s", err)
		}

		output.Success(fmt.Sprintf("Removed %s", name))
		return
	}

	if len(args) > 0 && args[0] == "create" {
		c := mustClient()
		if len(args) < 2 {
			output.Errorf("usage: airstrings env create <name>")
		}
		env, err := c.CreateEnvironment(client.CreateEnvRequest{Name: args[1]})
		if err != nil {
			output.Errorf("create environment: %s", err)
		}
		output.Success(fmt.Sprintf("Environment %q created (id: %s)", env.Name, env.ID))
		return
	}

	c := mustClient()
	envs, err := c.ListEnvironments()
	if err != nil {
		output.Errorf("list environments: %s", err)
	}

	_, wsCfg := mustWorkspace()

	if output.JSONMode {
		output.JSON(envs)
		return
	}

	headers := []string{"ID", "NAME", "DEFAULT", "SEALED", "ACTIVE"}
	var rows [][]string
	for _, e := range envs {
		def := ""
		if e.IsDefault {
			def = "✓"
		}
		sealed := ""
		if e.IsSealed {
			sealed = "✓"
		}
		active := ""
		if e.ID == wsCfg.ActiveEnv {
			active = "✓"
		}
		rows = append(rows, []string{e.ID, e.Name, def, sealed, active})
	}
	output.Table(headers, rows)
}

// --- API key commands ---

func handleAPIKey(args []string) {
	if len(args) == 0 || args[0] != "rotate" {
		output.Errorf("usage: airstrings apikey rotate [--env <name>]")
	}
	handleAPIKeyRotate(args[1:])
}

func handleAPIKeyRotate(args []string) {
	envName := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--env" {
			i++
			if i < len(args) {
				envName = args[i]
			}
		}
	}

	wsDir, wsCfg := mustWorkspace()

	var cred *workspace.Credential
	if envName != "" {
		for i := range wsCfg.Credentials {
			if strings.EqualFold(wsCfg.Credentials[i].EnvName, envName) {
				cred = &wsCfg.Credentials[i]
				break
			}
		}
		if cred == nil {
			output.Errorf("environment %q not found", envName)
		}
	} else {
		var err error
		cred, err = wsCfg.ActiveCredential()
		if err != nil {
			output.Errorf("%s", err)
		}
	}

	result, err := workspace.RotateKey(wsDir, wsCfg, cred)
	if err != nil {
		output.Errorf("rotate key: %s", err)
	}

	if !result.Revoked {
		fmt.Fprintf(os.Stderr, "WARNING: failed to revoke old key %s (%s) — it is still active and must be revoked manually via the dashboard or DELETE /api-keys/%s\n",
			result.OldKeyID, result.RevokeErr, result.OldKeyID)
	}

	if output.JSONMode {
		output.JSON(result)
		return
	}

	key := cred.APIKey
	masked := key[:8] + "..." + key[len(key)-4:]
	if result.Revoked {
		output.Success(fmt.Sprintf("Rotated API key for %s — new key %s (old key %s revoked)", cred.EnvName, masked, result.OldKeyID))
	} else {
		output.Success(fmt.Sprintf("Rotated API key for %s — new key %s", cred.EnvName, masked))
	}
}

// --- String commands ---

func handleStrings(args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	c := mustClient()

	switch args[0] {
	case "list", "ls":
		handleStringList(c, args[1:])
	case "get":
		if len(args) < 2 {
			output.Errorf("usage: airstrings strings get <key>")
		}
		handleStringGet(c, args[1])
	case "set":
		if len(args) < 3 {
			output.Errorf("usage: airstrings strings set <key> <locale>=<value> ...")
		}
		handleStringSet(c, args[1], args[2:])
	case "create":
		if len(args) < 3 {
			output.Errorf("usage: airstrings strings create <key> <locale>=<value> [--format text|icu]")
		}
		handleStringCreate(c, args[1], args[2:])
	case "delete", "rm":
		if len(args) < 2 {
			output.Errorf("usage: airstrings strings delete <key>")
		}
		handleStringDelete(c, args[1])
	default:
		output.Errorf("unknown strings command: %s", args[0])
	}
}

func handleStringList(c *client.Client, args []string) {
	opts := client.ListStringsOpts{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--locale":
			i++
			if i < len(args) {
				opts.Locale = args[i]
			}
		case "--section":
			i++
			if i < len(args) {
				opts.Section = args[i]
			}
		case "--limit":
			i++
			if i < len(args) {
				fmt.Sscanf(args[i], "%d", &opts.Limit)
			}
		}
	}

	var entries []client.StringEntry
	var hasMore bool

	if opts.Limit > 0 {
		list, err := c.ListStrings(opts)
		if err != nil {
			output.Errorf("list strings: %s", err)
		}
		entries = list.Data
		hasMore = list.Pagination.HasMore
	} else {
		all, err := c.ListAllStrings(opts)
		if err != nil {
			output.Errorf("list strings: %s", err)
		}
		entries = all
	}

	if output.JSONMode {
		output.JSON(entries)
		return
	}

	headers := []string{"KEY", "FORMAT", "LOCALES", "SECTION"}
	var rows [][]string
	for _, s := range entries {
		locales := make([]string, 0, len(s.Values))
		for loc := range s.Values {
			locales = append(locales, loc)
		}
		sec := "-"
		if s.SectionID != nil {
			sec = *s.SectionID
		}
		rows = append(rows, []string{s.Key, s.Format, strings.Join(locales, ", "), sec})
	}
	output.Table(headers, rows)

	if hasMore {
		fmt.Printf("\n(more results available)\n")
	}
}

func handleStringGet(c *client.Client, key string) {
	s, err := c.GetString(key)
	if err != nil {
		output.Errorf("get string: %s", err)
	}

	if output.JSONMode {
		output.JSON(s)
		return
	}

	fmt.Printf("Key:    %s\n", s.Key)
	fmt.Printf("Format: %s\n", s.Format)
	fmt.Println("Values:")
	for loc, val := range s.Values {
		fmt.Printf("  %s: %s\n", loc, val)
	}
}

func handleStringSet(c *client.Client, key string, pairs []string) {
	values := make(map[string]*string)
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			output.Errorf("invalid format %q — expected locale=value", pair)
		}
		v := parts[1]
		values[parts[0]] = &v
	}

	s, err := c.UpsertString(key, client.UpsertStringRequest{Values: values})
	if err != nil {
		output.Errorf("set string: %s", err)
	}

	if output.JSONMode {
		output.JSON(s)
		return
	}

	output.Success(fmt.Sprintf("Updated %s (%d locales)", s.Key, len(s.Values)))
}

func handleStringCreate(c *client.Client, key string, args []string) {
	req := client.CreateStringRequest{
		Key:    key,
		Values: make(map[string]string),
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			i++
			if i < len(args) {
				req.Format = args[i]
			}
		case "--section":
			i++
			if i < len(args) {
				req.SectionID = &args[i]
			}
		default:
			parts := strings.SplitN(args[i], "=", 2)
			if len(parts) == 2 {
				req.Values[parts[0]] = parts[1]
			}
		}
	}

	s, err := c.CreateString(req)
	if err != nil {
		output.Errorf("create string: %s", err)
	}

	if output.JSONMode {
		output.JSON(s)
		return
	}

	output.Success(fmt.Sprintf("Created %s (%d locales)", s.Key, len(s.Values)))
}

func handleStringDelete(c *client.Client, key string) {
	if err := c.DeleteString(key); err != nil {
		output.Errorf("delete string: %s", err)
	}
	output.Success(fmt.Sprintf("Deleted %s", key))
}

// --- Section commands ---

func handleSections(args []string) {
	c := mustClient()

	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list", "ls":
		list, err := c.ListSections()
		if err != nil {
			output.Errorf("list sections: %s", err)
		}

		if output.JSONMode {
			output.JSON(list)
			return
		}

		headers := []string{"ID", "NAME", "STRINGS", "DESCRIPTION"}
		var rows [][]string
		for _, s := range list.Data {
			desc := s.Description
			if len(desc) > 40 {
				desc = desc[:40] + "..."
			}
			rows = append(rows, []string{s.ID, s.Name, fmt.Sprintf("%d", s.StringCount), desc})
		}
		output.Table(headers, rows)

	case "create":
		if len(args) < 2 {
			output.Errorf("usage: airstrings sections create <name> [--description <desc>]")
		}
		req := client.CreateSectionRequest{Name: args[1]}
		for i := 2; i < len(args); i++ {
			if args[i] == "--description" || args[i] == "-d" {
				i++
				if i < len(args) {
					req.Description = args[i]
				}
			}
		}
		sec, err := c.CreateSection(req)
		if err != nil {
			output.Errorf("create section: %s", err)
		}
		output.Success(fmt.Sprintf("Section %q created (id: %s)", sec.Name, sec.ID))

	case "delete", "rm":
		if len(args) < 2 {
			output.Errorf("usage: airstrings sections delete <id>")
		}
		if err := c.DeleteSection(args[1]); err != nil {
			output.Errorf("delete section: %s", err)
		}
		output.Success(fmt.Sprintf("Section %s deleted", args[1]))

	default:
		output.Errorf("unknown sections command: %s", args[0])
	}
}

// --- Bundle commands ---

func handleBundles(args []string) {
	if len(args) > 0 && args[0] == "pull" {
		handleBundlesPull(args[1:])
		return
	}

	c := mustClient()

	bundles, err := c.ListBundles()
	if err != nil {
		output.Errorf("list bundles: %s", err)
	}

	if output.JSONMode {
		output.JSON(bundles)
		return
	}

	headers := []string{"LOCALE", "REVISION", "STRINGS", "SIZE", "CREATED"}
	var rows [][]string
	for _, b := range bundles {
		size := fmt.Sprintf("%.1fKB", float64(b.SizeBytes)/1024)
		rows = append(rows, []string{b.Locale, fmt.Sprintf("%d", b.Revision), fmt.Sprintf("%d", b.StringCount), size, b.CreatedAt})
	}
	output.Table(headers, rows)
}

const firstPullHint = `First pull — commit this folder so your apps ship with bundled fallback strings.
  iOS:     add the folder to your app target as a folder reference (SPM: resources: [.copy("airstrings")])
  Android: copy or map the folder into src/main/assets/
  Web:     Node seeds from <cwd>/airstrings/bundles/ automatically; browsers import bundle JSON at build time
Then run: airstrings doctor   (verifies your project is wired up)
See: docs/contracts/bundled-fallback.md
`

func handleBundlesPull(args []string) {
	dirArg := ""
	locale := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--locale":
			i++
			if i < len(args) {
				locale = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Errorf("unknown flag: %s", args[i])
			}
			if dirArg != "" {
				output.Errorf("usage: airstrings bundles pull [dir] [--locale <bcp47>]")
			}
			dirArg = args[i]
		}
	}

	wsDir, wsCfg := mustWorkspace()
	dir, err := bundlepull.ResolveDir(wsDir, wsCfg, dirArg)
	if err != nil {
		output.Errorf("%s", err)
	}

	c, err := workspace.ResolveClient(wsCfg)
	if err != nil {
		output.Errorf("%s", err)
	}
	cred, err := wsCfg.ActiveCredential()
	if err != nil {
		output.Errorf("%s", err)
	}

	res, err := bundlepull.Pull(c, bundlepull.Options{
		Dir:        dir,
		Locale:     locale,
		EnvName:    cred.EnvName,
		CLIVersion: version,
	})
	if err != nil {
		output.Errorf("bundles pull: %s", err)
	}

	if res.FirstPull {
		fmt.Fprint(os.Stderr, firstPullHint)
	}

	if output.JSONMode {
		output.JSON(res.JSON())
		return
	}

	locales := make([]string, 0, len(res.Pulled))
	for _, b := range res.Pulled {
		locales = append(locales, b.Locale)
	}
	output.Success(fmt.Sprintf("Pulled %d bundles (locales: %s) into %s", len(res.Pulled), strings.Join(locales, ", "), displayPath(dir)))
}

func displayPath(p string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(cwd, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return p
	}
	return rel
}

func handleDoctor(args []string) {
	dirArg := ""
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			output.Errorf("unknown flag: %s", a)
		}
		if dirArg != "" {
			output.Errorf("usage: airstrings doctor [dir]")
		}
		dirArg = a
	}

	wsDir, wsCfg := mustWorkspace()
	dir, err := doctor.ResolveDir(wsDir, wsCfg, dirArg)
	if err != nil {
		output.Errorf("%s", err)
	}

	report := doctor.Run(filepath.Dir(wsDir), dir)

	if output.JSONMode {
		output.JSON(report)
	} else {
		printDoctorReport(report)
	}

	if report.HasMissing() {
		os.Exit(1)
	}
}

func printDoctorReport(rep *doctor.Report) {
	fmt.Printf("Bundles dir: %s\n", displayPath(rep.BundlesDir))
	ok, missing, manual := 0, 0, 0
	for _, c := range rep.Checks {
		var marker string
		switch c.Status {
		case doctor.StatusOK:
			marker, ok = "✓", ok+1
		case doctor.StatusMissing:
			marker, missing = "✗", missing+1
		default:
			marker, manual = "•", manual+1
		}
		detail := c.Detail
		if c.Path != "" && c.Path != rep.BundlesDir {
			detail = displayPath(c.Path) + ": " + detail
		}
		fmt.Printf("%s %-9s %s\n", marker, c.Name, detail)
		if c.Status != doctor.StatusOK && c.Fix != "" {
			for i, line := range strings.Split(c.Fix, "\n") {
				if i == 0 {
					fmt.Printf("    fix: %s\n", line)
				} else {
					fmt.Printf("         %s\n", line)
				}
			}
		}
	}
	fmt.Printf("\n%d ok, %d missing, %d manual\n", ok, missing, manual)
}

func handlePublish(args []string) {
	c := mustClient()

	var locales []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			locales = append(locales, a)
		}
	}

	resp, err := c.PublishBundles(locales)
	if err != nil {
		output.Errorf("publish: %s", err)
	}

	if output.JSONMode {
		output.JSON(resp)
		return
	}

	for _, r := range resp.Results {
		if r.Status == "ok" && r.Bundle != nil {
			fmt.Printf("  %s %s  rev %d  (%d strings, %.1fKB)\n", output.Check,
				r.Locale, r.Bundle.Revision, r.Bundle.StringCount, float64(r.Bundle.SizeBytes)/1024)
		} else {
			fmt.Printf("  ✗ %s  %s\n", r.Locale, r.Error)
		}
	}
	output.Success(fmt.Sprintf("Published at %s", resp.PublishedAt.Format("2006-01-02 15:04:05 UTC")))
}

// --- Locale commands ---

func handleLocales(args []string) {
	c := mustClient()

	locales, err := c.ListLocales()
	if err != nil {
		output.Errorf("list locales: %s", err)
	}

	if output.JSONMode {
		output.JSON(locales)
		return
	}

	headers := []string{"LOCALE", "STRINGS"}
	var rows [][]string
	for _, l := range locales {
		rows = append(rows, []string{l.Locale, fmt.Sprintf("%d", l.StringCount)})
	}
	output.Table(headers, rows)
}

// --- Import commands ---

func handleImport(args []string) {
	c := mustClient()

	if len(args) == 0 {
		output.Errorf("usage: airstrings import csv <file> | airstrings import status <id>")
	}

	switch args[0] {
	case "csv":
		if len(args) < 2 {
			output.Errorf("usage: airstrings import csv <file>")
		}
		data, err := os.ReadFile(args[1])
		if err != nil {
			output.Errorf("read file: %s", err)
		}
		status, err := c.CreateImport(data, nil)
		if err != nil {
			output.Errorf("import: %s", err)
		}
		if output.JSONMode {
			output.JSON(status)
			return
		}
		output.Success(fmt.Sprintf("Import started (id: %s, rows: %d)", status.ID, status.TotalRows))

	case "status":
		if len(args) < 2 {
			output.Errorf("usage: airstrings import status <id>")
		}
		status, err := c.GetImport(args[1])
		if err != nil {
			output.Errorf("get import: %s", err)
		}
		if output.JSONMode {
			output.JSON(status)
			return
		}
		fmt.Printf("Import %s: %s\n", status.ID, status.Status)
		fmt.Printf("  Created: %d  Updated: %d  Skipped: %d  Errors: %d\n",
			status.CreatedRows, status.UpdatedRows, status.SkippedRows, len(status.Errors))

	default:
		output.Errorf("unknown import command: %s", args[0])
	}
}

// --- Workspace commands ---

func handleInit(args []string) {
	if len(args) < 1 {
		output.Errorf("usage: airstrings init <api-key> [--url <base-url>] [--purge]")
	}

	// Parse --purge flag before passing to parseKeyAndURL
	var purge bool
	var filteredArgs []string
	for _, arg := range args {
		if arg == "--purge" {
			purge = true
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	apiKey, baseURL := parseKeyAndURL(filteredArgs)

	cwd, err := os.Getwd()
	if err != nil {
		output.Errorf("get working directory: %s", err)
	}

	// Handle existing workspace
	wsDir := filepath.Join(cwd, ".airstrings")
	if _, err := os.Stat(filepath.Join(wsDir, "config.json")); err == nil {
		if purge {
			// Remove everything and start fresh
			if err := os.RemoveAll(wsDir); err != nil {
				output.Errorf("remove workspace: %s", err)
			}
		} else {
			// Keep CSVs: remove only config, re-init will recreate it
			if err := os.Remove(filepath.Join(wsDir, "config.json")); err != nil {
				output.Errorf("remove old config: %s", err)
			}
		}
	}

	// Validate key and discover project/environments
	proj, envs := validateAndDiscover(apiKey, baseURL)

	// Create workspace with credentials
	wsCfg := workspace.WorkspaceConfig{
		ProjectID:   proj.ID,
		ProjectName: proj.Name,
	}
	activeEnvName := addCredentials(&wsCfg, apiKey, baseURL, envs)

	if err := workspace.Init(cwd, wsCfg); err != nil {
		output.Errorf("init workspace: %s", err)
	}

	// Create section dirs for remote sections
	c := client.New(apiKey, baseURL, proj.ID, wsCfg.ActiveEnv)
	sections, err := c.ListSections()
	sectionCount := 0
	if err == nil && len(sections.Data) > 0 {
		for _, sec := range sections.Data {
			workspace.CreateSectionDir(wsDir, sec.Name)
		}
		sectionCount = len(sections.Data)
	}

	if output.JSONMode {
		output.JSON(map[string]any{
			"project":      proj.Name,
			"environment":  activeEnvName,
			"environments": len(envs),
			"sections":     sectionCount,
		})
		return
	}

	output.Success(fmt.Sprintf("Workspace initialized for %s / %s", proj.Name, activeEnvName))
	if len(envs) > 1 {
		fmt.Printf("  %d environments available. Use: airstrings env use <name>\n", len(envs))
	}
	if sectionCount > 0 {
		fmt.Printf("  Sections: %d\n", sectionCount)
	}
}

func handleLocal(args []string) {
	if len(args) == 0 {
		output.Errorf("usage: airstrings local <set|rm|ls> ...")
	}

	switch args[0] {
	case "set":
		handleLocalSet(args[1:])
	case "rm", "remove":
		handleLocalRm(args[1:])
	case "ls", "list":
		handleLocalLs(args[1:])
	default:
		output.Errorf("unknown local command: %s", args[0])
	}
}

func handleLocalSet(args []string) {
	if len(args) < 2 {
		output.Errorf("usage: airstrings local set <key> <locale>=<value> [--format text|icu] [--section <name>]")
	}

	key := args[0]
	format := "text"
	section := ""
	values := make(map[string]string)

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--format":
			i++
			if i < len(args) {
				format = args[i]
			}
		case "--section":
			i++
			if i < len(args) {
				section = args[i]
			}
		default:
			parts := strings.SplitN(args[i], "=", 2)
			if len(parts) == 2 {
				values[parts[0]] = parts[1]
			} else {
				output.Errorf("invalid format %q — expected locale=value", args[i])
			}
		}
	}

	if len(values) == 0 {
		output.Errorf("at least one locale=value pair is required")
	}

	wsDir, err := workspace.Find()
	if err != nil {
		output.Errorf("%s", err)
	}

	if section != "" {
		if err := workspace.ValidateSectionName(section); err != nil {
			output.Errorf("%s", err)
		}
	}

	path := workspace.CSVPath(wsDir, section)
	if err := workspace.SetRows(path, key, values, format); err != nil {
		output.Errorf("set rows: %s", err)
	}

	if output.JSONMode {
		output.JSON(map[string]any{
			"key":     key,
			"locales": len(values),
			"section": section,
			"format":  format,
		})
		return
	}

	loc := fmt.Sprintf("%d locale(s)", len(values))
	if section != "" {
		output.Success(fmt.Sprintf("Set %s — %s [%s] in %s", key, loc, format, section))
	} else {
		output.Success(fmt.Sprintf("Set %s — %s [%s]", key, loc, format))
	}
}

func handleLocalRm(args []string) {
	if len(args) < 1 {
		output.Errorf("usage: airstrings local rm <key> [--locale <loc>] [--section <name>]")
	}

	key := args[0]
	locale := ""
	section := ""

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--locale":
			i++
			if i < len(args) {
				locale = args[i]
			}
		case "--section":
			i++
			if i < len(args) {
				section = args[i]
			}
		}
	}

	wsDir, err := workspace.Find()
	if err != nil {
		output.Errorf("%s", err)
	}

	path := workspace.CSVPath(wsDir, section)
	if err := workspace.RemoveRows(path, key, locale); err != nil {
		output.Errorf("remove rows: %s", err)
	}

	if output.JSONMode {
		output.JSON(map[string]any{"key": key, "locale": locale, "section": section})
		return
	}

	if locale != "" {
		output.Success(fmt.Sprintf("Removed %s/%s", key, locale))
	} else {
		output.Success(fmt.Sprintf("Removed %s (all locales)", key))
	}
}

func handleLocalLs(args []string) {
	section := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--section" {
			i++
			if i < len(args) {
				section = args[i]
			}
		}
	}

	wsDir, err := workspace.Find()
	if err != nil {
		output.Errorf("%s", err)
	}

	var allRows []localRow

	if section != "" {
		path := workspace.CSVPath(wsDir, section)
		rows, err := workspace.ReadCSV(path)
		if err != nil {
			output.Errorf("read CSV: %s", err)
		}
		for _, r := range rows {
			allRows = append(allRows, localRow{Section: section, Row: r})
		}
	} else {
		paths, err := workspace.AllCSVPaths(wsDir)
		if err != nil {
			output.Errorf("scan workspace: %s", err)
		}
		for secName, path := range paths {
			rows, err := workspace.ReadCSV(path)
			if err != nil {
				output.Errorf("read %s: %s", path, err)
			}
			for _, r := range rows {
				allRows = append(allRows, localRow{Section: secName, Row: r})
			}
		}
	}

	if output.JSONMode {
		output.JSON(allRows)
		return
	}

	if len(allRows) == 0 {
		fmt.Println("No local strings found.")
		return
	}

	headers := []string{"KEY", "LOCALE", "VALUE", "FORMAT", "SECTION"}
	var rows [][]string
	for _, r := range allRows {
		sec := r.Section
		if sec == "" {
			sec = "-"
		}
		val := r.Row.Value
		if len(val) > 50 {
			val = val[:50] + "..."
		}
		rows = append(rows, []string{r.Row.Key, r.Row.Locale, val, r.Row.Format, sec})
	}
	output.Table(headers, rows)
}

type localRow struct {
	Section string        `json:"section"`
	Row     workspace.Row `json:"row"`
}

func handlePush(args []string) {
	section := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--section" {
			i++
			if i < len(args) {
				section = args[i]
			}
		}
	}

	wsDir, err := workspace.Find()
	if err != nil {
		output.Errorf("%s", err)
	}

	wsCfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		output.Errorf("%s", err)
	}

	c, err := workspace.ResolveClient(wsCfg)
	if err != nil {
		output.Errorf("%s", err)
	}

	var progress workspace.ProgressFunc
	if !output.JSONMode {
		progress = func(phase string, done, total int) {
			switch phase {
			case "uploading":
				fmt.Fprintf(os.Stderr, "\r  Uploading strings...")
			case "processing":
				fmt.Fprintf(os.Stderr, "\r  Processing %d/%d rows...", done, total)
			}
		}
	}

	result, err := workspace.Push(c, wsDir, section, progress)
	if err != nil {
		if progress != nil {
			fmt.Fprint(os.Stderr, "\r\033[K") // clear progress line
		}
		output.Errorf("push: %s", err)
	}

	if progress != nil {
		fmt.Fprint(os.Stderr, "\r\033[K") // clear progress line
	}

	if output.JSONMode {
		output.JSON(result)
		return
	}

	output.Success(fmt.Sprintf("Pushed %d strings (%d errors)", result.Upserted, result.Errors))
	if len(result.Sections) > 0 {
		fmt.Printf("  Sections: %s\n", strings.Join(result.Sections, ", "))
	}
	if len(result.FailedKeys) > 0 {
		fmt.Fprintf(os.Stderr, "\nFailed keys:\n")
		for _, fe := range result.FailedKeys {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", fe.Key, fe.Message)
		}
	}
}

func handlePull(args []string) {
	section := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--section" {
			i++
			if i < len(args) {
				section = args[i]
			}
		}
	}

	wsDir, err := workspace.Find()
	if err != nil {
		output.Errorf("%s", err)
	}

	wsCfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		output.Errorf("%s", err)
	}

	c, err := workspace.ResolveClient(wsCfg)
	if err != nil {
		output.Errorf("%s", err)
	}

	// Warn about overwrite
	paths, _ := workspace.AllCSVPaths(wsDir)
	if len(paths) > 0 {
		fmt.Fprintln(os.Stderr, "Warning: local CSVs will be overwritten with remote state.")
	}

	result, err := workspace.Pull(c, wsDir, section)
	if err != nil {
		output.Errorf("pull: %s", err)
	}

	if output.JSONMode {
		output.JSON(result)
		return
	}

	output.Success(fmt.Sprintf("Pulled %d strings into %d files", result.StringCount, result.FileCount))
	if len(result.Sections) > 0 {
		fmt.Printf("  Sections: %s\n", strings.Join(result.Sections, ", "))
	}
}

// --- MCP commands ---

func handleMCP(args []string) {
	if len(args) == 0 {
		output.Errorf("usage: airstrings mcp <install|uninstall|status>")
	}

	switch args[0] {
	case "install":
		claudeDesktop := false
		for _, a := range args[1:] {
			if a == "--claude-desktop" {
				claudeDesktop = true
			}
		}
		handleMCPInstall(claudeDesktop)
	case "uninstall":
		claudeDesktop := false
		for _, a := range args[1:] {
			if a == "--claude-desktop" {
				claudeDesktop = true
			}
		}
		handleMCPUninstall(claudeDesktop)
	case "status":
		handleMCPStatus()
	default:
		output.Errorf("unknown mcp command: %s", args[0])
	}
}

// findMCPBinary locates the airstrings-mcp binary.
// Checks: 1) next to this binary, 2) in PATH.
func findMCPBinary() (string, error) {
	// Check next to the current executable
	exe, err := os.Executable()
	if err == nil {
		exe, _ = filepath.EvalSymlinks(exe)
		sibling := filepath.Join(filepath.Dir(exe), "airstrings-mcp")
		if _, err := os.Stat(sibling); err == nil {
			return sibling, nil
		}
	}

	// Check PATH
	path, err := exec.LookPath("airstrings-mcp")
	if err == nil {
		abs, _ := filepath.Abs(path)
		return abs, nil
	}

	return "", fmt.Errorf("airstrings-mcp not found — install it with: brew install symbionix-sl/airstrings/airstrings")
}

// claudeDesktopConfigPath returns the path to Claude Desktop's config.
func claudeDesktopConfigPath() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	}
	// Linux / Windows fallback
	return filepath.Join(home, ".config", "claude", "claude_desktop_config.json")
}

func handleMCPInstall(claudeDesktop bool) {
	mcpBin, err := findMCPBinary()
	if err != nil {
		output.Errorf("%s", err)
	}

	if claudeDesktop {
		installMCPDesktop(mcpBin)
	} else {
		installMCPClaudeCode(mcpBin)
	}
}

func installMCPClaudeCode(mcpBin string) {
	// Use `claude mcp add` — the official way to register MCP servers
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		output.Errorf("claude CLI not found — install Claude Code first: https://docs.anthropic.com/en/docs/claude-code")
	}

	// Remove existing first (ignore errors if not present)
	exec.Command(claudeBin, "mcp", "remove", "airstrings").Run()

	// Add via claude CLI with --scope user for global availability
	cmd := exec.Command(claudeBin, "mcp", "add", "--scope", "user", "airstrings", mcpBin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		output.Errorf("claude mcp add failed: %s\n%s", err, string(out))
	}

	output.Success("AirStrings MCP installed for Claude Code")
	fmt.Printf("  Binary: %s\n", mcpBin)
	fmt.Println("\n  Restart Claude Code to activate.")
}

func installMCPDesktop(mcpBin string) {
	settingsPath := claudeDesktopConfigPath()

	settings := make(map[string]any)
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		json.Unmarshal(data, &settings)
	}

	mcpServers, ok := settings["mcpServers"].(map[string]any)
	if !ok {
		mcpServers = make(map[string]any)
	}

	mcpServers["airstrings"] = map[string]any{
		"command": mcpBin,
	}
	settings["mcpServers"] = mcpServers

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		output.Errorf("create config dir: %s", err)
	}
	out, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(settingsPath, out, 0644); err != nil {
		output.Errorf("write settings: %s", err)
	}

	output.Success("AirStrings MCP installed for Claude Desktop")
	fmt.Printf("  Binary:   %s\n", mcpBin)
	fmt.Printf("  Settings: %s\n", settingsPath)
	fmt.Println("\n  Restart Claude Desktop to activate.")
}

func handleMCPUninstall(claudeDesktop bool) {
	if claudeDesktop {
		uninstallMCPDesktop()
	} else {
		uninstallMCPClaudeCode()
	}
}

func uninstallMCPClaudeCode() {
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		output.Errorf("claude CLI not found")
	}

	cmd := exec.Command(claudeBin, "mcp", "remove", "airstrings")
	out, err := cmd.CombinedOutput()
	if err != nil {
		output.Errorf("claude mcp remove failed: %s\n%s", err, string(out))
	}

	output.Success("AirStrings MCP removed from Claude Code")
}

func uninstallMCPDesktop() {
	settingsPath := claudeDesktopConfigPath()

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		output.Errorf("no settings found at %s", settingsPath)
	}

	settings := make(map[string]any)
	json.Unmarshal(data, &settings)

	mcpServers, ok := settings["mcpServers"].(map[string]any)
	if !ok {
		output.Success("AirStrings MCP not found in Claude Desktop")
		return
	}

	if _, ok := mcpServers["airstrings"]; !ok {
		output.Success("AirStrings MCP not found in Claude Desktop")
		return
	}

	delete(mcpServers, "airstrings")
	settings["mcpServers"] = mcpServers

	out, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(settingsPath, out, 0644)

	output.Success("AirStrings MCP removed from Claude Desktop")
}

func handleMCPStatus() {
	mcpBin, binErr := findMCPBinary()

	fmt.Println("AirStrings MCP Status")
	fmt.Println()

	// Binary
	if binErr != nil {
		fmt.Println("  Binary:         not found")
	} else {
		fmt.Printf("  Binary:         %s\n", mcpBin)
	}

	// Claude Code — check via `claude mcp list`
	ccInstalled := false
	if claudeBin, err := exec.LookPath("claude"); err == nil {
		out, err := exec.Command(claudeBin, "mcp", "list").CombinedOutput()
		if err == nil && strings.Contains(string(out), "airstrings") {
			ccInstalled = true
		}
	}
	if ccInstalled {
		fmt.Println("  Claude Code:    installed")
	} else {
		fmt.Println("  Claude Code:    not installed")
	}

	// Claude Desktop — check config file
	cdInstalled := false
	cdPath := claudeDesktopConfigPath()
	if data, err := os.ReadFile(cdPath); err == nil {
		var s map[string]any
		json.Unmarshal(data, &s)
		if servers, ok := s["mcpServers"].(map[string]any); ok {
			if _, ok := servers["airstrings"]; ok {
				cdInstalled = true
			}
		}
	}
	if cdInstalled {
		fmt.Println("  Claude Desktop: installed")
	} else {
		fmt.Println("  Claude Desktop: not installed")
	}
}
