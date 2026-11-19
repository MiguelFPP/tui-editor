# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-11-18

Docker panel and release/documentation tooling.

### Added

- Docker panel backed by the modular Moby client: container list with name,
  image, state, health and ports.
- One-shot `stats` sample and paginated `logs` (`Tail=100`, `Follow=false`) for
  the selected container.
- `start`, `stop` and `restart` actions behind an explicit confirmation.
- Lazy polling: the ticker only runs while the Docker tab is visible; a hidden
  tab issues no daemon calls and reserves no memory.
- GoReleaser config, a tag-driven release workflow, a VHS demo, `CONTRIBUTING.md`
  and `SECURITY.md`.
- `--version` now reports the injected commit alongside the version in release
  builds.

## [0.2.0] - 2026-11-18

Interactive Git and fast in-TUI editing.

### Added

- Interactive Git panel: per-file and per-hunk staging, multiline commits, branch
  create/delete/merge, stash push/pop, conflict resolution (`ours`/`theirs`/abort)
  and `fetch`/`pull`/`push`.
- In-TUI editing of the open file with `ctrl+s`, saving against the last read so
  external changes are never overwritten; a pending draft survives navigation
  and warns before quitting.
- External editor launch (`E`/`ctrl+e`) resolved with `exec.LookPath`, run without
  a shell and with the absolute file path.
- Syntax-highlighted preview with line numbers, scrolling and binary/size
  detection.

## [0.1.0] - 2026-11-18

First runnable MVP.

### Added

- Bilingual-ready Bubble Tea shell: alternate screen, mouse support, root tabs,
  focus handling, help overlay, spinner, error toasts and status bar.
- Safe filesystem adapter with file tree, ignores, path validation against the
  repository root and bounds on entries and depth.
- Read-only Git adapter (status, diff, branches, log) using the native binary
  with `--porcelain=v2 -z`, no shell and no human-output scraping.
- Fast preview with Chroma syntax highlighting and diff rendering.
- `--version`, `--repo`, `--editor`, `--docker-host` and `--debug-log` flags.
- Hexagonal layout (`domain`, `ports`, `adapters`, `tui`) with `main` as the
  composition root, plus shared fakes for tests.

[0.3.0]: https://github.com/MiguelFPP/tui-editor/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/MiguelFPP/tui-editor/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/MiguelFPP/tui-editor/releases/tag/v0.1.0
