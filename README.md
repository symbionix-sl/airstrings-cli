# airstrings

CLI and MCP server for [AirStrings](https://airstrings.com): over-the-air string updates for iOS, Android, web and React Native apps, and Go backends.

Manage strings and locales from the terminal or let an AI agent do it over MCP. `airstrings publish` has AirStrings sign the bundle (Ed25519) and push it to the CDN; the SDKs verify the signature before showing anything.

Listed in the [MCP Registry](https://registry.modelcontextprotocol.io) as `io.github.symbionix-sl/airstrings-cli`.

## Install

```bash
npm install -g @airstrings/cli                    # any platform, Node 18+
brew install symbionix-sl/airstrings/airstrings   # macOS / Linux
go install github.com/symbionix-sl/airstrings-cli/cmd/airstrings@latest   # Go 1.26.1+
```

Installs `airstrings` and `airstrings-mcp`.

## Quick start

```bash
cd my-project
airstrings init                                   # log in, create a project, bind this folder
airstrings strings set welcome.title en="Hello" es="Hola" --format text
airstrings push                                   # send local strings to staging
airstrings publish                                # sign + publish staging bundles
airstrings sdk-config                             # IDs, public key and SDK snippets for production
```

With no key, `init` opens the browser to log in (an owner of your organization approves), stores an org key in `~/.config/airstrings/credentials.json`, creates a project named after the folder and writes `.airstrings/config.json`. `--name <name>` names the project, `--project <id>` binds an existing one, `airstrings init <project-key>` binds to the key's project. Without a terminal it waits up to 90 s for approval; on timeout, `--no-browser` or CI it exits 9 with the URL: approve, then re-run the same command.

Staging is active after `init`. Production is **protected**: it rejects direct writes and publishing, and changes reach it only by promotion, applied by a human in the dashboard or by `airstrings promote --to production` with a full-power org key.

## Keys

| Key | Prefix | Covers | Get it |
|---|---|---|---|
| Org key | `as_org_` | every project of the org | `airstrings login` |
| Project key | `as_proj_` | every environment of one project | dashboard → API keys |
| Legacy environment key | 64 hex | one environment | created before 0.18 |

Precedence: `AIRSTRINGS_ORG_API_KEY` > `AIRSTRINGS_API_KEY` > workspace key > stored login. In CI use a project key in `AIRSTRINGS_API_KEY`; across several projects use a gated (non-full-power) org key from the dashboard in `AIRSTRINGS_ORG_API_KEY`. No `init` needed when one of these is set.

## Commands

```bash
# Account and context
airstrings login | logout             # store / revoke the org key
airstrings org                        # stored logins; `org use <name|id>` switches
airstrings status                     # project, env, key scope, protection + next step
airstrings project                    # current project; `project ls` lists the org's
airstrings --project <id> <command>   # run against another project (org key)

# Environments
airstrings env                        # environments: protected/open, key stored or not
airstrings env use staging            # switch (shorthand: airstrings -e -u staging)
airstrings env rm <name>              # remove environment credentials
airstrings locales                    # locales with string counts
airstrings sdk-config [--env <name>]  # IDs + public keys + SDK snippets (default production)

# API keys
airstrings apikey ls                  # the project's keys (✓ = stored in this workspace)
airstrings apikey rotate              # rotate the workspace key
airstrings apikey revoke              # revoke the workspace's stored keys (e.g. after a leak)

# Strings (set/rm edit local CSVs; --push also writes to the API)
airstrings strings ls [--local] [--section <s>] [--locale <l>] [--limit <n>] [--key-prefix <p>]
airstrings strings get <key>
airstrings strings set <key> en="Hello" es="Hola" --format text|icu [--section <s>] [--push]
airstrings strings rm <key> [--locale <l>] [--section <s>] [--push]
airstrings sections list | create <name> [--description <d>] | delete <id>
airstrings push | pull [--section <s>]   # sync local CSVs with the API (draft strings)
airstrings import csv <file>             # bulk import; `import status <id>` to follow

# Publishing
airstrings publish [locale...]        # sign + publish bundles (all locales by default)
airstrings bundles                    # published bundles
airstrings promote preview            # diff staging → production + dashboard link to apply
airstrings promote --to production    # apply (full-power org key only)

# A/B experiments on a string (text only)
airstrings variants create <key>
airstrings variants set <key> <variant> en="Hi there"
airstrings variants allocation <key> control=50 <variant>=50
airstrings variants start|stop|status|rm <key>
airstrings variants promote <key> <variant>   # winner becomes the base string

# Org shared-key bucket
airstrings shared init <api-key>      # save the bucket's scoped write key
airstrings shared ls | add | rm | import <file>
```

Publishing to a protected environment exits 7 with the next step: publish to staging, then a human promotes at the printed link. `airstrings <command> --help` shows every flag.

## Workspace

`init` creates `.airstrings/` at the project root; commands find it from any subdirectory.

```
.airstrings/
  config.json                  # project, active env, workspace keys
  strings.csv                  # unsectioned strings
  onboarding/onboarding.csv    # one CSV per section
```

The CSVs are plain files you can commit.

## Offline fallback bundles

```bash
airstrings bundles pull [dir] [--locale <bcp47>]
airstrings doctor                     # check the bundled-fallback integration
```

Downloads the published, signed bundles of the active environment into `airstrings/bundles/` (or `dir`, saved to the workspace config) with a `manifest.json`, verifying every signature first. Commit the folder: SDKs seed from it on startup, re-verifying each bundle, so the app has strings on a cold offline start or in SSG/SSR builds. Re-run it in CI or before each release.

`pull` fetches **draft** strings as editable CSVs; `bundles pull` fetches **published, signed** bundles for shipping.

## MCP server

```bash
airstrings mcp install                  # Claude Code
airstrings mcp install --claude-desktop # Claude Desktop
airstrings mcp status | uninstall
```

Other clients: run `airstrings-mcp` over stdio. Details: [cmd/airstrings-mcp/README.md](cmd/airstrings-mcp/README.md).

| Tool | Does |
|---|---|
| `airstrings_login`, `airstrings_init` | log in, bind a folder to a project |
| `airstrings_status`, `airstrings_env_list` | active context, environments |
| `airstrings_strings_set`, `airstrings_strings_rm`, `airstrings_strings_ls` | edit and list local strings (optional `push`) |
| `airstrings_push`, `airstrings_pull` | sync local strings with the API |
| `airstrings_publish` | sign + publish bundles |
| `airstrings_promote_preview` | diff between environments + dashboard link |
| `airstrings_sdk_config` | IDs, public keys and SDK snippets |
| `airstrings_variant_set`, `_start`, `_stop`, `_status`, `_promote` | A/B experiments |

## Scripting

`--json` works on every command. Errors then go to stderr as `{"error":{"message":"…","next_step":"…","exit_code":7}}`.

| Exit | Meaning |
|---|---|
| 0 | ok |
| 1 | generic error |
| 2 | usage / bad input |
| 3 | auth: bad or expired key, read key used for a write, or no key for this environment |
| 4 | not found |
| 5 | network (retry) |
| 6 | rate limited (retry) |
| 7 | environment protected: publish to staging, a human promotes |
| 8 | plan limit reached: upgrade |
| 9 | login pending: open the printed URL, approve, re-run |

## License

MIT, see [LICENSE](LICENSE).
