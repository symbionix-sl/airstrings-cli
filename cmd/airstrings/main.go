package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/symbionix-sl/airstrings-cli/internal/bundlepull"
	"github.com/symbionix-sl/airstrings-cli/internal/client"
	"github.com/symbionix-sl/airstrings-cli/internal/doctor"
	"github.com/symbionix-sl/airstrings-cli/internal/guide"
	"github.com/symbionix-sl/airstrings-cli/internal/output"
	"github.com/symbionix-sl/airstrings-cli/internal/workspace"
)

var version = "dev"

func init() { client.UserAgent = "airstrings-cli/" + version }

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	// Check for --json and --project anywhere
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json":
			output.JSONMode = true
		case args[i] == "--project" && i+1 < len(args):
			i++
			workspace.ProjectFlag = args[i]
		case strings.HasPrefix(args[i], "--project="):
			workspace.ProjectFlag = strings.TrimPrefix(args[i], "--project=")
		default:
			filtered = append(filtered, args[i])
		}
	}
	args = filtered

	for _, a := range args {
		if a == "--help" || a == "-h" {
			target := ""
			for _, t := range args {
				if !strings.HasPrefix(t, "-") {
					target = t
					break
				}
			}
			printCommandHelp(target)
			os.Exit(0)
		}
	}

	args, done := handleShorthandFlags(args)
	if done {
		return
	}

	cmd := args[0]
	args = args[1:]

	switch cmd {
	case "login":
		handleLogin(args)
	case "logout":
		handleLogout(args)
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
	case "shared":
		handleShared(args)
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
	case "push":
		handlePush(args)
	case "pull":
		handlePull(args)
	case "promote":
		handlePromote(args)
	case "sdk-config":
		handleSDKConfig(args)
	case "variants":
		handleVariants(args)
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
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Print(`airstrings — AirStrings CLI

Usage: airstrings <command> [options]

Setup:
  init [<api-key>] [--name <name>]      Bind this folder to a project. With no key
       [--url <base-url>] [--purge]     it logs in and creates a project named
                                        after the folder (--project to reuse one)
  login [--no-browser]                  Log in through the browser; stores an
                                        org key in ~/.config/airstrings
  logout                                Revoke and forget the stored org key
  status                                Show active project, environment, and key

Navigation:
  project                               Show current project info
  project ls                            List the org's projects (org key)
  env                                   List every environment: protected/open,
                                        key in workspace or not (✓ = active)
  env use <name>                        Switch active environment
  env add <api-key> [--url <base-url>]  Add a legacy environment key (deprecated)
  env rm <name>                         Remove environment credentials
  -e -u <env-name> [command]            Switch environment (shorthand), then run command
  locales                               List locales with string counts
  sdk-config [--env <name>]             IDs + public keys + SDK snippets for an
                                        environment (default: production). Works
                                        with any key of the project

API keys:
  apikey ls                             List the project's keys
  apikey rotate [--env <name>]          Rotate the workspace API key

Strings:
  strings ls [--local] [--section <name>] [--locale <loc>]
             [--limit <n>] [--cursor <c>] [--key-prefix <p>]
                          List strings (remote by default); --local reads the
                          workspace CSVs offline (no credentials needed).
                          --limit/--cursor page results; --json includes
                          pagination.next_cursor for the next page
  strings get <key>
  strings set <key> <locale>=<value>... --format text|icu [--section <name>] [--push]
                          Write to local CSVs; --push also upserts the key to the API
  strings rm <key> [--locale <loc>] [--section <name>] [--push]
                          Remove from local CSVs; --push also removes from the API

Shared (org shared-key bucket):
  shared init <api-key> [--url <base-url>]
                                       Save the bucket's scoped write key to
                                       .airstrings/config.json (or set
                                       AIRSTRINGS_SHARED_API_KEY to override)
  shared ls [--section <name>] [--locale <loc>] [--limit <n>] [--cursor <c>]
            [--key-prefix <p>]         List the shared bucket's strings
  shared add <key> <locale>=<value>... --format text|icu
                                       Upsert a shared key directly to the API
  shared rm <key>                      Delete a shared key
  shared import <file>                 Import a CSV into the shared bucket

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
  doctor [dir] [--no-input]
                          Verify bundled-fallback integration in this project

Promote:
  promote preview [--from <env>] [--to <env>]
                          Preview the pending string diff between two environments
                          (defaults: from = active env, to = default env). Read-only;
                          a human applies it at the dashboard link printed last.
  promote --to <env> [--from <env>]
                          Apply the promotion (full-power org key only)

Variants (A/B experiments on a string):
  variants create <key>                            Create an experiment (control 100%)
  variants set <key> <variant> <locale>=<value>... Set a variant's text (text only)
  variants allocation <key> <variant>=<pct>...     Set the traffic split
  variants start|stop|status|rm <key>              Start/stop/inspect/delete
  variants rm-variant <key> <variant>              Remove one variant (reallocated)
  variants promote <key> <variant>                 Promote a winning variant into the
                                                   base string (distinct from 'promote',
                                                   which promotes between environments)

Import:
  import csv <file>       Import strings from CSV file
  import status <id>      Check import status

Workspace:
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
  --project <id|name>     Run against another project of the org (org key)
  <command> --help        Show help for a command

Environment variables (headless / CI — no 'init' needed):
  AIRSTRINGS_API_KEY      Project key; overrides the workspace credential. For
                          CI, create a project key in the dashboard.
  AIRSTRINGS_ORG_API_KEY  Org key; wins over every other key. In CI prefer a
                          project key; for CI across several projects use a
                          gated (non-full-power) org key from the dashboard
  AIRSTRINGS_PROJECT_ID   Skip project resolution (one fewer API call)
  AIRSTRINGS_ENV_ID       Skip environment resolution
  AIRSTRINGS_BASE_URL     API base URL (default https://api.airstrings.com)
  AIRSTRINGS_NO_BROWSER   Print the login URL without opening a browser
  NO_COLOR                Disable colored output

Exit codes:
  0 ok   1 error   2 usage   3 auth / missing key   4 not-found   5 network
  6 rate-limited   7 environment protected   8 plan limit   9 login pending
  Exit 9 prints {"status":"pending","verification_uri_complete",...}: open the
  URL, approve, then re-run the same command.
  With --json, errors are {"error":{"message","next_step","exit_code"}} on stderr.

`)
}

var commandHelp = map[string]string{
	"init": `Usage: airstrings init [<api-key>] [--name <name>] [--url <base-url>] [--purge]

Bind this folder to a project and write .airstrings/config.json.

  No key        Uses AIRSTRINGS_ORG_API_KEY, AIRSTRINGS_API_KEY or the stored
                login; with none, runs airstrings login first. An org key
                creates a project named after the folder (--project <id> to
                bind an existing one). Never reuses a project by name.
  Project key   Binds to the key's project. In an existing workspace it
                switches to the key in place, keeping local strings.

Without a terminal the login opens the browser and waits up to 90 s for
approval. With --no-browser or CI, or on timeout, it exits 9 with the URL:
approve, then re-run the same command.

Flags:
  --name <name>             Name for the created project
  --url, --base-url <url>   API base URL (default https://api.airstrings.com)
  --no-browser              Print the login URL without opening a browser
  --purge                   Re-init and remove local strings
`,
	"login": `Usage: airstrings login [--no-browser] [--url <base-url>]

Log in through the browser. An owner of the organization approves the request,
then an org key is stored in ~/.config/airstrings/credentials.json (0600).
Logging in again revokes the previous key. Without a terminal it waits up to
90 s; with --no-browser or CI, or on timeout, it exits 9 with the URL.
`,
	"logout": `Usage: airstrings logout [--url <base-url>]

Revoke the stored org key and remove it from credentials.json.
`,
	"status": `Usage: airstrings status

Show active project, environment, key scope (read/write), protection
(protected/open) and the one next step for this state.
`,
	"sdk-config": `Usage: airstrings sdk-config [--env <name>]

Print the organization, project and environment IDs plus the environment's
Ed25519 public key(s), and a ready-to-paste initializer for the Web, React
Native, iOS, Android and Go SDKs. Defaults to the production (default) environment.

Works with any key of the project — a staging key can print production's SDK
config, so a workspace never needs a production key just to set up an SDK.
Every value printed is public, not a secret: safe to show, commit and embed.

Flags:
  --env <name>            Environment name or ID (default: production)
`,
	"project": `Usage: airstrings project [ls]

Show current project info. 'project ls' lists the org's projects (org key).
`,
	"env": `Usage: airstrings env [use|add|rm|create] [options]

  env                                   List every environment: protected/open,
                                        key in workspace or not (✓ = active)
  env use <name>                        Switch active environment
  env add <api-key> [--url <base-url>]  Add a legacy environment key (deprecated)
  env rm <name>                         Remove environment credentials
  env create <name>                     Create a new environment
  -e -u <env-name> [command]            Switch environment (shorthand), then run command

Flags:
  --url, --base-url <url>   API base URL (env add)
`,
	"apikey": `Usage: airstrings apikey <ls|rotate> [--env <name>]

  apikey ls       List the project's keys (scope, permission, prefix)
  apikey rotate   Rotate the workspace key. A project key is revoked and
                  replaced in one call

Flags:
  --env <name>              Rotate the key for a specific environment
                            (default: active environment)
`,
	"strings": `Usage: airstrings strings <ls|get|set|rm> [options]

  strings ls [--local] [--section <name>] [--locale <loc>]
             [--limit <n>] [--cursor <c>] [--key-prefix <p>]
                          List strings (remote by default); --local reads the
                          workspace CSVs offline (no credentials needed).
                          --limit/--cursor page results; --json includes
                          pagination.next_cursor for the next page
  strings get <key>
  strings set <key> <locale>=<value>... --format text|icu [--section <name>] [--push]
                          Write to local CSVs; --push also upserts the key to the API
  strings rm <key> [--locale <loc>] [--section <name>] [--push]
                          Remove from local CSVs; --push also removes from the API
`,
	"shared": `Usage: airstrings shared <init|ls|add|rm|import> [options]

Manage the org shared-key bucket — a normal project reached via its own scoped
write API key. Run 'shared init <api-key>' once to save the key to
.airstrings/config.json, then ls/add/rm/import use it automatically. The scoped
key self-identifies the bucket's project and environment, so no project or
environment IDs are needed.

  shared init <api-key> [--url <base-url>]
                                       Save the bucket's scoped write key locally
  shared ls [--section <name>] [--locale <loc>] [--limit <n>] [--cursor <c>]
            [--key-prefix <p>]         List the shared bucket's strings
  shared add <key> <locale>=<value>... --format text|icu
                                       Upsert a shared key directly to the API
  shared rm <key>                      Delete a shared key
  shared import <file>                 Import a CSV into the shared bucket

Environment variables (override the saved key):
  AIRSTRINGS_SHARED_API_KEY   Scoped write key for the shared bucket
  AIRSTRINGS_BASE_URL         API base URL (default https://api.airstrings.com)
`,
	"sections": `Usage: airstrings sections <list|create|delete>

  sections list
  sections create <name> [--description <desc>]
  sections delete <id>

Flags:
  --description, -d <desc>  Section description (sections create)
`,
	"bundles": `Usage: airstrings bundles [pull] [options]

  bundles                 List published bundles
  bundles pull [dir] [--locale <bcp47>]
                          Pull published, signed bundles into a committable
                          seed folder (default airstrings/bundles/) for
                          offline fallback. Distinct from 'pull', which
                          fetches draft strings as editable CSVs

Flags:
  --locale <bcp47>        Pull a single locale (bundles pull)
`,
	"publish": `Usage: airstrings publish [locale...]

Publish bundles (all locales if none specified).
`,
	"doctor": `Usage: airstrings doctor [dir] [--no-input]

Verify bundled-fallback integration in this project.

Flags:
  --no-input              Skip the interactive ignore prompt
`,
	"locales": `Usage: airstrings locales

List locales with string counts.
`,
	"import": `Usage: airstrings import <csv|status> [options]

  import csv <file>       Import strings from CSV file
  import status <id>      Check import status
`,
	"promote": `Usage: airstrings promote preview [--from <env-name>] [--to <env-name>]
       airstrings promote --to <env-name> [--from <env-name>]

'preview' shows the pending string diff between two environments, read-only,
ending with the dashboard link where a human applies the promotion.
Without 'preview' the promotion is applied: needs a full-power org key;
other keys exit 7 with the dashboard link.

  --from <env-name>   Source environment (default: active env)
  --to <env-name>     Target environment (default: default env)
`,
	"variants": `Usage: airstrings variants <create|set|allocation|start|stop|rm|rm-variant|status|promote> <key> [args]

Manage an A/B experiment on a string. An experiment holds variant texts and a
traffic allocation. Mutations edit the draft experiment — publish to apply the
change to the CDN.

  variants create <key>                   Create an experiment (control at 100%)
  variants set <key> <variant> <locale>=<value>...
                                          Set a variant's text for one or more
                                          locales (text only; a new variant is
                                          added to the allocation at 0%)
  variants allocation <key> <variant>=<pct>...
                                          Set the traffic split across variants
  variants start <key>                    Start the experiment
  variants stop <key>                     Stop the experiment (CDN is unchanged
                                          until the next publish)
  variants status <key>                   Show the experiment (404 → exit 4)
  variants rm <key>                       Delete the experiment
  variants rm-variant <key> <variant>     Remove one non-control variant; its
                                          allocation is redistributed across the
                                          remaining arms
  variants promote <key> <variant>        Promote a winning variant into the
                                          base string and republish

'variants promote' promotes a winning variant into the base string. This is
different from 'airstrings promote', which promotes strings between environments.

There is no experiment name (no --name) and no format flag (text only).
`,
	"push": `Usage: airstrings push [--section <name>]

Push local strings to API.

Flags:
  --section <name>        Push only the given section
`,
	"pull": `Usage: airstrings pull [--section <name>]

Pull remote draft strings to local CSVs (published bundles: bundles pull).

Flags:
  --section <name>        Pull only the given section
`,
	"mcp": `Usage: airstrings mcp <install|uninstall|status>

  mcp install                  Install MCP server for Claude Code
  mcp install --claude-desktop Install MCP server for Claude Desktop
  mcp uninstall                Remove MCP server from Claude Code
  mcp uninstall --claude-desktop
                               Remove MCP server from Claude Desktop
  mcp status                   Check MCP installation status
`,
}

func printCommandHelp(cmd string) {
	if h, ok := commandHelp[cmd]; ok {
		fmt.Print(h)
		return
	}
	printUsage()
}

// mustWorkspace finds and loads the workspace config, or exits with an error.
func mustWorkspace() (string, *workspace.WorkspaceConfig) {
	wsDir, err := workspace.Find()
	if err != nil {
		output.Errorf("no workspace found — run: airstrings init")
	}
	wsCfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		output.Errorf("load workspace: %s", err)
	}
	return wsDir, wsCfg
}

// mustClient returns a ready API client for the nearest workspace (if any);
// see workspace.Resolve for key, project and environment precedence.
func mustClient() *client.Client {
	var wsCfg *workspace.WorkspaceConfig
	if wsDir, err := workspace.Find(); err == nil {
		if wsCfg, err = workspace.LoadConfig(wsDir); err != nil {
			output.Errorf("load workspace: %s", err)
		}
	}
	return clientFor(wsCfg)
}

// clientFor returns a client for a command that has already loaded a workspace.
func clientFor(wsCfg *workspace.WorkspaceConfig) *client.Client {
	c, auth, err := workspace.Resolve(wsCfg)
	if err != nil {
		failResolve(err)
	}
	if strings.HasPrefix(auth.Source, "env:") {
		noticeEnvOverride()
	}
	return c
}

func failResolve(err error) {
	var usage *workspace.UsageError
	if errors.As(err, &usage) {
		output.FailNext(output.ExitUsage, usage.Message, usage.NextStep)
	}
	var apiErr *client.APIError
	var netErr *client.NetworkError
	if errors.As(err, &apiErr) || errors.As(err, &netErr) {
		failAPI("resolve credentials", err)
	}
	output.Errorf("%s", err)
}

func noticeEnvOverride() {
	if name, ok := workspace.EnvOverride(); ok {
		fmt.Fprintf(os.Stderr, "Using environment %s from AIRSTRINGS_ENV_ID (overrides the workspace's active environment).\n", client.StripControl(name))
	}
}

// mustSharedClient returns a client for the org shared-key bucket, resolved from
// AIRSTRINGS_SHARED_API_KEY. Unset key is a usage error naming the variable.
func mustSharedClient() *client.Client {
	c, err := workspace.SharedClient()
	if err != nil {
		var apiErr *client.APIError
		var netErr *client.NetworkError
		if errors.As(err, &apiErr) || errors.As(err, &netErr) {
			failAPI("resolve shared bucket credentials", err)
		}
		output.Fail(output.ExitUsage, "%s", err)
	}
	return c
}

// failAPI exits with an error code reflecting the failure class (auth, not
// found, network, rate-limited) so scripts and agents can branch on it.
func failAPI(verb string, err error) {
	code := output.ExitGeneric
	var apiErr *client.APIError
	var netErr *client.NetworkError
	switch {
	case errors.As(err, &apiErr):
		next := apiErr.Body.Error.NextStep
		msg := strings.TrimSuffix(apiErr.Error(), "\nNext step: "+next)
		output.FailNext(apiErr.ExitCode(), verb+": "+msg, next)
	case errors.As(err, &netErr):
		code = output.ExitNetwork
	}
	output.Fail(code, "%s: %s", verb, err)
}

// rollback restores a CSV after a failed --push and describes the outcome.
func rollback(restore func() error) string {
	if err := restore(); err != nil {
		return fmt.Sprintf(" (could not restore local CSV: %s — it still has the unpushed change)", err)
	}
	return " (local CSV restored — nothing changed)"
}

// handleShorthandFlags applies leading -e -u <name> flags and returns the
// remaining args; done is true when no command follows the flags.
func handleShorthandFlags(args []string) ([]string, bool) {
	if len(args) < 3 || (args[0] != "-e" && args[0] != "--env") {
		return args, false
	}
	wsDir, wsCfg := mustWorkspace()
	for len(args) > 0 && (args[0] == "-e" || args[0] == "--env") {
		if len(args) < 3 || (args[1] != "-u" && args[1] != "--use") {
			output.Fail(output.ExitUsage, "usage: airstrings -e -u <env-name> [command]")
		}
		switchEnv(wsCfg, args[2])
		args = args[3:]
	}
	if err := workspace.SaveConfig(wsDir, wsCfg); err != nil {
		output.Errorf("save workspace: %s", err)
	}
	if len(args) > 0 {
		return args, false
	}
	printStatus(wsDir, wsCfg)
	return nil, true
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

	var names []string
	for _, c := range wsCfg.Credentials {
		names = append(names, c.EnvName)
	}
	if len(wsCfg.Credentials) == 0 {
		output.Fail(output.ExitNotFound, "environment %q not found. Available: %s", name, strings.Join(names, ", "))
	}
	cred := wsCfg.Credentials[0]
	c := client.New(cred.APIKey, cred.BaseURL, wsCfg.ProjectID, cred.EnvID)
	envs, err := c.ListEnvironments()
	if err != nil {
		failAPI("list environments", err)
	}
	names = names[:0]
	for _, e := range envs {
		names = append(names, e.Name)
		if !strings.EqualFold(e.Name, name) && e.ID != name {
			continue
		}
		if e.IsSealed {
			h := guide.ProtectedNoKey(e.Name, guide.PromoteURL(c.DashboardURL(""), wsCfg.ProjectID, e.ID))
			output.FailNext(output.ExitProtected, h.Message, h.NextStep)
		}
		h := guide.OpenNoKey(e.Name, guide.APIKeysURL(c.DashboardURL(""), wsCfg.ProjectID))
		output.FailNext(output.ExitAuth, h.Message, h.NextStep)
	}
	output.Fail(output.ExitNotFound, "environment %q does not exist in this project. Available: %s", name, strings.Join(names, ", "))
}

// statusClient builds an API client the same way mustClient does (env-var auth
// wins over the workspace) but never exits: any failure yields a nil client so
// status can degrade instead of aborting.
func statusClient() *client.Client {
	var wsCfg *workspace.WorkspaceConfig
	if wsDir, err := workspace.Find(); err == nil {
		wsCfg, _ = workspace.LoadConfig(wsDir)
	}
	c, _, err := workspace.Resolve(wsCfg)
	if err != nil {
		return nil
	}
	return c
}

func statusInfo(wsCfg *workspace.WorkspaceConfig, auth workspace.Auth) guide.Details {
	return guide.Inspect(statusClient(), auth.Key, auth.HasKey(wsCfg), auth.FullPower() == true)
}

func protectionLine(d guide.Details) string {
	if d.Summary == "" {
		return "unknown (API unreachable)"
	}
	return d.Summary
}

func printHint(h *guide.Hint) {
	if h == nil {
		return
	}
	fmt.Printf("\n%s\n", h.Message)
	if h.NextStep != "" {
		fmt.Printf("Next step: %s\n", h.NextStep)
	}
}

func printStatus(wsDir string, wsCfg *workspace.WorkspaceConfig) {
	cred, err := wsCfg.ActiveCredential()
	if err != nil {
		output.Errorf("%s", err)
	}

	url := cred.BaseURL
	if url == "" {
		url = "https://api.airstrings.com"
	}

	auth, _ := workspace.ResolveAuth(wsCfg)
	info := statusInfo(wsCfg, auth)

	if output.JSONMode {
		envs := make([]map[string]any, 0, len(wsCfg.Credentials))
		for _, c := range wsCfg.Credentials {
			envs = append(envs, map[string]any{
				"env_id":   c.EnvID,
				"env_name": c.EnvName,
				"active":   c.EnvID == wsCfg.ActiveEnv,
			})
		}
		output.JSON(map[string]any{
			"source":            "workspace",
			"workspace_dir":     wsDir,
			"mode":              workspace.DetectMode(wsDir),
			"project_id":        wsCfg.ProjectID,
			"project_name":      wsCfg.ProjectName,
			"env_id":            cred.EnvID,
			"env_name":          cred.EnvName,
			"base_url":          url,
			"protection":        info.Protection,
			"protection_by_env": info.ProtectionByEnv,
			"key_scope":         info.KeyScope,
			"key_type":          auth.Type(),
			"key_source":        auth.Source,
			"full_power":        auth.FullPower(),
			"hint":              info.Hint,
			"environments":      envs,
		})
		return
	}

	fmt.Printf("Project:  %s (%s)\n", wsCfg.ProjectName, wsCfg.ProjectID)
	fmt.Printf("Env:      %s (%s)\n", cred.EnvName, cred.EnvID)
	fmt.Printf("API URL:  %s\n", url)
	fmt.Printf("Key:      %s (%s, %s, %s)\n", maskKey(auth.Key), auth.Type(), info.KeyScope, auth.Source)
	fmt.Printf("Protection: %s\n", protectionLine(info))
	printHint(info.Hint)
}

// printEnvStatus reports the credentials sourced from AIRSTRINGS_* env vars.
// Unresolved project/env are shown as resolved-on-first-call. It makes one
// best-effort API call to report the protection mode, degrading to "unknown"
// rather than failing.
func printEnvStatus(env workspace.EnvAuth) {
	base := env.BaseURL
	if base == "" {
		base = "https://api.airstrings.com"
	}
	auth := workspace.Auth{Key: env.APIKey, BaseURL: env.BaseURL, Source: env.Source}
	info := statusInfo(nil, auth)

	if output.JSONMode {
		output.JSON(map[string]any{
			"source":            "env",
			"project_id":        env.ProjectID,
			"env_id":            env.EnvID,
			"base_url":          base,
			"protection":        info.Protection,
			"protection_by_env": info.ProtectionByEnv,
			"key_scope":         info.KeyScope,
			"key_type":          auth.Type(),
			"key_source":        auth.Source,
			"full_power":        auth.FullPower(),
			"hint":              info.Hint,
		})
		return
	}

	projectID := env.ProjectID
	if projectID == "" {
		projectID = "(resolved from key on first call)"
	}
	envID := env.EnvID
	if envID == "" {
		envID = "(default env, resolved from key)"
	}
	fmt.Printf("Source:   %s\n", env.Source)
	fmt.Printf("Project:  %s\n", projectID)
	fmt.Printf("Env:      %s\n", envID)
	fmt.Printf("API URL:  %s\n", base)
	fmt.Printf("Key:      %s (%s, %s)\n", maskKey(auth.Key), auth.Type(), info.KeyScope)
	fmt.Printf("Protection: %s\n", protectionLine(info))
	printHint(info.Hint)
}

// --- Auth commands ---

func handleProjectLs() {
	var wsCfg *workspace.WorkspaceConfig
	if wsDir, err := workspace.Find(); err == nil {
		wsCfg, _ = workspace.LoadConfig(wsDir)
	}
	auth, ok := workspace.OrgAuth(wsCfg)
	if !ok {
		output.FailNext(output.ExitUsage, "listing projects needs an org key", "Run: airstrings login (or set AIRSTRINGS_ORG_API_KEY)")
	}
	projects, err := client.New(auth.Key, auth.BaseURL, "", "").ListProjects()
	if err != nil {
		failAPI("list projects", err)
	}
	rows := make([][]string, len(projects))
	for i, p := range projects {
		rows[i] = []string{p.ID, client.StripControl(p.Name), strconv.Itoa(p.StringCount)}
	}
	output.Auto(projects, []string{"ID", "NAME", "STRINGS"}, rows)
}

func handleLogin(args []string) {
	baseURL, noBrowser := parseLoginFlags(args)
	key := login(baseURL, noBrowser)
	if output.JSONMode {
		output.JSON(map[string]any{"status": "logged_in", "org_id": key.OrgID, "org_name": key.OrgName, "full_power": key.FullPower, "base_url": key.BaseURL})
		return
	}
	output.Success(fmt.Sprintf("Logged in to %s", client.StripControl(key.OrgName)))
}

func parseLoginFlags(args []string) (baseURL string, noBrowser bool) {
	baseURL = os.Getenv("AIRSTRINGS_BASE_URL")
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--url", "--base-url":
			if i+1 >= len(args) {
				output.Fail(output.ExitUsage, "%s requires a value", args[i])
			}
			i++
			baseURL = args[i]
		case "--no-browser":
			noBrowser = true
		default:
			output.Fail(output.ExitUsage, "unknown argument: %s", args[i])
		}
	}
	if baseURL == "" {
		baseURL = client.DefaultBaseURL
	}
	if err := client.ValidateBaseURL(baseURL); err != nil {
		output.Fail(output.ExitUsage, "%s", err)
	}
	return baseURL, noBrowser
}

// login runs the device flow and returns the stored org key. Every run opens
// the approval page unless the browser is disabled. Interactive runs block
// until approval. Otherwise it polls for up to 90 s, except a first run
// without an opened browser (or in CI), which exits 9 with the approval URL.
func login(baseURL string, noBrowser bool) *workspace.OrgKey {
	p, fresh, err := workspace.StartLogin(baseURL, client.ClientName(version))
	if err != nil {
		var apiErr *client.APIError
		if errors.Is(err, workspace.ErrCredStore) {
			output.FailNext(output.ExitAuth, err.Error(), workspace.CredStoreNextStep)
		}
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			output.FailNext(output.ExitAuth, "this API does not support login yet", "Use: airstrings init <api-key>")
		}
		failAPI("start login", err)
	}
	if fresh {
		fmt.Fprintf(os.Stderr, "To log in, open:\n  %s\nand check the code %s. An owner of your AirStrings organization must approve.\n", p.VerificationURIComplete, p.UserCode)
	}
	opened := !noBrowser && workspace.OpenBrowser(p.VerificationURIComplete)
	interactive := isInteractive()
	budget := workspace.PollBudget
	if interactive {
		budget = time.Until(p.ExpiresAt)
	} else if fresh && (!opened || os.Getenv("CI") != "") {
		failPending(p)
	}
	key, err := workspace.PollLogin(p, budget, time.Sleep)
	switch {
	case errors.Is(err, workspace.ErrLoginPending) && interactive:
		output.FailNext(output.ExitAuth, "the login code expired before it was approved", "Run: airstrings login")
	case errors.Is(err, workspace.ErrLoginPending):
		failPending(p)
	case errors.Is(err, workspace.ErrCredStore):
		output.FailNext(output.ExitAuth, err.Error(), workspace.CredStoreNextStep)
	case err != nil:
		failAPI("login", err)
	}
	return key
}

func failPending(p *workspace.PendingLogin) {
	next := workspace.PendingNextStep(p.VerificationURIComplete)
	if os.Getenv("CI") != "" {
		next += ". In CI, set AIRSTRINGS_API_KEY to a project key from the dashboard instead"
	}
	if output.JSONMode {
		output.JSON(map[string]any{
			"status":                    "pending",
			"verification_uri_complete": p.VerificationURIComplete,
			"user_code":                 p.UserCode,
			"expires_in":                int(time.Until(p.ExpiresAt).Seconds()),
			"next_step":                 next,
		})
	} else {
		fmt.Fprintf(os.Stderr, "Login pending: %s (code %s)\nNext step: %s\n", p.VerificationURIComplete, p.UserCode, next)
	}
	os.Exit(output.ExitAuthPending)
}

func isInteractive() bool {
	return stdinIsTTY() && output.IsTerminal(os.Stdout) && !output.JSONMode && os.Getenv("CI") == ""
}

func handleLogout(args []string) {
	baseURL, _ := parseLoginFlags(args)
	creds, err := workspace.LoadCreds()
	if err != nil {
		output.Errorf("%s", err)
	}
	creds.Pending = nil
	k := creds.OrgKey(baseURL)
	if k != nil {
		if err := client.New(k.APIKey, baseURL, "", "").RevokeOrgKey(k.KeyID); err != nil {
			output.Warnf("could not revoke the key on the server (%s); an owner can delete it in the dashboard", err)
		}
		creds.DeleteOrgKey(baseURL)
	}
	if err := workspace.SaveCreds(creds); err != nil {
		output.FailNext(output.ExitAuth, err.Error(), workspace.CredStoreNextStep)
	}
	if output.JSONMode {
		output.JSON(map[string]any{"status": "logged_out", "base_url": baseURL})
		return
	}
	if k == nil {
		output.Success("Not logged in")
		return
	}
	output.Success(fmt.Sprintf("Logged out of %s", client.StripControl(k.OrgName)))
}

// parseKeyAndURL extracts an API key and optional --url/--base-url from args.
func parseKeyAndURL(args []string) (string, string) {
	if len(args) < 1 {
		output.Fail(output.ExitUsage, "usage: provide an API key")
	}
	var apiKey string
	baseURL := os.Getenv("AIRSTRINGS_BASE_URL")
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--url", "--base-url":
			i++
			if i < len(args) {
				baseURL = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
			}
			if apiKey == "" {
				apiKey = args[i]
			}
		}
	}
	if apiKey == "" {
		output.Fail(output.ExitUsage, "usage: provide an API key")
	}
	if baseURL != "" {
		if err := client.ValidateBaseURL(baseURL); err != nil {
			output.Fail(output.ExitUsage, "%s", err)
		}
	}
	return apiKey, baseURL
}

// validateAndDiscover validates an API key and returns the project and environments.
func validateAndDiscover(apiKey, baseURL string) (*client.Project, []client.Environment) {
	c := client.New(apiKey, baseURL, "", "")
	proj, err := c.GetProject()
	if err != nil {
		failAPI("invalid API key", err)
	}

	c2 := client.New(apiKey, baseURL, proj.ID, "")
	envs, err := c2.ListEnvironments()
	if err != nil {
		failAPI("list environments", err)
	}
	return proj, envs
}

// addCredentials files a credential for each environment the key actually
// authenticates to, and returns those environments plus the active env name.
// API keys are env-scoped: an env-scoped GET 404s on any env but the key's own,
// so probing separates "envs the key can list" from "envs it can authenticate to".
func addCredentials(wsCfg *workspace.WorkspaceConfig, apiKey, baseURL, projectID string, envs []client.Environment) ([]client.Environment, string) {
	filed, err := workspace.BoundEnvs(apiKey, baseURL, projectID, envs)
	if err != nil {
		failAPI("verify environment access", err)
	}
	if len(filed) == 0 {
		output.Errorf("API key does not authenticate to any environment in this project")
	}
	for _, full := range filed {
		wsCfg.AddOrUpdate(workspace.Credential{
			APIKey:    apiKey,
			BaseURL:   baseURL,
			EnvID:     full.ID,
			EnvName:   full.Name,
			PublicKey: full.PublicKey,
		})
		if wsCfg.OrgID == "" {
			wsCfg.OrgID = full.OrganizationID
		}
	}

	var activeEnvID, activeEnvName string
	for _, env := range filed {
		if env.IsDefault || activeEnvID == "" {
			activeEnvID = env.ID
			activeEnvName = env.Name
		}
	}
	if wsCfg.ActiveEnv == "" && activeEnvID != "" {
		wsCfg.ActiveEnv = activeEnvID
	}
	return filed, activeEnvName
}

func handleStatus(args []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
	}
	if env, ok := workspace.EnvAuthFromEnv(); ok {
		printEnvStatus(env)
		return
	}
	wsDir, wsCfg := mustWorkspace()
	printStatus(wsDir, wsCfg)
}

// --- Project commands ---

func handleProject(args []string) {
	if len(args) > 0 && args[0] == "use" {
		output.Errorf("workspace is bound to one project — run: airstrings init --project <id> --purge")
	}
	if len(args) > 0 && args[0] == "ls" {
		handleProjectLs()
		return
	}
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
	}

	c := mustClient()
	proj, err := c.GetProject()
	if err != nil {
		failAPI("get project", err)
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
			output.Fail(output.ExitUsage, "usage: airstrings env use <name>")
		}
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", a)
			}
		}
		wsDir, wsCfg := mustWorkspace()
		switchEnv(wsCfg, args[1])
		if err := workspace.SaveConfig(wsDir, wsCfg); err != nil {
			output.Errorf("save workspace: %s", err)
		}
		cred, _ := wsCfg.ActiveCredential()
		if output.JSONMode {
			output.JSON(map[string]any{"active_env": cred.EnvName, "env_id": cred.EnvID, "project": wsCfg.ProjectName})
			return
		}
		output.Success(fmt.Sprintf("Switched to %s / %s", wsCfg.ProjectName, cred.EnvName))
		return
	}

	if len(args) > 0 && args[0] == "add" {
		if len(args) < 2 {
			output.Fail(output.ExitUsage, "usage: airstrings env add <api-key> [--url <base-url>]")
		}
		apiKey, baseURL := parseKeyAndURL(args[1:])
		wsDir, wsCfg := mustWorkspace()

		// Validate and discover environments for this key
		proj, envs := validateAndDiscover(apiKey, baseURL)
		filed, activeEnvName := addCredentials(wsCfg, apiKey, baseURL, proj.ID, envs)

		if err := workspace.SaveConfig(wsDir, wsCfg); err != nil {
			output.Errorf("save workspace: %s", err)
		}

		if output.JSONMode {
			names := make([]string, 0, len(filed))
			for _, env := range filed {
				names = append(names, env.Name)
			}
			output.JSON(map[string]any{"added": len(filed), "environments": names, "active_env": activeEnvName})
			return
		}

		output.Success(fmt.Sprintf("Added %d environment(s): %s", len(filed), activeEnvName))
		for _, env := range filed {
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
			output.Fail(output.ExitUsage, "usage: airstrings env rm <name>")
		}
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", a)
			}
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

		if output.JSONMode {
			output.JSON(map[string]any{"removed": name})
			return
		}
		output.Success(fmt.Sprintf("Removed %s", name))
		return
	}

	if len(args) > 0 && args[0] == "create" {
		if len(args) < 2 {
			output.Fail(output.ExitUsage, "usage: airstrings env create <name>")
		}
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", a)
			}
		}
		c := mustClient()
		env, err := c.CreateEnvironment(client.CreateEnvRequest{Name: args[1]})
		if err != nil {
			failAPI("create environment", err)
		}
		if output.JSONMode {
			output.JSON(map[string]any{"created": true, "env_id": env.ID, "env_name": env.Name})
			return
		}
		output.Success(fmt.Sprintf("Environment %q created (id: %s)", env.Name, env.ID))
		return
	}

	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
	}

	c := mustClient()
	envs, err := c.ListEnvironments()
	if err != nil {
		failAPI("list environments", err)
	}

	_, wsCfg := mustWorkspace()
	auth, _ := workspace.ResolveAuth(wsCfg)

	list := guide.EnvRows(envs, auth.HasKey(wsCfg), wsCfg.ActiveEnv)

	if output.JSONMode {
		output.JSON(list)
		return
	}

	headers := []string{"NAME", "ID", "PROTECTION", "KEY", "ACTIVE"}
	var rows [][]string
	for _, e := range list {
		name := e.Name
		if e.IsDefault {
			name += " (default)"
		}
		key := "no key"
		if e.KeyInWorkspace {
			key = "key in workspace"
		}
		active := ""
		if e.Active {
			active = "✓"
		}
		rows = append(rows, []string{name, e.ID, e.Protection, key, active})
	}
	output.Table(headers, rows)
}

// --- API key commands ---

func handleAPIKey(args []string) {
	switch {
	case len(args) > 0 && args[0] == "ls":
		handleAPIKeyLs(args[1:])
	case len(args) > 0 && args[0] == "rotate":
		handleAPIKeyRotate(args[1:])
	default:
		output.Fail(output.ExitUsage, "usage: airstrings apikey <ls|rotate>")
	}
}

func handleAPIKeyLs(args []string) {
	if len(args) > 0 {
		output.Fail(output.ExitUsage, "usage: airstrings apikey ls")
	}
	c := mustClient()
	list, err := c.ListAPIKeys()
	if err != nil {
		failAPI("list API keys", err)
	}
	names := map[string]string{}
	if envs, err := c.ListEnvironments(); err == nil {
		for _, e := range envs {
			names[e.ID] = client.StripControl(e.Name)
		}
	}
	rows := make([][]string, len(list.Data))
	for i, k := range list.Data {
		scope := k.Scope
		if k.EnvID != nil {
			scope += " (" + names[*k.EnvID] + ")"
		}
		rows[i] = []string{k.ID, client.StripControl(k.Name), scope, k.Permission, k.Prefix}
	}
	output.Auto(list, []string{"ID", "NAME", "SCOPE", "PERMISSION", "PREFIX"}, rows)
}

func maskKey(key string) string {
	if len(key) < 20 {
		return "****"
	}
	return key[:12] + "..." + key[len(key)-4:]
}

func handleAPIKeyRotate(args []string) {
	envName := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--env":
			i++
			if i < len(args) {
				envName = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
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

	if cred.APIKey == "" {
		output.FailNext(output.ExitUsage, "nothing to rotate: this workspace uses your login key, not a stored project key",
			fmt.Sprintf("Create a project key at %s/projects/%s/api-keys, then run: airstrings init <key>", client.DashboardBase(cred.BaseURL), wsCfg.ProjectID))
	}

	result, err := workspace.RotateKey(wsDir, wsCfg, cred)
	if err != nil {
		failAPI("rotate key", err)
	}

	if !result.Revoked {
		fmt.Fprintf(os.Stderr, "WARNING: failed to revoke old key %s (%s) — it is still active and must be revoked manually via the dashboard or DELETE /api-keys/%s\n",
			result.OldKeyID, result.RevokeErr, result.OldKeyID)
	}

	if output.JSONMode {
		output.JSON(result)
		return
	}

	masked := maskKey(cred.APIKey)
	if result.OldKeyID == "" {
		output.Success(fmt.Sprintf("Rotated project key — new key %s (old key revoked)", masked))
	} else if result.Revoked {
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

	switch args[0] {
	case "list", "ls":
		if hasFlag(args[1:], "--local") {
			listLocalStrings(args[1:])
			return
		}
		handleStringList(args[1:])
	case "get":
		if len(args) < 2 {
			output.Fail(output.ExitUsage, "usage: airstrings strings get <key>")
		}
		handleStringGet(mustClient(), args[1])
	case "set":
		handleStringSet(args[1:])
	case "rm":
		handleStringRm(args[1:])
	default:
		output.Fail(output.ExitUsage, "unknown strings command: %s", args[0])
	}
}

func handleStringList(args []string) {
	opts := client.ListStringsOpts{}
	keyPrefix := ""
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
		case "--cursor":
			i++
			if i < len(args) {
				opts.Cursor = args[i]
			}
		case "--key-prefix":
			i++
			if i < len(args) {
				keyPrefix = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
			}
		}
	}

	c := mustClient()

	var entries []client.StringEntry
	var page client.PaginationMeta

	// A limit (or an explicit cursor) requests a single bounded page; otherwise
	// page through everything. Bounded paging keeps output small for agents.
	if opts.Limit > 0 || opts.Cursor != "" {
		list, err := c.ListStrings(opts)
		if err != nil {
			failAPI("list strings", err)
		}
		entries = list.Data
		page = list.Pagination
	} else {
		all, err := c.ListAllStrings(opts)
		if err != nil {
			failAPI("list strings", err)
		}
		entries = all
	}

	if keyPrefix != "" {
		filtered := entries[:0]
		for _, s := range entries {
			if strings.HasPrefix(s.Key, keyPrefix) {
				filtered = append(filtered, s)
			}
		}
		entries = filtered
	}

	if output.JSONMode {
		output.JSON(map[string]any{
			"data": entries,
			"pagination": map[string]any{
				"has_more":    page.HasMore,
				"next_cursor": page.NextCursor,
			},
		})
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

	if page.HasMore {
		if page.NextCursor != "" {
			fmt.Printf("\n(more results — next: airstrings strings ls --cursor %s)\n", page.NextCursor)
		} else {
			fmt.Printf("\n(more results available)\n")
		}
	}
}

func handleStringGet(c *client.Client, key string) {
	s, err := c.GetString(key)
	if err != nil {
		failAPI("get string", err)
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

func handleStringSet(args []string) {
	if len(args) < 2 {
		output.Fail(output.ExitUsage, "usage: airstrings strings set <key> <locale>=<value> --format text|icu [--section <name>] [--push]")
	}

	key := args[0]
	format := ""
	section := ""
	push := false
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
		case "--push":
			push = true
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
			}
			parts := strings.SplitN(args[i], "=", 2)
			if len(parts) == 2 {
				values[parts[0]] = parts[1]
			} else {
				output.Fail(output.ExitUsage, "invalid format %q — expected locale=value", args[i])
			}
		}
	}

	if len(values) == 0 {
		output.Fail(output.ExitUsage, "at least one locale=value pair is required")
	}

	if format == "" {
		output.Fail(output.ExitUsage, "--format is required — must be 'text' or 'icu'")
	}
	if err := workspace.ValidateFormat(format); err != nil {
		output.Errorf("%s", err)
	}

	var warning string
	if flagged := workspace.FlagICUInText(format, values); len(flagged) > 0 {
		warning = fmt.Sprintf("text value with {…} placeholders in locale(s) %s — did you mean --format icu? text is served verbatim, braces are not interpolated", strings.Join(flagged, ", "))
		output.Warnf("%s: %s", key, warning)
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
	restore, err := workspace.Snapshot(path)
	if err != nil {
		output.Errorf("read %s: %s", path, err)
	}
	if err := workspace.SetRows(path, key, values, format); err != nil {
		output.Errorf("set rows: %s", err)
	}

	if push {
		c := mustClient()
		if err := workspace.PushKey(c, key, values, format, section); err != nil {
			failAPI(fmt.Sprintf("push %s%s", key, rollback(restore)), err)
		}
	}

	if output.JSONMode {
		out := map[string]any{
			"key":     key,
			"locales": len(values),
			"section": section,
			"format":  format,
			"pushed":  push,
		}
		if warning != "" {
			out["warning"] = warning
		}
		output.JSON(out)
		return
	}

	msg := fmt.Sprintf("Set %s — %d locale(s) [%s]", key, len(values), format)
	if section != "" {
		msg += " in " + section
	}
	if push {
		msg += " (pushed)"
	}
	output.Success(msg)
}

func handleStringRm(args []string) {
	if len(args) < 1 {
		output.Fail(output.ExitUsage, "usage: airstrings strings rm <key> [--locale <loc>] [--section <name>] [--push]")
	}

	key := args[0]
	locale := ""
	section := ""
	push := false

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
		case "--push":
			push = true
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
			}
		}
	}

	wsDir, err := workspace.Find()
	if err != nil {
		output.Errorf("%s", err)
	}

	path := workspace.CSVPath(wsDir, section)
	restore, err := workspace.Snapshot(path)
	if err != nil {
		output.Errorf("read %s: %s", path, err)
	}
	if err := workspace.RemoveRows(path, key, locale); err != nil {
		output.Errorf("remove rows: %s", err)
	}

	if push {
		c := mustClient()
		if err := workspace.PushKeyRemoval(c, key, locale); err != nil {
			failAPI(fmt.Sprintf("push removal %s%s", key, rollback(restore)), err)
		}
	}

	if output.JSONMode {
		output.JSON(map[string]any{"key": key, "locale": locale, "section": section, "pushed": push})
		return
	}

	var msg string
	if locale != "" {
		msg = fmt.Sprintf("Removed %s/%s", key, locale)
	} else {
		msg = fmt.Sprintf("Removed %s (all locales)", key)
	}
	if push {
		msg += " (pushed)"
	}
	output.Success(msg)
}

// --- Shared bucket commands ---

// handleShared manages the org shared-key bucket, a normal project reached via
// its own scoped write key in AIRSTRINGS_SHARED_API_KEY.
func handleShared(args []string) {
	if len(args) == 0 {
		output.Fail(output.ExitUsage, "usage: airstrings shared <init|ls|add|rm|import> [options]")
	}

	switch args[0] {
	case "init":
		handleSharedInit(args[1:])
	case "ls", "list":
		handleSharedList(args[1:])
	case "add":
		handleSharedAdd(args[1:])
	case "rm":
		handleSharedRm(args[1:])
	case "import":
		handleSharedImport(args[1:])
	default:
		output.Fail(output.ExitUsage, "unknown shared command: %s", args[0])
	}
}

func handleSharedInit(args []string) {
	if len(args) < 1 {
		output.Fail(output.ExitUsage, "usage: airstrings shared init <api-key> [--url <base-url>]")
	}
	apiKey, baseURL := parseKeyAndURL(args)

	cred, err := workspace.ResolveSharedCredential(apiKey, baseURL)
	if err != nil {
		failAPI("resolve shared bucket", err)
	}

	if wsDir, findErr := workspace.Find(); findErr == nil {
		cfg, loadErr := workspace.LoadConfig(wsDir)
		if loadErr != nil {
			output.Errorf("load workspace: %s", loadErr)
		}
		cfg.Shared = cred
		if saveErr := workspace.SaveConfig(wsDir, cfg); saveErr != nil {
			output.Errorf("save workspace: %s", saveErr)
		}
	} else {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			output.Errorf("get working directory: %s", cwdErr)
		}
		if initErr := workspace.Init(cwd, workspace.WorkspaceConfig{Shared: cred}); initErr != nil {
			output.Errorf("init workspace: %s", initErr)
		}
	}

	masked := apiKey
	if len(masked) > 12 {
		masked = masked[:8] + "..." + masked[len(masked)-4:]
	}
	output.Success(fmt.Sprintf("Shared bucket key saved to .airstrings/config.json (%s) — shared ls/add/rm/import will use it", masked))
}

func handleSharedList(args []string) {
	opts := client.ListStringsOpts{}
	keyPrefix := ""
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
		case "--cursor":
			i++
			if i < len(args) {
				opts.Cursor = args[i]
			}
		case "--key-prefix":
			i++
			if i < len(args) {
				keyPrefix = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
			}
		}
	}

	c := mustSharedClient()

	var entries []client.StringEntry
	var page client.PaginationMeta

	if opts.Limit > 0 || opts.Cursor != "" {
		list, err := c.ListStrings(opts)
		if err != nil {
			failAPI("list shared strings", err)
		}
		entries = list.Data
		page = list.Pagination
	} else {
		all, err := c.ListAllStrings(opts)
		if err != nil {
			failAPI("list shared strings", err)
		}
		entries = all
	}

	if keyPrefix != "" {
		filtered := entries[:0]
		for _, s := range entries {
			if strings.HasPrefix(s.Key, keyPrefix) {
				filtered = append(filtered, s)
			}
		}
		entries = filtered
	}

	if output.JSONMode {
		output.JSON(map[string]any{
			"data": entries,
			"pagination": map[string]any{
				"has_more":    page.HasMore,
				"next_cursor": page.NextCursor,
			},
		})
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

	if page.HasMore {
		if page.NextCursor != "" {
			fmt.Printf("\n(more results — next: airstrings shared ls --cursor %s)\n", page.NextCursor)
		} else {
			fmt.Printf("\n(more results available)\n")
		}
	}
}

func handleSharedAdd(args []string) {
	if len(args) < 2 {
		output.Fail(output.ExitUsage, "usage: airstrings shared add <key> <locale>=<value>... --format text|icu")
	}

	key := args[0]
	format := ""
	values := make(map[string]string)

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--format":
			i++
			if i < len(args) {
				format = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
			}
			parts := strings.SplitN(args[i], "=", 2)
			if len(parts) == 2 {
				values[parts[0]] = parts[1]
			} else {
				output.Fail(output.ExitUsage, "invalid format %q — expected locale=value", args[i])
			}
		}
	}

	if len(values) == 0 {
		output.Fail(output.ExitUsage, "at least one locale=value pair is required")
	}
	if format == "" {
		output.Fail(output.ExitUsage, "--format is required — must be 'text' or 'icu'")
	}
	if err := workspace.ValidateFormat(format); err != nil {
		output.Errorf("%s", err)
	}

	var warning string
	if flagged := workspace.FlagICUInText(format, values); len(flagged) > 0 {
		warning = fmt.Sprintf("text value with {…} placeholders in locale(s) %s — did you mean --format icu? text is served verbatim, braces are not interpolated", strings.Join(flagged, ", "))
		output.Warnf("%s: %s", key, warning)
	}

	req := client.UpsertStringRequest{Format: format, Values: make(map[string]*string, len(values))}
	for loc, val := range values {
		v := val
		req.Values[loc] = &v
	}

	c := mustSharedClient()
	if _, err := c.UpsertString(key, req); err != nil {
		failAPI(fmt.Sprintf("add %s", key), err)
	}

	if output.JSONMode {
		out := map[string]any{
			"key":     key,
			"locales": len(values),
			"format":  format,
		}
		if warning != "" {
			out["warning"] = warning
		}
		output.JSON(out)
		return
	}

	output.Success(fmt.Sprintf("Added %s to shared bucket — %d locale(s) [%s]", key, len(values), format))
}

func handleSharedRm(args []string) {
	if len(args) < 1 {
		output.Fail(output.ExitUsage, "usage: airstrings shared rm <key>")
	}

	key := args[0]
	for _, a := range args[1:] {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
	}

	c := mustSharedClient()
	if err := c.DeleteString(key); err != nil {
		failAPI(fmt.Sprintf("remove %s", key), err)
	}

	if output.JSONMode {
		output.JSON(map[string]any{"key": key, "removed": true})
		return
	}

	output.Success(fmt.Sprintf("Removed %s from shared bucket", key))
}

func handleSharedImport(args []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
	}
	if len(args) < 1 {
		output.Fail(output.ExitUsage, "usage: airstrings shared import <file>")
	}

	c := mustSharedClient()
	data, err := os.ReadFile(args[0])
	if err != nil {
		output.Errorf("read file: %s", err)
	}
	status, err := c.CreateImport(data, nil)
	if err != nil {
		failAPI("import", err)
	}

	if output.JSONMode {
		output.JSON(status)
		return
	}

	output.Success(fmt.Sprintf("Import started (id: %s, rows: %d)", status.ID, status.TotalRows))
}

// --- Section commands ---

func handleSections(args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list", "ls":
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", a)
			}
		}
		c := mustClient()
		list, err := c.ListSections()
		if err != nil {
			failAPI("list sections", err)
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
			output.Fail(output.ExitUsage, "usage: airstrings sections create <name> [--description <desc>]")
		}
		req := client.CreateSectionRequest{Name: args[1]}
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--description", "-d":
				i++
				if i < len(args) {
					req.Description = args[i]
				}
			default:
				if strings.HasPrefix(args[i], "-") {
					output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
				}
			}
		}
		c := mustClient()
		sec, err := c.CreateSection(req)
		if err != nil {
			failAPI("create section", err)
		}
		if output.JSONMode {
			output.JSON(map[string]any{"created": true, "section_id": sec.ID, "name": sec.Name})
			return
		}
		output.Success(fmt.Sprintf("Section %q created (id: %s)", sec.Name, sec.ID))

	case "delete", "rm":
		if len(args) < 2 {
			output.Fail(output.ExitUsage, "usage: airstrings sections delete <id>")
		}
		for _, a := range args[2:] {
			if strings.HasPrefix(a, "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", a)
			}
		}
		c := mustClient()
		if err := c.DeleteSection(args[1]); err != nil {
			failAPI("delete section", err)
		}
		if output.JSONMode {
			output.JSON(map[string]any{"deleted": args[1]})
			return
		}
		output.Success(fmt.Sprintf("Section %s deleted", args[1]))

	default:
		output.Fail(output.ExitUsage, "unknown sections command: %s", args[0])
	}
}

// --- Bundle commands ---

func handleBundles(args []string) {
	if len(args) > 0 && args[0] == "pull" {
		handleBundlesPull(args[1:])
		return
	}

	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
	}

	c := mustClient()

	bundles, err := c.ListBundles()
	if err != nil {
		failAPI("list bundles", err)
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
  iOS:          add the folder to your app target as a folder reference (SPM: resources: [.copy("airstrings")])
  Android:      copy or map the folder into src/main/assets/
  Web:          Node seeds from <cwd>/airstrings/bundles/ automatically; browsers import bundle JSON at build time
  React Native: require() each bundle JSON, or ship via the iOS + Android steps above (RN uses both native bundles)
  Go:           seeds from <cwd>/airstrings/bundles/ automatically, or embed the folder and pass it as Config.Seed
Then run: airstrings doctor   (verifies your project is wired up)
See: https://docs.airstrings.com/docs/specs/bundled-fallback
`

const refreshPullHint = `Bundles refreshed — rebuild your app to ship the updated strings.
New locale files must be embedded like the first pull (iOS folder ref / Android assets / Web import / RN require / Go embed).
Verify wiring: airstrings doctor
See: https://docs.airstrings.com/docs/specs/bundled-fallback
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
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
			}
			if dirArg != "" {
				output.Fail(output.ExitUsage, "usage: airstrings bundles pull [dir] [--locale <bcp47>]")
			}
			dirArg = args[i]
		}
	}

	wsDir, wsCfg := mustWorkspace()
	dir, err := bundlepull.ResolveDir(wsDir, wsCfg, dirArg)
	if err != nil {
		output.Errorf("%s", err)
	}

	c := clientFor(wsCfg)
	envName := ""
	if cred, err := wsCfg.ActiveCredential(); err == nil {
		envName = cred.EnvName
	}

	res, err := bundlepull.Pull(c, bundlepull.Options{
		Dir:        dir,
		Locale:     locale,
		EnvName:    envName,
		CLIVersion: version,
	})
	if err != nil {
		failAPI("bundles pull", err)
	}

	if res.FirstPull {
		fmt.Fprint(os.Stderr, firstPullHint)
	} else {
		fmt.Fprint(os.Stderr, refreshPullHint)
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
	noInput := false
	for _, a := range args {
		if a == "--no-input" {
			noInput = true
			continue
		}
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
		if dirArg != "" {
			output.Fail(output.ExitUsage, "usage: airstrings doctor [dir] [--no-input]")
		}
		dirArg = a
	}

	wsDir, wsCfg := mustWorkspace()
	dir, err := doctor.ResolveDir(wsDir, wsCfg, dirArg)
	if err != nil {
		output.Errorf("%s", err)
	}

	root := filepath.Dir(wsDir)
	report := doctor.Run(root, dir)

	if output.JSONMode {
		output.JSON(report)
	} else {
		printDoctorReport(report)
		if !noInput && stdinIsTTY() && report.HasMissing() {
			changed, err := doctor.PromptIgnores(report, root, os.Stdin, os.Stdout)
			if err != nil {
				output.Errorf("%s", err)
			}
			if changed {
				fmt.Printf("\n%s\n", doctorSummary(report))
			}
		}
	}

	if report.HasMissing() {
		os.Exit(1)
	}
}

func stdinIsTTY() bool {
	return output.IsTerminal(os.Stdin)
}

func printDoctorReport(rep *doctor.Report) {
	fmt.Printf("Bundles dir: %s\n", displayPath(rep.BundlesDir))
	for _, c := range rep.Checks {
		var marker string
		switch c.Status {
		case doctor.StatusOK:
			marker = "✓"
		case doctor.StatusMissing:
			marker = "✗"
		default:
			marker = "•"
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
	fmt.Printf("\n%s\n", doctorSummary(rep))
}

func doctorSummary(rep *doctor.Report) string {
	ok, missing, manual, ignored := 0, 0, 0, 0
	for _, c := range rep.Checks {
		switch c.Status {
		case doctor.StatusOK:
			ok++
		case doctor.StatusMissing:
			missing++
		case doctor.StatusIgnored:
			ignored++
		default:
			manual++
		}
	}
	s := fmt.Sprintf("%d ok, %d missing, %d manual", ok, missing, manual)
	if ignored > 0 {
		s += fmt.Sprintf(", %d ignored", ignored)
	}
	return s
}

func handlePublish(args []string) {
	var locales []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
		locales = append(locales, a)
	}

	c := mustClient()
	resp, err := c.PublishBundles(locales)
	if err != nil {
		failAPI("publish", err)
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

// --- Promote commands ---

func handlePromote(args []string) {
	switch {
	case len(args) > 0 && args[0] == "preview":
		handlePromotePreview(args[1:])
	case len(args) > 0 && strings.HasPrefix(args[0], "--"):
		handlePromoteApply(args)
	default:
		output.Fail(output.ExitUsage, "usage: airstrings promote [preview] [--from <env-name>] [--to <env-name>]")
	}
}

func handlePromoteApply(args []string) {
	c, envs, resp := previewPromotion(args)
	from, to := envDisplayName(envs, resp.SourceEnvID), envDisplayName(envs, resp.TargetEnvID)
	if len(resp.Entries) == 0 {
		if output.JSONMode {
			output.JSON(map[string]any{"keys_promoted": 0})
			return
		}
		fmt.Printf("Nothing to promote: %s matches %s\n", to, from)
		return
	}
	keys := make([]string, len(resp.Entries))
	for i, e := range resp.Entries {
		keys[i] = e.Key
	}
	total := &client.PromoteResponse{}
	for start := 0; start < len(keys); start += 500 {
		res, err := c.Promote(client.PromoteRequest{SourceEnvID: resp.SourceEnvID, TargetEnvID: resp.TargetEnvID, Keys: keys[start:min(start+500, len(keys))]})
		if err != nil {
			failAPI("promote", err)
		}
		total.KeysPromoted += res.KeysPromoted
		total.PublishResults = append(total.PublishResults, res.PublishResults...)
	}
	if output.JSONMode {
		output.JSON(total)
		return
	}
	output.Success(fmt.Sprintf("Promoted %d keys %s → %s (%d added, %d updated)", total.KeysPromoted, from, to, resp.Summary.Added, resp.Summary.Updated))
	for _, r := range total.PublishResults {
		if r.Status != "ok" {
			output.Warnf("publish %s failed: %s", r.Locale, client.StripControl(r.Error))
		}
	}
}

func previewPromotion(args []string) (*client.Client, []client.Environment, *client.PromotionPreview) {
	fromName := ""
	toName := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--from":
			i++
			if i < len(args) {
				fromName = args[i]
			}
		case "--to":
			i++
			if i < len(args) {
				toName = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
			}
			output.Fail(output.ExitUsage, "usage: airstrings promote [preview] [--from <env-name>] [--to <env-name>]")
		}
	}

	c := mustClient()
	envs, err := c.ListEnvironments()
	if err != nil {
		failAPI("list environments", err)
	}

	sourceID := c.EnvID()
	if fromName != "" {
		sourceID = resolveEnvID(envs, fromName)
	}

	targetID := ""
	for _, e := range envs {
		if e.IsDefault {
			targetID = e.ID
			break
		}
	}
	if toName != "" {
		targetID = resolveEnvID(envs, toName)
	}

	if sourceID == "" {
		output.Fail(output.ExitUsage, "could not resolve source environment — pass --from <env-name>")
	}
	if targetID == "" {
		output.Fail(output.ExitUsage, "could not resolve target environment — pass --to <env-name>")
	}
	if sourceID == targetID {
		output.Fail(output.ExitUsage, "source and target are the same environment (%s) — pass an explicit --from <env-name>. Under env-var auth without AIRSTRINGS_ENV_ID the active env IS the default, so --from must be set explicitly", envDisplayName(envs, sourceID))
	}

	resp, err := c.PromotionPreview(sourceID, targetID)
	if err != nil {
		failAPI("promotion preview", err)
	}
	resp.SourceEnvID, resp.TargetEnvID = sourceID, targetID
	return c, envs, resp
}

func handlePromotePreview(args []string) {
	c, envs, resp := previewPromotion(args)
	sourceID, targetID := resp.SourceEnvID, resp.TargetEnvID
	resp.ApplyURL = guide.PromoteURL(c.DashboardURL(""), c.ProjectID(), targetID)

	if output.JSONMode {
		output.JSON(resp)
		return
	}

	fmt.Printf("Preview %s → %s: %d added, %d updated, %d extra\n",
		envDisplayName(envs, sourceID), envDisplayName(envs, targetID),
		resp.Summary.Added, resp.Summary.Updated, resp.Summary.Extra)

	headers := []string{"KEY", "LOCALE", "CHANGE"}
	var rows [][]string
	for _, e := range resp.Entries {
		for _, l := range e.Locales {
			rows = append(rows, []string{e.Key, l.Locale, l.ChangeType})
		}
	}
	output.Table(headers, rows)
	fmt.Printf("\nApply in the dashboard: %s\n", resp.ApplyURL)
}

func handleSDKConfig(args []string) {
	envName := ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--env" && i+1 < len(args):
			i++
			envName = args[i]
		case strings.HasPrefix(args[i], "-"):
			output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
		default:
			output.Fail(output.ExitUsage, "usage: airstrings sdk-config [--env <name>]")
		}
	}

	c := mustClient()
	cfg, err := c.GetSDKConfig()
	if err != nil {
		failAPI("get sdk config", err)
	}

	var env *client.SDKEnvironment
	var names []string
	for i := range cfg.Environments {
		e := &cfg.Environments[i]
		names = append(names, e.Name)
		if (envName == "" && e.IsDefault) || (envName != "" && (strings.EqualFold(e.Name, envName) || e.ID == envName)) {
			env = e
		}
	}
	if env == nil {
		output.Fail(output.ExitNotFound, "environment %q does not exist in this project. Available: %s", envName, strings.Join(names, ", "))
	}

	if output.JSONMode {
		output.JSON(guide.SDKConfig(cfg, *env, c.BaseURL()))
		return
	}

	fmt.Printf("%s\n\n", guide.PublicNotice)
	printSDKConfig(cfg, *env, c.BaseURL())
}

func printSDKConfig(cfg *client.SDKConfig, env client.SDKEnvironment, apiBaseURL string) {
	keys := make([]string, len(env.PublicKeys))
	for i, k := range env.PublicKeys {
		keys[i] = k.PublicKey
	}
	snippets := guide.Snippets(cfg.OrgID, cfg.ProjectID, env.ID, keys, apiBaseURL)

	fmt.Printf("Organization:  %s\n", cfg.OrgID)
	fmt.Printf("Project:       %s\n", cfg.ProjectID)
	fmt.Printf("Environment:   %s (%s, %s)\n", env.Name, env.ID, guide.Protection(env.IsSealed))
	for _, k := range keys {
		fmt.Printf("Public key:    %s\n", k)
	}
	if len(keys) == 0 {
		fmt.Println("Public key:    (none)")
	}
	for _, sn := range guide.SnippetOrder {
		fmt.Printf("\n%s:\n\n%s\n", sn.Label, snippets[sn.Key])
	}
}

func resolveEnvID(envs []client.Environment, name string) string {
	for _, e := range envs {
		if strings.EqualFold(e.Name, name) || e.ID == name {
			return e.ID
		}
	}
	var names []string
	for _, e := range envs {
		names = append(names, e.Name)
	}
	output.Fail(output.ExitUsage, "environment %q not found. Available: %s", name, strings.Join(names, ", "))
	return ""
}

func envDisplayName(envs []client.Environment, id string) string {
	for _, e := range envs {
		if e.ID == id {
			return e.Name
		}
	}
	return id
}

// --- Variants (A/B experiment) commands ---

func handleVariants(args []string) {
	if len(args) == 0 {
		output.Fail(output.ExitUsage, "usage: airstrings variants <create|set|allocation|start|stop|rm|rm-variant|status|promote> <key> [args]")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "create":
		handleVariantsCreate(rest)
	case "set":
		handleVariantsSet(rest)
	case "allocation":
		handleVariantsAllocation(rest)
	case "start":
		handleVariantsStart(rest)
	case "stop":
		handleVariantsStop(rest)
	case "rm":
		handleVariantsRm(rest)
	case "rm-variant":
		handleVariantsRmVariant(rest)
	case "status":
		handleVariantsStatus(rest)
	case "promote":
		handleVariantsPromote(rest)
	default:
		output.Fail(output.ExitUsage, "unknown variants command: %s", sub)
	}
}

func variantsKey(args []string, usage string) string {
	key := ""
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
		if key != "" {
			output.Fail(output.ExitUsage, "usage: %s", usage)
		}
		key = a
	}
	if key == "" {
		output.Fail(output.ExitUsage, "usage: %s", usage)
	}
	return key
}

func nudgePublish() {
	fmt.Println("Publish to apply the change to the CDN: airstrings publish")
}

func handleVariantsCreate(args []string) {
	key := variantsKey(args, "airstrings variants create <key>")
	c := mustClient()
	exp, err := c.PutExperiment(key, map[string]int{"control": 100}, map[string]map[string]string{})
	if err != nil {
		failAPI("create experiment "+key, err)
	}
	if output.JSONMode {
		output.JSON(exp)
		return
	}
	output.Success(fmt.Sprintf("Created experiment for %s (control at 100%%)", key))
}

func handleVariantsSet(args []string) {
	usage := "usage: airstrings variants set <key> <variant> <locale>=<value>..."
	if len(args) < 3 {
		output.Fail(output.ExitUsage, "%s", usage)
	}
	for _, a := range args[:2] {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
	}
	key, variant := args[0], args[1]
	values := make(map[string]string)
	for i := 2; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
		}
		parts := strings.SplitN(args[i], "=", 2)
		if len(parts) != 2 {
			output.Fail(output.ExitUsage, "invalid format %q — expected locale=value", args[i])
		}
		values[parts[0]] = parts[1]
	}
	if len(values) == 0 {
		output.Fail(output.ExitUsage, "at least one locale=value pair is required")
	}

	c := mustClient()
	exp, err := c.GetExperiment(key)
	if err != nil {
		failAPI("get experiment "+key, err)
	}

	allocation := exp.Allocation
	if allocation == nil {
		allocation = map[string]int{}
	}
	if _, ok := allocation[variant]; !ok {
		allocation[variant] = 0
	}
	variants := exp.Variants
	if variants == nil {
		variants = map[string]map[string]string{}
	}
	if variants[variant] == nil {
		variants[variant] = map[string]string{}
	}
	for loc, val := range values {
		variants[variant][loc] = val
	}

	updated, err := c.PutExperiment(key, allocation, variants)
	if err != nil {
		failAPI("update experiment "+key, err)
	}
	if output.JSONMode {
		output.JSON(updated)
		return
	}
	output.Success(fmt.Sprintf("Set variant %q on %s — %d locale(s)", variant, key, len(values)))
	nudgePublish()
}

func handleVariantsAllocation(args []string) {
	usage := "usage: airstrings variants allocation <key> <variant>=<pct>..."
	if len(args) < 2 {
		output.Fail(output.ExitUsage, "%s", usage)
	}
	if strings.HasPrefix(args[0], "-") {
		output.Fail(output.ExitUsage, "unknown flag: %s", args[0])
	}
	key := args[0]
	pcts := make(map[string]int)
	for i := 1; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
		}
		parts := strings.SplitN(args[i], "=", 2)
		if len(parts) != 2 {
			output.Fail(output.ExitUsage, "invalid format %q — expected variant=pct", args[i])
		}
		pct, err := strconv.Atoi(parts[1])
		if err != nil {
			output.Fail(output.ExitUsage, "invalid percentage %q — expected an integer", parts[1])
		}
		pcts[parts[0]] = pct
	}
	if len(pcts) == 0 {
		output.Fail(output.ExitUsage, "at least one variant=pct pair is required")
	}

	c := mustClient()
	exp, err := c.GetExperiment(key)
	if err != nil {
		failAPI("get experiment "+key, err)
	}
	allocation := exp.Allocation
	if allocation == nil {
		allocation = map[string]int{}
	}
	for v, p := range pcts {
		allocation[v] = p
	}
	variants := exp.Variants
	if variants == nil {
		variants = map[string]map[string]string{}
	}

	updated, err := c.PutExperiment(key, allocation, variants)
	if err != nil {
		failAPI("update experiment "+key, err)
	}
	if output.JSONMode {
		output.JSON(updated)
		return
	}
	output.Success(fmt.Sprintf("Updated allocation for %s", key))
}

func handleVariantsStart(args []string) {
	key := variantsKey(args, "airstrings variants start <key>")
	c := mustClient()
	exp, err := c.StartExperiment(key)
	if err != nil {
		failAPI("start experiment "+key, err)
	}
	if output.JSONMode {
		output.JSON(exp)
		return
	}
	output.Success(fmt.Sprintf("Started experiment for %s", key))
}

func handleVariantsStop(args []string) {
	key := variantsKey(args, "airstrings variants stop <key>")
	c := mustClient()
	exp, err := c.StopExperiment(key)
	if err != nil {
		failAPI("stop experiment "+key, err)
	}
	if output.JSONMode {
		output.JSON(exp)
		return
	}
	output.Success(fmt.Sprintf("Stopped experiment for %s", key))
	nudgePublish()
}

func handleVariantsRm(args []string) {
	key := variantsKey(args, "airstrings variants rm <key>")
	c := mustClient()
	if err := c.DeleteExperiment(key); err != nil {
		failAPI("remove experiment "+key, err)
	}
	if output.JSONMode {
		output.JSON(map[string]any{"key": key, "deleted": true})
		return
	}
	output.Success(fmt.Sprintf("Removed experiment for %s", key))
}

func handleVariantsRmVariant(args []string) {
	usage := "usage: airstrings variants rm-variant <key> <variant>"
	var pos []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
		pos = append(pos, a)
	}
	if len(pos) != 2 {
		output.Fail(output.ExitUsage, "%s", usage)
	}
	key, variant := pos[0], pos[1]

	c := mustClient()
	exp, err := c.DeleteVariant(key, variant)
	if err != nil {
		failAPI("remove variant "+variant, err)
	}
	if output.JSONMode {
		output.JSON(exp)
		return
	}
	output.Success(fmt.Sprintf("Removed variant %q from %s", variant, key))
	headers := []string{"VARIANT", "ALLOCATION"}
	var rows [][]string
	for _, name := range variantNames(exp) {
		rows = append(rows, []string{name, fmt.Sprintf("%d%%", exp.Allocation[name])})
	}
	output.Table(headers, rows)
	nudgePublish()
}

func handleVariantsStatus(args []string) {
	key := variantsKey(args, "airstrings variants status <key>")
	c := mustClient()
	exp, err := c.GetExperiment(key)
	if err != nil {
		failAPI("get experiment "+key, err)
	}
	if output.JSONMode {
		output.JSON(exp)
		return
	}
	fmt.Printf("Experiment %s — status: %s\n", exp.Key, exp.Status)
	headers := []string{"VARIANT", "ALLOCATION", "LOCALES"}
	var rows [][]string
	for _, name := range variantNames(exp) {
		rows = append(rows, []string{name, fmt.Sprintf("%d%%", exp.Allocation[name]), fmt.Sprintf("%d", len(exp.Variants[name]))})
	}
	output.Table(headers, rows)
}

func handleVariantsPromote(args []string) {
	usage := "usage: airstrings variants promote <key> <variant>"
	var pos []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
		pos = append(pos, a)
	}
	if len(pos) != 2 {
		output.Fail(output.ExitUsage, "%s", usage)
	}
	key, variant := pos[0], pos[1]

	c := mustClient()
	resp, err := c.PromoteVariant(key, variant)
	if err != nil {
		failAPI("promote variant "+variant, err)
	}
	if output.JSONMode {
		output.JSON(resp)
		return
	}
	output.Success(fmt.Sprintf("Promoted variant %q on %s into the base string", variant, key))
	if len(resp.PublishResults) > 0 {
		headers := []string{"LOCALE", "STATUS"}
		var rows [][]string
		for _, p := range resp.PublishResults {
			s := p.Status
			if p.Error != "" {
				s += " (" + p.Error + ")"
			}
			rows = append(rows, []string{p.Locale, s})
		}
		output.Table(headers, rows)
	}
}

func variantNames(exp *client.Experiment) []string {
	seen := map[string]bool{}
	var names []string
	for name := range exp.Allocation {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for name := range exp.Variants {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// --- Locale commands ---

func handleLocales(args []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
	}

	c := mustClient()
	locales, err := c.ListLocales()
	if err != nil {
		failAPI("list locales", err)
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
	if len(args) == 0 {
		output.Fail(output.ExitUsage, "usage: airstrings import csv <file> | airstrings import status <id>")
	}
	for _, a := range args[1:] {
		if strings.HasPrefix(a, "-") {
			output.Fail(output.ExitUsage, "unknown flag: %s", a)
		}
	}

	switch args[0] {
	case "csv":
		if len(args) < 2 {
			output.Fail(output.ExitUsage, "usage: airstrings import csv <file>")
		}
		c := mustClient()
		data, err := os.ReadFile(args[1])
		if err != nil {
			output.Errorf("read file: %s", err)
		}
		status, err := c.CreateImport(data, nil)
		if err != nil {
			failAPI("import", err)
		}
		if output.JSONMode {
			output.JSON(status)
			return
		}
		output.Success(fmt.Sprintf("Import started (id: %s, rows: %d)", status.ID, status.TotalRows))

	case "status":
		if len(args) < 2 {
			output.Fail(output.ExitUsage, "usage: airstrings import status <id>")
		}
		c := mustClient()
		status, err := c.GetImport(args[1])
		if err != nil {
			failAPI("get import", err)
		}
		if output.JSONMode {
			output.JSON(status)
			return
		}
		fmt.Printf("Import %s: %s\n", status.ID, status.Status)
		fmt.Printf("  Created: %d  Updated: %d  Skipped: %d  Errors: %d\n",
			status.CreatedRows, status.UpdatedRows, status.SkippedRows, len(status.Errors))

	default:
		output.Fail(output.ExitUsage, "unknown import command: %s", args[0])
	}
}

// --- Workspace commands ---

// describeWorkspace reads the project and active environment names from an
// existing workspace config, with fallbacks when the config can't be read.
func describeWorkspace(wsDir string) (projName, envName string) {
	projName, envName = "this project", "the active environment"
	cfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		return projName, envName
	}
	if cfg.ProjectName != "" {
		projName = cfg.ProjectName
	}
	if cred, err := cfg.ActiveCredential(); err == nil {
		envName = cred.EnvName
	}
	return projName, envName
}

func handleInit(args []string) {
	var purge, noBrowser bool
	var apiKey, name string
	baseURL := os.Getenv("AIRSTRINGS_BASE_URL")
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--purge":
			purge = true
		case "--no-browser":
			noBrowser = true
		case "--url", "--base-url", "--name":
			if i+1 >= len(args) {
				output.Fail(output.ExitUsage, "%s requires a value", args[i])
			}
			i++
			if args[i-1] == "--name" {
				name = args[i]
			} else {
				baseURL = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") || apiKey != "" {
				output.Fail(output.ExitUsage, "unknown argument: %s", args[i])
			}
			apiKey = args[i]
		}
	}
	if baseURL != "" {
		if err := client.ValidateBaseURL(baseURL); err != nil {
			output.Fail(output.ExitUsage, "%s", err)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		output.Errorf("get working directory: %s", err)
	}

	wsDir := filepath.Join(cwd, ".airstrings")
	rebind := ""
	if _, err := os.Stat(filepath.Join(wsDir, "config.json")); err == nil {
		if purge {
			if err := os.RemoveAll(wsDir); err != nil {
				output.Errorf("remove workspace: %s", err)
			}
		} else if old, err := workspace.LoadConfig(wsDir); err == nil && client.KeyType(apiKey) == "project" {
			rebind = old.ProjectID
		} else {
			projName, envName := describeWorkspace(wsDir)
			if apiKey == "" && name == "" && workspace.ProjectFlag == "" {
				if output.JSONMode {
					output.JSON(map[string]any{"project": projName, "environment": envName, "created": false, "already_initialized": true})
					return
				}
				fmt.Printf("Workspace already initialized for %s / %s. Run: airstrings status\n", projName, envName)
				return
			}
			output.Errorf("workspace already initialized for %s / %s\n"+
				"to switch to a project key, run:\n"+
				"  airstrings init <project-key>\n"+
				"(use --purge to wipe and re-init)", projName, envName)
		}
	}

	opts := workspace.SetupOptions{APIKey: apiKey, BaseURL: baseURL, Name: name, Project: workspace.ProjectFlag}
	if rebind != "" {
		opts.Project = rebind
	}
	res, err := workspace.Setup(cwd, opts)
	if errors.Is(err, workspace.ErrNoKey) {
		loginURL := baseURL
		if loginURL == "" {
			loginURL = client.DefaultBaseURL
		}
		login(loginURL, noBrowser)
		res, err = workspace.Setup(cwd, opts)
	}
	var usage *workspace.UsageError
	if rebind != "" && errors.As(err, &usage) {
		output.FailNext(output.ExitUsage, "this key belongs to another project than this workspace ("+rebind+")",
			"Use a key for "+rebind+", or run: airstrings init <key> --purge (wipes local strings)")
	}
	if err != nil {
		failResolve(err)
	}

	out := map[string]any{
		"project_id":   res.ProjectID,
		"project":      res.ProjectName,
		"created":      res.Created,
		"environment":  res.ActiveEnvName,
		"environments": res.Environments,
		"sections":     res.Sections,
	}
	var prod client.SDKEnvironment
	if res.SDK != nil {
		for _, e := range res.SDK.Environments {
			if e.IsDefault {
				prod = e
			}
		}
		out["sdk_config"] = guide.SDKConfig(res.SDK, prod, baseURL)
	}
	if output.JSONMode {
		output.JSON(out)
		return
	}

	verb := "Workspace initialized for"
	if res.Created {
		verb = "Created project and initialized workspace for"
	}
	output.Success(fmt.Sprintf("%s %s / %s", verb, client.StripControl(res.ProjectName), res.ActiveEnvName))
	if res.Sections > 0 {
		fmt.Printf("  Sections: %d\n", res.Sections)
	}
	if res.SDK != nil {
		fmt.Printf("\nSDK setup for %s (embed in your app):\n\n", prod.Name)
		printSDKConfig(res.SDK, prod, baseURL)
	}
	fmt.Printf("\nNext: airstrings strings set <key> en=\"...\" --format text --push, then airstrings publish\n")
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func listLocalStrings(args []string) {
	section := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--section":
			i++
			if i < len(args) {
				section = args[i]
			}
		case "--local":
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
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
		switch args[i] {
		case "--section":
			i++
			if i < len(args) {
				section = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
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

	c := clientFor(wsCfg)

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
		failAPI("push", err)
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
		switch args[i] {
		case "--section":
			i++
			if i < len(args) {
				section = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", args[i])
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

	c := clientFor(wsCfg)

	// Warn about overwrite
	paths, _ := workspace.AllCSVPaths(wsDir)
	if len(paths) > 0 {
		fmt.Fprintln(os.Stderr, "Warning: local CSVs will be overwritten with remote state.")
	}

	result, err := workspace.Pull(c, wsDir, section)
	if err != nil {
		failAPI("pull", err)
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
		output.Fail(output.ExitUsage, "usage: airstrings mcp <install|uninstall|status>")
	}

	switch args[0] {
	case "install":
		claudeDesktop := false
		for _, a := range args[1:] {
			switch a {
			case "--claude-desktop":
				claudeDesktop = true
			default:
				if strings.HasPrefix(a, "-") {
					output.Fail(output.ExitUsage, "unknown flag: %s", a)
				}
			}
		}
		handleMCPInstall(claudeDesktop)
	case "uninstall":
		claudeDesktop := false
		for _, a := range args[1:] {
			switch a {
			case "--claude-desktop":
				claudeDesktop = true
			default:
				if strings.HasPrefix(a, "-") {
					output.Fail(output.ExitUsage, "unknown flag: %s", a)
				}
			}
		}
		handleMCPUninstall(claudeDesktop)
	case "status":
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "-") {
				output.Fail(output.ExitUsage, "unknown flag: %s", a)
			}
		}
		handleMCPStatus()
	default:
		output.Fail(output.ExitUsage, "unknown mcp command: %s", args[0])
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

	// Claude Code — check via `claude mcp list`
	ccInstalled := false
	if claudeBin, err := exec.LookPath("claude"); err == nil {
		out, err := exec.Command(claudeBin, "mcp", "list").CombinedOutput()
		if err == nil && strings.Contains(string(out), "airstrings") {
			ccInstalled = true
		}
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

	if output.JSONMode {
		bin := ""
		if binErr == nil {
			bin = mcpBin
		}
		output.JSON(map[string]any{
			"binary":         bin,
			"binary_found":   binErr == nil,
			"claude_code":    ccInstalled,
			"claude_desktop": cdInstalled,
		})
		return
	}

	fmt.Println("AirStrings MCP Status")
	fmt.Println()
	if binErr != nil {
		fmt.Println("  Binary:         not found")
	} else {
		fmt.Printf("  Binary:         %s\n", mcpBin)
	}
	if ccInstalled {
		fmt.Println("  Claude Code:    installed")
	} else {
		fmt.Println("  Claude Code:    not installed")
	}
	if cdInstalled {
		fmt.Println("  Claude Desktop: installed")
	} else {
		fmt.Println("  Claude Desktop: not installed")
	}
}
