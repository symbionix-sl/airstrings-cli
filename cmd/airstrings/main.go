package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/symbionix/airstrings-cli/internal/client"
	"github.com/symbionix/airstrings-cli/internal/config"
	"github.com/symbionix/airstrings-cli/internal/output"
	"github.com/symbionix/airstrings-cli/internal/workspace"
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

	cmd := args[0]
	args = args[1:]

	switch cmd {
	case "profile":
		handleProfile(args)
	case "project":
		handleProject(args)
	case "envs":
		handleEnvs(args)
	case "strings":
		handleStrings(args)
	case "sections":
		handleSections(args)
	case "bundles":
		handleBundles(args)
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

Profile Management:
  profile add <name> --key <api-key> [--url <base-url>] [--env <env-id>]
  profile set-key <new-api-key> [--profile <name>]
  profile list
  profile use <name>
  profile remove <name>
  profile show

Project & Environments:
  project                 Show current project info
  envs                    List environments
  envs create <name>      Create environment
  locales                 List locales with string counts

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
  publish [locale...]     Publish bundles (all locales if none specified)

Import:
  import csv <file>       Import strings from CSV file
  import status <id>      Check import status

Workspace:
  init [--profile <name>]                          Initialize local workspace
  local set <key> <locale>=<value> [--format text|icu] [--section <name>]
  local rm <key> [--locale <loc>] [--section <name>]
  local ls [--section <name>]                      List local strings
  push [--section <name>]                          Push local strings to API
  pull [--section <name>]                          Pull remote strings to local

MCP:
  mcp install                  Install MCP server for Claude Code
  mcp install --claude-desktop Install MCP server for Claude Desktop
  mcp uninstall                Remove MCP server from Claude Code
  mcp status                   Check MCP installation status

Flags:
  --json                  Output as JSON (works with any command)

`)
}

// mustClient loads config and returns a ready API client.
func mustClient() *client.Client {
	cfg, err := config.Load()
	if err != nil {
		output.Errorf("load config: %s", err)
	}

	prof, err := cfg.Active()
	if err != nil {
		output.Errorf("%s", err)
	}

	if prof.APIKey == "" {
		output.Errorf("no API key set for profile %q", cfg.ActiveProfile)
	}

	return client.New(prof.APIKey, prof.BaseURL, prof.ProjectID, prof.EnvID)
}

// resolveEnvID ensures the client has an env ID, fetching the default if needed.
func resolveEnvID(c *client.Client) *client.Client {
	if c.EnvID() != "" {
		return c
	}
	// Auto-detect: use the default environment
	envs, err := c.ListEnvironments()
	if err != nil {
		output.Errorf("list environments: %s", err)
	}
	for _, e := range envs {
		if e.IsDefault {
			return client.New(
				"", // will be filled from the original client internals
				"", "", e.ID,
			)
		}
	}
	output.Errorf("no default environment found — set one with: airstrings profile add ... --env <env_id>")
	return nil
}

// --- Profile commands ---

func handleProfile(args []string) {
	if len(args) == 0 {
		handleProfileShow()
		return
	}

	switch args[0] {
	case "add":
		handleProfileAdd(args[1:])
	case "list", "ls":
		handleProfileList()
	case "use":
		if len(args) < 2 {
			output.Errorf("usage: airstrings profile use <name>")
		}
		handleProfileUse(args[1])
	case "remove", "rm":
		if len(args) < 2 {
			output.Errorf("usage: airstrings profile remove <name>")
		}
		handleProfileRemove(args[1])
	case "set-key":
		if len(args) < 2 {
			output.Errorf("usage: airstrings profile set-key <new-api-key> [--profile <name>]")
		}
		handleProfileSetKey(args[1:])
	case "show":
		handleProfileShow()
	default:
		output.Errorf("unknown profile command: %s", args[0])
	}
}

func handleProfileAdd(args []string) {
	if len(args) < 1 {
		output.Errorf("usage: airstrings profile add <name> --key <api-key> [--url <base-url>] [--env <env-id>]")
	}

	name := args[0]
	var apiKey, baseURL, envID string

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--key", "-k":
			i++
			if i < len(args) {
				apiKey = args[i]
			}
		case "--url":
			i++
			if i < len(args) {
				baseURL = args[i]
			}
		case "--env":
			i++
			if i < len(args) {
				envID = args[i]
			}
		}
	}

	if apiKey == "" {
		output.Errorf("--key is required")
	}

	cfg, err := config.Load()
	if err != nil {
		output.Errorf("load config: %s", err)
	}

	// Auto-detect project ID and env ID by calling the API
	c := client.New(apiKey, baseURL, "", "")
	proj, err := c.GetProject()
	if err != nil {
		output.Errorf("validate API key: %s", err)
	}

	projectID := proj.ID

	// If no env specified, auto-detect: prefer default, fall back to first
	if envID == "" {
		c2 := client.New(apiKey, baseURL, projectID, "")
		envs, err := c2.ListEnvironments()
		if err != nil {
			output.Errorf("list environments: %s", err)
		}
		for _, e := range envs {
			if e.IsDefault {
				envID = e.ID
				break
			}
		}
		if envID == "" && len(envs) > 0 {
			envID = envs[0].ID
		}
	}

	cfg.Profiles[name] = config.Profile{
		Name:      name,
		APIKey:    apiKey,
		BaseURL:   baseURL,
		ProjectID: projectID,
		EnvID:     envID,
	}

	if cfg.ActiveProfile == "" {
		cfg.ActiveProfile = name
	}

	if err := cfg.Save(); err != nil {
		output.Errorf("save config: %s", err)
	}

	output.Success(fmt.Sprintf("Profile %q added (project: %s, env: %s)", name, proj.Name, envID))
	if cfg.ActiveProfile == name {
		fmt.Println("  → set as active profile")
	}
}

func handleProfileList() {
	cfg, err := config.Load()
	if err != nil {
		output.Errorf("load config: %s", err)
	}

	if len(cfg.Profiles) == 0 {
		fmt.Println("No profiles configured. Run: airstrings profile add <name> --key <api-key>")
		return
	}

	if output.JSONMode {
		output.JSON(cfg.Profiles)
		return
	}

	headers := []string{"NAME", "PROJECT", "ENV", "URL", "ACTIVE"}
	var rows [][]string
	for name, p := range cfg.Profiles {
		active := ""
		if name == cfg.ActiveProfile {
			active = "✓"
		}
		url := p.BaseURL
		if url == "" {
			url = "production"
		}
		rows = append(rows, []string{name, p.ProjectID, p.EnvID, url, active})
	}
	output.Table(headers, rows)
}

func handleProfileUse(name string) {
	cfg, err := config.Load()
	if err != nil {
		output.Errorf("load config: %s", err)
	}

	if _, ok := cfg.Profiles[name]; !ok {
		output.Errorf("profile %q not found", name)
	}

	cfg.ActiveProfile = name
	if err := cfg.Save(); err != nil {
		output.Errorf("save config: %s", err)
	}

	output.Success(fmt.Sprintf("Switched to profile %q", name))
}

func handleProfileRemove(name string) {
	cfg, err := config.Load()
	if err != nil {
		output.Errorf("load config: %s", err)
	}

	if _, ok := cfg.Profiles[name]; !ok {
		output.Errorf("profile %q not found", name)
	}

	delete(cfg.Profiles, name)
	if cfg.ActiveProfile == name {
		cfg.ActiveProfile = ""
		for n := range cfg.Profiles {
			cfg.ActiveProfile = n
			break
		}
	}

	if err := cfg.Save(); err != nil {
		output.Errorf("save config: %s", err)
	}

	output.Success(fmt.Sprintf("Profile %q removed", name))
}

func handleProfileSetKey(args []string) {
	newKey := args[0]
	profileName := ""

	for i := 1; i < len(args); i++ {
		if args[i] == "--profile" || args[i] == "-p" {
			i++
			if i < len(args) {
				profileName = args[i]
			}
		}
	}

	cfg, err := config.Load()
	if err != nil {
		output.Errorf("load config: %s", err)
	}

	if profileName == "" {
		profileName = cfg.ActiveProfile
	}
	if profileName == "" {
		output.Errorf("no active profile — specify one with --profile <name>")
	}

	prof, ok := cfg.Profiles[profileName]
	if !ok {
		output.Errorf("profile %q not found", profileName)
	}

	// Validate the new key against the API
	c := client.New(newKey, prof.BaseURL, "", "")
	proj, err := c.GetProject()
	if err != nil {
		output.Errorf("validate new key: %s", err)
	}

	oldPrefix := prof.APIKey[:8]
	prof.APIKey = newKey
	prof.ProjectID = proj.ID

	// Re-detect env if it changed
	c2 := client.New(newKey, prof.BaseURL, proj.ID, "")
	envs, err := c2.ListEnvironments()
	if err == nil {
		found := false
		for _, e := range envs {
			if e.ID == prof.EnvID {
				found = true
				break
			}
		}
		if !found && len(envs) > 0 {
			// Env no longer valid, pick default or first
			for _, e := range envs {
				if e.IsDefault {
					prof.EnvID = e.ID
					found = true
					break
				}
			}
			if !found {
				prof.EnvID = envs[0].ID
			}
		}
	}

	cfg.Profiles[profileName] = prof
	if err := cfg.Save(); err != nil {
		output.Errorf("save config: %s", err)
	}

	output.Success(fmt.Sprintf("Profile %q key updated: %s... → %s... (project: %s)",
		profileName, oldPrefix, newKey[:8], proj.Name))
}

func handleProfileShow() {
	cfg, err := config.Load()
	if err != nil {
		output.Errorf("load config: %s", err)
	}

	prof, err := cfg.Active()
	if err != nil {
		output.Errorf("%s", err)
	}

	if output.JSONMode {
		output.JSON(prof)
		return
	}

	fmt.Printf("Active profile: %s\n", cfg.ActiveProfile)
	fmt.Printf("  Project:  %s\n", prof.ProjectID)
	fmt.Printf("  Env:      %s\n", prof.EnvID)
	url := prof.BaseURL
	if url == "" {
		url = "https://api.airstrings.com (production)"
	}
	fmt.Printf("  API URL:  %s\n", url)
	fmt.Printf("  Key:      %s...%s\n", prof.APIKey[:8], prof.APIKey[len(prof.APIKey)-4:])
}

// --- Project commands ---

func handleProject(args []string) {
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

func handleEnvs(args []string) {
	c := mustClient()

	if len(args) > 0 && args[0] == "create" {
		if len(args) < 2 {
			output.Errorf("usage: airstrings envs create <name>")
		}
		env, err := c.CreateEnvironment(client.CreateEnvRequest{Name: args[1]})
		if err != nil {
			output.Errorf("create environment: %s", err)
		}
		output.Success(fmt.Sprintf("Environment %q created (id: %s)", env.Name, env.ID))
		return
	}

	envs, err := c.ListEnvironments()
	if err != nil {
		output.Errorf("list environments: %s", err)
	}

	if output.JSONMode {
		output.JSON(envs)
		return
	}

	headers := []string{"ID", "NAME", "DEFAULT", "SEALED"}
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
		rows = append(rows, []string{e.ID, e.Name, def, sealed})
	}
	output.Table(headers, rows)
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

	list, err := c.ListStrings(opts)
	if err != nil {
		output.Errorf("list strings: %s", err)
	}

	if output.JSONMode {
		output.JSON(list)
		return
	}

	headers := []string{"KEY", "FORMAT", "LOCALES", "SECTION"}
	var rows [][]string
	for _, s := range list.Data {
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

	if list.Pagination.HasMore {
		fmt.Printf("\n(more results available — use --limit or pagination)\n")
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
			fmt.Printf("  ✓ %s  rev %d  (%d strings, %.1fKB)\n",
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
		status, err := c.CreateImport(client.CreateImportRequest{
			Format: "csv",
			Data:   string(data),
		})
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
		fmt.Printf("  Imported: %d  Skipped: %d  Errors: %d\n",
			status.ImportedCount, status.SkippedCount, status.ErrorCount)

	default:
		output.Errorf("unknown import command: %s", args[0])
	}
}

// --- Workspace commands ---

func handleInit(args []string) {
	profileName := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--profile" || args[i] == "-p" {
			i++
			if i < len(args) {
				profileName = args[i]
			}
		}
	}

	cfg, err := config.Load()
	if err != nil {
		output.Errorf("load config: %s", err)
	}

	if profileName == "" {
		profileName = cfg.ActiveProfile
	}
	if profileName == "" {
		output.Errorf("no active profile — run: airstrings profile add <name>")
	}

	prof, ok := cfg.Profiles[profileName]
	if !ok {
		output.Errorf("profile %q not found", profileName)
	}

	// Validate credentials
	c := client.New(prof.APIKey, prof.BaseURL, prof.ProjectID, prof.EnvID)
	proj, err := c.GetProject()
	if err != nil {
		output.Errorf("validate credentials: %s", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		output.Errorf("get working directory: %s", err)
	}

	wsCfg := workspace.WorkspaceConfig{
		Profile:   profileName,
		ProjectID: prof.ProjectID,
		EnvID:     prof.EnvID,
		BaseURL:   prof.BaseURL,
	}
	if err := workspace.Init(cwd, wsCfg); err != nil {
		output.Errorf("init workspace: %s", err)
	}

	// Check for remote sections and create local dirs
	wsPath := cwd + "/.airstrings"
	sections, err := c.ListSections()
	sectionCount := 0
	if err == nil && len(sections.Data) > 0 {
		for _, sec := range sections.Data {
			workspace.CreateSectionDir(wsPath, sec.Name)
		}
		sectionCount = len(sections.Data)
	}

	if output.JSONMode {
		output.JSON(map[string]any{
			"project":  proj.Name,
			"profile":  profileName,
			"sections": sectionCount,
		})
		return
	}

	output.Success(fmt.Sprintf("Workspace initialized for %q (profile: %s)", proj.Name, profileName))
	if sectionCount > 0 {
		fmt.Printf("  Sections: %d initialized\n", sectionCount)
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
	Section string         `json:"section"`
	Row     workspace.Row  `json:"row"`
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

	result, err := workspace.Push(c, wsDir, section)
	if err != nil {
		output.Errorf("push: %s", err)
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
