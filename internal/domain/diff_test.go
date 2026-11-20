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

func TestDiffEqual(t *testing.T) {
	base := Diff{
		Path:      "main.go",
		Additions: 1,
		Deletions: 1,
		Hunks: []DiffHunk{{
			Header: "@@ -1,2 +1,2 @@", OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
			Lines: []DiffLine{
				{Kind: Context, Text: "uno", OldLine: 1, NewLine: 1},
				{Kind: Removed, Text: "dos", OldLine: 2},
				{Kind: Added, Text: "DOS", NewLine: 2},
			},
		}},
	}

	if !base.Equal(base) {
		t.Error("Equal(itself) = false, want true")
	}

	cases := []struct {
		name  string
		other Diff
	}{
		{name: "other path", other: func() Diff { d := base; d.Path = "otro.go"; return d }()},
		{name: "other orig path", other: func() Diff { d := base; d.OrigPath = "viejo.go"; return d }()},
		{name: "other staged", other: func() Diff { d := base; d.Staged = true; return d }()},
		{name: "binary", other: func() Diff { d := base; d.Binary = true; return d }()},
		{name: "truncated", other: func() Diff { d := base; d.Truncated = true; return d }()},
		{name: "new file", other: func() Diff { d := base; d.New = true; return d }()},
		{name: "deleted file", other: func() Diff { d := base; d.Deleted = true; return d }()},
		{name: "other additions", other: func() Diff { d := base; d.Additions = 2; return d }()},
		{name: "other deletions", other: func() Diff { d := base; d.Deletions = 0; return d }()},
		{name: "no hunks", other: Diff{Path: "main.go", Additions: 1, Deletions: 1}},
		{name: "other header", other: func() Diff {
			d := base
			d.Hunks = []DiffHunk{{
				Header: "@@ -5,2 +5,2 @@", OldStart: 5, OldLines: 2, NewStart: 5, NewLines: 2,
				Lines: base.Hunks[0].Lines,
			}}

			return d
		}()},
		{name: "other section", other: func() Diff {
			d := base
			d.Hunks = []DiffHunk{{
				Header: base.Hunks[0].Header, Section: "func main()", OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
				Lines: base.Hunks[0].Lines,
			}}

			return d
		}()},
		{name: "fewer lines", other: func() Diff {
			d := base
			d.Hunks = []DiffHunk{{Header: base.Hunks[0].Header, Lines: base.Hunks[0].Lines[:2]}}

			return d
		}()},
		{name: "other line text", other: func() Diff {
			d := base
			lines := append([]DiffLine(nil), base.Hunks[0].Lines...)
			lines[2].Text = "OTRA"
			d.Hunks = []DiffHunk{{Header: base.Hunks[0].Header, Lines: lines}}

			return d
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if base.Equal(tc.other) {
				t.Error("Equal() = true, want the diffs to differ")
			}
		})
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
