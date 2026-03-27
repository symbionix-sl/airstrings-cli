# airstrings

Command-line interface for [AirStrings](https://airstrings.com) — manage strings, locales, bundles, and publishing from the terminal.

## Install

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

Create a profile with your API key:

```bash
airstrings profile add myproject --key ask_live_xxxxxxxxxxxx
```

This validates the key against the API and auto-detects your project ID and default environment.

### Multiple profiles

```bash
airstrings profile add staging --key ask_test_xxxxxxxxxxxx --url https://staging.api.airstrings.com
airstrings profile use staging
airstrings profile list
```

## Usage

```
airstrings <command> [options]
```

### Project & Environments

```bash
airstrings project                  # Show project info
airstrings envs                     # List environments
airstrings envs create staging      # Create a new environment
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

### JSON Output

Add `--json` to any command for machine-readable output:

```bash
airstrings strings list --json
airstrings project --json | jq '.name'
```

## Configuration

Config is stored at `~/.airstrings/config.json`. Each profile contains:

| Field | Description |
|---|---|
| `api_key` | Scoped API key for the project |
| `base_url` | API base URL (defaults to production) |
| `project_id` | Auto-detected from API key |
| `env_id` | Environment ID (auto-detected or manual) |

### Profile commands

```bash
airstrings profile add <name> --key <api-key> [--url <base-url>] [--env <env-id>]
airstrings profile list
airstrings profile use <name>
airstrings profile show
airstrings profile set-key <new-key>
airstrings profile remove <name>
```

## Requirements

- Go 1.26.1+
- An AirStrings account with an API key

## License

Proprietary. Copyright Symbionix SL.
