package domain

import "testing"

// hunkOf builds one hunk from its lines.
func hunkOf(lines ...DiffLine) DiffHunk {
	return DiffHunk{Header: "@@ -1,2 +1,2 @@", Lines: lines}
}

// sumFilled counts the filled cells of a row list.
func sumFilled(rows []SplitRow) (old, new int) {
	for _, row := range rows {
		if row.Old.Filled {
			old++
		}
		if row.New.Filled {
			new++
		}
	}

	return old, new
}

func TestAlignDiffPairsContextOnBothSides(t *testing.T) {
	aligned := AlignDiff(Diff{Hunks: []DiffHunk{hunkOf(
		DiffLine{Kind: Context, Text: "uno", OldLine: 1, NewLine: 1},
		DiffLine{Kind: Context, Text: "dos", OldLine: 2, NewLine: 2},
	)}})

	if len(aligned) != 1 {
		t.Fatalf("hunks = %d, want 1", len(aligned))
	}

	rows := aligned[0].Rows
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	for index, row := range rows {
		if !row.Old.Filled || !row.New.Filled {
			t.Errorf("row %d = %+v, want both sides filled", index, row)
		}
		if row.Old.Text != row.New.Text {
			t.Errorf("row %d texts differ: %q vs %q", index, row.Old.Text, row.New.Text)
		}
	}
}

func TestAlignDiffPairsChangesByIndex(t *testing.T) {
	aligned := AlignDiff(Diff{Hunks: []DiffHunk{hunkOf(
		DiffLine{Kind: Removed, Text: "vieja", OldLine: 1},
		DiffLine{Kind: Added, Text: "nueva", NewLine: 1},
	)}})

	rows := aligned[0].Rows
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want the pair on one row", len(rows))
	}
	if rows[0].Old.Text != "vieja" || rows[0].New.Text != "nueva" {
		t.Errorf("row = %+v, want the pair aligned", rows[0])
	}
	if rows[0].Old.Kind != Removed || rows[0].New.Kind != Added {
		t.Errorf("kinds = %v/%v, want removed/added", rows[0].Old.Kind, rows[0].New.Kind)
	}
}

func TestAlignDiffPadsTheShortSide(t *testing.T) {
	aligned := AlignDiff(Diff{Hunks: []DiffHunk{hunkOf(
		DiffLine{Kind: Removed, Text: "a", OldLine: 1},
		DiffLine{Kind: Removed, Text: "b", OldLine: 2},
		DiffLine{Kind: Added, Text: "c", NewLine: 1},
	)}})

	rows := aligned[0].Rows
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if !rows[0].Old.Filled || !rows[0].New.Filled {
		t.Errorf("row 0 = %+v, want both sides filled", rows[0])
	}
	if !rows[1].Old.Filled || rows[1].New.Filled {
		t.Errorf("row 1 = %+v, want the right side empty", rows[1])
	}

	old, new := sumFilled(rows)
	if old != 2 || new != 1 {
		t.Errorf("filled = %d/%d, want 2/1", old, new)
	}
}

func TestAlignDiffKeepsASingleAddedFile(t *testing.T) {
	aligned := AlignDiff(Diff{New: true, Hunks: []DiffHunk{hunkOf(
		DiffLine{Kind: Added, Text: "uno", NewLine: 1},
		DiffLine{Kind: Added, Text: "dos", NewLine: 2},
	)}})

	rows := aligned[0].Rows
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	for index, row := range rows {
		if row.Old.Filled {
			t.Errorf("row %d = %+v, want the left side empty for a new file", index, row)
		}
		if !row.New.Filled {
			t.Errorf("row %d = %+v, want the right side filled", index, row)
		}
	}
}

func TestAlignDiffKeepsADeletedFile(t *testing.T) {
	aligned := AlignDiff(Diff{Deleted: true, Hunks: []DiffHunk{hunkOf(
		DiffLine{Kind: Removed, Text: "uno", OldLine: 1},
	)}})

	row := aligned[0].Rows[0]
	if !row.Old.Filled || row.New.Filled {
		t.Errorf("row = %+v, want only the left side", row)
	}
}

func TestAlignDiffKeepsTheNoNewlineMarkerOnItsSide(t *testing.T) {
	aligned := AlignDiff(Diff{Hunks: []DiffHunk{hunkOf(
		DiffLine{Kind: Removed, Text: "vieja", OldLine: 1},
		DiffLine{Kind: NoNewline},
		DiffLine{Kind: Added, Text: "nueva", NewLine: 1},
		DiffLine{Kind: NoNewline},
	)}})

	rows := aligned[0].Rows
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Old.Text != "vieja" || rows[0].New.Text != "nueva" {
		t.Errorf("row 0 = %+v, want the pair aligned", rows[0])
	}

	// Both markers belong to the removed and the added line, so both land on the same row.
	if rows[1].Old.Kind != NoNewline || rows[1].New.Kind != NoNewline {
		t.Errorf("row 1 = %+v, want a marker on each side", rows[1])
	}
}

func TestAlignDiffWalksSeveralBlocks(t *testing.T) {
	aligned := AlignDiff(Diff{Hunks: []DiffHunk{hunkOf(
		DiffLine{Kind: Context, Text: "cabecera", OldLine: 1, NewLine: 1},
		DiffLine{Kind: Removed, Text: "a", OldLine: 2},
		DiffLine{Kind: Added, Text: "b", NewLine: 2},
		DiffLine{Kind: Context, Text: "cola", OldLine: 3, NewLine: 3},
		DiffLine{Kind: Removed, Text: "c", OldLine: 4},
	)}})

	rows := aligned[0].Rows
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	if rows[0].Old.Text != "cabecera" || rows[3].Old.Text != "c" {
		t.Errorf("rows = %+v, want the context and the last change", rows)
	}
	if rows[3].New.Filled {
		t.Errorf("row 3 = %+v, want the right side empty", rows[3])
	}
}

func TestAlignDiffNumbersBothSides(t *testing.T) {
	aligned := AlignDiff(Diff{Hunks: []DiffHunk{hunkOf(
		DiffLine{Kind: Removed, Text: "vieja", OldLine: 7},
		DiffLine{Kind: Added, Text: "nueva", NewLine: 9},
	)}})

	row := aligned[0].Rows[0]
	if row.Old.Number != 7 {
		t.Errorf("old number = %d, want 7", row.Old.Number)
	}
	if row.New.Number != 9 {
		t.Errorf("new number = %d, want 9", row.New.Number)
	}
}

func TestAlignDiffHandlesAnEmptyDiff(t *testing.T) {
	if got := AlignDiff(Diff{}); len(got) != 0 {
		t.Errorf("AlignDiff() = %v, want no hunks", got)
	}
}
