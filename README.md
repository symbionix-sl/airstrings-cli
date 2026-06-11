# airstrings

Command-line interface for [AirStrings](https://airstrings.com) — manage strings, locales, bundles, and publishing from the terminal.

## Install

### Homebrew

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
airstrings init ask_live_xxxxxxxxxxxx
```

This validates the key, auto-detects your project and environments, and stores everything in `.airstrings/config.json`. Each project has its own workspace — no shared global config.

### Environments

```bash
airstrings env                      # list environments (✓ = active)
airstrings env use staging          # switch to staging
airstrings env add <api-key>        # add credentials for another environment
airstrings env rm staging           # remove environment credentials
airstrings -e -u staging            # shorthand for env use
airstrings status                   # show current project, env, and key
```

## Usage

```
airstrings <command> [options]
```

### Project & Environments

```bash
airstrings project                  # Show project info
airstrings env                      # List environments
airstrings env use staging          # Switch active environment
airstrings env create staging       # Create a new environment
airstrings locales                  # List locales with string counts
```

### Strings

`strings set` and `strings rm` are local-first: they edit the workspace CSVs and never call the API unless `--push` is given. `strings create` and `strings delete` are aliases of `set` and `rm`.

```bash
airstrings strings list                          # List all strings (remote)
airstrings strings list --locale en --limit 50   # Filter by locale
airstrings strings get welcome.title             # Get a single string (remote)
airstrings strings set welcome.title en="Hello" es="Hola"            # Write to local CSVs
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
```

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
# Initialize workspace
airstrings init ask_live_xxxxxxxxxxxx

# Add strings locally (no API calls)
airstrings strings set onboarding.welcome en="Welcome!" it="Benvenuto!" --section onboarding
airstrings strings set onboarding.welcome de="Willkommen!" es="¡Bienvenido!" fr="Bienvenue!" --section onboarding
airstrings strings set app.tagline en="The best app" it="La migliore app" --format text

# List local strings
airstrings local ls
airstrings local ls --section onboarding

# Edit and remove
airstrings strings set onboarding.welcome en="Welcome to the app!" --section onboarding
airstrings strings rm old.key --section onboarding

# Sync a single key immediately while editing
airstrings strings set app.tagline en="The best app" --push

# Push everything to AirStrings
airstrings push
airstrings push --section onboarding   # push single section

# Pull remote strings to local
airstrings pull
```

The old `local set`, `local rm`, and `local ls` commands are deprecated aliases — they still work but print a warning to stderr.

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

The former `airstrings_local_set/rm/ls` names still work as deprecated aliases and will be removed in a future minor release.

### JSON Output

Add `--json` to any command for machine-readable output:

```bash
airstrings strings list --json
airstrings project --json | jq '.name'
```

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
