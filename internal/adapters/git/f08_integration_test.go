//go:build integration

package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// bareRemote builds a bare repository the tests push to and pull from, without a network.
func bareRemote(t *testing.T) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}

	return dir
}

// cloneRepo copies one remote into a second working tree the tests drive with plain git.
func cloneRepo(t *testing.T, remote string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "clon")
	if out, err := exec.Command("git", "clone", "-q", remote, dir).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v: %s", err, out)
	}

	return dir
}

// gitAt runs one setup command in any directory, with a fixed identity.
func gitAt(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME="+authorName,
		"GIT_AUTHOR_EMAIL="+authorEmail,
		"GIT_COMMITTER_NAME="+authorName,
		"GIT_COMMITTER_EMAIL="+authorEmail,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}

	return strings.TrimSpace(string(out))
}

// writeIn writes one file inside an arbitrary working tree.
func writeIn(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v, want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v, want nil", path, err)
	}
}

// readIn returns the exact text of one file of an arbitrary working tree.
func readIn(t *testing.T, dir, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v, want nil", name, err)
	}

	return string(data)
}

func TestIntegrationBranchesCreateCheckoutAndDelete(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	service := r.service()
	ctx := context.Background()

	if err := service.CreateBranch(ctx, "feature/f08", "main"); err != nil {
		t.Fatalf("CreateBranch() error = %v, want nil", err)
	}

	status, err := service.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if status.Branch != "feature/f08" {
		t.Errorf("Branch = %q, want the branch created and entered", status.Branch)
	}

	branches, err := service.Branches(ctx)
	if err != nil {
		t.Fatalf("Branches() error = %v, want nil", err)
	}
	listed := false
	for _, branch := range branches {
		if branch.Name == "feature/f08" {
			listed = true
			if !branch.Current {
				t.Errorf("feature/f08 = %+v, want it marked as current", branch)
			}
		}
	}
	if !listed {
		t.Errorf("Branches() = %+v, want the new branch", branches)
	}

	if err := service.Checkout(ctx, "main"); err != nil {
		t.Fatalf("Checkout(main) error = %v, want nil", err)
	}
	if status, err := service.Status(ctx); err != nil || status.Branch != "main" {
		t.Errorf("Status() = %+v, %v, want the main branch", status, err)
	}

	// No guessing: a branch that does not exist is refused instead of created.
	if err := service.Checkout(ctx, "no-existe"); err == nil {
		t.Error("Checkout(no-existe) error = nil, want a failure")
	}
	if branches, err := service.Branches(ctx); err == nil {
		for _, branch := range branches {
			if branch.Name == "no-existe" {
				t.Error("Checkout created a branch, want no guessing")
			}
		}
	}

	// The current branch never goes away.
	if err := service.DeleteBranch(ctx, "main", true); err == nil {
		t.Error("DeleteBranch(main) error = nil, want a failure")
	}

	if err := service.DeleteBranch(ctx, "feature/f08", false); err != nil {
		t.Fatalf("DeleteBranch() error = %v, want nil", err)
	}
	if branches, err := service.Branches(ctx); err != nil {
		t.Fatalf("Branches() error = %v, want nil", err)
	} else {
		for _, branch := range branches {
			if branch.Name == "feature/f08" {
				t.Errorf("Branches() = %+v, want the deleted branch gone", branches)
			}
		}
	}
}

func TestIntegrationBranchesDirtyWorktree(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	r.run("checkout", "-q", "-b", "otra")
	r.write("base.txt", "otra\n")
	r.commit("cambio en otra")
	r.run("checkout", "-q", "main")

	// The uncommitted change of base.txt would be lost by the switch.
	r.write("base.txt", "local\n")

	err := r.service().Checkout(context.Background(), "otra")
	if !errors.Is(err, domain.ErrDirtyWorktree) {
		t.Fatalf("Checkout() error = %v, want %v", err, domain.ErrDirtyWorktree)
	}
	if got := readIn(t, r.dir, "base.txt"); got != "local\n" {
		t.Errorf("base.txt = %q, want the local change untouched", got)
	}
}

func TestIntegrationBranchesDeleteWithoutForceKeepsUnmergedWork(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	r.run("checkout", "-q", "-b", "otra")
	r.write("otra.txt", "contenido\n")
	r.commit("trabajo sin fusionar")
	r.run("checkout", "-q", "main")

	service := r.service()
	ctx := context.Background()

	if err := service.DeleteBranch(ctx, "otra", false); err == nil {
		t.Fatal("DeleteBranch() error = nil, want the unmerged branch refused")
	}
	if err := service.DeleteBranch(ctx, "otra", true); err != nil {
		t.Fatalf("DeleteBranch(force) error = %v, want nil", err)
	}
}

func TestIntegrationStashPushApplyPopAndDrop(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	service := r.service()
	ctx := context.Background()

	if err := service.StashPush(ctx, "nada"); !errors.Is(err, domain.ErrNoChanges) {
		t.Fatalf("StashPush() on a clean tree error = %v, want %v", err, domain.ErrNoChanges)
	}

	r.write("base.txt", "uno\ndos\n")
	if err := service.StashPush(ctx, "trabajo a medias"); err != nil {
		t.Fatalf("StashPush() error = %v, want nil", err)
	}

	if status, err := service.Status(ctx); err != nil || status.HasChanges() {
		t.Errorf("Status() = %+v, %v, want a clean tree after the stash", status, err)
	}
	if got := readIn(t, r.dir, "base.txt"); got != "uno\n" {
		t.Errorf("base.txt = %q, want the stashed change out of the worktree", got)
	}

	stashes, err := service.Stashes(ctx)
	if err != nil {
		t.Fatalf("Stashes() error = %v, want nil", err)
	}
	if len(stashes) != 1 || stashes[0].Message != "trabajo a medias" || stashes[0].Branch != "main" {
		t.Fatalf("Stashes() = %+v, want one entry with the custom message", stashes)
	}

	// apply restores the change and keeps the entry.
	if err := service.StashApply(ctx, 0); err != nil {
		t.Fatalf("StashApply() error = %v, want nil", err)
	}
	if got := readIn(t, r.dir, "base.txt"); got != "uno\ndos\n" {
		t.Errorf("base.txt = %q, want the applied change", got)
	}
	if stashes, err := service.Stashes(ctx); err != nil || len(stashes) != 1 {
		t.Errorf("Stashes() = %+v, %v, want the entry kept after apply", stashes, err)
	}

	// drop forgets the entry without touching the worktree.
	if err := service.StashDrop(ctx, 0); err != nil {
		t.Fatalf("StashDrop() error = %v, want nil", err)
	}
	if stashes, err := service.Stashes(ctx); err != nil || len(stashes) != 0 {
		t.Errorf("Stashes() = %+v, %v, want the entry dropped", stashes, err)
	}
	if got := readIn(t, r.dir, "base.txt"); got != "uno\ndos\n" {
		t.Errorf("base.txt = %q, want the worktree untouched by the drop", got)
	}

	// pop restores the change and removes the entry.
	if err := service.StashPush(ctx, ""); err != nil {
		t.Fatalf("StashPush() without a message error = %v, want nil", err)
	}
	if stashes, err := service.Stashes(ctx); err != nil || len(stashes) != 1 {
		t.Fatalf("Stashes() = %+v, %v, want one entry", stashes, err)
	}
	if err := service.StashPop(ctx, 0); err != nil {
		t.Fatalf("StashPop() error = %v, want nil", err)
	}
	if got := readIn(t, r.dir, "base.txt"); got != "uno\ndos\n" {
		t.Errorf("base.txt = %q, want the popped change", got)
	}
	if stashes, err := service.Stashes(ctx); err != nil || len(stashes) != 0 {
		t.Errorf("Stashes() = %+v, %v, want the popped entry gone", stashes, err)
	}
}

func TestIntegrationStashIndexOutOfRangeAndMissingEntry(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	service := r.service()
	ctx := context.Background()

	if err := service.StashApply(ctx, -1); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("StashApply(-1) error = %v, want %v", err, domain.ErrInvalidArgument)
	}
	if err := service.StashApply(ctx, 0); err == nil {
		t.Error("StashApply(0) without entries error = nil, want a failure")
	}
}

func TestIntegrationStashConflictsKeepTheEntry(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "base\n")
	r.commit("initial commit")

	service := r.service()
	ctx := context.Background()

	r.write("base.txt", "guardado\n")
	if err := service.StashPush(ctx, "conflicto"); err != nil {
		t.Fatalf("StashPush() error = %v, want nil", err)
	}

	r.write("base.txt", "principal\n")
	r.commit("cambio principal")

	if err := service.StashApply(ctx, 0); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("StashApply() error = %v, want %v", err, domain.ErrConflict)
	}

	status, err := service.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	conflicts := status.Conflicts()
	if len(conflicts) != 1 || conflicts[0].Path != "base.txt" || conflicts[0].Kind != domain.BothModified {
		t.Errorf("Conflicts() = %+v, want base.txt as BothModified", conflicts)
	}
	if stashes, err := service.Stashes(ctx); err != nil || len(stashes) != 1 {
		t.Errorf("Stashes() = %+v, %v, want the entry kept after the conflict", stashes, err)
	}

	if err := service.StashPop(ctx, 0); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("StashPop() error = %v, want %v on the pending conflict", err, domain.ErrConflict)
	}
}

func TestIntegrationMergeFastForwardAndUpToDate(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	r.run("checkout", "-q", "-b", "lado")
	r.write("lado.txt", "lado\n")
	r.commit("trabajo en lado")
	lado := r.run("rev-parse", "HEAD")
	r.run("checkout", "-q", "main")

	service := r.service()
	ctx := context.Background()

	result, err := service.Merge(ctx, "lado")
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if result.Status != domain.FastForward {
		t.Errorf("Status = %d, want %d", result.Status, domain.FastForward)
	}
	if head := r.run("rev-parse", "HEAD"); head != lado {
		t.Errorf("HEAD = %s, want the merged branch %s", head, lado)
	}
	if status, err := service.Status(ctx); err != nil || status.HasChanges() {
		t.Errorf("Status() = %+v, %v, want a clean tree after the merge", status, err)
	}

	result, err = service.Merge(ctx, "lado")
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if result.Status != domain.UpToDate {
		t.Errorf("Status = %d, want %d", result.Status, domain.UpToDate)
	}
}

func TestIntegrationMergeCreatesAMergeCommit(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	r.run("checkout", "-q", "-b", "lado")
	r.write("lado.txt", "lado\n")
	r.commit("trabajo en lado")
	r.run("checkout", "-q", "main")
	r.write("main.txt", "main\n")
	r.commit("trabajo en main")

	service := r.service()
	ctx := context.Background()

	result, err := service.Merge(ctx, "lado")
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if result.Status != domain.Clean {
		t.Errorf("Status = %d, want %d", result.Status, domain.Clean)
	}
	if parents := r.run("rev-list", "--parents", "-n", "1", "HEAD"); len(strings.Fields(parents)) != 3 {
		t.Errorf("parents = %q, want a merge commit with two parents", parents)
	}
	if status, err := service.Status(ctx); err != nil || status.HasChanges() {
		t.Errorf("Status() = %+v, %v, want the merge committed", status, err)
	}
}

func TestIntegrationMergeConflictsAbortAndResolve(t *testing.T) {
	r := newRepo(t)
	r.write("conflicto.txt", "base\n")
	r.commit("initial commit")

	r.run("checkout", "-q", "-b", "lado")
	r.write("conflicto.txt", "lado\n")
	r.commit("cambio de lado")
	r.run("checkout", "-q", "main")
	r.write("conflicto.txt", "principal\n")
	r.commit("cambio principal")

	service := r.service()
	ctx := context.Background()

	result, err := service.Merge(ctx, "lado")
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil: a conflict is an outcome, not a failure", err)
	}
	if result.Status != domain.Conflicted || len(result.Conflicts) != 1 {
		t.Fatalf("Merge() = %+v, want one conflicted file", result)
	}
	if result.Conflicts[0].Path != "conflicto.txt" || result.Conflicts[0].Kind != domain.BothModified {
		t.Errorf("conflict = %+v, want conflicto.txt as BothModified", result.Conflicts[0])
	}

	if _, err := service.Merge(ctx, "lado"); !errors.Is(err, domain.ErrMergeInProgress) {
		t.Fatalf("Merge() during a merge error = %v, want %v", err, domain.ErrMergeInProgress)
	}

	if err := service.ResolveConflict(ctx, "conflicto.txt", domain.Ours); err != nil {
		t.Fatalf("ResolveConflict(ours) error = %v, want nil", err)
	}
	if got := readIn(t, r.dir, "conflicto.txt"); got != "principal\n" {
		t.Errorf("conflicto.txt = %q, want our side", got)
	}
	if status, err := service.Status(ctx); err != nil || len(status.Conflicts()) != 0 {
		t.Errorf("Status() = %+v, %v, want the conflict resolved", status, err)
	}

	if err := service.AbortMerge(ctx); err != nil {
		t.Fatalf("AbortMerge() error = %v, want nil", err)
	}
	if got := readIn(t, r.dir, "conflicto.txt"); got != "principal\n" {
		t.Errorf("conflicto.txt = %q, want the abort to restore HEAD", got)
	}
	if err := service.AbortMerge(ctx); !errors.Is(err, domain.ErrNoMergeInProgress) {
		t.Fatalf("AbortMerge() without a merge error = %v, want %v", err, domain.ErrNoMergeInProgress)
	}

	result, err = service.Merge(ctx, "lado")
	if err != nil || result.Status != domain.Conflicted {
		t.Fatalf("Merge() = %+v, %v, want the conflict again", result, err)
	}
	if err := service.ResolveConflict(ctx, "conflicto.txt", domain.Theirs); err != nil {
		t.Fatalf("ResolveConflict(theirs) error = %v, want nil", err)
	}
	if got := readIn(t, r.dir, "conflicto.txt"); got != "lado\n" {
		t.Errorf("conflicto.txt = %q, want their side", got)
	}
}

func TestIntegrationSyncFetchPushAndPull(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	remote := bareRemote(t)
	r.run("remote", "add", "origin", remote)
	r.run("push", "-q", "-u", "origin", "main")

	service := r.service()
	ctx := context.Background()

	other := cloneRepo(t, remote)
	writeIn(t, other, "base.txt", "uno\ndos\n")
	gitAt(t, other, "add", "-A")
	gitAt(t, other, "commit", "-q", "-m", "desde el clon")
	gitAt(t, other, "push", "-q", "origin", "HEAD:main")

	if status, err := service.Status(ctx); err != nil || status.Behind != 0 {
		t.Fatalf("Status() = %+v, %v, want nothing to pull before the fetch", status, err)
	}

	if err := service.Fetch(ctx); err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	status, err := service.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if status.Behind != 1 {
		t.Errorf("Behind = %d, want 1 after the fetch", status.Behind)
	}

	result, err := service.Pull(ctx)
	if err != nil {
		t.Fatalf("Pull() error = %v, want nil", err)
	}
	if result.Status != domain.FastForward {
		t.Errorf("Status = %d, want %d", result.Status, domain.FastForward)
	}
	if got := readIn(t, r.dir, "base.txt"); got != "uno\ndos\n" {
		t.Errorf("base.txt = %q, want the change of the remote", got)
	}

	r.write("local.txt", "local\n")
	r.run("add", "-A")
	r.run("commit", "-q", "-m", "commit local")

	if err := service.Push(ctx); err != nil {
		t.Fatalf("Push() error = %v, want nil", err)
	}
	if got, want := gitAt(t, remote, "rev-parse", "refs/heads/main"), r.run("rev-parse", "HEAD"); got != want {
		t.Errorf("remote main = %s, want %s", got, want)
	}

	result, err = service.Pull(ctx)
	if err != nil {
		t.Fatalf("Pull() error = %v, want nil", err)
	}
	if result.Status != domain.UpToDate {
		t.Errorf("Status = %d, want %d", result.Status, domain.UpToDate)
	}
}

func TestIntegrationSyncRejectsANonFastForwardPush(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	remote := bareRemote(t)
	r.run("remote", "add", "origin", remote)
	r.run("push", "-q", "-u", "origin", "main")

	other := cloneRepo(t, remote)
	writeIn(t, other, "otro.txt", "otro\n")
	gitAt(t, other, "add", "-A")
	gitAt(t, other, "commit", "-q", "-m", "desde el clon")
	gitAt(t, other, "push", "-q", "origin", "HEAD:main")

	r.write("local.txt", "local\n")
	r.run("add", "-A")
	r.run("commit", "-q", "-m", "commit local")

	if err := r.service().Push(context.Background()); !errors.Is(err, domain.ErrNonFastForward) {
		t.Fatalf("Push() error = %v, want %v", err, domain.ErrNonFastForward)
	}
}

func TestIntegrationSyncWithoutUpstream(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	service := r.service()
	ctx := context.Background()

	// Without remotes there is nothing to prune, so the fetch is a no-op.
	if err := service.Fetch(ctx); err != nil {
		t.Errorf("Fetch() error = %v, want nil without remotes", err)
	}
	if err := service.Push(ctx); !errors.Is(err, domain.ErrNoUpstream) {
		t.Errorf("Push() error = %v, want %v", err, domain.ErrNoUpstream)
	}
	if _, err := service.Pull(ctx); !errors.Is(err, domain.ErrNoUpstream) {
		t.Errorf("Pull() error = %v, want %v", err, domain.ErrNoUpstream)
	}
}
