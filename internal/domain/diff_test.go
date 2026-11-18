package domain

import (
	"strings"
	"testing"
)

func TestDiffIsEmpty(t *testing.T) {
	if !(Diff{}).IsEmpty() {
		t.Error("Diff{}.IsEmpty() = false, want true")
	}

	if (Diff{Hunks: []DiffHunk{{Header: "@@ -1 +1 @@"}}}).IsEmpty() {
		t.Error("Diff with a hunk: IsEmpty() = true, want false")
	}

	if (Diff{Binary: true}).IsEmpty() {
		t.Error("binary diff: IsEmpty() = true, want false")
	}
}

func TestDiffLineHoldsTextWithoutPrefixOrNewline(t *testing.T) {
	hunk := DiffHunk{
		Header:   "@@ -1,2 +1,2 @@",
		OldStart: 1,
		OldLines: 2,
		NewStart: 1,
		NewLines: 2,
		Section:  "",
		Lines: []DiffLine{
			{Kind: Context, Text: "línea común", OldLine: 1, NewLine: 1},
			{Kind: Removed, Text: "vieja", OldLine: 2},
			{Kind: Added, Text: "nueva", NewLine: 2},
			{Kind: NoNewline},
		},
	}
	diff := Diff{Path: "main.go", Additions: 1, Deletions: 1, Hunks: []DiffHunk{hunk}}

	if diff.IsEmpty() {
		t.Fatal("IsEmpty() = true, want false")
	}
	if diff.Staged || diff.Binary || diff.Truncated {
		t.Errorf("flags = staged:%v binary:%v truncated:%v, want all false", diff.Staged, diff.Binary, diff.Truncated)
	}

	for _, line := range diff.Hunks[0].Lines {
		if strings.ContainsAny(line.Text, "\n\r") {
			t.Errorf("line %q keeps a newline", line.Text)
		}
	}

	last := diff.Hunks[0].Lines[3]
	if last.Kind != NoNewline {
		t.Errorf("last line kind = %v, want NoNewline", last.Kind)
	}
	if last.OldLine != 0 || last.NewLine != 0 {
		t.Errorf("no-newline line numbers = %d/%d, want 0/0", last.OldLine, last.NewLine)
	}

	removed := diff.Hunks[0].Lines[1]
	if removed.NewLine != 0 || removed.OldLine != 2 {
		t.Errorf("removed line numbers = %d/%d, want 2/0", removed.OldLine, removed.NewLine)
	}

	added := diff.Hunks[0].Lines[2]
	if added.OldLine != 0 || added.NewLine != 2 {
		t.Errorf("added line numbers = %d/%d, want 0/2", added.OldLine, added.NewLine)
	}
}

func TestDiffKindsAreDistinct(t *testing.T) {
	kinds := []DiffLineKind{Context, Added, Removed, NoNewline}
	seen := make(map[DiffLineKind]bool, len(kinds))

	for _, kind := range kinds {
		if seen[kind] {
			t.Fatalf("DiffLineKind(%d) is duplicated", kind)
		}
		seen[kind] = true
	}
}
