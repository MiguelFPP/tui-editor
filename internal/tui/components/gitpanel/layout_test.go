package gitpanel

import (
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

func TestParseLayout(t *testing.T) {
	cases := map[string]Layout{
		"":         LayoutSplit,
		"split":    LayoutSplit,
		"single":   LayoutSingle,
		" SINGLE ": LayoutSingle,
		"otro":     LayoutSplit,
	}

	for raw, want := range cases {
		if got := ParseLayout(raw); got != want {
			t.Errorf("ParseLayout(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestLayoutString(t *testing.T) {
	if got := LayoutSplit.String(); got != "split" {
		t.Errorf("LayoutSplit.String() = %q, want split", got)
	}
	if got := LayoutSingle.String(); got != "single" {
		t.Errorf("LayoutSingle.String() = %q, want single", got)
	}
}

// TestSplitByWidth covers the threshold axis: 100 is the first width that splits.
func TestSplitByWidth(t *testing.T) {
	cases := map[int]bool{40: false, 80: false, 99: false, 100: true, 120: true, 200: true}

	for width, want := range cases {
		m := open(t, gitFake(t))
		m.SetSize(width, 20)

		if got := m.split; got != want {
			t.Errorf("width %d: split = %v, want %v", width, got, want)
		}
	}
}

// TestSplitBySection checks that only Cambios and Diff get the two-column shape.
func TestSplitBySection(t *testing.T) {
	for _, section := range []Section{Changes, Diff, Branches, Stash, History} {
		m := goTo(t, open(t, gitFake(t)), section)
		m.SetSize(120, 20)

		want := section == Changes || section == Diff
		if got := m.split; got != want {
			t.Errorf("section %v: split = %v, want %v", section, got, want)
		}
	}
}

// TestSplitRespectsThePersistedPreference checks that single wins over a wide terminal.
func TestSplitRespectsThePersistedPreference(t *testing.T) {
	m := NewWithLayout(gitFake(t), keymap.Registry{}, LayoutSingle)
	m.SetSize(200, 30)

	if m.split {
		t.Fatal("LayoutSingle must never split, whatever the width")
	}
	if m.listW != 200 || m.previewW != 0 {
		t.Fatalf("listW, previewW = %d, %d, want 200, 0", m.listW, m.previewW)
	}
	if got := m.previewRect.Dx(); got != 0 {
		t.Fatalf("previewRect width = %d, want 0", got)
	}
}

func TestSplitWidthsClampsBothEnds(t *testing.T) {
	cases := []struct {
		panelW       int
		list, review int
	}{
		{panelW: 100, list: 40, review: 60},
		{panelW: 120, list: 48, review: 72},
		{panelW: 200, list: 80, review: 120},
	}

	for _, tc := range cases {
		m := open(t, gitFake(t))
		m.SetSize(tc.panelW, 20)

		if m.listW != tc.list || m.previewW != tc.review {
			t.Errorf("width %d: listW, previewW = %d, %d, want %d, %d",
				tc.panelW, m.listW, m.previewW, tc.list, tc.review)
		}
		if total := m.listW + m.previewW; total != tc.panelW {
			t.Errorf("width %d: listW+previewW = %d, want %d", tc.panelW, total, tc.panelW)
		}
	}
}

// TestSplitWidthsKeepTheMinimums forces the 40 % ratio into both clamps.
func TestSplitWidthsKeepTheMinimums(t *testing.T) {
	m := open(t, gitFake(t))

	if list, review := m.splitWidths(40); list+review != 40 {
		t.Fatalf("splitWidths(40) = %d, %d, want the whole width", list, review)
	}

	m.split = true
	if list, review := m.splitWidths(100); list < minGitList || review < minGitPreview {
		t.Fatalf("splitWidths(100) = %d, %d, want >= %d and >= %d", list, review, minGitList, minGitPreview)
	}
	if list, review := m.splitWidths(60); list < minGitList || review < 0 {
		t.Fatalf("splitWidths(60) = %d, %d, want the list kept and no negative preview", list, review)
	}
}

// TestColumnRectsTileTheBody checks that both columns fill the body without overlapping.
func TestColumnRectsTileTheBody(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetBounds(4, 6, 120, 20)

	if !m.split {
		t.Fatal("120 cells must split")
	}
	if m.listRect.Overlaps(m.previewRect) {
		t.Fatalf("listRect %v overlaps previewRect %v", m.listRect, m.previewRect)
	}
	if got, want := m.listRect.Max.X, m.previewRect.Min.X; got != want {
		t.Errorf("listRect.Max.X = %d, want previewRect.Min.X = %d", got, want)
	}
	if got, want := m.previewRect.Max.X, m.bounds.Min.X+m.width; got != want {
		t.Errorf("previewRect.Max.X = %d, want %d", got, want)
	}
	if got, want := m.listRect.Min.Y, m.bounds.Min.Y+headerRows; got != want {
		t.Errorf("listRect.Min.Y = %d, want %d", got, want)
	}
	if got, want := m.listRect.Dy(), m.bodyHeight(); got != want {
		t.Errorf("listRect height = %d, want %d", got, want)
	}
}

// TestSingleKeepsOneColumnRect checks the fallback geometry below the threshold.
func TestSingleKeepsOneColumnRect(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetBounds(0, 0, 80, 16)

	if m.split {
		t.Fatal("80 cells must stay in one column")
	}
	if got, want := m.listRect.Dx(), m.width; got != want {
		t.Errorf("listRect width = %d, want %d", got, want)
	}
	if got := m.previewRect.Dx(); got != 0 {
		t.Errorf("previewRect width = %d, want 0", got)
	}
}

// TestSplitSurvivesResize walks across the threshold in both directions.
func TestSplitSurvivesResize(t *testing.T) {
	m := open(t, gitFake(t))

	for _, step := range []struct {
		width int
		want  bool
	}{{120, true}, {80, false}, {100, true}, {99, false}} {
		m.SetSize(step.width, 20)
		if m.split != step.want {
			t.Fatalf("width %d: split = %v, want %v", step.width, m.split, step.want)
		}
	}
}
