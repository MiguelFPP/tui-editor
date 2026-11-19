package common

import (
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// sidesRows renders an aligned diff with the given sides and splits it into lines.
func sidesRows(d domain.Diff, width int, sides DiffSides) []string {
	return strings.Split(RenderDiffSplitSides(d, width, true, sides), "\n")
}

// lineDiff is a two-row hunk: a context line plus one changed line.
func lineDiff() domain.Diff {
	return domain.Diff{
		Path: "main.go",
		Hunks: []domain.DiffHunk{{
			Header:   "@@ -1,2 +1,2 @@",
			OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
			Lines: []domain.DiffLine{
				{Kind: domain.Context, Text: "contexto", OldLine: 1, NewLine: 1},
				{Kind: domain.Removed, Text: "vieja", OldLine: 2},
				{Kind: domain.Added, Text: "nueva", NewLine: 2},
			},
		}},
	}
}

// TestRenderDiffSplitSidesMapsByLineNumber takes every cell from the line its number names.
func TestRenderDiffSplitSidesMapsByLineNumber(t *testing.T) {
	body := plainANSI(strings.Join(sidesRows(lineDiff(), 100, DiffSides{Old: "uno\nVIEJA\n", New: "uno\nNUEVA\n"}), "\n"))

	for _, want := range []string{"uno", "VIEJA", "NUEVA"} {
		if !strings.Contains(body, want) {
			t.Errorf("got = %q, want %q", body, want)
		}
	}
	for _, gone := range []string{"contexto", "vieja", "nueva"} {
		if strings.Contains(body, gone) {
			t.Errorf("got = %q, want %q replaced by the file text", body, gone)
		}
	}
}

// TestRenderDiffSplitSidesFallsBackPerSide keeps the side without content on the classic pass.
func TestRenderDiffSplitSidesFallsBackPerSide(t *testing.T) {
	body := plainANSI(strings.Join(sidesRows(lineDiff(), 100, DiffSides{New: "uno\nNUEVA\n"}), "\n"))

	for _, want := range []string{"contexto", "vieja", "NUEVA"} {
		if !strings.Contains(body, want) {
			t.Errorf("got = %q, want %q", body, want)
		}
	}
	if strings.Contains(body, "nueva") {
		t.Errorf("got = %q, want the exact %q instead of the hunk text", body, "NUEVA")
	}

	// A path without a lexer leaves both sides on the fallback path.
	d := domain.Diff{Path: "notas.zzz", Hunks: lineDiff().Hunks}
	sides := DiffSides{Old: unknownHighlightSource, New: unknownHighlightSource}
	if got, want := RenderDiffSplitSides(d, 100, true, sides), RenderDiffSplit(d, 100, true); got != want {
		t.Errorf("got = %q, want %q", got, want)
	}
}

// TestRenderDiffSplitSidesPadsEmptyCells keeps the empty side of an unpaired line blank.
func TestRenderDiffSplitSidesPadsEmptyCells(t *testing.T) {
	d := domain.Diff{
		Path: "main.go",
		Hunks: []domain.DiffHunk{{
			Header:   "@@ -1,2 +1,3 @@",
			OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 3,
			Lines: []domain.DiffLine{
				{Kind: domain.Context, Text: "uno", OldLine: 1, NewLine: 1},
				{Kind: domain.Removed, Text: "dos", OldLine: 2},
				{Kind: domain.Added, Text: "DOS", NewLine: 2},
				{Kind: domain.Added, Text: "tres", NewLine: 3},
			},
		}},
	}

	const width = 100
	rows := sidesRows(d, width, DiffSides{Old: "uno\ndos\n", New: "uno\nDOS\ntres\n"})

	for index, row := range rows {
		if got := len([]rune(plainANSI(row))); got > width+len([]rune(splitSeparator)) {
			t.Errorf("row %d = %q is %d cells, want at most %d", index, plainANSI(row), got, width+len([]rune(splitSeparator)))
		}
	}

	found := false
	for _, row := range rows {
		visible := plainANSI(row)
		if !strings.Contains(visible, "tres") {
			continue
		}

		found = true
		left, _, ok := strings.Cut(visible, splitSeparator)
		if !ok {
			t.Fatalf("row = %q, want the side separator", visible)
		}
		if strings.TrimSpace(left) != "" {
			t.Errorf("left = %q, want it empty for an unpaired line", left)
		}
	}
	if !found {
		t.Error("the added line was not rendered")
	}
}

// TestRenderDiffSplitDelegatesWithNoSides keeps RenderDiffSplit byte for byte the same.
func TestRenderDiffSplitDelegatesWithNoSides(t *testing.T) {
	d := mixedDiff()

	for _, color := range []bool{false, true} {
		got := RenderDiffSplit(d, 100, color)
		want := RenderDiffSplitSides(d, 100, color, DiffSides{})
		if got != want {
			t.Errorf("color %t: got = %q, want %q", color, got, want)
		}
	}
}

// TestRenderDiffSplitSidesKeepsNoNewlineMarker keeps the marker visible with exact content.
func TestRenderDiffSplitSidesKeepsNoNewlineMarker(t *testing.T) {
	d := domain.Diff{Path: "main.go", Hunks: []domain.DiffHunk{{
		Header: "@@ -1,1 +1,1 @@",
		Lines: []domain.DiffLine{
			{Kind: domain.Removed, Text: "vieja", OldLine: 1},
			{Kind: domain.NoNewline},
			{Kind: domain.Added, Text: "nueva", NewLine: 1},
			{Kind: domain.NoNewline},
		},
	}}}

	body := plainANSI(strings.Join(sidesRows(d, 100, DiffSides{Old: "VIEJA\n", New: "NUEVA\n"}), "\n"))
	if !strings.Contains(body, noNewlineText) {
		t.Errorf("got = %q, want %q", body, noNewlineText)
	}
}
