# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `airstrings bundles pull [dir] [--locale <bcp47>]` — pulls the active environment's published, signed bundles into a committable seed directory (default `airstrings/bundles/` at the workspace root) so SDKs can serve strings offline on cold starts. Every downloaded artifact is verified CLI-side (Ed25519 signature against the embedded key, plus project/locale/revision cross-checks against the API metadata) and written byte-identical to the CDN object. Pulls are atomic (staged, then moved into place), idempotent with mirror semantics (stale locale files removed, unmanaged files untouched), and record provenance in `manifest.json`. A custom `[dir]` is persisted to `.airstrings/config.json` under `bundles_dir`. Distinct from `airstrings pull`, which fetches draft strings as editable CSVs.
