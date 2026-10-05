# airstrings

CLI and MCP server for [AirStrings](https://airstrings.com): over-the-air string updates for iOS, Android, web and React Native apps.

Manage strings and locales from the terminal or let an AI agent do it over MCP. `airstrings publish` has AirStrings sign the bundle (Ed25519) and push it to the CDN; the SDKs verify the signature before showing anything.

```bash
npm install -g @airstrings/cli        # or: brew install symbionix-sl/airstrings/airstrings
airstrings mcp install                # register the MCP server with Claude Code
```

Listed in the [MCP Registry](https://registry.modelcontextprotocol.io) as `io.github.symbionix-sl/airstrings-cli`.

## Install

### npm (any platform)

```bash
npm install -g @airstrings/cli
airstrings <command>
```

Requires Node 18+. Installs `airstrings` and `airstrings-mcp`.

### Homebrew (macOS / Linux)

```bash
brew install symbionix-sl/airstrings/airstrings
```

### From source

```bash
go install github.com/symbionix-sl/airstrings-cli/cmd/airstrings@latest
```

### Build locally

```bash
git clone git@github.com:symbionix-sl/airstrings-cli.git
cd airstrings-cli
go build -o airstrings ./cmd/airstrings
```

## Setup

Initialize a workspace in your project directory:

```bash
cd my-project
airstrings init <staging-write-key>
```

Use the **staging write key** shown during onboarding in the dashboard. API keys are 64-character hex strings (e.g. `3f9c…e21a`) scoped to one project and one environment. This validates the key, auto-detects your project and environments, and stores everything in `.airstrings/config.json`. Each project has its own workspace — no shared global config.

Production is **protected** by default: it rejects direct writes and publishing, and changes reach it only when a human promotes staging in the dashboard. An agent therefore never needs a production key — it publishes to staging, and `airstrings sdk-config --env production` gives it everything the SDK needs for production.

### Environments

```bash
airstrings env                      # every environment: protected/open, key in workspace or not
airstrings env use staging          # switch to staging
airstrings env add <api-key>        # add credentials for another environment
airstrings env rm staging           # remove environment credentials
airstrings -e -u staging            # shorthand for env use
airstrings status                   # project, env, key scope, protection + next step
```

### SDK configuration

```bash
airstrings sdk-config                     # production (default) IDs, public key, SDK snippets
airstrings sdk-config --env staging       # another environment
airstrings sdk-config --env production --json
```

Prints the organization, project and environment IDs, the environment's Ed25519 public key(s), and a ready-to-paste initializer for the Web, React Native, iOS and Android SDKs. Public keys are not secret, so any key of the project works — a staging key prints production's config.

## Usage

```
airstrings <command> [options]
```

### Project & Environments

```bash
airstrings project                  # Show project info
airstrings env                      # List environments with protection and key state
airstrings env use staging          # Switch active environment
airstrings env create staging       # Create a new environment
airstrings locales                  # List locales with string counts
```

### Strings

`strings set` and `strings rm` are local-first: they edit the workspace CSVs and never call the API unless `--push` is given.

```bash
airstrings strings list                          # List all strings (remote)
airstrings strings list --locale en --limit 50   # Filter by locale
airstrings strings ls --local                    # List local workspace strings (offline, no credentials)
airstrings strings ls --local --section onboarding   # List one section, offline
airstrings strings get welcome.title             # Get a single string (remote)
airstrings strings set welcome.title en="Hello" es="Hola" --format text   # Write to local CSVs
airstrings strings set app.name en="My App" --format text --push     # Also upsert to the API
airstrings strings rm old.unused.key             # Remove from local CSVs
airstrings strings rm old.unused.key --push      # Also delete from the API
airstrings strings rm welcome.title --locale es --push   # Remove one locale, locally and remotely
```

### Sections

```bash
airstrings sections list
airstrings sections create onboarding --description "Onboarding flow strings"
airstrings sections delete sec_xxxxx
```

### Bundles & Publishing

```bash
airstrings bundles                  # List published bundles
airstrings publish                  # Publish all locales
airstrings publish en es            # Publish specific locales
airstrings promote preview          # Diff staging → production; prints the dashboard link where a human applies it
```

Publishing to a protected environment exits with code 7 and tells you the next step: publish to staging, then ask a human to promote at the printed link.

### Offline-safe builds

Ship published bundles inside your app so SDKs can serve strings with no network — cold offline starts, SSG/SSR builds, CI:

```bash
airstrings bundles pull
```

This downloads the published, signed bundles for the active environment into `airstrings/bundles/` at the workspace root (plus a `manifest.json` provenance record), verifying every Ed25519 signature before writing. Commit the folder:

```bash
git add airstrings/bundles
git commit -m "chore: update bundled fallback strings"
```

SDKs detect the folder automatically and seed from it on startup, re-verifying every bundle before use. Run the pull in CI or as a pre-release step to keep the committed snapshot fresh.

```bash
airstrings bundles pull dist/seed         # custom output dir (persisted to workspace config)
airstrings bundles pull --locale en-US    # restrict to one locale
```

Not the same as `airstrings pull`: `pull` fetches **draft** workspace strings as editable CSVs for the editing workflow, while `bundles pull` fetches **published, signed** bundles — immutable delivery artifacts for shipping. The two never share an output location.

### Import

```bash
airstrings import csv strings.csv   # Import strings from CSV
airstrings import status imp_xxxxx  # Check import progress
```

### Workspace

The workspace workflow lets you manage strings locally and sync with the API. This is the recommended workflow for AI-assisted string management.

```bash
# Initialize workspace with the staging write key from onboarding
airstrings init <staging-write-key>

# Add strings locally (no API calls)
airstrings strings set onboarding.welcome en="Welcome!" it="Benvenuto!" --format text --section onboarding
airstrings strings set onboarding.welcome de="Willkommen!" es="¡Bienvenido!" fr="Bienvenue!" --format text --section onboarding
airstrings strings set app.tagline en="The best app" it="La migliore app" --format text

# List local strings (offline, no credentials)
airstrings strings ls --local
airstrings strings ls --local --section onboarding

# Edit and remove
airstrings strings set onboarding.welcome en="Welcome to the app!" --format text --section onboarding
airstrings strings rm old.key --section onboarding

# Sync a single key immediately while editing
airstrings strings set app.tagline en="The best app" --format text --push

# Push everything to AirStrings
airstrings push
airstrings push --section onboarding   # push single section

# Pull remote strings to local
airstrings pull
```

The `airstrings init` command creates a `.airstrings/` folder in your project root:

```
.airstrings/
  config.json              # workspace config (credentials, project, active env)
  strings.csv              # unsectioned strings
  onboarding/onboarding.csv  # section strings
  settings/settings.csv
```

Each section gets its own subdirectory with a CSV file. Unsectioned strings live in the root `strings.csv`. All files are plain CSV and can be committed to version control.

### MCP Server

AirStrings provides an MCP server so AI assistants like Claude can manage strings directly through structured tool calls.

```bash
airstrings mcp install                  # for Claude Code
airstrings mcp install --claude-desktop # for Claude Desktop
airstrings mcp status                   # check installation
```

That's it. Restart Claude and the tools are available.

#### Example: AI-assisted localization

```
You: "Translate my app's onboarding screen into Italian, German, Spanish, and French"

Claude uses airstrings_strings_set:
  key: "onboarding.welcome"
  values: {"it": "Benvenuto!", "de": "Willkommen!", "es": "¡Bienvenido!", "fr": "Bienvenue!"}
  section: "onboarding"

Claude uses airstrings_strings_set:
  key: "onboarding.subtitle"
  values: {"it": "Inizia il tuo viaggio", "de": "Beginne deine Reise", "es": "Comienza tu viaje", "fr": "Commencez votre voyage"}
  section: "onboarding"

Claude uses airstrings_push:
  section: "onboarding"

-> Pushed 2 strings (0 errors)
   Sections: onboarding
```

Instead of generating CSV files, the AI calls structured MCP tools -- one call per string. This saves tokens and eliminates CSV formatting errors.

#### Available MCP tools

| Tool | Description |
|------|-------------|
| `airstrings_init` | Initialize workspace |
| `airstrings_strings_set` | Add/update string in local CSV (optional `push` to sync the key to the API immediately) |
| `airstrings_strings_rm` | Remove string from local CSV (optional `push` to mirror the removal to the API immediately) |
| `airstrings_strings_ls` | List local strings |
| `airstrings_push` | Push local strings to API |
| `airstrings_pull` | Pull remote strings to local |
| `airstrings_publish` | Publish bundles to CDN |

### JSON Output

Add `--json` to any command for machine-readable output:

```bash
airstrings strings list --json
airstrings project --json | jq '.name'
```

With `--json`, errors are written to stderr as `{"error":{"message":"…","next_step":"…","exit_code":7}}`.

### Exit codes

| code | meaning |
|------|---------|
| 0 | ok |
| 1 | generic error |
| 2 | usage / bad input |
| 3 | auth: bad or expired key, read key used for a write, or no key in this workspace for an open environment |
| 4 | not found |
| 5 | network (retry) |
| 6 | rate limited (retry) |
| 7 | environment protected — publish to staging, a human promotes |
| 8 | plan limit reached — upgrade |

## Configuration

Config is stored per-project in `.airstrings/config.json` (like `.git/config`). No global config — each workspace is self-contained with its own credentials and active environment.

```bash
airstrings init <api-key> [--url <base-url>]      # create workspace and authenticate
airstrings env add <api-key> [--url <base-url>]   # add environment credentials
airstrings env rm <name>                          # remove environment credentials
airstrings env use <name>                         # switch environment
airstrings status                                 # show active context
```

The workspace is found by walking up the directory tree, so commands work from any subdirectory.

## Requirements

- Go 1.26.1+
- An AirStrings account with an API key

## License

MIT — see [LICENSE](LICENSE).
