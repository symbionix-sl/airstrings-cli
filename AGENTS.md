# AirStrings CLI — agent guide

`airstrings` manages localized strings for the AirStrings platform. This file is
the contract for driving it from an AI agent or script. Read it once; every
command below is stable and machine-friendly.

## Golden rules

- Add `--json` to any command for structured stdout. Human text goes to stdout,
  errors and progress go to stderr.
- Branch on the exit code, not on message text:

  | code | meaning      | retry? |
  |------|--------------|--------|
  | 0    | ok           | —      |
  | 1    | generic error| no     |
  | 2    | usage / bad input | no (fix the command) |
  | 3    | auth (bad/expired key) | no (fix credentials) |
  | 4    | not found    | no     |
  | 5    | network      | yes (backoff) |
  | 6    | rate limited | yes (backoff) |

- Set `NO_COLOR=1` (or just pipe — non-TTY auto-disables color) for clean output.

## Auth — two ways

**Workspace (interactive / committed repo):** `airstrings init <api-key>` writes
`.airstrings/config.json` (mode 0600) at the repo root. Found by walking up from
the cwd, like `.git`. Survives restarts.

**Environment variables (headless / CI / ephemeral agent):** set
`AIRSTRINGS_API_KEY` and skip `init`. It overrides any workspace credential.

```
AIRSTRINGS_API_KEY      scoped key; project + default env resolved from it
AIRSTRINGS_PROJECT_ID   optional — skip project lookup (one fewer API call)
AIRSTRINGS_ENV_ID       optional — skip environment lookup
AIRSTRINGS_BASE_URL     optional — default https://api.airstrings.com
```

A scoped key maps to exactly one project + one environment. With just the key,
the CLI resolves both automatically; supply the IDs to make calls fully stateless
(zero discovery round-trips).

Confirm what you're pointed at any time: `airstrings status --json` →
`{source, project_id, env_id, base_url, environments[...], mode, workspace_dir}`.
`source` is `"workspace"` or `"env"`.

## Core commands

All accept `--json`.

```
airstrings status                      # who am I / what env (discovery)
airstrings project                     # project metadata
airstrings locales                     # locales + string counts
airstrings env                         # list environments (✓ = active)

airstrings strings ls                  # remote strings; pages automatically
airstrings strings ls --limit 50       # one bounded page
airstrings strings ls --cursor <c>     # next page (from pagination.next_cursor)
airstrings strings ls --key-prefix home.   # filter by key prefix
airstrings strings ls --local          # read workspace CSVs offline (no creds)
airstrings strings get <key>

airstrings strings set <key> en="Hello" it="Ciao" --format text|icu [--section s] [--push]
airstrings strings rm  <key> [--locale en] [--section s] [--push]
#   set/rm write local CSVs; --push also syncs that one key to the API now.

airstrings push [--section s]          # upload all local CSVs to the API
airstrings pull [--section s]          # download remote drafts to CSVs (OVERWRITES local)

airstrings sections list|create <name>|delete <id>
airstrings publish [locale...]         # sign + publish bundles to the CDN
airstrings bundles                     # list published bundles
airstrings bundles pull [dir]          # download signed bundles for offline fallback
airstrings import csv <file> | status <id>
airstrings apikey rotate [--env name]
```

### Paging large lists

`strings ls --json` returns `{ "data": [...], "pagination": { "has_more": bool,
"next_cursor": string } }`. Loop: pass `next_cursor` back via `--cursor` until
`has_more` is false. Prefer `--limit`/`--cursor` or `--key-prefix` over an
unbounded `ls` so results stay small.

## Workspace files (git- and agent-friendly)

```
.airstrings/config.json   # credentials + project/active env (0600 — never commit; gitignore it)
strings.csv               # flat mode: key,locale,value,format (one row per key+locale)
<section>/<section>.csv   # sectioned mode
```

CSVs are deterministically sorted (by key, then locale) so diffs are stable.
Edit via `strings set`/`rm` (atomic, validated) rather than hand-writing CSV —
the commands handle RFC-4180 quoting and format checks for you.

`format` is required on `set` and must be `text` or `icu`. A `text` value
containing `{…}` triggers a non-fatal warning (it is served verbatim, braces are
not interpolated) — use `icu` for interpolation.

## Typical agent flow

```
export AIRSTRINGS_API_KEY=ask_live_xxx
airstrings status --json                                   # confirm target
airstrings strings set welcome.title en="Welcome" --format text --push
airstrings strings ls --key-prefix welcome. --json
airstrings publish en --json                               # ship it
```

## MCP

An MCP server (`airstrings-mcp`) exposes a subset of these as tools for clients
without shell access (e.g. Claude Desktop): `airstrings mcp install`. If you have
a shell, prefer the CLI directly — it is more composable and supports paging,
`--json`, and the exit codes above.
