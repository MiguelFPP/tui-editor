# demo/demo.tape — reproducible tui-editor demo (VHS v0.12.1)
#
# Generate the GIF on a host that has VHS, ttyd and ffmpeg installed:
#
#   docker compose --profile prod build   # build the runtime image first
#   vhs demo/demo.tape                    # writes demo/demo.gif
#
# The tape drives the real TUI through a pseudo-terminal, so it needs a
# working Docker daemon. Files and Git run against this repository via
# `make run`. The Docker panel is only populated under `make dev-docker`,
# the single profile that mounts /var/run/docker.sock; that mount grants
# root-equivalent access to the host and is never enabled by default
# (see SECURITY.md). Swap the command below for `make dev-docker` to show
# live containers.
#
# No secrets are typed here; the commit message is a throwaway.

Output demo/demo.gif

Set Shell        "bash"
Set FontFamily   "JetBrains Mono"
Set FontSize     15
Set Width        1100
Set Height       720
Set Padding      24
Set Theme        "Catppuccin Mocha"
Set WindowBar    "Colorful"
Set WindowBarSize 40
Set BorderRadius 8
Set TypingSpeed  35ms
Set CursorBlink  true

Sleep 600ms

# Start the production image (Bubble Tea needs a real TTY).
Type "make run"
Enter
Sleep 6s

# Files tab: walk the tree, open a file and show the highlighted preview.
Type "1"
Sleep 600ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "l"
Sleep 500ms
Type "enter"
Sleep 1600ms
Type "e"
Sleep 900ms
Type "esc"
Sleep 700ms

# Git tab: stage a file, open the commit modal and commit.
Type "2"
Sleep 1400ms
Type "s"
Sleep 900ms
Type "c"
Sleep 700ms
Type "docs: demo commit"
Sleep 400ms
Type "ctrl+s"
Sleep 1600ms

# Docker tab: under `make dev-docker` this lists the running containers.
Type "3"
Sleep 1600ms
Type "r"
Sleep 1200ms

Type "q"
Sleep 900ms
