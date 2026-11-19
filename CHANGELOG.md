# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.0] - 2026-11-19

Configurable themes and keybindings.

### Added

- Theme catalog with Tokyo Night (default), Catppuccin Mocha, Dracula and
  Gruvbox Dark. `--theme`, `TUI_EDITOR_THEME` and `--list-themes` select the
  palette; the preview highlighting follows the same Chroma style.
- Optional JSON configuration at `os.UserConfigDir()/tui-editor/config.json`,
  overridable with `--config` and `TUI_EDITOR_CONFIG`. It is validated strictly
  and resolved as `flags > env > file > defaults`.
- Configurable keybindings for the root, file tree, editor, Git and Docker
  scopes. `--list-keys` prints every action, its scope, its defaults and
  whether it can be overridden.
- Help overlay, status bar and panel footers render the effective keys.
- `config.example.json` template with an example theme and key overrides; it is
  never loaded automatically.
- `--init-config` writes a minimal base file (`theme: tokyonight-night`,
  `keys: {}`) to the resolved path and exits; `--force` allows overwriting an
  existing file.
- `--print-config` prints the resolved configuration as JSON (`config_path`,
  `theme`, `keys`) and exits without starting the TUI.
- A global `config` action (default `,`) that opens a read-only overlay with the
  loaded file, the active theme, the key overrides and a restart notice.

### Changed

- `internal/tui/styles` is now a theme catalog; `styles.Apply` rebuilds every
  token and the Chroma style used by the preview.
- Key handling is centralized in `internal/tui/keymap`; components receive the
  registry through `Deps`.
- Modal actions and `ctrl+c` remain fixed to avoid locking the user out.
- The help overlay and the status bar list the new `config` action generated from
  the registry.

### Fixed

- `git_up`/`git_down`, `docker_up`/`docker_down` and the Docker logs actions now
  match their names: `j`/`down` moves down and `k`/`up` moves up.

## [0.4.0] - 2026-11-19

UI improvements: clickable tabs, syntax highlighting while editing, panel
frames, and a Docker panel scoped to the open project.

### Added

- The top tabs (`Files`, `Git`, `Docker`) respond to the mouse. A click lands
  exactly on the drawn label and goes through the same path as the `1`/`2`/`3`
  keys, so the focus rules and the Docker ticker are unchanged.
- The Docker panel lists only the containers of the open project by default,
  matched through the compose `working_dir` label with the project name as a
  fallback. `a` toggles between the project and every container, keeps the
  selected one and names the mode in the title and the footer.
- The draft keeps its syntax highlighting while editing: gutter, colours and
  tab expansion now come from the same lexer and formatter as the preview.
- A visible caret in the draft, with its position in the header, wheel
  scrolling and click-to-position.
- Every panel sits in a rounded frame; the focused one uses the accent colour.

### Changed

- All colours and style tokens live in `internal/tui/styles`, replacing six
  duplicated private style structs scattered across the components.
- The editor opens the draft on the first line instead of the last one.

### Fixed

- A click in the editor landed `treeW` columns and two rows away from the
  pointer: the root router passed absolute terminal coordinates to a component
  that reads them as panel-relative.

## [0.3.2] - 2026-11-19

Installer update fix.

### Fixed

- The installers now replace the binary through a staged file and an atomic
  rename. Updating while `tui-editor` was running previously failed with
  `ETXTBSY` ("text file busy") and a confusing error after the download had
  already succeeded; the running process now keeps working and the next launch
  uses the new version. Windows uses the same pattern because it refuses to
  overwrite a running `exe`. An interrupted install no longer leaves a
  half-written executable behind.

## [0.3.1] - 2026-11-19

One-line installation for every supported platform.

### Added

- `scripts/install.sh` for Linux and macOS: detects the platform, resolves the
  latest release without the GitHub API, downloads the matching archive, verifies
  its SHA-256 against `checksums.txt` and installs to `/usr/local/bin` (root) or
  `~/.local/bin`.
- `scripts/install.ps1` for Windows: same flow with `amd64`/`arm64` detection,
  checksum verification, install into `%LOCALAPPDATA%\Programs\tui-editor` and
  user `PATH` registration.

### Changed

- The README separates the requirements for running the static binary from the
  requirements for developing the project, and documents building from source
  with the ldflags that inject the version.

### Fixed

- The README download snippet pointed at `releases/latest/download/<asset>`,
  which cannot work because the asset name embeds the version.

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

[Unreleased]: https://github.com/MiguelFPP/tui-editor/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/MiguelFPP/tui-editor/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/MiguelFPP/tui-editor/compare/v0.3.1...v0.4.0
[0.3.2]: https://github.com/MiguelFPP/tui-editor/commit/a9896df
[0.3.1]: https://github.com/MiguelFPP/tui-editor/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/MiguelFPP/tui-editor/commit/954fcf4
[0.2.0]: https://github.com/MiguelFPP/tui-editor/commit/cb7629a
[0.1.0]: https://github.com/MiguelFPP/tui-editor/commit/0dd4538
