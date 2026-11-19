package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestGitTabForwardsMotionAndReleaseToThePanel proves the root does not swallow the events the
// Git splitter needs.
func TestGitTabForwardsMotionAndReleaseToThePanel(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = drain(t, m, m.Init())
	m = update(t, m, typed('2'))

	list := m.git.ListBounds()
	edge := m.git.PreviewBounds().Min.X - 1
	body := list.Min.Y

	// Start the drag on the shared border, then move it right.
	m = update(t, m, click(edge, body))
	m = update(t, m, tea.MouseMotionMsg{X: edge + 8, Y: body, Button: tea.MouseLeft})

	grown := m.git.ListBounds().Dx()
	if grown <= list.Dx() {
		t.Fatalf("list width = %d, want more than %d after the drag", grown, list.Dx())
	}

	m = update(t, m, tea.MouseReleaseMsg{X: edge + 8, Y: body, Button: tea.MouseLeft})
	if got := m.git.ListBounds().Dx(); got != grown {
		t.Errorf("list width = %d after the release, want %d", got, grown)
	}
}

// TestFilesTabStillResizesTheTree keeps the tree splitter working.
func TestFilesTabStillResizesTheTree(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = drain(t, m, m.Init())

	before := m.treeWidthNow(m.width)
	y := m.splitterRect.Min.Y + 1
	m = update(t, m, click(m.splitterRect.Min.X, y))
	m = update(t, m, tea.MouseMotionMsg{X: m.splitterRect.Min.X + 10, Y: y, Button: tea.MouseLeft})

	if got := m.treeWidthNow(m.width); got <= before {
		t.Errorf("tree width = %d, want more than %d", got, before)
	}
}

// TestShiftArrowsBelongToTheActiveTab checks the keys reach the right panel.
func TestShiftArrowsBelongToTheActiveTab(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = drain(t, m, m.Init())

	treeBefore := m.treeWidthNow(m.width)
	model, cmd := press(t, m, tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	model = drain(t, model, cmd)

	if got := model.treeWidthNow(model.width); got <= treeBefore {
		t.Errorf("tree width = %d, want more than %d in the Files tab", got, treeBefore)
	}

	// In the Git tab the same key resizes the list and leaves the tree alone.
	model = update(t, model, typed('2'))
	listBefore := model.git.ListBounds().Dx()
	model, cmd = press(t, model, tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	model = drain(t, model, cmd)

	if got := model.git.ListBounds().Dx(); got <= listBefore {
		t.Errorf("git list width = %d, want more than %d", got, listBefore)
	}
}
