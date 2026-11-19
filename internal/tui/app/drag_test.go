package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

// dragWidth and dragHeight are the terminal the drag tests lay out against.
const (
	dragWidth  = 120
	dragHeight = 40
)

// splitterRow is a body row that crosses the shared border.
const splitterRow = headerRows + 1

// motion is one drag motion with the left button held.
func motion(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// motionWithoutButton is a bare motion, as the cell-motion protocol may report it.
func motionWithoutButton(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{X: x, Y: y}
}

// release is the end of a drag.
func release(x, y int) tea.MouseReleaseMsg {
	return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// pressMsg feeds one mouse message and returns the model with its command still to run.
func pressMsg(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want app.Model", next)
	}

	return model, cmd
}

// startDrag clicks the shared border and returns the model holding the handle.
func startDrag(t *testing.T, m Model) Model {
	t.Helper()

	model, cmd := pressMsg(t, m, click(m.splitterRect.Min.X, m.splitterRect.Min.Y))
	if cmd != nil {
		t.Fatalf("splitter click returned a command %v, want nil", cmd)
	}
	if !model.dragging {
		t.Fatal("dragging = false, want the shared border to start a drag")
	}

	return model
}

func TestSplitterClickStartsADragWithoutChangingTheFocus(t *testing.T) {
	t.Run("from the tree", func(t *testing.T) {
		m := newSized(t, testDeps(t), dragWidth, dragHeight)
		if m.Focus() != FocusTree {
			t.Fatalf("Focus() = %v, want tree before the drag", m.Focus())
		}

		model := startDrag(t, m)

		if model.dragDirty {
			t.Error("dragDirty = true, want a drag that has not moved yet")
		}
		if model.Focus() != FocusTree {
			t.Errorf("Focus() = %v, want tree kept", model.Focus())
		}
	})

	t.Run("from the editor", func(t *testing.T) {
		m := newSized(t, testDeps(t), dragWidth, dragHeight)
		m = openFile(t, m, "main.go")
		if m.Focus() != FocusEditor {
			t.Fatalf("Focus() = %v, want editor before the drag", m.Focus())
		}

		model := startDrag(t, m)

		if model.Focus() != FocusEditor {
			t.Errorf("Focus() = %v, want editor kept", model.Focus())
		}
	})
}

func TestDragMotionResizesTheTree(t *testing.T) {
	m := newSized(t, testDeps(t), dragWidth, dragHeight)
	if got, want := m.treeWidthNow(dragWidth), 42; got != want {
		t.Fatalf("treeWidthNow(%d) = %d, want the automatic %d", dragWidth, got, want)
	}

	const target = 59

	model := startDrag(t, m)
	model, cmd := pressMsg(t, model, motion(target, splitterRow))
	if cmd != nil {
		t.Errorf("command = %v, want nil from a motion", cmd)
	}

	if got, want := model.treeWidthNow(dragWidth), target+1; got != want {
		t.Errorf("treeWidthNow(%d) = %d, want %d", dragWidth, got, want)
	}
	if !model.dragDirty {
		t.Error("dragDirty = false, want the moved border marked dirty")
	}
	if !model.dragging {
		t.Error("dragging = false, want the handle still held")
	}
	if got, want := model.splitterRect.Min.X, target; got != want {
		t.Errorf("splitterRect.Min.X = %d, want the handle under the cursor at %d", got, want)
	}
	if got, want := outerRect(model.treeRect).Dx(), target+1; got != want {
		t.Errorf("tree frame = %d cells wide, want %d", got, want)
	}
	if got, want := outerRect(model.editorRect).Dx(), dragWidth-target-1; got != want {
		t.Errorf("editor frame = %d cells wide, want %d", got, want)
	}
}

func TestDragMotionIsClampedAtBothEnds(t *testing.T) {
	cases := []struct {
		name string
		x    int
		want int
	}{
		{name: "at the left edge", x: 0, want: minTree},
		{name: "far left of the canvas", x: -50, want: minTree},
		{name: "just below the minimum", x: minTree - 2, want: minTree},
		{name: "at the editor minimum", x: dragWidth - minEditorBox, want: dragWidth - minEditorBox},
		{name: "far right of the canvas", x: 10_000, want: dragWidth - minEditorBox},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := startDrag(t, newSized(t, testDeps(t), dragWidth, dragHeight))
			model, _ = pressMsg(t, model, motion(tc.x, splitterRow))

			if got := model.treeWidthNow(dragWidth); got != tc.want {
				t.Errorf("treeWidthNow(%d) = %d, want the clamped %d", dragWidth, got, tc.want)
			}
			if !model.dragDirty {
				t.Error("dragDirty = false, want a moved border")
			}
			if got := outerRect(model.editorRect).Dx(); got != dragWidth-tc.want {
				t.Errorf("editor frame = %d cells wide, want %d", got, dragWidth-tc.want)
			}
		})
	}
}

func TestDragReleaseSavesTheLayoutOnce(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := testDeps(t)
	deps.SaveLayout = recorder

	const target = 59

	m := startDrag(t, newSized(t, deps, dragWidth, dragHeight))
	m, _ = pressMsg(t, m, motion(target, splitterRow))

	if len(*calls) != 0 {
		t.Fatalf("SaveLayout calls = %d before the release, want 0", len(*calls))
	}

	model, cmd := pressMsg(t, m, release(target, splitterRow))
	if cmd == nil {
		t.Fatal("no command, want the layout saved on release")
	}
	if model.dragging || model.dragDirty {
		t.Errorf("dragging/dirty = %v/%v, want both cleared by the release", model.dragging, model.dragDirty)
	}

	out := cmd()
	msg, ok := out.(layoutSavedMsg)
	if !ok {
		t.Fatalf("command returned %T, want app.layoutSavedMsg", out)
	}
	if msg.err != nil {
		t.Errorf("layoutSavedMsg.err = %v, want nil", msg.err)
	}
	if len(*calls) != 1 {
		t.Fatalf("SaveLayout calls = %d, want exactly 1", len(*calls))
	}
	if got := (*calls)[0]; got.width != target+1 || got.collapsed {
		t.Errorf("call = %+v, want {width:%d collapsed:false}", got, target+1)
	}

	// The drag is over: a second release saves nothing.
	model, again := pressMsg(t, model, release(target, splitterRow))
	if again != nil {
		t.Errorf("command = %v, want nil once the drag ended", again)
	}
	if len(*calls) != 1 {
		t.Errorf("SaveLayout calls = %d, want still 1", len(*calls))
	}
}

func TestDragReleaseWithoutAMoveDoesNotSave(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := testDeps(t)
	deps.SaveLayout = recorder

	m := startDrag(t, newSized(t, deps, dragWidth, dragHeight))

	// The handle sits on the cell it already occupies, so the width does not change.
	m, _ = pressMsg(t, m, motion(m.splitterRect.Min.X, splitterRow))

	model, cmd := pressMsg(t, m, release(m.splitterRect.Min.X, splitterRow))
	if cmd != nil {
		t.Errorf("command = %v, want nil without a width change", cmd)
	}
	if model.dragging || model.dragDirty {
		t.Errorf("dragging/dirty = %v/%v, want both cleared", model.dragging, model.dragDirty)
	}
	if len(*calls) != 0 {
		t.Errorf("SaveLayout calls = %d, want 0", len(*calls))
	}
}

func TestMotionWithoutADragIsIgnored(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := testDeps(t)
	deps.SaveLayout = recorder

	m := newSized(t, deps, dragWidth, dragHeight)
	before := m.treeWidthNow(dragWidth)

	for _, msg := range []tea.MouseMotionMsg{motion(80, splitterRow), motionWithoutButton(10, splitterRow)} {
		model, cmd := pressMsg(t, m, msg)

		if cmd != nil {
			t.Errorf("command = %v, want nil", cmd)
		}
		if model.dragging || model.dragDirty {
			t.Errorf("dragging/dirty = %v/%v, want both false without a drag", model.dragging, model.dragDirty)
		}
		if got := model.treeWidthNow(dragWidth); got != before {
			t.Errorf("treeWidthNow(%d) = %d, want %d kept", dragWidth, got, before)
		}
	}

	// A release nobody started neither saves nor panics.
	model, cmd := pressMsg(t, m, release(80, splitterRow))
	if cmd != nil || len(*calls) != 0 {
		t.Errorf("command/calls = %v/%d, want no save without a drag", cmd, len(*calls))
	}
	if got := model.treeWidthNow(dragWidth); got != before {
		t.Errorf("treeWidthNow(%d) = %d, want %d kept", dragWidth, got, before)
	}
}

func TestMotionOnAnotherTabCancelsTheDrag(t *testing.T) {
	for _, tab := range []Tab{GitTab, DockerTab} {
		t.Run(tab.String(), func(t *testing.T) {
			recorder, calls := layoutRecorder()

			deps := testDeps(t)
			deps.SaveLayout = recorder

			m := startDrag(t, newSized(t, deps, dragWidth, dragHeight))
			m, _ = pressMsg(t, m, motion(59, splitterRow))

			model, _ := pressMsg(t, m, tabKey(tab))
			model, cmd := pressMsg(t, model, motion(59, splitterRow))

			if model.dragging || model.dragDirty {
				t.Errorf("dragging/dirty = %v/%v, want the drag cancelled", model.dragging, model.dragDirty)
			}
			if cmd != nil {
				t.Errorf("command = %v, want nil", cmd)
			}
			if len(*calls) != 0 {
				t.Errorf("SaveLayout calls = %d, want the cancellation to save nothing", len(*calls))
			}

			model, cmd = pressMsg(t, model, release(59, splitterRow))
			if cmd != nil || len(*calls) != 0 {
				t.Errorf("command/calls = %v/%d, want no save after the cancellation", cmd, len(*calls))
			}
		})
	}
}

func TestClickAfterADragStillReachesThePanel(t *testing.T) {
	m := newSized(t, testDeps(t), dragWidth, dragHeight)
	m = drain(t, m, m.Init())

	// A drag that never gets its release leaves an orphaned handle behind.
	m = startDrag(t, m)
	m, _ = pressMsg(t, m, motion(m.splitterRect.Min.X+20, splitterRow))

	model, cmd := pressMsg(t, m, click(m.treeRect.Min.X, m.treeRect.Min.Y+1))
	if cmd != nil {
		t.Errorf("command = %v, want nil from the row click", cmd)
	}
	if model.dragging || model.dragDirty {
		t.Errorf("dragging/dirty = %v/%v, want the click to drop the orphan drag", model.dragging, model.dragDirty)
	}
	if got := model.Focus(); got != FocusTree {
		t.Errorf("Focus() = %v, want tree", got)
	}
	if node, ok := model.tree.Selected(); !ok || node.Path != "long.txt" {
		t.Errorf("Selected() = %q (%v), want long.txt from the clicked row", node.Path, ok)
	}
}

func TestDragReleaseWithoutASaverIsSafe(t *testing.T) {
	m := newSized(t, testDeps(t), dragWidth, dragHeight) // no SaveLayout injected
	m = startDrag(t, m)
	m, _ = pressMsg(t, m, motion(59, splitterRow))

	model, cmd := pressMsg(t, m, release(59, splitterRow))
	if cmd != nil {
		t.Errorf("command = %v, want nil without a saver", cmd)
	}
	if model.dragging || model.dragDirty {
		t.Errorf("dragging/dirty = %v/%v, want both cleared", model.dragging, model.dragDirty)
	}
	if got, want := model.treeWidthNow(dragWidth), 60; got != want {
		t.Errorf("treeWidthNow(%d) = %d, want the dragged %d kept", dragWidth, got, want)
	}
}

func TestCollapseButtonClickTogglesWithoutStartingADrag(t *testing.T) {
	m := newSized(t, testDeps(t), dragWidth, dragHeight)
	m = openFile(t, m, "main.go") // the editor owns the keyboard

	before := m.treeWidthNow(dragWidth)
	model, cmd := pressMsg(t, m, click(m.collapseRect.Min.X, m.collapseRect.Min.Y))

	if cmd != nil {
		t.Errorf("command = %v, want nil without a saver", cmd)
	}
	if model.dragging || model.dragDirty {
		t.Errorf("dragging/dirty = %v/%v, want the button click kept away from the handle", model.dragging, model.dragDirty)
	}
	if !model.treeCollapsed {
		t.Fatal("treeCollapsed = false, want the button to hide the tree")
	}
	if got := model.treeWidthNow(dragWidth); got != 0 {
		t.Errorf("treeWidthNow(%d) = %d, want 0 while hidden", dragWidth, got)
	}
	if got := model.Focus(); got != FocusEditor {
		t.Errorf("Focus() = %v, want editor kept", got)
	}

	// The hidden tree keeps its width: the second click restores the previous split.
	model, _ = pressMsg(t, model, click(model.collapseRect.Min.X, model.collapseRect.Min.Y))
	if model.treeCollapsed {
		t.Error("treeCollapsed = true, want the second click to show the tree again")
	}
	if got := model.treeWidthNow(dragWidth); got != before {
		t.Errorf("treeWidthNow(%d) = %d, want %d restored", dragWidth, got, before)
	}
}

func TestDraggedHandleTakesTheAccentToken(t *testing.T) {
	accent := frameEscape(t, styles.BorderFocused)
	muted := frameEscape(t, styles.Border)

	m := newSized(t, testDeps(t), dragWidth, dragHeight)
	m = openFile(t, m, "main.go") // the editor owns the keyboard, so the tree border is muted

	x := m.splitterRect.Min.X
	row := strings.Split(m.View().Content, "\n")[splitterRow]
	assertFrame(t, row, x, muted, true)
	assertFrame(t, row, x, accent, false)

	// While the handle is held, the shared border carries the accent on its tree-side cell.
	m = startDrag(t, m)
	m, _ = pressMsg(t, m, motion(x+10, splitterRow))

	row = strings.Split(m.View().Content, "\n")[splitterRow]
	assertFrame(t, row, m.splitterRect.Min.X, accent, true)
}

func TestDragOnANarrowCanvasKeepsTheLayout(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := testDeps(t)
	deps.SaveLayout = recorder

	m := newSized(t, deps, minWidth, minHeight)
	if !m.canSplit() {
		t.Fatal("canSplit() = false, want the handle on the minimum canvas")
	}

	x, y := m.splitterRect.Min.X, m.splitterRect.Min.Y
	m = startDrag(t, m)

	for _, target := range []int{0, x, minWidth - 1, minWidth + 10, -5} {
		m, _ = pressMsg(t, m, motion(target, y))
	}

	model, cmd := pressMsg(t, m, release(x, y))

	// The canvas cannot fit both panels at any other width, so nothing moved.
	if got, want := model.treeWidthNow(minWidth), splitWidth(minWidth, 0); got != want {
		t.Errorf("treeWidthNow(%d) = %d, want the clamped %d", minWidth, got, want)
	}
	if model.dragging || model.dragDirty {
		t.Errorf("dragging/dirty = %v/%v, want both cleared", model.dragging, model.dragDirty)
	}
	if cmd != nil || len(*calls) != 0 {
		t.Errorf("command/calls = %v/%d, want no save without a width change", cmd, len(*calls))
	}

	rows := strings.Split(model.View().Content, "\n")
	if len(rows) != minHeight {
		t.Fatalf("rendered %d rows, want %d", len(rows), minHeight)
	}
	for i, row := range rows {
		if got := lipgloss.Width(row); got > minWidth {
			t.Errorf("row %d = %d cells, want at most %d", i, got, minWidth)
		}
		if i >= headerRows && i < headerRows+bodyHeight(minHeight) {
			if got := lipgloss.Width(row); got != minWidth {
				t.Errorf("body row %d = %d cells, want the full width %d", i, got, minWidth)
			}
		}
	}
	if got := outerRect(model.editorRect).Dx(); got != minWidth-model.treeWidthNow(minWidth) {
		t.Errorf("editor frame = %d cells wide, want %d", got, minWidth-model.treeWidthNow(minWidth))
	}
}
