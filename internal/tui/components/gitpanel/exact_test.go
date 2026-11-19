package gitpanel

import (
	"context"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
	"github.com/MiguelFPP/tui-editor/internal/tui/common"
)

// hunkDiff is a two-row hunk of main.go: an unchanged line plus one changed line.
func hunkDiff(unchanged, removed, added string) domain.Diff {
	return domain.Diff{
		Hunks: []domain.DiffHunk{{
			Header:   "@@ -1,2 +1,2 @@",
			OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
			Lines: []domain.DiffLine{
				{Kind: domain.Context, Text: unchanged, OldLine: 1, NewLine: 1},
				{Kind: domain.Removed, Text: removed, OldLine: 2},
				{Kind: domain.Added, Text: added, NewLine: 2},
			},
		}},
	}
}

// panelWithSides answers the diff d of main.go plus the real content of both of its sides.
func panelWithSides(t *testing.T, d domain.Diff, oldSide, newSide string) (*fakes.GitService, *fakes.FileService) {
	t.Helper()

	git := fakes.NewGitService()
	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		return domain.GitStatus{Branch: "main", Files: []domain.GitFileStatus{
			{Path: "main.go", Index: domain.GitUnmodified, Worktree: domain.GitModified},
		}}, nil
	}
	git.OnBranches = func(context.Context) ([]domain.Branch, error) {
		return []domain.Branch{{Name: "main", Current: true}}, nil
	}
	git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
		diff := d
		diff.Path = req.Path
		diff.Staged = req.Staged

		return diff, nil
	}
	git.OnFileAt = func(_ context.Context, rev domain.FileRev, path domain.RelPath) (domain.RevisionContent, error) {
		return domain.RevisionContent{Path: path, Rev: rev, Text: oldSide}, nil
	}

	files := fakes.NewFileService()
	files.OnReadFile = func(_ context.Context, path domain.RelPath) (domain.FileContent, error) {
		return domain.FileContent{Path: path, Text: newSide}, nil
	}

	return git, files
}

// openExact opens the panel wide and loads the diff of main.go with both of its sides.
func openExact(t *testing.T, git *fakes.GitService, files *fakes.FileService) Model {
	t.Helper()

	m := openWithFiles(t, git, files)
	m.SetSize(120, 20)

	return selectPath(t, m, "main.go")
}

// TestExactSidesReachThePreview shows the preview with the real file text.
func TestExactSidesReachThePreview(t *testing.T) {
	git, files := panelWithSides(t, hunkDiff("package hunk", "var hunk = 1", "var hunk = 2"),
		"package real\nvar uno = 1\n", "package real\nvar uno = 2\n")
	m := openExact(t, git, files)

	if !m.splitDiffWanted() {
		t.Skipf("preview width %d is below the threshold", m.previewWidth())
	}

	body := plain(strings.Join(m.diffRows(), "\n"))
	for _, want := range []string{"package real", "var uno = 1", "var uno = 2"} {
		if !strings.Contains(body, want) {
			t.Errorf("diffRows() = %q, want %q", body, want)
		}
	}
	for _, gone := range []string{"package hunk", "var hunk"} {
		if strings.Contains(body, gone) {
			t.Errorf("diffRows() = %q, want %q replaced by the file text", body, gone)
		}
	}
}

// TestHighlightKeyIncludesTheExactMode changes the cache key once the sides arrive.
func TestHighlightKeyIncludesTheExactMode(t *testing.T) {
	git, files := panelWithSides(t, hunkDiff("package hunk", "var hunk = 1", "var hunk = 2"),
		"package real\nvar uno = 1\n", "package real\nvar uno = 2\n")
	m := openExact(t, git, files)

	withSides := m.highlightKey
	m.sides = diffSides{}
	m.sync()

	if m.highlightKey == withSides {
		t.Errorf("highlightKey = %q, want a new key without the exact mode", m.highlightKey)
	}
}

// TestExactHighlightDoesNotChangeVisibleText keeps the visible characters of the exact render.
func TestExactHighlightDoesNotChangeVisibleText(t *testing.T) {
	git, files := panelWithSides(t, hunkDiff("uno", "dos", "DOS"), "uno\ndos\n", "uno\nDOS\n")
	m := openExact(t, git, files)

	if !m.splitDiffWanted() {
		t.Skipf("preview width %d is below the threshold", m.previewWidth())
	}

	got := plain(m.diffBody)
	want := common.RenderDiffSplit(m.diff, m.previewWidth(), false)
	if got != want {
		t.Errorf("got = %q, want %q", got, want)
	}
}

// TestHighlightCacheSkipsRepeatedExactRendering keeps the exact preview cached across renders.
func TestHighlightCacheSkipsRepeatedExactRendering(t *testing.T) {
	git, files := panelWithSides(t, hunkDiff("package hunk", "var hunk = 1", "var hunk = 2"),
		"package real\nvar uno = 1\n", "package real\nvar uno = 2\n")
	m := openExact(t, git, files)

	if len(m.highlighted) == 0 {
		t.Fatal("highlighted = nil, want the cached preview")
	}

	key := m.highlightKey
	for range 3 {
		m.sync()
	}

	if m.highlightKey != key {
		t.Errorf("highlightKey = %q, want %q", m.highlightKey, key)
	}
}
