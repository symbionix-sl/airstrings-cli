package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/symbionix/airstrings-cli/internal/client"
	"github.com/symbionix/airstrings-cli/internal/config"
	"github.com/symbionix/airstrings-cli/internal/output"
)

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
	case "help", "--help", "-h":
		printUsage()
	case "version", "--version":
		fmt.Println("airstrings v0.1.0")
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
