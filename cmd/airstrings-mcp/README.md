# airstrings-mcp

MCP server for [AirStrings](https://airstrings.com): lets an AI agent manage strings, publish signed bundles and run A/B experiments on strings for iOS, Android, web, React Native and Go apps. JSON-RPC 2.0 over stdio, listed in the [MCP Registry](https://registry.modelcontextprotocol.io) as `io.github.symbionix-sl/airstrings-cli`.

It ships with the [`airstrings` CLI](../../README.md) and works on the same `.airstrings/` workspace, so the agent and the terminal see the same strings.

## Install

```bash
npm install -g @airstrings/cli                    # or: brew install symbionix-sl/airstrings/airstrings
airstrings mcp install                            # Claude Code (user scope)
airstrings mcp install --claude-desktop           # Claude Desktop
```

Any other client: run `airstrings-mcp` over stdio.

```json
{ "mcpServers": { "airstrings": { "command": "airstrings-mcp" } } }
```

The server works on the workspace found from its working directory, walking up like `.git`.

## Auth

`airstrings_init` binds a folder to a project. With no key it uses `AIRSTRINGS_ORG_API_KEY`, `AIRSTRINGS_API_KEY` or the stored login; with none it returns `status: "pending"` and a `verification_uri_complete`. An organization owner approves it in the browser, then the agent calls the tool again.

Production is **protected**: writes and publishing go to staging, and a human promotes to production in the dashboard. The server can preview a promotion but never applies one.

## Tools

| Tool | Does |
|---|---|
| `airstrings_login` | Log in and store an org key (pending until an owner approves) |
| `airstrings_init` | Bind a folder to a project: `api_key`, `name`, `project`, `org`, `dir` |
| `airstrings_status` | Project, active environment, key scope, protection and the next step |
| `airstrings_env_list` | Environments with protection and stored-key state |
| `airstrings_strings_set` | Add or update a key in the local CSVs: `key`, `values`, `format` (`text`/`icu`), `section`, `push` |
| `airstrings_strings_rm` | Remove a key, or one `locale`, from the local CSVs; `push` mirrors it to the API |
| `airstrings_strings_ls` | List local strings, optionally one `section` |
| `airstrings_push` | Push local strings to the API (all or one `section`) |
| `airstrings_pull` | Pull draft strings into the local CSVs (overwrites them) |
| `airstrings_publish` | Sign and publish bundles to the CDN (all or some `locales`) |
| `airstrings_promote_preview` | Read-only diff between environments + the dashboard link to apply it |
| `airstrings_sdk_config` | IDs, Ed25519 public keys and SDK snippets for an environment (all public) |
| `airstrings_variant_set` | Create or replace a string's A/B experiment: `allocation`, `variants` |
| `airstrings_variant_status` | Current experiment for a key |
| `airstrings_variant_start` / `_stop` | Start or stop serving the variants |
| `airstrings_variant_promote` | Make a variant the base string |

API keys (list, rotate, revoke) are deliberately CLI-only.

## Typical flow

1. `airstrings_init`, then `airstrings_strings_set` once per key with every locale in `values`.
2. `airstrings_push`, then `airstrings_publish` to staging.
3. `airstrings_promote_preview` and hand the user the link to promote to production.
4. `airstrings_sdk_config` to wire the app's SDK.

## Build

```bash
go build -o airstrings-mcp ./cmd/airstrings-mcp
```

## License

MIT, see [LICENSE](../../LICENSE).
