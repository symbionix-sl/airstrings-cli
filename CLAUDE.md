# AirStrings CLI

Command-line interface for the AirStrings remote string management platform. Manages projects, strings, locales, sections, bundles, and imports via the AirStrings API.

## Quick Reference

```
airstrings <command> [options] [--json]
```

Binary: `cmd/airstrings/main.go` — single entry point, no subpackage commands.
Config: `.airstrings/config.json` — per-project workspace with credentials and active env.

## Architecture

```
cmd/airstrings/main.go     # CLI entry point, command routing, all handlers
cmd/airstrings-mcp/        # MCP server (JSON-RPC 2.0 over stdio)
  main.go                  # Server loop, stdin/stdout protocol
  protocol.go              # JSON-RPC + MCP types
  tools.go                 # Tool definitions and handlers
internal/
  bundlepull/               # bundles pull: download, verify, mirror, manifest
    bundlepull.go           # Pull flow, Ed25519 + canonical JSON verification
  client/                   # HTTP API client (zero external deps)
    client.go               # Base client, request/response, error handling
    projects.go             # Project, environment, locale, bundle operations
    strings.go              # String CRUD, sections, ListAllStrings
    imports.go              # CSV import, status polling
  output/                   # Terminal output (table, JSON, errors)
    output.go               # Table via tabwriter, JSON mode, Success/Errorf
  workspace/                # Workspace management (.airstrings/ folder)
    csv.go                  # Pure CSV read/write/edit operations
    workspace.go            # Init, Find, LoadConfig, credentials, ResolveClient
    sync.go                 # Push/pull + single-key push helpers (bridges local CSVs and API client)
```

All command handlers live in `main.go`. No command framework — plain `switch` on `os.Args`. This is intentional; do not introduce cobra, urfave/cli, or similar.

## Conventions

### Go Style

- **Go 1.26.1**, module path `github.com/symbionix-sl/airstrings-cli`
- **Zero external dependencies.** stdlib only. Do not add third-party packages
- **No interfaces unless you need two implementations.** Concrete types everywhere
- **Errors exit immediately** via `output.Errorf()` which prints to stderr and calls `os.Exit(1)`
- **No global state** except `output.JSONMode` (set once at startup from `--json` flag)
- **No context.Context yet.** Add when needed (timeouts, cancellation), not before

### Command Pattern

Every command follows the same structure:

1. Parse args (manual, no flag package for subcommands)
2. Call `mustClient()` to get an authenticated API client
3. Call one client method
4. Output: `--json` → `output.JSON(v)`, otherwise formatted text or `output.Table()`
5. Errors: `output.Errorf("verb noun: %s", err)`

Local-first commands (`strings set/rm`) skip step 2 unless `--push` is given — `mustClient()` is called per-subcommand, never upfront, so offline mutations need only a workspace, not credentials.

When adding a new command:
1. Add the case to the `switch` in `main()`
2. Write a `handleX(args []string)` function in `main.go`
3. Add the API method to the appropriate file in `internal/client/`
4. Add usage line to `printUsage()`

### API Client

- Auth via `X-API-Key` header (not Bearer)
- All requests go through `client.do(method, path, query, body, result)`
- Paths are built with `envPath()` (environment-scoped) or `projectPath()` (project-scoped)
- API errors are typed as `*APIError` with status code and structured body
- Base URL defaults to `https://api.airstrings.com`, overridable per credential

### Config

- Stored per-project in `.airstrings/config.json` (no global config)
- Workspace is found by walking up from cwd (like `.git`)
- `init <api-key>` creates the workspace and stores credentials in one step
- Each workspace is self-contained: credentials, active env, project info
- `env use` switches active environment within the workspace
- Config dir created with `0700` permissions, files with `0600`

### Output

- `--json` flag works with every command (parsed globally before routing)
- Tables use `text/tabwriter` for aligned columns
- Success messages prefixed with `✓`
- Error messages go to stderr, then `os.Exit(1)` — no error returns from handlers

### Workspace

Local workspace for AI-friendly string management. Initialized via `airstrings init`.

```
.airstrings/                  # Created by `airstrings init` in project root
  config.json                 # Workspace config (credentials, project, active env)
  strings.csv                 # Unsectioned strings (flat mode)
  home/home.csv               # Section "home" strings
  login/login.csv             # Section "login" strings
```

- `init <api-key>` creates workspace with credentials and section dirs
- `strings set/rm` manipulate CSVs locally without API calls; `--push` also syncs that single key to the API immediately (`workspace.PushKey`/`PushKeyRemoval`: upsert via `UpsertString` — creating the section remotely if needed — full-key removal via `DeleteString`, locale-only removal via nil-value upsert)
- `strings create`/`strings delete` are aliases of `strings set`/`strings rm`
- `strings ls --local` lists local workspace strings offline (no client constructed); the non-deprecated replacement for `local ls`. Shares `listLocalStrings` with the deprecated `local ls` handler
- `local set/rm/ls` are deprecated aliases: `set`/`rm` forward to the `strings` handlers, `ls` keeps its own listing implementation; all print a one-line warning to stderr
- `push` uploads all local strings to API in bulk via the import endpoint (creates sections remotely if needed)
- `pull` downloads all remote strings into organized CSVs (overwrites local state)
- Workspace is found by walking up from cwd (like `.git`). `workspace.Find()` handles this
- CSV format: `key,locale,value,format` — one row per key+locale pair

### MCP Server

Separate binary (`cmd/airstrings-mcp/`) exposing workspace operations as MCP tools via JSON-RPC 2.0 over stdio. Imports the same `internal/` packages as the CLI.

Build: `go build -o airstrings-mcp ./cmd/airstrings-mcp`

Configure in Claude Desktop or any MCP client:
```json
{
  "mcpServers": {
    "airstrings": {
      "command": "/path/to/airstrings-mcp"
    }
  }
}
```

Tools: `airstrings_init`, `airstrings_strings_set`, `airstrings_strings_rm`, `airstrings_strings_ls`, `airstrings_push`, `airstrings_pull`, `airstrings_publish`. `airstrings_strings_set`/`airstrings_strings_rm` accept an optional boolean `push` mirroring the CLI `--push` flag (syncs that single key to the API after the local write). The old `airstrings_local_set/rm/ls` names remain registered as deprecated aliases of the same handlers.

## Non-Negotiables

1. **No secrets in source.** API keys only in `.airstrings/config.json` (0600). Never log or print full keys — show first 8 and last 4 chars only
2. **No external dependencies.** stdlib is sufficient for this CLI. If you think you need a dep, you're wrong
3. **No command framework.** The switch-based routing is deliberate. It keeps the binary small and the code greppable
4. **Exit on error.** Handlers do not return errors. `output.Errorf()` is the only error path
5. **Validate at boundaries.** Check args before calling the API client. The API provides structured errors for everything else
6. **Config permissions matter.** Dir: 0700, files: 0600. The config contains API keys

## Testing

Run tests:
```bash
go test ./...
```

Tests should:
- Use the stdlib `testing` package only — no test dependencies
- Test client methods with httptest servers, not mocks
- Test config load/save with temp directories
- Test output formatting with captured stdout

## Building

```bash
go build -o airstrings ./cmd/airstrings
go build -o airstrings-mcp ./cmd/airstrings-mcp
```

No Makefile, no build tags, no CGO. Static binaries.

## API Spec

The OpenAPI 3.1 spec is at `../../api/openapi.yaml` (relative to this repo root). Consult it when adding or modifying API client methods.

## String Formats

Two formats: `text` (plain text) and `icu` (ICU MessageFormat). No other values valid. Default is `text` if omitted on create.
