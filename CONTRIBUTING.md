# Contributing to tui-editor

Thanks for helping improve `tui-editor`. This guide covers the local setup, the
branch and test workflow, and the commit rules.

## Requirements

- Docker
- Docker Compose v2 or newer

Go, Git and Node are **not** installed on the host. Every build, test, format
and dependency command runs inside the `dev` container through the `Makefile`.
Architecture notes, ADRs and performance budgets live outside this repository;
this repo keeps only the code, the `Makefile` and the public docs.

## Setup

```bash
git clone git@github.com:MiguelFPP/tui-editor.git
cd tui-editor
make build     # build the dev and runtime images
make dev       # run the TUI with hot reload (needs a TTY)
```

`make run` runs the production image. Both targets need a real terminal: use
`docker compose run`, not `docker compose up -d`.

To develop the Docker panel, use `make dev-docker`. It is the only profile that
mounts `/var/run/docker.sock`; see `SECURITY.md` for the risk.

Run `make help` for the full target list.

## Branching and pull requests

- `main` is the stable branch; `develop` is the integration branch.
- Branch names: `feat/fNN-slug`, `fix/slug`, `chore/slug`, `release/vX.Y.Z`,
  `hotfix/slug`.
- Every change lands through a pull request into `develop`.
- `main` only receives pull requests from `release/*` or `hotfix/*`.
- The required check to merge is the `test` job of GitHub Actions.

## Commits

- Use [Conventional Commits](https://www.conventionalcommits.org/): `feat:`,
  `fix:`, `docs:`, `chore:`, `test:`, `ci:`.
- Keep messages focused; one logical change per commit.
- Do **not** add a `Co-authored-by` trailer. The author is the contributor.

## Tests

The full suite (`make test`, `make vet`, `go test ./...`) is reserved for CI and
the final phase. While implementing a change, run only the packages you touch:

```bash
make test-pkg PKG='./internal/<pkg>/...'
make test-run PKG='./internal/<pkg>/...' RUN='TestName'
make vet-pkg  PKG='./internal/<pkg>/...'
make test-integration PKG='./internal/<pkg>/...' [RUN='TestIntegration...']
make bench-pkg PKG='./internal/<pkg>/...'
```

Integration tests use the `integration` build tag and must skip (not fail) when
Docker, the socket or the network is unavailable.

## Editing code

- Never run Go tooling on the host; use the container targets.
- Keep comments minimal and factual; avoid narrative docblocks.
- Add a dependency only with justification, pinning its exact version in
  `go.mod`, then run `make tidy` and `make build`.

## Docs and language

- Code, package names and technical docs are written in English.
- UI strings and error messages are written in Spanish.

## License

By contributing you agree that your work is released under the MIT License
(see `LICENSE`).
