// Package git implements the Git ports on top of the native git binary.
//
// Staging by hunk rebuilds a minimal unified patch and applies it with `git apply --cached`.
// That path has documented limitations: untracked, binary and renamed files are staged only as a
// whole, a truncated diff is rejected instead of rebuilt, and the hunks are applied complete. The
// patch keeps the hunk starts of the parsed diff and does not pass `--recount`, so `git apply`
// fails when the index no longer matches the pre-image the diff was read from.
package git
