# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
