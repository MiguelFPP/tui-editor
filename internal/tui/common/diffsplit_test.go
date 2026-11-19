package common

import (
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// mixedDiff has one hunk with context, a pair of changes and a single added line.
func mixedDiff() domain.Diff {
	return domain.Diff{
		Path: "main.go",
		Hunks: []domain.DiffHunk{{
			Header:   "@@ -1,3 +1,3 @@",
			OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
			Lines: []domain.DiffLine{
				{Kind: domain.Context, Text: "cabecera", OldLine: 1, NewLine: 1},
				{Kind: domain.Removed, Text: "vieja", OldLine: 2},
				{Kind: domain.Added, Text: "nueva", NewLine: 2},
				{Kind: domain.Added, Text: "extra", NewLine: 3},
			},
		}},
	}
}

// rowsOf renders a diff and splits it into lines.
func rowsOf(d domain.Diff, width int) []string {
	return strings.Split(RenderDiffSplit(d, width, false), "\n")
}

func TestRenderDiffSplitPutsOldLeftAndNewRight(t *testing.T) {
	rows := rowsOf(mixedDiff(), 80)

	if len(rows) < 2 {
		t.Fatalf("rows = %d, want the title and the body", len(rows))
	}
	if !strings.Contains(rows[0], "@@ -1,3 +1,3 @@") {
		t.Errorf("title row = %q, want the hunk header", rows[0])
	}

	body := strings.Join(rows[1:], "\n")
	if !strings.Contains(body, "cabecera") {
		t.Errorf("body = %q, want the context line", body)
	}

	// The changed pair shares one row: the removed text on the left of the separator.
	for _, row := range rows[1:] {
		if strings.Contains(row, "vieja") && strings.Contains(row, "nueva") {
			left, right, _ := strings.Cut(row, splitSeparator)
			if !strings.Contains(left, "vieja") || !strings.Contains(right, "nueva") {
				t.Errorf("row = %q, want old on the left and new on the right", row)
			}

			return
		}
	}

	t.Errorf("rows = %q, want one row with the pair aligned", rows)
}

func TestRenderDiffSplitPadsTheShortSide(t *testing.T) {
	rows := rowsOf(mixedDiff(), 80)

	// "extra" has no old line, so the left side of its row is blank.
	for _, row := range rows {
		if !strings.Contains(row, "extra") {
			continue
		}

		left, _, _ := strings.Cut(row, splitSeparator)
		if strings.Contains(left, "extra") {
			t.Errorf("row = %q, want the added line only on the right", row)
		}

		return
	}

	t.Errorf("rows = %q, want the added line", rows)
}

func TestRenderDiffSplitHandlesANewFile(t *testing.T) {
	d := domain.Diff{Path: "nuevo.go", New: true, Hunks: []domain.DiffHunk{{
		Header: "@@ -0,0 +1,2 @@",
		Lines: []domain.DiffLine{
			{Kind: domain.Added, Text: "uno", NewLine: 1},
			{Kind: domain.Added, Text: "dos", NewLine: 2},
		},
	}}}

	rows := rowsOf(d, 80)
	for _, row := range rows[1:] {
		if row == "" {
			continue
		}
		left, right, ok := strings.Cut(row, splitSeparator)
		if !ok {
			continue
		}
		if strings.TrimSpace(left) != "" {
			t.Errorf("left side = %q, want it empty for a new file", left)
		}
		if strings.TrimSpace(right) == "" {
			t.Errorf("right side = %q, want the added line", right)
		}
	}
}

func TestRenderDiffSplitFallsBackForBinaryAndEmpty(t *testing.T) {
	if got := RenderDiffSplit(domain.Diff{Binary: true}, 60, false); !strings.Contains(got, "binaria") {
		t.Errorf("RenderDiffSplit(binary) = %q, want the binary notice", got)
	}
	if got := RenderDiffSplit(domain.Diff{}, 60, false); !strings.Contains(got, "sin cambios") {
		t.Errorf("RenderDiffSplit(empty) = %q, want the empty notice", got)
	}
}

func TestRenderDiffSplitFitsTheWidth(t *testing.T) {
	for _, width := range []int{40, 52, 80, 120} {
		rows := rowsOf(mixedDiff(), width)
		for index, row := range rows {
			if got := len([]rune(row)); got > max(width, 0)+len([]rune(splitSeparator)) {
				t.Errorf("width %d: row %d = %d cells, want it bounded", width, index, got)
			}
		}
	}
}

func TestRenderDiffSplitMarksChanges(t *testing.T) {
	rows := strings.Join(rowsOf(mixedDiff(), 80), "\n")
	if !strings.Contains(rows, "-vieja") {
		t.Errorf("rows = %q, want the removed marker", rows)
	}
	if !strings.Contains(rows, "+nueva") {
		t.Errorf("rows = %q, want the added marker", rows)
	}
}

func TestRenderDiffSplitKeepsTheNoNewlineMarker(t *testing.T) {
	d := domain.Diff{Path: "main.go", Hunks: []domain.DiffHunk{{
		Header: "@@ -1,1 +1,1 @@",
		Lines: []domain.DiffLine{
			{Kind: domain.Removed, Text: "vieja", OldLine: 1},
			{Kind: domain.NoNewline},
			{Kind: domain.Added, Text: "nueva", NewLine: 1},
			{Kind: domain.NoNewline},
		},
	}}}

	rows := strings.Join(rowsOf(d, 80), "\n")
	if !strings.Contains(rows, noNewlineText) {
		t.Errorf("rows = %q, want the newline marker", rows)
	}
}
