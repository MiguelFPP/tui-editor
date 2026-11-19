# Security Policy

## Reporting a vulnerability

Please report suspected vulnerabilities privately through GitHub's
[security advisories](https://github.com/MiguelFPP/tui-editor/security/advisories/new)
instead of a public issue.

Include the affected version (`tui-editor --version`), a description of the
issue and, when possible, a minimal reproduction. You will get an initial reply
as soon as the report is triaged.

## Supported versions

| Version | Supported          |
| ------- | ------------------ |
| 0.3.x   | :white_check_mark: |
| 0.2.x   | :x:                |
| < 0.2   | :x:                |

Only the latest release line receives security fixes.

## Docker socket: root-equivalent access

`tui-editor` talks to the Docker daemon over `DOCKER_HOST` (default
`unix:///var/run/docker.sock`). Access to that socket is **equivalent to root
access on the host**: anyone who can reach it can start privileged containers,
mount the host filesystem and escape the sandbox.

Because of that:

- The socket is **never mounted by default**. In this repository only the
  opt-in `dev-docker` Compose profile mounts `/var/run/docker.sock`, and only
  for developing the Docker panel. The default `dev` and `app` services do not
  mount it.
- The daemon is never published over TCP by the project.
- Run the TUI as an unprivileged user and point `--docker-host` only at a
  daemon you control.
- Treat the Docker panel as read-mostly: `start`, `stop` and `restart` require
  explicit confirmation, and no destructive action runs silently.

If you do not need the Docker panel, do not mount the socket.

## Handling of untrusted input

- Every file path is validated against the repository root; traversal and
  symlink escapes are blocked.
- Git and the external editor are executed with argument lists, never through a
  shell.
- Previews, logs and buffers are bounded by size and line limits.

## Dependency updates

Dependencies are pinned in `go.mod`/`go.sum` and updated through the review
process. Report a vulnerable transitive dependency the same way as any other
finding.
