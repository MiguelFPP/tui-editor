//go:build integration

package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

const (
	fixedDate   = "2026-01-02T03:04:05+0000"
	authorName  = "Test User"
	authorEmail = "user@example.com"
)

// repo is one throwaway repository built by the test itself.
type repo struct {
	t    *testing.T
	dir  string
	root domain.RepoPath
}

func newRepo(t *testing.T) *repo {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git no está disponible: %v", err)
	}

	dir := t.TempDir()

	r := &repo{t: t, dir: dir}
	r.run("init", "-q", "-b", "main")
	r.run("config", "user.name", authorName)
	r.run("config", "user.email", authorEmail)

	root, err := domain.NewRepoPath(dir)
	if err != nil {
		t.Fatalf("NewRepoPath(%q) error = %v, want nil", dir, err)
	}
	r.root = root

	return r
}

// run executes a setup command; it never uses the adapter under test.
func (r *repo) run(args ...string) string {
	r.t.Helper()

	cmd := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME="+authorName,
		"GIT_AUTHOR_EMAIL="+authorEmail,
		"GIT_COMMITTER_NAME="+authorName,
		"GIT_COMMITTER_EMAIL="+authorEmail,
		"GIT_AUTHOR_DATE="+fixedDate,
		"GIT_COMMITTER_DATE="+fixedDate,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}

	return strings.TrimSpace(string(out))
}

func (r *repo) write(name, content string) {
	r.t.Helper()

	path := filepath.Join(r.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatalf("MkdirAll(%q) error = %v, want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.t.Fatalf("WriteFile(%q) error = %v, want nil", path, err)
	}
}

// runQuiet executes a setup command whose exit status is expected to fail.
func (r *repo) runQuiet(args ...string) {
	r.t.Helper()

	cmd := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	_ = cmd.Run()
}

func (r *repo) commit(subject string) {
	r.t.Helper()

	r.run("add", "-A")
	r.run("commit", "-q", "-m", subject)
}

// service builds the adapter under test over the temporary repository.
func (r *repo) service() *Service {
	r.t.Helper()

	service, err := New(r.root, NewExecRunner(r.root))
	if err != nil {
		r.t.Fatalf("New() error = %v, want nil", err)
	}

	return service
}

func TestIntegrationReadStatus(t *testing.T) {
	r := newRepo(t)
	r.write("normal.txt", "uno\ndos\n")
	r.write("with space.txt", "espacios\n")
	r.write("uni-é.txt", "acentuado\n")
	r.commit("initial commit")

	r.run("mv", "with space.txt", "renamed space.txt")
	r.write("normal.txt", "uno\nDOS\n")
	r.write("uni-é.txt", "acentuado cambiado\n")
	r.write("nuevo/sub.txt", "nuevo\n")

	status, err := r.service().Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}

	if status.Branch != "main" || status.Detached || status.Unborn {
		t.Errorf("status = %+v, want branch main on an attached head", status)
	}

	want := []domain.RelPath{"normal.txt", "nuevo", "renamed space.txt", "uni-é.txt"}
	got := make([]domain.RelPath, 0, len(status.Files))
	for _, file := range status.Files {
		got = append(got, file.Path)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}

	for _, file := range status.Files {
		switch file.Path {
		case "renamed space.txt":
			if file.OrigPath != "with space.txt" || !file.Renamed() {
				t.Errorf("rename = %+v, want the pre-image path", file)
			}
		case "nuevo":
			if !file.Untracked() {
				t.Errorf("nuevo = %+v, want an untracked directory", file)
			}
		}
	}
}

func TestIntegrationReadStatusSeesAnExternalEdit(t *testing.T) {
	r := newRepo(t)
	r.write("normal.txt", "uno\ndos\n")
	r.commit("initial commit")

	service := r.service()

	clean, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if clean.HasChanges() {
		t.Fatalf("files = %v, want a clean tree before the external edit", clean.Files)
	}

	// The edit happens outside the TUI: another process touches the worktree.
	r.write("normal.txt", "uno\nDOS\n")
	r.write("externo.go", "package externo\n")

	changed, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if !changed.HasChanges() {
		t.Fatal("HasChanges() = false, want the external edit to be visible on the next read")
	}
	if clean.Equal(changed) {
		t.Error("Equal() = true, want the snapshots to differ after an external edit")
	}

	want := []domain.RelPath{"externo.go", "normal.txt"}
	got := make([]domain.RelPath, 0, len(changed.Files))
	for _, file := range changed.Files {
		got = append(got, file.Path)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
	if !clean.SameRefs(changed) {
		t.Error("SameRefs() = false, want a worktree edit not to look like a ref change")
	}

	// A second identical read must compare equal, so the poll can skip the re-render.
	again, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if !changed.Equal(again) {
		t.Error("Equal() = false for two reads of the same tree, want the poll to detect no change")
	}
}

func TestIntegrationReadStatusConflict(t *testing.T) {
	r := newRepo(t)
	r.write("conflict.txt", "base\n")
	r.commit("base")
	r.run("checkout", "-q", "-b", "side")
	r.write("conflict.txt", "lado\n")
	r.commit("side")
	r.run("checkout", "-q", "main")
	r.write("conflict.txt", "principal\n")
	r.commit("main")
	r.runQuiet("merge", "side")

	status, err := r.service().Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}

	conflicted := status.Conflicted()
	if len(conflicted) != 1 || conflicted[0].Path != "conflict.txt" {
		t.Fatalf("conflicted = %+v, want conflict.txt", conflicted)
	}
	if conflicted[0].Staged() || conflicted[0].Unstaged() {
		t.Error("a conflicted file must be neither staged nor unstaged")
	}
}

func TestIntegrationReadStatusUnborn(t *testing.T) {
	r := newRepo(t)

	status, err := r.service().Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if !status.Unborn || status.Branch != "main" {
		t.Errorf("status = %+v, want an unborn main branch", status)
	}
}

func TestIntegrationReadDiffWorktreeStagedBinaryAndRename(t *testing.T) {
	r := newRepo(t)
	r.write("code.go", "package main\n\nfunc main() {\n\tprintln(\"hola\")\n\tprintln(\"mundo\")\n}\n")
	r.write("bin.dat", "\x00\x01binario\x00")
	r.write("renamed.txt", "contenido\n")
	r.commit("initial commit")

	r.write("code.go", "package main\n\nfunc main() {\n\tprintln(\"hola\")\n\tprintln(\"MUNDO\")\n}\n")
	r.write("bin.dat", "\x00\x02binario\x00")
	r.run("mv", "renamed.txt", "moved.txt")

	service := r.service()

	worktree, err := service.Diff(context.Background(), domain.DiffRequest{Path: "code.go"})
	if err != nil {
		t.Fatalf("Diff(code.go) error = %v, want nil", err)
	}
	if worktree.Path != "code.go" || worktree.Staged || worktree.IsEmpty() {
		t.Errorf("Diff(code.go) = %+v, want a worktree diff", worktree)
	}
	if worktree.Additions != 1 || worktree.Deletions != 1 {
		t.Errorf("counts = %d/%d, want 1/1", worktree.Additions, worktree.Deletions)
	}

	// A single-path pathspec hides the rename pair, so git answers with the post-image path.
	staged, err := service.Diff(context.Background(), domain.DiffRequest{Path: "moved.txt", Staged: true})
	if err != nil {
		t.Fatalf("Diff(moved.txt) error = %v, want nil", err)
	}
	if staged.Path != "moved.txt" || staged.OrigPath != "" || !staged.Staged {
		t.Errorf("Diff(moved.txt) = %+v, want the staged addition of the new path", staged)
	}
	if staged.Additions != 1 || staged.Deletions != 0 {
		t.Errorf("counts = %d/%d, want 1/0", staged.Additions, staged.Deletions)
	}

	binary, err := service.Diff(context.Background(), domain.DiffRequest{Path: "bin.dat"})
	if err != nil {
		t.Fatalf("Diff(bin.dat) error = %v, want nil", err)
	}
	if !binary.Binary || binary.IsEmpty() {
		t.Errorf("Diff(bin.dat) = %+v, want a binary diff", binary)
	}

	stat, err := service.DiffStat(context.Background(), false)
	if err != nil {
		t.Fatalf("DiffStat() error = %v, want nil", err)
	}
	if stat.IsEmpty() {
		t.Fatal("DiffStat() = empty, want the worktree changes")
	}
	for _, file := range stat.Files {
		if file.Path == "bin.dat" && !file.Binary {
			t.Errorf("bin.dat = %+v, want the binary marker", file)
		}
	}

	stagedStat, err := service.DiffStat(context.Background(), true)
	if err != nil {
		t.Fatalf("DiffStat(staged) error = %v, want nil", err)
	}
	found := false
	for _, file := range stagedStat.Files {
		if file.Path == "moved.txt" && file.OrigPath == "renamed.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("DiffStat(staged) = %+v, want the staged rename", stagedStat.Files)
	}
}

func TestIntegrationReadBranchesAndStashes(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	bare := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}
	r.run("remote", "add", "origin", bare)
	r.run("push", "-q", "-u", "origin", "main")

	r.run("branch", "feature-x")
	r.run("update-ref", "-d", "refs/remotes/origin/feature-x")

	branches, err := r.service().Branches(context.Background())
	if err != nil {
		t.Fatalf("Branches() error = %v, want nil", err)
	}
	if len(branches) != 2 {
		t.Fatalf("branches = %+v, want main and feature-x", branches)
	}

	byName := map[string]domain.Branch{}
	for _, branch := range branches {
		byName[branch.Name] = branch
	}
	if !byName["main"].Current || byName["main"].Upstream != "origin/main" {
		t.Errorf("main = %+v, want the current branch with its upstream", byName["main"])
	}
	if byName["feature-x"].Current {
		t.Errorf("feature-x = %+v, want a non-current branch", byName["feature-x"])
	}

	r.write("base.txt", "uno\ndos\n")
	r.run("stash", "push", "-q")
	r.write("base.txt", "uno\ntres\n")
	r.run("stash", "push", "-q", "-m", "trabajo a medias")

	stashes, err := r.service().Stashes(context.Background())
	if err != nil {
		t.Fatalf("Stashes() error = %v, want nil", err)
	}
	if len(stashes) != 2 {
		t.Fatalf("stashes = %+v, want two entries", stashes)
	}
	if stashes[0].Index != 0 || stashes[0].Message != "trabajo a medias" || stashes[0].Branch != "main" {
		t.Errorf("stash@{0} = %+v, want the custom message", stashes[0])
	}
	if stashes[1].Index != 1 || !strings.Contains(stashes[1].Message, "initial commit") {
		t.Errorf("stash@{1} = %+v, want the default message", stashes[1])
	}
	if stashes[0].Created.IsZero() {
		t.Error("Created is zero, want the parsed date")
	}
}

func TestIntegrationReadCommitsAndShow(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\ndos\n")
	r.commit("initial commit")

	r.write("base.txt", "uno\nDOS\n")
	r.write("bin.dat", "\x00\x01binario\x00")
	r.commit("cambia base y añade binario")

	r.write("tercero.txt", "tres\n")
	r.commit("tercer commit")

	service := r.service()
	ctx := context.Background()

	page, err := service.Commits(ctx, domain.CommitQuery{Ref: "main", Limit: 2})
	if err != nil {
		t.Fatalf("Commits() error = %v, want nil", err)
	}
	if len(page.Commits) != 2 || !page.Truncated {
		t.Fatalf("page = %+v, want two commits and the truncation flag", page)
	}
	if page.Commits[0].Subject != "tercer commit" || page.Commits[0].Date.IsZero() {
		t.Errorf("commit = %+v, want the newest commit with its date", page.Commits[0])
	}

	skipped, err := service.Commits(ctx, domain.CommitQuery{Ref: "main", Limit: 1, Skip: 1})
	if err != nil {
		t.Fatalf("Commits(skip) error = %v, want nil", err)
	}
	if len(skipped.Commits) != 1 || skipped.Commits[0].Hash != page.Commits[1].Hash {
		t.Errorf("skipped = %+v, want the second commit of the page", skipped.Commits)
	}

	byPath, err := service.Commits(ctx, domain.CommitQuery{Ref: "main", Path: "base.txt", Limit: 5})
	if err != nil {
		t.Fatalf("Commits(path) error = %v, want nil", err)
	}
	if len(byPath.Commits) != 2 {
		t.Errorf("Commits(path) = %d commits, want only those touching base.txt", len(byPath.Commits))
	}

	hash := page.Commits[0].Hash
	detail, err := service.Show(ctx, hash)
	if err != nil {
		t.Fatalf("Show() error = %v, want nil", err)
	}
	if detail.Commit.Hash != hash || len(detail.Files) != 1 || detail.Files[0].Path != "tercero.txt" {
		t.Errorf("Show() = %+v, want the commit with its file", detail)
	}
	if additions, deletions := detail.Totals(); additions != 1 || deletions != 0 {
		t.Errorf("Totals() = %d/%d, want 1/0", additions, deletions)
	}

	withBody := page.Commits[1].Hash
	detail, err = service.Show(ctx, withBody)
	if err != nil {
		t.Fatalf("Show(second) error = %v, want nil", err)
	}
	if len(detail.Files) != 2 {
		t.Errorf("files = %+v, want the text and the binary file", diffPaths(detail.Files))
	}

	if _, err := service.Show(ctx, strings.Repeat("0", 40)); err == nil {
		t.Error("Show(unknown hash) error = nil, want a failure")
	}
}

func TestIntegrationReadNotARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git no está disponible: %v", err)
	}

	root, err := domain.NewRepoPath(t.TempDir())
	if err != nil {
		t.Fatalf("NewRepoPath() error = %v, want nil", err)
	}

	service, err := New(root, NewExecRunner(root))
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	ctx := context.Background()
	if _, err := service.Status(ctx); !errors.Is(err, domain.ErrNotRepository) {
		t.Errorf("Status() error = %v, want %v", err, domain.ErrNotRepository)
	}
	if _, err := service.Diff(ctx, domain.DiffRequest{}); !errors.Is(err, domain.ErrNotRepository) {
		t.Errorf("Diff() error = %v, want %v", err, domain.ErrNotRepository)
	}
	// `git diff --numstat --cached` outside a repository falls back to --no-index and fails
	// with a usage error instead of the usual message, so only the failure is guaranteed.
	if _, err := service.DiffStat(ctx, true); err == nil {
		t.Error("DiffStat() error = nil, want a failure outside a repository")
	}
	if _, err := service.Branches(ctx); !errors.Is(err, domain.ErrNotRepository) {
		t.Errorf("Branches() error = %v, want %v", err, domain.ErrNotRepository)
	}
	if _, err := service.Commits(ctx, domain.CommitQuery{}); !errors.Is(err, domain.ErrNotRepository) {
		t.Errorf("Commits() error = %v, want %v", err, domain.ErrNotRepository)
	}
}

func TestIntegrationReadTimeout(t *testing.T) {
	r := newRepo(t)

	runner := NewExecRunner(r.root)
	runner.Timeout = time.Nanosecond

	service, err := New(r.root, runner)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	_, err = service.Status(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Status() error = %v, want %v", err, context.DeadlineExceeded)
	}
}
