# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
