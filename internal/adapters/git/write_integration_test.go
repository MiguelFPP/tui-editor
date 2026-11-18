//go:build integration

package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// manyLines builds a file whose edits land far enough apart to become two hunks.
func manyLines() string {
	words := []string{
		"uno", "dos", "tres", "cuatro", "cinco", "seis", "siete", "ocho", "nueve", "diez",
		"once", "doce", "trece", "catorce", "quince", "dieciseis", "diecisiete", "dieciocho",
		"diecinueve", "veinte",
	}

	return strings.Join(words, "\n") + "\n"
}

// indexBlob returns the exact bytes of one path in the index.
func indexBlob(t *testing.T, r *repo, path string) string {
	t.Helper()

	out, err := exec.Command("git", "-C", r.dir, "show", ":"+path).Output()
	if err != nil {
		t.Fatalf("git show :%s: %v", path, err)
	}

	return string(out)
}

// stagedDiff reads the staged diff of one path through the adapter under test.
func stagedDiff(t *testing.T, service *Service, path domain.RelPath) domain.Diff {
	t.Helper()

	diff, err := service.Diff(context.Background(), domain.DiffRequest{Path: path, Staged: true})
	if err != nil {
		t.Fatalf("Diff(staged %s) error = %v, want nil", path, err)
	}

	return diff
}

// worktreeDiff reads the worktree diff of one path through the adapter under test.
func worktreeDiff(t *testing.T, service *Service, path domain.RelPath) domain.Diff {
	t.Helper()

	diff, err := service.Diff(context.Background(), domain.DiffRequest{Path: path})
	if err != nil {
		t.Fatalf("Diff(%s) error = %v, want nil", path, err)
	}

	return diff
}

// textsOf lists the text of every line of one kind in a diff.
func textsOf(diff domain.Diff, kind domain.DiffLineKind) []string {
	var texts []string

	for _, hunk := range diff.Hunks {
		for _, line := range hunk.Lines {
			if line.Kind == kind {
				texts = append(texts, line.Text)
			}
		}
	}

	return texts
}

func TestIntegrationWriteStageAndUnstageFiles(t *testing.T) {
	r := newRepo(t)
	r.write("mod.txt", "uno\ndos\n")
	r.write("del.txt", "adios\n")
	r.commit("initial commit")

	r.write("mod.txt", "uno\nDOS\n")
	r.write("nuevo.txt", "recien creado\n")
	if err := os.Remove(filepath.Join(r.dir, "del.txt")); err != nil {
		t.Fatalf("Remove(del.txt) error = %v, want nil", err)
	}

	service := r.service()
	ctx := context.Background()

	if err := service.Stage(ctx, []domain.RelPath{"nuevo.txt", "mod.txt", "del.txt"}); err != nil {
		t.Fatalf("Stage() error = %v, want nil", err)
	}

	staged, err := service.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	for _, file := range staged.Files {
		if !file.Staged() {
			t.Errorf("%s is not staged: %+v", file.Path, file)
		}
	}

	if err := service.Unstage(ctx, []domain.RelPath{"nuevo.txt", "mod.txt", "del.txt"}); err != nil {
		t.Fatalf("Unstage() error = %v, want nil", err)
	}

	worktree, err := service.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	for _, file := range worktree.Files {
		if file.Staged() {
			t.Errorf("%s is still staged after Unstage: %+v", file.Path, file)
		}
	}
	if staged, err := service.DiffStat(ctx, true); err != nil || !staged.IsEmpty() {
		t.Errorf("DiffStat(staged) = %+v, %v, want an empty index", staged, err)
	}
}

func TestIntegrationWriteStageHunks(t *testing.T) {
	r := newRepo(t)
	r.write("code.txt", manyLines())
	r.commit("initial commit")

	r.write("code.txt", strings.Replace(strings.Replace(manyLines(), "dos", "DOS", 1), "diecinueve", "DIECINUEVE", 1))

	service := r.service()
	ctx := context.Background()

	diff := worktreeDiff(t, service, "code.txt")
	if len(diff.Hunks) != 2 {
		t.Fatalf("hunks = %d, want 2: the edits must be far apart", len(diff.Hunks))
	}

	if err := service.StageHunks(ctx, diff, []int{0}); err != nil {
		t.Fatalf("StageHunks() error = %v, want nil", err)
	}

	staged, err := service.DiffStat(ctx, true)
	if err != nil {
		t.Fatalf("DiffStat(staged) error = %v, want nil", err)
	}
	if additions, deletions := staged.Additions, staged.Deletions; additions != 1 || deletions != 1 {
		t.Errorf("staged counts = %d/%d, want 1/1", additions, deletions)
	}

	pending := worktreeDiff(t, service, "code.txt")
	if len(pending.Hunks) != 1 {
		t.Fatalf("worktree hunks = %d, want only the second change pending", len(pending.Hunks))
	}
	if got := textsOf(pending, domain.Added); !slices.Equal(got, []string{"DIECINUEVE"}) {
		t.Errorf("pending additions = %q, want only the second change", got)
	}
	if got := textsOf(pending, domain.Removed); !slices.Equal(got, []string{"diecinueve"}) {
		t.Errorf("pending removals = %q, want only the second change", got)
	}

	if err := service.StageHunks(ctx, pending, []int{0}); err != nil {
		t.Fatalf("StageHunks(second) error = %v, want nil", err)
	}
	if remaining := worktreeDiff(t, service, "code.txt"); !remaining.IsEmpty() {
		t.Errorf("worktree diff = %+v, want every change staged", remaining)
	}
	if blob := indexBlob(t, r, "code.txt"); !strings.Contains(blob, "DOS") || !strings.Contains(blob, "DIECINUEVE") {
		t.Errorf("index content =\n%s\nwant both changes", blob)
	}
}

func TestIntegrationWriteUnstageHunks(t *testing.T) {
	r := newRepo(t)
	r.write("code.txt", manyLines())
	r.commit("initial commit")

	r.write("code.txt", strings.Replace(strings.Replace(manyLines(), "dos", "DOS", 1), "diecinueve", "DIECINUEVE", 1))
	r.run("add", "--", "code.txt")

	service := r.service()
	ctx := context.Background()

	diff := stagedDiff(t, service, "code.txt")
	if len(diff.Hunks) != 2 {
		t.Fatalf("staged hunks = %d, want 2", len(diff.Hunks))
	}

	if err := service.UnstageHunks(ctx, diff, []int{0}); err != nil {
		t.Fatalf("UnstageHunks() error = %v, want nil", err)
	}

	remaining := stagedDiff(t, service, "code.txt")
	if len(remaining.Hunks) != 1 {
		t.Fatalf("staged hunks = %d, want only the second change left", len(remaining.Hunks))
	}
	if got := textsOf(remaining, domain.Added); !slices.Equal(got, []string{"DIECINUEVE"}) {
		t.Errorf("staged additions = %q, want only the second change", got)
	}

	pending := worktreeDiff(t, service, "code.txt")
	if len(pending.Hunks) != 1 {
		t.Fatalf("worktree hunks = %d, want the unstaged change back", len(pending.Hunks))
	}
	if got := textsOf(pending, domain.Added); !slices.Equal(got, []string{"DOS"}) {
		t.Errorf("worktree additions = %q, want only the first change", got)
	}
	if blob := indexBlob(t, r, "code.txt"); !strings.Contains(blob, "DIECINUEVE") || strings.Contains(blob, "DOS") {
		t.Errorf("index content =\n%s\nwant only the second change staged", blob)
	}
}

func TestIntegrationWriteHunksOfNewAndDeletedFiles(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "base\n")
	r.commit("initial commit")

	r.write("nuevo.txt", "uno\ndos\n")
	r.run("add", "--", "nuevo.txt")

	if err := os.Remove(filepath.Join(r.dir, "base.txt")); err != nil {
		t.Fatalf("Remove(base.txt) error = %v, want nil", err)
	}
	r.run("add", "--", "base.txt")

	service := r.service()
	ctx := context.Background()

	added := stagedDiff(t, service, "nuevo.txt")
	if !added.New || len(added.Hunks) != 1 {
		t.Fatalf("staged diff = %+v, want a new file with one hunk", added)
	}
	if err := service.UnstageHunks(ctx, added, []int{0}); err != nil {
		t.Fatalf("UnstageHunks(new) error = %v, want nil", err)
	}
	if diff := stagedDiff(t, service, "nuevo.txt"); !diff.IsEmpty() {
		t.Errorf("staged diff = %+v, want the new file out of the index", diff)
	}

	removed := stagedDiff(t, service, "base.txt")
	if !removed.Deleted || len(removed.Hunks) != 1 {
		t.Fatalf("staged diff = %+v, want a deleted file with one hunk", removed)
	}
	if err := service.UnstageHunks(ctx, removed, []int{0}); err != nil {
		t.Fatalf("UnstageHunks(deleted) error = %v, want nil", err)
	}
	if diff := stagedDiff(t, service, "base.txt"); !diff.IsEmpty() {
		t.Errorf("staged diff = %+v, want the deletion out of the index", diff)
	}
	if err := service.StageHunks(ctx, removed, []int{0}); err != nil {
		t.Fatalf("StageHunks(deleted) error = %v, want nil", err)
	}
	if diff := stagedDiff(t, service, "base.txt"); !diff.Deleted {
		t.Errorf("staged diff = %+v, want the deletion staged again", diff)
	}
}

func TestIntegrationWriteHunksKeepCRLFAndMissingNewline(t *testing.T) {
	r := newRepo(t)
	r.write("crlf.txt", "a\r\nb\r\nc\r\n")
	r.write("noeol.txt", "uno\ndos\ntres")
	r.commit("initial commit")

	r.write("crlf.txt", "a\r\nB\r\nc\r\n")
	r.write("noeol.txt", "uno\ndos\nTRES")

	service := r.service()
	ctx := context.Background()

	for _, path := range []domain.RelPath{"crlf.txt", "noeol.txt"} {
		diff := worktreeDiff(t, service, path)
		if len(diff.Hunks) != 1 {
			t.Fatalf("%s: hunks = %d, want 1", path, len(diff.Hunks))
		}
		if err := service.StageHunks(ctx, diff, []int{0}); err != nil {
			t.Fatalf("StageHunks(%s) error = %v, want nil", path, err)
		}
	}

	if blob := indexBlob(t, r, "crlf.txt"); blob != "a\r\nB\r\nc\r\n" {
		t.Errorf("index crlf.txt = %q, want the CRLF line endings preserved", blob)
	}
	if blob := indexBlob(t, r, "noeol.txt"); blob != "uno\ndos\nTRES" {
		t.Errorf("index noeol.txt = %q, want the missing final newline preserved", blob)
	}
}

func TestIntegrationWriteHunksOfAPathWithSpecialNames(t *testing.T) {
	r := newRepo(t)
	r.write("with space.txt", "uno\ndos\n")
	r.write("uni-é.txt", "uno\ndos\n")
	r.commit("initial commit")

	r.write("with space.txt", "uno\nDOS\n")
	r.write("uni-é.txt", "uno\nDOS\n")

	service := r.service()
	ctx := context.Background()

	for _, path := range []domain.RelPath{"with space.txt", "uni-é.txt"} {
		diff := worktreeDiff(t, service, path)
		if err := service.StageHunks(ctx, diff, []int{0}); err != nil {
			t.Fatalf("StageHunks(%q) error = %v, want nil", path, err)
		}
		if diff := stagedDiff(t, service, path); diff.IsEmpty() {
			t.Errorf("staged diff of %q = empty, want the staged change", path)
		}
	}
}

func TestIntegrationWriteCommitMultilineMessage(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")

	r.write("base.txt", "uno\ndos\n")

	service := r.service()
	ctx := context.Background()

	if err := service.Commit(ctx, "solo los cambios"); !errors.Is(err, domain.ErrNoChanges) {
		t.Fatalf("Commit() error = %v, want %v", err, domain.ErrNoChanges)
	}

	if err := service.Stage(ctx, []domain.RelPath{"base.txt"}); err != nil {
		t.Fatalf("Stage() error = %v, want nil", err)
	}
	if err := service.Commit(ctx, "titulo\r\n\r\ncuerpo con ñ\r\n\r\n"); err != nil {
		t.Fatalf("Commit() error = %v, want nil", err)
	}

	body := r.run("log", "-1", "--format=%B")
	if !strings.Contains(body, "titulo\n\ncuerpo con ñ") {
		t.Errorf("body = %q, want the normalized message", body)
	}
	if strings.Contains(body, "\r") {
		t.Errorf("body = %q, want no carriage return", body)
	}
	if status, err := service.Status(ctx); err != nil || status.HasChanges() {
		t.Errorf("Status() = %+v, %v, want a clean tree after the commit", status, err)
	}

	if err := service.Commit(ctx, "nada que hacer"); !errors.Is(err, domain.ErrNoChanges) {
		t.Errorf("Commit() with an empty index error = %v, want %v", err, domain.ErrNoChanges)
	}
	if got := r.run("rev-list", "--count", "HEAD"); got != "2" {
		t.Errorf("commit count = %s, want 2: no extra commits", got)
	}
}

func TestIntegrationWriteCommitOnAnUnbornBranch(t *testing.T) {
	r := newRepo(t)
	r.write("primero.txt", "hola\n")

	service := r.service()
	ctx := context.Background()

	if err := service.Stage(ctx, []domain.RelPath{"primero.txt"}); err != nil {
		t.Fatalf("Stage() error = %v, want nil", err)
	}
	if err := service.Unstage(ctx, []domain.RelPath{"primero.txt"}); err != nil {
		t.Fatalf("Unstage() on an unborn branch error = %v, want nil", err)
	}
	if status, err := service.Status(ctx); err != nil || len(status.Unstaged()) != 1 {
		t.Fatalf("Status() = %+v, %v, want the file back to untracked", status, err)
	}

	if err := service.Stage(ctx, []domain.RelPath{"primero.txt"}); err != nil {
		t.Fatalf("Stage() error = %v, want nil", err)
	}
	if err := service.Commit(ctx, "primer commit"); err != nil {
		t.Fatalf("Commit() error = %v, want nil", err)
	}
	if got := r.run("rev-list", "--count", "HEAD"); got != "1" {
		t.Errorf("commit count = %s, want 1", got)
	}
}

func TestIntegrationWriteCommitRespectsAFailingHook(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")
	r.write("base.txt", "uno\ndos\n")

	hook := filepath.Join(r.dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'revisión fallida' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(pre-commit) error = %v, want nil", err)
	}

	service := r.service()
	ctx := context.Background()

	if err := service.Stage(ctx, []domain.RelPath{"base.txt"}); err != nil {
		t.Fatalf("Stage() error = %v, want nil", err)
	}

	err := service.Commit(ctx, "no debe entrar")
	if err == nil {
		t.Fatal("Commit() error = nil, want the hook failure")
	}
	if errors.Is(err, domain.ErrNoChanges) {
		t.Errorf("Commit() error = %v, want the hook failure, not %v", err, domain.ErrNoChanges)
	}
	if got := r.run("rev-list", "--count", "HEAD"); got != "1" {
		t.Errorf("commit count = %s, want 1: the hook blocked the commit", got)
	}
	if status, err := service.Status(ctx); err != nil || len(status.Staged()) != 1 {
		t.Errorf("Status() = %+v, %v, want the change still staged", status, err)
	}
}

func TestIntegrationWriteReportsALockedIndex(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")
	r.write("base.txt", "uno\ndos\n")

	lock := filepath.Join(r.dir, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(index.lock) error = %v, want nil", err)
	}

	service := r.service()
	ctx := context.Background()

	if err := service.Stage(ctx, []domain.RelPath{"base.txt"}); !errors.Is(err, domain.ErrGitIndexLocked) {
		t.Fatalf("Stage() error = %v, want %v", err, domain.ErrGitIndexLocked)
	}

	if err := os.Remove(lock); err != nil {
		t.Fatalf("Remove(index.lock) error = %v, want nil", err)
	}
	if err := service.Stage(ctx, []domain.RelPath{"base.txt"}); err != nil {
		t.Fatalf("Stage() after removing the lock error = %v, want nil", err)
	}
}

func TestIntegrationWriteSerialisesConcurrentStages(t *testing.T) {
	const writers = 8

	r := newRepo(t)
	r.write("base.txt", "base\n")
	r.commit("initial commit")

	paths := make([]domain.RelPath, writers)
	for i := range paths {
		paths[i] = domain.RelPath(fmt.Sprintf("file-%d.txt", i))
		r.write(string(paths[i]), fmt.Sprintf("contenido %d\n", i))
	}

	service := r.service()
	ctx := context.Background()

	failures := make(chan error, writers)

	var wg sync.WaitGroup
	for _, path := range paths {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := service.Stage(ctx, []domain.RelPath{path}); err != nil {
				failures <- fmt.Errorf("Stage(%q): %w", path, err)
			}
		}()
	}
	wg.Wait()
	close(failures)

	for err := range failures {
		t.Errorf("concurrent write error = %v, want nil", err)
	}

	staged, err := service.DiffStat(ctx, true)
	if err != nil {
		t.Fatalf("DiffStat(staged) error = %v, want nil", err)
	}
	got := make([]domain.RelPath, 0, len(staged.Files))
	for _, file := range staged.Files {
		got = append(got, file.Path)
	}
	slices.Sort(got)
	want := slices.Clone(paths)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("staged files = %v, want %v", got, want)
	}
}

func TestIntegrationWriteCancellation(t *testing.T) {
	r := newRepo(t)
	r.write("base.txt", "uno\n")
	r.commit("initial commit")
	r.write("base.txt", "uno\ndos\n")

	service := r.service()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := service.Stage(ctx, []domain.RelPath{"base.txt"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stage() error = %v, want %v", err, context.Canceled)
	}
	if status, err := service.Status(context.Background()); err != nil || len(status.Staged()) != 0 {
		t.Errorf("Status() = %+v, %v, want nothing staged after the cancellation", status, err)
	}
	if err := service.Commit(ctx, "no debe entrar"); !errors.Is(err, context.Canceled) {
		t.Errorf("Commit() error = %v, want %v", err, context.Canceled)
	}
}
