package gitpanel

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
	"github.com/MiguelFPP/tui-editor/internal/tui/common"
)

// highlightedDiffLinesToTest delegates to the shared helper under test.
func highlightedDiffLinesToTest(path string, lines []string) []string {
	return common.HighlightDiffLines(domain.RelPath(path), lines)
}

// goDiffFake answers the diff of any path with Go source lines.
func goDiffFake(t *testing.T) *fakes.GitService {
	t.Helper()

	git := gitFake(t)
	git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
		return domain.Diff{
			Path: req.Path, Staged: req.Staged,
			Hunks: []domain.DiffHunk{
				{
					Header:   "@@ -1,3 +1,3 @@",
					OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
					Lines: []domain.DiffLine{
						{Kind: domain.Context, Text: "package main", OldLine: 1, NewLine: 1},
						{Kind: domain.Context, Text: "", OldLine: 2, NewLine: 2},
						{Kind: domain.Removed, Text: "func uno() {", OldLine: 3},
						{Kind: domain.Added, Text: "func uno() int {", NewLine: 3},
					},
				},
			},
		}, nil
	}

	return git
}

// goDiffLines renders a Go diff body: the gutter, the marker and the source of every line.
func goDiffLines() []string {
	return []string{
		"@@ -1,3 +1,3 @@",
		" 12  12 │  package main",
		" 13  13 │",
		" 14     │ -func uno() {",
		" 14     │ +func uno() int {",
	}
}

// TestHighlightDiffLinesKeepsGutterAndMarkers checks that only the source text is coloured.
func TestHighlightDiffLinesKeepsGutterAndMarkers(t *testing.T) {
	lines := goDiffLines()
	got := highlightedDiffLinesToTest("main.go", lines)

	if len(got) != len(lines) {
		t.Fatalf("lines = %d, want %d", len(got), len(lines))
	}
	if got[0] != lines[0] {
		t.Errorf("hunk title = %q, want it untouched", got[0])
	}

	for index := 1; index < len(lines); index++ {
		visible := plain(got[index])
		want := plain(lines[index])
		if visible != want {
			t.Errorf("line %d = %q, want %q", index, visible, want)
		}
		if strings.HasSuffix(want, "│") {
			continue // an empty source line has nothing to colour
		}
		if !strings.Contains(got[index], "\x1b[") {
			t.Errorf("line %d = %q, want it coloured", index, got[index])
		}
	}
}

// TestHighlightDiffLinesKeepsAMinusMarker verifies the removed line keeps its marker.
func TestHighlightDiffLinesKeepsAMinusMarker(t *testing.T) {
	got := highlightedDiffLinesToTest("main.go", []string{" 14     │ -func uno() {"})

	if len(got) != 1 {
		t.Fatalf("lines = %d, want 1", len(got))
	}
	if visible := plain(got[0]); visible != " 14     │ -func uno() {" {
		t.Errorf("line = %q, want the marker and the gutter kept", visible)
	}
}

// TestHighlightDiffLinesFallsBackWithoutALexer returns the very same lines.
func TestHighlightDiffLinesFallsBackWithoutALexer(t *testing.T) {
	lines := []string{"@@ -1 +1 @@", " 1   1 │ texto cualquiera"}
	got := highlightedDiffLinesToTest("notas.xyz", lines)

	if len(got) != len(lines) {
		t.Fatalf("lines = %d, want %d", len(got), len(lines))
	}
	for index := range lines {
		if got[index] != lines[index] {
			t.Errorf("line %d = %q, want the plain %q", index, got[index], lines[index])
		}
	}
}

// TestHighlightDiffLinesHandlesAnEmptySlice avoids a needless lexer run.
func TestHighlightDiffLinesHandlesAnEmptySlice(t *testing.T) {
	if got := highlightedDiffLinesToTest("main.go", nil); got != nil {
		t.Errorf("lines = %v, want nil", got)
	}
}

// TestPreviewUsesTheHighlightedBody checks that the panel renders the cached colouring. A wide
// preview uses the aligned layout, which colours each side in its own pass.
func TestPreviewUsesTheHighlightedBody(t *testing.T) {
	m := open(t, goDiffFake(t))
	m.SetSize(120, 20)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	if len(m.highlighted) == 0 {
		t.Fatal("the panel did not cache the highlighted preview")
	}

	joined := strings.Join(m.diffRows(), "\n")
	if !strings.Contains(joined, "\x1b[") {
		t.Error("diffRows() carries no ANSI escapes, want the coloured body")
	}
	if visible := plain(joined); !strings.Contains(visible, "package mai") {
		t.Errorf("diffRows() = %q, want the source kept", visible)
	}
	if m.splitDiffWanted() && !strings.Contains(plain(joined), "│") {
		t.Errorf("diffRows() = %q, want the aligned separator", plain(joined))
	}
}

// TestHighlightCacheSurvivesRepeatedRenders keeps the colouring out of View.
func TestHighlightCacheSurvivesRepeatedRenders(t *testing.T) {
	m := open(t, goDiffFake(t))
	m.SetSize(120, 20)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	key := m.highlightKey
	first := m.highlighted
	m.sync()

	if m.highlightKey != key {
		t.Errorf("highlightKey = %q, want %q", m.highlightKey, key)
	}
	if len(m.highlighted) != len(first) {
		t.Fatalf("highlighted = %d lines, want %d", len(m.highlighted), len(first))
	}

	for range 3 {
		_ = m.View()
	}
	if m.highlightKey != key {
		t.Errorf("highlightKey = %q after three renders, want %q", m.highlightKey, key)
	}
}

// TestHighlightCacheInvalidatesOnWidth checks the key reacts to the width.
func TestHighlightCacheInvalidatesOnWidth(t *testing.T) {
	m := open(t, goDiffFake(t))
	m.SetSize(120, 20)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	before := m.highlightKey
	m.SetSize(140, 20)

	if m.highlightKey == before {
		t.Errorf("highlightKey = %q, want a new key after the resize", m.highlightKey)
	}
}

// TestBinaryDiffStaysUncoloured keeps binary diffs readable.
func TestBinaryDiffStaysUncoloured(t *testing.T) {
	git := gitFake(t)
	git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
		return domain.Diff{Path: req.Path, Binary: true}, nil
	}

	m := open(t, git)
	m.SetSize(120, 20)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	joined := plain(strings.Join(m.diffRows(), "\n"))
	if !strings.Contains(joined, "binario") {
		t.Errorf("diffRows() = %q, want the binary notice", joined)
	}
	if strings.Contains(joined, "@@") {
		t.Errorf("diffRows() = %q, want no hunks", joined)
	}
}
