# airstrings

Command-line interface for [AirStrings](https://airstrings.com) — manage strings, locales, bundles, and publishing from the terminal.

## Install

### Homebrew

```bash
brew install symbionix-sl/airstrings/airstrings
```

### From source

```bash
go install github.com/symbionix/airstrings-cli/cmd/airstrings@latest
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

```bash
airstrings strings list                          # List all strings
airstrings strings list --locale en --limit 50   # Filter by locale
airstrings strings get welcome.title             # Get a single string
airstrings strings create app.name en="My App" es="Mi App" --format text
airstrings strings set welcome.title en="Hello" es="Hola"
airstrings strings delete old.unused.key
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
airstrings local set onboarding.welcome en="Welcome!" it="Benvenuto!" --section onboarding
airstrings local set onboarding.welcome de="Willkommen!" es="¡Bienvenido!" fr="Bienvenue!" --section onboarding
airstrings local set app.tagline en="The best app" it="La migliore app" --format text

# List local strings
airstrings local ls
airstrings local ls --section onboarding

# Edit and remove
airstrings local set onboarding.welcome en="Welcome to the app!" --section onboarding
airstrings local rm old.key --section onboarding

# Push to AirStrings
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

Claude uses airstrings_local_set:
  key: "onboarding.welcome"
  values: {"it": "Benvenuto!", "de": "Willkommen!", "es": "¡Bienvenido!", "fr": "Bienvenue!"}
  section: "onboarding"

Claude uses airstrings_local_set:
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
| `airstrings_local_set` | Add/update string in local CSV |
| `airstrings_local_rm` | Remove string from local CSV |
| `airstrings_local_ls` | List local strings |
| `airstrings_push` | Push local strings to API |
| `airstrings_pull` | Pull remote strings to local |
| `airstrings_publish` | Publish bundles to CDN |

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

Proprietary. Copyright Symbionix SL.
