package gitpanel

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

// openWithFiles builds the panel with a worktree reader and drains Init.
func openWithFiles(t *testing.T, git *fakes.GitService, files *fakes.FileService) Model {
	t.Helper()

	m := NewWithOptions(git, keymap.Default(), Options{Files: files})
	m.SetSize(72, 14)

	return answers(t, m, m.Init())
}

// selectChange loads the diff of the file under the cursor of the Changes section.
func selectChange(t *testing.T, m Model) Model {
	t.Helper()

	m = goTo(t, m, Changes)

	return settle(t, m, key(tea.KeyEnter))
}

// selectPath moves the Changes cursor to one path and loads its diff.
func selectPath(t *testing.T, m Model, path domain.RelPath) Model {
	t.Helper()

	m = goTo(t, m, Changes)

	for index, file := range m.status.Files {
		if file.Path != path {
			continue
		}

		for m.cursors[Changes] < index {
			m = press(t, m, key(tea.KeyDown))
		}
		for m.cursors[Changes] > index {
			m = press(t, m, key(tea.KeyUp))
		}

		return selectChange(t, m)
	}

	t.Fatalf("no change with path %q", path)

	return m
}

// sideFake answers the status, a one-hunk diff of any path and both sides of it.
func sideFake(t *testing.T) (*fakes.GitService, *fakes.FileService) {
	t.Helper()

	git := gitFake(t)
	git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
		return domain.Diff{
			Path: req.Path, Staged: req.Staged,
			Hunks: []domain.DiffHunk{changeAt(1, "uno", "UNO")},
		}, nil
	}
	git.OnFileAt = func(_ context.Context, rev domain.FileRev, path domain.RelPath) (domain.RevisionContent, error) {
		return domain.RevisionContent{Path: path, Rev: rev, Text: rev.String() + ":" + string(path)}, nil
	}

	files := fakes.NewFileService()
	files.OnReadFile = func(_ context.Context, path domain.RelPath) (domain.FileContent, error) {
		return domain.FileContent{Path: path, Text: "worktree:" + string(path)}, nil
	}

	return git, files
}

// fileArgs returns the arguments of the last recorded call of one method of the file fake.
func fileArgs(t *testing.T, files *fakes.FileService, method string) []any {
	t.Helper()

	calls := files.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method == method {
			return calls[i].Args
		}
	}

	t.Fatalf("no call to %s was recorded", method)

	return nil
}

func TestWorktreeDiffReadsTheIndexAndTheWorktree(t *testing.T) {
	git, files := sideFake(t)
	m := openWithFiles(t, git, files)
	m = selectPath(t, m, "normal.txt")

	if got, want := git.CallCount("FileAt"), 1; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := files.CallCount("ReadFile"), 1; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}

	args := lastArgs(t, git, "FileAt")
	if rev, ok := args[0].(domain.FileRev); !ok || rev != domain.RevIndex {
		t.Errorf("got = %v, want %v", args[0], domain.RevIndex)
	}
	if path, ok := args[1].(domain.RelPath); !ok || path != "normal.txt" {
		t.Errorf("got = %v, want %v", args[1], domain.RelPath("normal.txt"))
	}
	if path, ok := fileArgs(t, files, "ReadFile")[0].(domain.RelPath); !ok || path != "normal.txt" {
		t.Errorf("got = %v, want %v", path, domain.RelPath("normal.txt"))
	}

	if got, want := m.sides.path, domain.RelPath("normal.txt"); got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if m.sides.staged {
		t.Errorf("got = %v, want %v", m.sides.staged, false)
	}
	if got, want := m.sides.old, "index:normal.txt"; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.sides.new, "worktree:normal.txt"; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.sides.oldOK, true; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.sides.newOK, true; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
}

func TestStagedDiffReadsHeadAndTheIndex(t *testing.T) {
	git, files := sideFake(t)
	m := openWithFiles(t, git, files)

	m = settle(t, m, key('d'))
	if !m.staged {
		t.Fatalf("got = %v, want %v", m.staged, true)
	}

	m = selectPath(t, m, "normal.txt")

	if got, want := files.CallCount("ReadFile"), 0; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := git.CallCount("FileAt"), 2; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}

	revs := make([]domain.FileRev, 0, 2)
	for _, call := range git.Calls() {
		if call.Method != "FileAt" {
			continue
		}

		rev, ok := call.Args[0].(domain.FileRev)
		if !ok {
			t.Fatalf("FileAt revision = %T, want domain.FileRev", call.Args[0])
		}

		revs = append(revs, rev)
	}
	if len(revs) != 2 || revs[0] != domain.RevHead || revs[1] != domain.RevIndex {
		t.Errorf("got = %v, want %v", revs, []domain.FileRev{domain.RevHead, domain.RevIndex})
	}

	if got, want := m.sides.old, "HEAD:normal.txt"; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.sides.new, "index:normal.txt"; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if !m.sides.oldOK || !m.sides.newOK {
		t.Errorf("got = %v/%v, want %v/%v", m.sides.oldOK, m.sides.newOK, true, true)
	}
}

func TestRenameDiffReadsThePreImage(t *testing.T) {
	git, files := sideFake(t)
	git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
		return domain.Diff{
			Path: req.Path, OrigPath: "viejo.txt", Staged: req.Staged,
			Hunks: []domain.DiffHunk{changeAt(1, "uno", "UNO")},
		}, nil
	}

	m := openWithFiles(t, git, files)
	m = selectPath(t, m, "nuevo.txt")

	args := lastArgs(t, git, "FileAt")
	if path, ok := args[1].(domain.RelPath); !ok || path != "viejo.txt" {
		t.Errorf("got = %v, want %v", args[1], domain.RelPath("viejo.txt"))
	}
	if path, ok := fileArgs(t, files, "ReadFile")[0].(domain.RelPath); !ok || path != "nuevo.txt" {
		t.Errorf("got = %v, want %v", path, domain.RelPath("nuevo.txt"))
	}

	if got, want := m.sides.oldPath, domain.RelPath("viejo.txt"); got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.sides.newPath, domain.RelPath("nuevo.txt"); got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.sides.old, "index:viejo.txt"; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.sides.new, "worktree:nuevo.txt"; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
}

func TestSidesAreNotReadWithoutAFileService(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	if !m.loaded {
		t.Fatalf("got = %v, want %v", m.loaded, true)
	}
	if got, want := git.CallCount("FileAt"), 0; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if m.sides != (diffSides{}) {
		t.Errorf("got = %v, want %v", m.sides, diffSides{})
	}
	if got, want := m.seqs.sides, 1; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
}

func TestMissingOldSideKeepsTheResultUsable(t *testing.T) {
	git, files := sideFake(t)
	git.OnFileAt = func(context.Context, domain.FileRev, domain.RelPath) (domain.RevisionContent, error) {
		return domain.RevisionContent{}, fmt.Errorf("git: FileAt: %w", domain.ErrNotFound)
	}

	m := openWithFiles(t, git, files)
	m = selectPath(t, m, "normal.txt")

	if got, want := m.sides.oldOK, false; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.sides.newOK, true; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.sides.new, "worktree:normal.txt"; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if m.errs.diff != nil || m.Err() != nil {
		t.Errorf("got = %v/%v, want no error", m.errs.diff, m.Err())
	}
}

func TestBinaryDiffReadsNoSide(t *testing.T) {
	git, files := sideFake(t)
	git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
		return domain.Diff{Path: req.Path, Staged: req.Staged, Binary: true}, nil
	}

	m := openWithFiles(t, git, files)
	m = selectPath(t, m, "normal.txt")

	if got, want := git.CallCount("FileAt")+files.CallCount("ReadFile"), 0; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if m.sides != (diffSides{}) {
		t.Errorf("got = %v, want %v", m.sides, diffSides{})
	}
}

func TestStaleSidesAnswerIsDiscarded(t *testing.T) {
	git, files := sideFake(t)
	m := openWithFiles(t, git, files)
	m = selectPath(t, m, "normal.txt")

	before := m.sides
	stale := sidesLoadedMsg{
		seq: m.seqs.sides - 1, path: m.request.Path, staged: m.request.Staged,
		old: domain.RevisionContent{Path: m.request.Path, Text: "viejo"},
		new: domain.RevisionContent{Path: m.request.Path, Text: "nuevo"},
	}

	next, cmd := m.Update(stale)
	if cmd != nil {
		t.Errorf("got = %v, want %v", cmd(), nil)
	}
	if next.sides != before {
		t.Errorf("got = %v, want %v", next.sides, before)
	}
}

func TestSwitchingFileReloadsTheSides(t *testing.T) {
	git, files := sideFake(t)
	m := openWithFiles(t, git, files)

	m = selectPath(t, m, "normal.txt")
	if got, want := m.sides.new, "worktree:normal.txt"; got != want {
		t.Fatalf("got = %v, want %v", got, want)
	}

	m = selectPath(t, m, "nuevo.txt")

	if got, want := m.sides.path, domain.RelPath("nuevo.txt"); got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.sides.old, "index:nuevo.txt"; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.sides.new, "worktree:nuevo.txt"; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := m.seqs.sides, 2; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
}
