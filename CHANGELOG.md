# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.19.0] - 2026-10-08

### Added

- `init --org <org_id>` binds the folder to that organization: it uses the stored login for that org, or logs in. An approval from another org exits 3 ("Approved for <name> (<id>), but this setup is for <org>…"). A folder linked to another org gets a new project in `<org_id>` and prints "This folder was linked to <old>; re-linked to <new>. Previous local files moved to .airstrings.<old_org_id>"; the old org's project is not touched.
- `login --org <org_id>` rejects an approval from another org (exit 3).
- `org` lists the stored logins for the API URL (✓ = active); `org use <name|id>` sets the active org.
- `logout --org <org_id>` revokes and forgets one org's login.
- `status` shows the workspace's organization.
- MCP `airstrings_init` and `airstrings_login` take an optional `org`.

### Changed

- One org key is stored per organization and API URL. Logging in to another org keeps the existing logins; logging in again to the same org still replaces and revokes its old key. The newly approved org becomes active, unless `--org` asked for another org.
- A workspace always uses its own org's login; outside a workspace the active org is used (else the most recent login).
- `logout` revokes the active org's login only.

### Fixed

- Table headers and their rule line now align with the rows (`project ls`, `env ls`, `org`, …).

## [0.18.4] - 2026-10-06

### Fixed

- Re-running `login` / keyless `init` while a login is pending reopens the approval page (unless `--no-browser` or `AIRSTRINGS_NO_BROWSER`), so a closed tab no longer leaves the login stuck until the code expires.
- Stdin or stdout redirected to `/dev/null` is no longer treated as a terminal, so `login` / `init` run that way by an agent wait 90 s instead of blocking until the code expires.
- A login code that expires while `login` / `init` is waiting is replaced with a fresh one in the same run (new link printed and opened) instead of exiting 3. Without budget left the run exits 9 with the new link.

## [0.18.3] - 2026-10-06

### Fixed

- `init`, `sdk-config` and MCP `airstrings_sdk_config` add the API base URL to every SDK snippet (`apiBaseURL` / `APIBaseURL`) when the CLI targets a non-default API such as staging. Without it the SDK loaded nothing. Production output is unchanged.

## [0.18.2] - 2026-10-06

### Changed

- `login` / keyless `init` without a terminal open the browser and wait up to 90 s for approval, finishing in one run. They exit 9 right away only with `--no-browser` or `CI`; on timeout they exit 9 as before, and each re-run waits up to 90 s (was 30 s).
- Pending `next_step`: "Approve in the browser at <url>, then run the same command again".
- The browser opener honors `$BROWSER`.

## [0.18.1] - 2026-10-06

### Fixed

- Login pending (exit 9, MCP `status: "pending"`): `next_step` names the approval URL instead of the `verification_uri_complete` field name.

## [0.18.0] - 2026-10-06

### Added

- `airstrings login` / `logout`: browser login approved by an org owner; stores an org key (`as_org_…`) in `~/.config/airstrings/credentials.json` (0600). Logging in again revokes the previous key. Without a terminal the command exits 9 and prints `{status:"pending", verification_uri_complete, user_code, expires_in, next_step}`; re-run after approving.
- `airstrings init` without a key logs in, creates a project named after the folder (`--name`, or `--project <id>` to bind an existing one) and writes the workspace. A name collision exits 2 naming the existing project.
- Org and project keys (`as_org_…`, `as_proj_…`). One project key covers every environment. `init <project-key>` in an existing workspace switches keys in place and keeps local strings.
- `AIRSTRINGS_ORG_API_KEY` (wins over every other key), global `--project <id|name>`, `project ls`, `apikey ls`, `promote --to <env>` (full-power org key), `AIRSTRINGS_NO_BROWSER`.
- `status` reports `key_type`, `key_source` and `full_power`.
- MCP: `airstrings_login`; `airstrings_init` takes an optional key, `name` and `project`. The MCP server honors the env keys and the stored login.

### Changed

- Guidance points at the project-key page and `airstrings init <key>`; `env add` is deprecated (legacy environment keys only).
- `apikey rotate` with a project key revokes and replaces it in one call.
- A legacy key now picks its own environment as active, not the project default (a staging-only key no longer 404s on production).

## [0.17.1] - 2026-10-06

### Fixed

- `sdk-config` (text and `--json`) and the MCP tool `airstrings_sdk_config` state that every value is public, not a secret (`notice` field in JSON). Agents no longer stop on the Ed25519 public keys thinking they are credentials.

## [0.17.0] - 2026-10-05

### Added

- `sdk-config` and MCP tool `airstrings_sdk_config` print a Go initializer for the new Go SDK (`github.com/symbionix-sl/airstrings-sdk-go`).
- `bundles pull` first-pull hint covers Go seeding (`<cwd>/airstrings/bundles/` or an embedded folder via `Config.Seed`).

## [0.16.1]

### Added

- MCP tools `airstrings_status` and `airstrings_env_list`, mirroring `status --json` and `env --json`.
- When `AIRSTRINGS_ENV_ID` selects a different environment than the workspace's active one, a one-line notice naming it is printed to stderr.

### Changed

- `status` labels protection per environment (`Protection: staging: open · production: protected`; `protection_by_env` in JSON).
- A workspace holding only a key for protected production is told to create a staging write key, `env add` it, publish to staging, then promote.
- `sdk-config --json` (and the MCP tool) report `environment.protected` instead of `is_sealed`.
- Web and React Native SDK snippets include the required `locale` field.

### Fixed

- `-e -u <env-name>` followed by a command now switches and runs the command.
- `env add` treats a 403 on another environment's probe like a 404 (servers now answer 403 with guidance for environments of the same project).

## [0.16.0] - Unreleased

### Added

- `airstrings sdk-config [--env <name>]` and MCP tool `airstrings_sdk_config`: organization, project and environment IDs, Ed25519 public key(s), protection, and ready-to-paste Web / React Native / iOS / Android initializers. Works with any key of the project, so a staging key prints production's SDK config.
- `promote preview` ends with the dashboard link where a human applies the promotion (`apply_url` in JSON and in the MCP tool result).
- `status` shows the key scope (read/write) and the one next step for the current environment state (`key_scope`, `hint` in JSON).
- `env` lists every remote environment with `protected`/`open` and whether this workspace holds a key for it (`protection`, `key_in_workspace`, `active` in JSON).
- Exit code 7 (environment protected) and 8 (plan limit). With `--json`, errors are written to stderr as `{"error":{"message","next_step","exit_code"}}`; API `next_step` guidance is shown in text mode and in MCP tool errors.

### Changed

- `env use <name>` for an environment with no key in the workspace explains the state instead of "not found": protected → no key needed, use `sdk-config` and promotion (exit 7); open → create a key at the dashboard link and `env add` it (exit 3).
- `status --json` `protection` is now `"protected"`, `"open"` or `"unknown"` (was `"yolo"` for open).
- Protected-environment and quota API errors no longer exit 3 (auth); see the new codes above.

### Fixed

- `strings set/rm --push`: when the push fails, the local CSV is restored and the error says so.
- `init --url` is accepted before the key, and `AIRSTRINGS_BASE_URL` is honored.

## [0.15.0] - 2026-09-17

### Added

- `bundles pull` fails over to the fallback host reported by the API (`fallback_url` in bundle status) when the CDN is unreachable. With a fallback present, the CDN attempt gets a 5s deadline to response headers; on timeout, transport error or HTTP 5xx the bundle is downloaded once from the fallback host, with a warning on stderr. 4xx responses and signature failures never fail over.
- The host that succeeded is tried first for the remaining locales in the same pull. Without a fallback URL, behaviour is unchanged. Signature verification is unchanged.

## [0.14.1] - 2026-07-18

### Fixed

- Env-scoped API keys are no longer misfiled under every environment in the project. `ListEnvironments` returns all of a project's environments, but a key authenticates to only one; `init`/`env add` now verify per environment and store the credential only under the environment the key actually authenticates to. Previously a single key was written under every environment, silently overwriting other environments' stored keys.
- `airstrings init` refuses to run against an already-initialized workspace unless `--purge` is passed, so it can no longer wipe stored credentials. To add another environment, use `airstrings env add <key>`.

## [0.13.7] - 2026-07-16

### Fixed

- npm installer on Windows: extraction now uses `%SystemRoot%\System32\tar.exe` instead of the first `tar` on PATH. Git for Windows puts GNU tar first, which misparses `C:\...` paths as remote hosts and cannot read `.zip` archives, breaking `npm install -g @airstrings/cli` for most Windows users.

## [0.13.6] - 2026-07-15

### Added

- Install from npm: `npx @airstrings/cli <command>`, or `npm install -g @airstrings/cli`. The installed commands are `airstrings` and `airstrings-mcp` — only the package name is scoped. The package downloads the prebuilt binary for your platform on install and verifies it against the `SHA256SUMS` published with each release, refusing to install on mismatch. Requires Node 18+.

## [0.13.2] - 2026-07-15

### Added

- Releases now publish a `SHA256SUMS` asset covering every archive.

## [0.13.1] - 2026-07-15

### Added

- Windows (amd64) builds are now published with every release, and installable with [Scoop](https://scoop.sh): `scoop bucket add airstrings https://github.com/symbionix-sl/homebrew-airstrings` then `scoop install airstrings`. Ships `airstrings.exe` and `airstrings-mcp.exe`.

## [0.13.0] - 2026-07-14

### Added

- `airstrings variants` — manage an A/B experiment on a string: `create`, `set`, `allocation`, `start`, `stop`, `status`, `rm`, and `promote`. An experiment splits traffic across candidate variant values and reports live status.
- MCP tools `airstrings_variant_set`, `airstrings_variant_status`, `airstrings_variant_start`, `airstrings_variant_stop`, and `airstrings_variant_promote` exposing the variant workflow to agents without shell access.
- A `variants` operation targeting a sealed/protected production experiment returns `403` → exit code `3` (auth), consistent with the existing publish/import protection.
- `variants promote` promotes the winning variant's value into the base string in place — distinct from `airstrings promote` (environment promotion), which promotes strings from one environment to another.

## [0.12.0] - 2026-07-13

### Added

- `airstrings promote preview` — read-only diff of what a staging→production promotion would change (`--from`/`--to` env names; defaults to active→default env). Promotions are still applied in the webapp; `promote apply` is intentionally not provided.
- MCP tool `airstrings_promote_preview` exposing the same read-only diff to agents.
- `airstrings status` now reports a `protection` field (`protected`/`yolo`/`unknown`) derived from the default environment's sealed state. It is best-effort — status makes one network call for it and degrades to `unknown` on any error, never failing.

## [0.11.0] - 2026-07-10

### Added

- `bundles pull` now prints React Native embedding guidance alongside iOS, Android, and Web on the first pull.
- Every subsequent `bundles pull` prints a refresh reminder (rebuild to ship updated strings, re-embed any new locale files, run `airstrings doctor`) — previously the embedding guidance printed only on the first pull. Both hints go to stderr, so stdout and `--json` output stay clean for scripts and agents.

## [0.10.0] - 2026-07-10

### Added

- `airstrings init` now writes a `.airstrings/.gitignore` (ignoring `config.json` and `doctor.json`) whenever one is not already present, so the workspace credential file never lands in the project's git repo. An existing or user-customized `.gitignore` is left untouched, and the string CSVs remain committable.

### Security

- Keeps the API key in `.airstrings/config.json` out of git by default. Note: passing the key as a positional argument to `init` still exposes it via shell history and the process list — use the `AIRSTRINGS_API_KEY` environment variable for headless/CI to avoid both.

## [0.9.1] - 2026-07-10

### Added

- Per-command `--help`/`-h` on every command — prints that command's usage and exits `0` without executing anything.

### Fixed

- Unknown `-`-prefixed flags are now rejected with exit `2` (usage) instead of silently ignored. Previously `airstrings publish --help` executed a real publish of all locales, and `push`, `pull`, and `apikey rotate` likewise executed. Note for scripts: invocations passing unrecognized flags now fail with exit `2`.

## [0.9.0] - 2026-06-19

### Added

- Environment-variable auth for headless/CI use — no `airstrings init` required. `AIRSTRINGS_API_KEY` provides credentials and overrides any on-disk workspace; the project and default environment are resolved from the scoped key automatically. `AIRSTRINGS_PROJECT_ID` and `AIRSTRINGS_ENV_ID` skip those lookups for fully stateless, zero-discovery calls, and `AIRSTRINGS_BASE_URL` overrides the API base URL. `airstrings status` reports `source: "env"` when env-var auth is active.
- Distinct exit codes so scripts and agents can branch on the failure class: `1` generic, `2` usage/bad input, `3` auth, `4` not found, `5` network, `6` rate limited (previously every error exited `1`).
- `airstrings strings ls` gains `--cursor <c>` and `--key-prefix <p>`, and its `--json` output now includes a `pagination` object (`has_more`, `next_cursor`) so large string sets can be paged instead of dumped. The text mode prints the next-page command when more results exist.
- `--json` output added to mutations that previously printed only a success line: `sections create`/`delete`, `env use`/`add`/`rm`/`create`, and `mcp status`.
- `AGENTS.md` — a single read-once reference for driving the CLI from an AI agent (auth, exit codes, `--json` shapes, paging, workspace format).
- `NO_COLOR` is honored, and the success marker is no longer colorized when stdout is not a TTY (no ANSI escapes leak into pipes or logs).

### Changed

- **Breaking (JSON):** `airstrings strings ls --json` now returns `{ "data": [...], "pagination": { "has_more", "next_cursor" } }` instead of a bare array. Read `.data` for the entries.
- `airstrings status --json` is now a discovery payload: it adds `source`, `workspace_dir`, `mode`, and an `environments` array alongside the existing project/env/base_url fields.

## [0.8.0] - 2026-06-19

### Changed

- `airstrings strings set` now requires `--format` (`text` or `icu`). The previous implicit `text` default was removed; an unspecified or invalid format is rejected before the local CSV write. The `airstrings_strings_set` MCP tool likewise makes `format` a required parameter.

### Added

- `airstrings strings set` warns when a `text`-format value contains a `{…}` placeholder and suggests `--format icu`, since `text` is served verbatim and braces are not interpolated. The write still proceeds (braces can be legitimate in plain text). The warning prints to stderr and is included as a `warning` field in `--json` output and in the `airstrings_strings_set` MCP tool result.

### Removed

- `airstrings strings create` / `airstrings strings delete` — undocumented aliases of `strings set` / `strings rm`. Use the canonical commands.

## [0.7.0] - 2026-06-12

### Removed

- `airstrings local set/rm/ls` — deprecated in v0.5.0. Use `airstrings strings set`, `airstrings strings rm`, and `airstrings strings ls --local`.
- MCP tool names `airstrings_local_set`, `airstrings_local_rm`, `airstrings_local_ls` — deprecated in v0.5.0. Use `airstrings_strings_set`, `airstrings_strings_rm`, `airstrings_strings_ls`.

## [0.6.0] - 2026-06-11

### Added

- `airstrings strings ls --local` (also `strings list --local`) — offline workspace listing that reads the `.airstrings` CSVs directly, requiring no credentials or API client. Remote-only flags (`--limit`, `--locale`) are ignored in local mode. Replaces the deprecated `local ls`.
- `airstrings doctor` interactive ignores — when stdin is a TTY (and `--json` is not set), each `missing` finding prompts `Ignore this check in future runs? [y/N/q]`. Accepted checks are persisted to `.airstrings/doctor.json` (0600) as `<platform>:<relpath>` keys and reported as `ignored` on later runs: shown with a `•` marker, included in `--json` output with `"status": "ignored"`, and excluded from the missing count and the non-zero exit. The new `--no-input` flag disables prompting; non-TTY stdin and `--json` never prompt, so CI behavior is unchanged.

### Fixed

- `airstrings doctor` no longer fails dual-build apps over SPM library packages: `Package.swift` files that never reference AirStrings are skipped entirely, and when an Xcode check passes, `missing` SPM findings (including `.process`) are downgraded to `manual` hints — SPM package resources land in the package's own bundle, so only the artifact that ships the app bundle needs the seed. Pure-SPM projects keep the strict behavior.
- `airstrings doctor` now detects Bazel workspaces rooted above the project: `MODULE.bazel`, `WORKSPACE`, and `WORKSPACE.bazel` markers are also looked up in up to 3 parent directories of the project root. The BUILD-file content scan stays bounded to the project tree.

## [0.5.0] - 2026-06-11

### Changed

- **Breaking:** `airstrings strings set` and `airstrings strings rm` are now local-first. They write to the workspace CSVs (the former `local set`/`local rm` behavior, including `--format` and `--section`) instead of calling the API, and work fully offline. Add the new `--push` flag to also sync that single key to the API immediately: `set --push` upserts the key (creating the remote section if needed), `rm --push` deletes the key remotely (or clears just one locale with `--locale`). `strings list/ls` and `strings get` remain remote read-only. `strings create` and `strings delete` are now aliases of `set` and `rm`. The JSON output of `set`/`rm` gains an additive `pushed` field.
- MCP server: the workspace mutation tools are renamed to match the new CLI namespace — `airstrings_local_set/rm/ls` become `airstrings_strings_set/rm/ls` (same handlers and behavior). `airstrings_strings_set` and `airstrings_strings_rm` gain an optional boolean `push` parameter mirroring the CLI `--push` flag: when true, the key is also synced to the API immediately after the local CSV write, and a client resolution or API failure is returned as a tool error.

### Deprecated

- `airstrings local set/rm/ls` — the commands still work, forward to the new `strings` handlers, and print a deprecation warning to stderr. They will be removed in a future minor release.
- MCP tool names `airstrings_local_set/rm/ls` — still registered as aliases of the same handlers, with a deprecation note in their tool descriptions. They will be removed in a future minor release.

## [0.4.3] - 2026-06-11

### Added

- `airstrings doctor [dir]` — verifies bundled-fallback integration in the host project, locally and with no API calls. Checks the seed directory (manifest plus bundle files), detects host platforms with a bounded filesystem scan (Xcode, SPM, Bazel, Android, web), and verifies each one references the seed folder correctly — flagging common mistakes like Xcode group references instead of folder references and SPM `.process` instead of `.copy` — with exact fix steps for anything not wired up. Exits non-zero when any check is missing. The first-pull hint after `airstrings bundles pull` now points to it.

## [0.4.2] - 2026-06-11

### Changed

- Release workflow actions bumped to latest majors (checkout v6, setup-go v6, upload-artifact v7, download-artifact v8) for the Node 24 runner requirement. No functional CLI changes.

## [0.4.1] - 2026-06-10

### Fixed

- `airstrings bundles pull` no longer rewrites `manifest.json` when a pull changes nothing — `generated_at` and `cli_version` alone never force a rewrite, so the file stays byte-untouched and repeated pulls with no upstream changes produce zero diff, making the command idempotent for CI diff guards. The manifest is still rewritten whenever directory contents change, and a malformed `manifest.json` on disk is rewritten to valid content.

## [0.4.0] - 2026-06-10

### Added

- `airstrings bundles pull [dir] [--locale <bcp47>]` — pulls the active environment's published, signed bundles into a committable seed directory (default `airstrings/bundles/` at the workspace root) so SDKs can serve strings offline on cold starts. Every downloaded artifact is verified CLI-side (Ed25519 signature against the embedded key, plus project/locale/revision cross-checks against the API metadata) and written byte-identical to the CDN object. Pulls are atomic (staged, then moved into place), idempotent with mirror semantics (stale locale files removed, unmanaged files untouched), and record provenance in `manifest.json`. A custom `[dir]` is persisted to `.airstrings/config.json` under `bundles_dir`. Distinct from `airstrings pull`, which fetches draft strings as editable CSVs.
