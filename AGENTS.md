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
  | 7    | environment protected (production) | no — publish to staging; a human promotes |
  | 8    | plan limit reached | no — a human upgrades |
  | 9    | login pending | yes — after the user approves the printed URL |

  Exit 3 also covers a read key used for a write.

- With `--json`, errors go to stderr as one JSON object:
  `{"error":{"message":"…","next_step":"…","exit_code":7}}`. `next_step` is the
  single action that unblocks you (a command or a dashboard link). MCP tool
  errors carry the same message and `Next step:` text.

- Any command + `--help`/`-h` prints its help text and exits 0 — nothing is
  executed, no side effects. Safe to probe usage this way.
- Unknown flags exit 2 (usage) — the command does not run. Fix the invocation.
- Set `NO_COLOR=1` (or just pipe — non-TTY auto-disables color) for clean output.

## Auth — three ways

**Login (one command after signup):** `airstrings init --json` in the repo. With
no key it logs in through the browser, stores an **org key** (`as_org_…`) in
`~/.config/airstrings/credentials.json` (0600), creates a project named after
the folder and writes `.airstrings/config.json`. It never reuses a project by
name: a name collision exits 2 and names the existing project (use
`--project <id>` or `--name <other>`).

Without a terminal the command opens the browser and waits up to 90 s for an
owner of the organization to approve, then finishes. With `--no-browser` or `CI`
set, or if nobody approves in time, it **exits 9** and prints
`{status:"pending", verification_uri_complete, user_code, expires_in, next_step}`.
Give the URL to the user, then re-run the same command; each re-run waits up to
90 s. Don't pass `--no-browser` unless the machine has no browser.

**Project key:** `airstrings init <as_proj_…>` binds the folder to the key's
project (create one at `<webapp>/projects/{p}/api-keys`). One project key covers
every environment of the project. In an existing workspace it switches keys in
place and keeps local strings.

**Environment variables (headless / CI / ephemeral agent):** skip `init`.

```
AIRSTRINGS_ORG_API_KEY  org key; wins over every other key. CI across several
                        projects: a gated (non-full-power) org key from the dashboard
AIRSTRINGS_API_KEY      project key (preferred in CI: create one in the dashboard)
AIRSTRINGS_PROJECT_ID   optional — skip project lookup (one fewer API call)
AIRSTRINGS_ENV_ID       optional — skip environment lookup
AIRSTRINGS_BASE_URL     optional — default https://api.airstrings.com
AIRSTRINGS_NO_BROWSER   optional — print the login URL, don't open a browser
```

Precedence: `AIRSTRINGS_ORG_API_KEY` > `AIRSTRINGS_API_KEY` > workspace key >
stored login. An org key works on any project of the org: `--project <id|name>`
picks one, `airstrings project ls` lists them. Legacy 64-hex environment keys
keep working for their one environment.

Confirm what you're pointed at any time: `airstrings status --json` →
`{source, project_id, env_id, base_url, protection, key_scope, key_type, key_source, full_power, hint, environments[...], mode, workspace_dir}`.
`source` is `"workspace"` or `"env"`. `protection` is `"protected"` (production
rejects direct writes — changes reach it only by promotion), `"open"` (production
accepts direct publishing), or `"unknown"`. `key_scope` is `"read"`, `"write"` or
`"unknown"`. `key_type` is `"org"`, `"project"`, `"environment"` (legacy) or
`"none"`; `key_source` says where it came from. `hint` is `{message, next_step}` for the current state. All are
best-effort and degrade to `"unknown"`/null rather than failing.

## Environments: staging active, protected production

`init` makes **staging** the active environment. Production is protected by
default: it rejects direct writes and publishing.

- Ship: write and `publish` to staging, then run `promote preview` and hand the
  printed dashboard link to a human, who applies the promotion. A full-power
  org key can apply it: `airstrings promote --to production`.
- SDK setup for production: `airstrings sdk-config --env production` — works
  with any key of the project. Everything it returns (IDs, Ed25519 public keys,
  snippets) is public, not a secret: show it, commit it and embed it in app
  code. It never contains an API key.
- `airstrings env` lists every environment with `protection` and
  `key_in_workspace`.

## Core commands

All accept `--json`.

```
airstrings status                      # who am I / what env (discovery)
airstrings project                     # project metadata
airstrings locales                     # locales + string counts
airstrings env                         # every env: protected/open, key in workspace or not
airstrings sdk-config [--env <name>]   # org/project/env IDs + public keys + SDK snippets

airstrings strings ls                  # remote strings; pages automatically
airstrings strings ls --limit 50       # one bounded page
airstrings strings ls --cursor <c>     # next page (from pagination.next_cursor)
airstrings strings ls --key-prefix home.   # filter by key prefix
airstrings strings ls --local          # read workspace CSVs offline (no creds)
airstrings strings get <key>

airstrings strings set <key> en="Hello" it="Ciao" --format text|icu [--section s] [--push]
airstrings strings rm  <key> [--locale en] [--section s] [--push]
#   set/rm write local CSVs; --push also syncs that one key to the API now.
#   If the push fails, the CSV is restored and the error says so.

airstrings push [--section s]          # upload all local CSVs to the API
airstrings pull [--section s]          # download remote drafts to CSVs (OVERWRITES local)

airstrings sections list|create <name>|delete <id>
airstrings publish [locale...]         # sign + publish bundles to the CDN
airstrings bundles                     # list published bundles
airstrings bundles pull [dir]          # download signed bundles for offline fallback
airstrings promote preview [--from <env>] [--to <env>]  # preview env→env diff (read-only) + apply_url for a human
airstrings variants create <key>                        # create an A/B experiment on a string
airstrings variants set <key> <name> en="…" [--format text|icu]  # add/update a variant value
airstrings variants allocation <key> <name>=<pct> ...   # split traffic across the experiment's variants
airstrings variants start|stop <key>                    # start / stop serving the experiment
airstrings variants status <key>                        # experiment state + per-variant allocation
airstrings variants promote <key> <name>                # promote the winning variant into the base string
airstrings variants rm <key>                            # delete the experiment
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
airstrings init --json                                     # browser opens, user approves; exit 9 → re-run
airstrings status --json                                   # confirm target
airstrings strings set welcome.title en="Welcome" --format text --push
airstrings strings ls --key-prefix welcome. --json
airstrings publish en --json                               # ship it
```

## MCP

An MCP server (`airstrings-mcp`) exposes a subset of these as tools for clients
without shell access (e.g. Claude Desktop): `airstrings mcp install`. It honors
the same environment variables and stored login. `airstrings_login` and a
keyless `airstrings_init` return `status: "pending"` with the approval URL; call
again after the user approves. If you have
a shell, prefer the CLI directly — it is more composable and supports paging,
`--json`, and the exit codes above.
