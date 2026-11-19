package gitpanel

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// splitDiff opens the panel at 120x20 with a two-hunk diff loaded and the Diff section active.
func splitDiff(t *testing.T) Model {
	t.Helper()

	m := open(t, twoHunkFake(t))
	m.SetSize(120, 20)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	return goTo(t, m, Diff)
}

// listPoint and previewPoint give one point inside each column of a panel at the origin.
func listPoint(m Model, row int) (int, int) {
	return m.listRect.Min.X + 1, m.listRect.Min.Y + row
}

func previewPoint(m Model, row int) (int, int) {
	return m.previewRect.Min.X + 1, m.previewRect.Min.Y + row
}

// wheelAt sends one wheel event to a point of the body.
func wheelAt(t *testing.T, m Model, x, y int, up bool) Model {
	t.Helper()

	button := tea.MouseWheelDown
	if up {
		button = tea.MouseWheelUp
	}

	next, _ := m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: button})

	return next
}

// TestClickOnTheListMovesOnlyTheListCursor keeps the hunk cursor untouched.
func TestClickOnTheListMovesOnlyTheListCursor(t *testing.T) {
	m := splitDiff(t)
	m.hunkCursor = 1

	x, y := listPoint(m, 2)
	next := clickAt(t, m, x, y)

	if !next.split {
		t.Fatal("the panel must stay split")
	}
	if got := next.cursors[Changes]; got != 2 {
		t.Errorf("cursors[Changes] = %d, want 2", got)
	}
	if got := next.hunkCursor; got != 1 {
		t.Errorf("hunkCursor = %d, want 1 untouched by the list", got)
	}
}

// TestClickOnThePreviewMovesOnlyTheHunkCursor protects the selected file.
func TestClickOnThePreviewMovesOnlyTheHunkCursor(t *testing.T) {
	m := splitDiff(t)
	m.cursors[Changes] = 1
	m.hunkCursor = 0

	before, ok := m.selectedFile()
	if !ok {
		t.Fatal("the fake must provide a selected file")
	}

	// The second hunk starts at row 4: header row, two lines of the first hunk and its title.
	x, y := previewPoint(m, m.hunkRows[1].start)
	next := clickAt(t, m, x, y)

	if got := next.hunkCursor; got != 1 {
		t.Errorf("hunkCursor = %d, want 1", got)
	}
	if got := next.cursors[Changes]; got != 1 {
		t.Errorf("cursors[Changes] = %d, want 1 untouched by the preview", got)
	}

	after, ok := next.selectedFile()
	if !ok || after.Path != before.Path {
		t.Errorf("selected file = %v, want %v", after.Path, before.Path)
	}
}

// TestClickOnThePreviewHeaderKeepsTheHunk returns no hunk for the title row.
func TestClickOnThePreviewHeaderKeepsTheHunk(t *testing.T) {
	m := splitDiff(t)
	m.hunkCursor = 1

	x, y := previewPoint(m, 0)
	next := clickAt(t, m, x, y)

	if got := next.hunkCursor; got != 1 {
		t.Errorf("hunkCursor = %d, want 1 after clicking the header", got)
	}
}

// TestClickOutsideBothColumnsChangesNothing covers the frame and the empty space.
func TestClickOutsideBothColumnsChangesNothing(t *testing.T) {
	m := splitDiff(t)
	m.cursors[Changes] = 2
	m.hunkCursor = 1

	for _, point := range [][2]int{
		{0, 0},
		{1, m.listRect.Min.Y - 1},
		{m.listRect.Max.X, m.listRect.Min.Y},
		{m.listRect.Max.X - 1, m.listRect.Max.Y},
	} {
		next := clickAt(t, m, point[0], point[1])
		if next.cursors[Changes] != 2 || next.hunkCursor != 1 {
			t.Errorf("click at %v changed the cursors to %d, %d",
				point, next.cursors[Changes], next.hunkCursor)
		}
	}
}

// TestWheelScrollsTheColumnUnderThePointer keeps both offsets independent.
func TestWheelScrollsTheColumnUnderThePointer(t *testing.T) {
	m := open(t, twoHunkFake(t))
	m.SetSize(120, 8)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))
	m = goTo(t, m, Diff)

	if !m.split {
		t.Fatal("the panel must split")
	}

	x, y := previewPoint(m, 1)
	m = wheelAt(t, m, x, y, false)
	if m.scrollPreview == 0 {
		t.Error("the wheel on the preview must scroll it")
	}
	if m.scrollList != 0 {
		t.Errorf("scrollList = %d, want 0 untouched", m.scrollList)
	}

	preview := m.scrollPreview
	lx, ly := listPoint(m, 1)
	m = wheelAt(t, m, lx, ly, true)
	if m.scrollPreview != preview {
		t.Errorf("scrollPreview = %d, want %d untouched", m.scrollPreview, preview)
	}
}

// TestWheelOutsideTheColumnsIsIgnored protects the offsets from the frame.
func TestWheelOutsideTheColumnsIsIgnored(t *testing.T) {
	m := splitDiff(t)
	m.scrollList, m.scrollPreview = 1, 1

	next := wheelAt(t, m, 0, 0, false)
	if next.scrollList != 1 || next.scrollPreview != 1 {
		t.Errorf("offsets = %d, %d, want 1, 1", next.scrollList, next.scrollPreview)
	}
}

// TestSingleModeWheelKeepsTheSectionOffset keeps the pre-split behaviour below the threshold.
func TestSingleModeWheelKeepsTheSectionOffset(t *testing.T) {
	m := open(t, twoHunkFake(t))
	m.SetSize(72, 8)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))
	m = goTo(t, m, Diff)

	if m.split {
		t.Fatal("72 cells must stay in one column")
	}

	m = wheelAt(t, m, 10, bodyRowY(0, 1), false)
	if m.offsets[Diff] == 0 {
		t.Error("the wheel must scroll the active section in single mode")
	}
	if m.scrollPreview != 0 {
		t.Errorf("scrollPreview = %d, want the column offsets unused", m.scrollPreview)
	}
}

// TestKeyboardKeepsTheListColumnVisible follows the cursor past the viewport.
func TestKeyboardKeepsTheListColumnVisible(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(120, 8)
	m.setStatusForTest(files(12))

	if !m.split {
		t.Fatal("the panel must split")
	}

	for range 8 {
		m = press(t, m, key(tea.KeyDown))
	}

	cursor := m.cursors[Changes]
	if m.scrollList == 0 && cursor >= m.bodyHeight() {
		t.Errorf("scrollList = 0, want the list to follow the cursor %d", cursor)
	}
	if last := m.scrollList + m.bodyHeight(); cursor >= last {
		t.Errorf("cursor %d is outside the window %d..%d", cursor, m.scrollList, last)
	}
}

// setStatusForTest replaces the status without going through a command.
func (m *Model) setStatusForTest(status domain.GitStatus) {
	m.status = status
	m.sync()
}

// files builds a status with n changed files.
func files(n int) domain.GitStatus {
	status := status()
	status.Files = nil
	for i := range n {
		status.Files = append(status.Files, domain.GitFileStatus{
			Path:     domain.RelPath("archivo_" + string(rune('a'+i)) + ".go"),
			Index:    domain.GitModified,
			Worktree: domain.GitModified,
		})
	}

	return status
}
