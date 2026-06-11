# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
