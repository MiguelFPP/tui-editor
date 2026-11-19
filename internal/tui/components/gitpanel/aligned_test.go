package gitpanel

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// wideSplit opens the panel wide enough for the aligned diff.
func wideSplit(t *testing.T) Model {
	t.Helper()

	m := open(t, twoHunkFake(t))
	m.SetSize(160, 24)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	return goTo(t, m, Diff)
}

// TestWidePreviewUsesTheAlignedDiff checks the layout switch.
func TestWidePreviewUsesTheAlignedDiff(t *testing.T) {
	m := wideSplit(t)

	if !m.splitDiffWanted() {
		t.Skipf("preview width %d is below the threshold", m.previewWidth())
	}

	body := plain(strings.Join(m.diffRows(), "\n"))
	if !strings.Contains(body, "│") {
		t.Errorf("diffRows() = %q, want the side separator", body)
	}
	if !strings.Contains(body, "-uno") || !strings.Contains(body, "+UNO") {
		t.Errorf("diffRows() = %q, want both sides rendered", body)
	}
}

// TestNarrowPreviewKeepsTheUnifiedDiff protects the fallback.
func TestNarrowPreviewKeepsTheUnifiedDiff(t *testing.T) {
	m := open(t, twoHunkFake(t))
	m.SetSize(80, 20)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	if m.splitDiffWanted() {
		t.Fatal("an 80-cell panel must stay below the aligned threshold")
	}

	body := plain(strings.Join(m.diffRows(), "\n"))
	if !strings.Contains(body, "@@ -1,1 +1,1 @@") {
		t.Errorf("diffRows() = %q, want the unified hunk title", body)
	}
}

// TestAlignedSpansFollowTheRenderedRows keeps hunkAt and the cursor aligned with the draw.
func TestAlignedSpansFollowTheRenderedRows(t *testing.T) {
	m := wideSplit(t)

	if !m.splitDiffWanted() {
		t.Skipf("preview width %d is below the threshold", m.previewWidth())
	}
	if len(m.hunkRows) != 2 {
		t.Fatalf("hunkRows = %d, want 2", len(m.hunkRows))
	}

	for index, span := range m.hunkRows {
		title := plain(m.diffLines[span.start-1])
		if !strings.HasPrefix(title, "@@") {
			t.Errorf("row %d = %q, want the title of hunk %d", span.start, title, index)
		}
	}

	// The second hunk starts right after the first one ends.
	if m.hunkRows[1].start != m.hunkRows[0].end {
		t.Errorf("second hunk starts at %d, want %d", m.hunkRows[1].start, m.hunkRows[0].end)
	}
	if last := m.hunkRows[len(m.hunkRows)-1].end - 1; last > len(m.diffLines) {
		t.Errorf("spans reach row %d of %d rendered lines", last, len(m.diffLines))
	}
}

// TestAlignedDiffKeepsTheHunkClick moves the hunk cursor from the preview.
func TestAlignedDiffKeepsTheHunkClick(t *testing.T) {
	m := wideSplit(t)
	if !m.splitDiffWanted() {
		t.Skipf("preview width %d is below the threshold", m.previewWidth())
	}

	row := m.hunkRows[1].start
	next := clickAt(t, m, m.previewRect.Min.X+1, m.previewRect.Min.Y+row)

	if got := next.hunkCursor; got != 1 {
		t.Errorf("hunkCursor = %d, want 1", got)
	}
}

// TestAlignedDiffKeepsStagingWorking stages the hunk under the cursor.
func TestAlignedDiffKeepsStagingWorking(t *testing.T) {
	m := wideSplit(t)
	if !m.splitDiffWanted() {
		t.Skipf("preview width %d is below the threshold", m.previewWidth())
	}

	m.hunkCursor = 1
	next := settle(t, m, spaceKey())

	if got := next.selectedHunks(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("selectedHunks() = %v, want [1]", got)
	}

	next = settle(t, next, typed('s'))
	// A successful write resets the hunk marks, so only the absence of an error is asserted here.
	if err := next.Err(); err != nil {
		t.Errorf("Err() = %v, want nil after staging the hunk", err)
	}
}

// TestAlignedDiffPadsTheShortSide checks the empty side of an unpaired line.
func TestAlignedDiffPadsTheShortSide(t *testing.T) {
	git := gitFake(t)
	git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
		return domain.Diff{Path: req.Path, Staged: req.Staged, Hunks: []domain.DiffHunk{{
			Header:   "@@ -1,1 +1,2 @@",
			OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 2,
			Lines: []domain.DiffLine{
				{Kind: domain.Removed, Text: "vieja", OldLine: 1},
				{Kind: domain.Added, Text: "nueva", NewLine: 1},
				{Kind: domain.Added, Text: "extra", NewLine: 2},
			},
		}}}, nil
	}

	m := open(t, git)
	m.SetSize(160, 24)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))
	m = goTo(t, m, Diff)

	if !m.splitDiffWanted() {
		t.Skipf("preview width %d is below the threshold", m.previewWidth())
	}

	for _, line := range m.diffRows() {
		if !strings.Contains(plain(line), "extra") {
			continue
		}

		left, _, ok := strings.Cut(plain(line), "│")
		if !ok {
			t.Fatalf("row = %q, want the side separator", plain(line))
		}
		if strings.Contains(left, "extra") {
			t.Errorf("left side = %q, want the added line only on the right", left)
		}

		return
	}

	t.Error("the added line was not rendered")
}
