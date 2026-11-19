package gitpanel

import (
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

// shiftKey builds a shifted arrow key, the way the runtime delivers it.
func shiftKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: tea.ModShift}
}

// motionAt builds one drag motion message.
func motionAt(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// releaseAt builds one mouse release message.
func releaseAt(x, y int) tea.MouseReleaseMsg {
	return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// saveRecorder collects the layout save calls.
type saveRecorder struct {
	layouts   []Layout
	widths    []int
	collapsed []bool
}

func (r *saveRecorder) save(layout Layout, width int, collapsed bool) error {
	r.layouts = append(r.layouts, layout)
	r.widths = append(r.widths, width)
	r.collapsed = append(r.collapsed, collapsed)

	return nil
}

// drainSaves runs the save command, if any.
func drainSaves(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		return m
	}

	msg, ok := cmd().(LayoutSavedMsg)
	if !ok {
		t.Fatalf("save command returned %T, want LayoutSavedMsg", cmd())
	}
	if msg.Err != nil {
		t.Fatalf("LayoutSavedMsg.Err = %v, want nil", msg.Err)
	}

	return m
}

// splitterPanel builds a split panel with a save seam.
func splitterPanel(t *testing.T, rec *saveRecorder) Model {
	t.Helper()

	m := NewWithOptions(gitFake(t), keymap.Default(), Options{
		Layout:     LayoutSplit,
		SaveLayout: rec.save,
	})
	m.SetSize(120, 20)

	return m
}

// TestSplitterRectSitsBetweenColumns places the handle on the shared border.
func TestSplitterRectSitsBetweenColumns(t *testing.T) {
	m := splitterPanel(t, &saveRecorder{})

	if m.splitterRect.Dx() == 0 || m.splitterRect.Dy() == 0 {
		t.Fatalf("splitterRect = %v, want a handle", m.splitterRect)
	}
	if m.splitterRect.Min.X != m.listRect.Max.X {
		t.Errorf("splitter starts at %d, want the list edge %d", m.splitterRect.Min.X, m.listRect.Max.X)
	}
	if m.splitterRect.Max.X != m.previewRect.Min.X {
		t.Errorf("splitter ends at %d, want the preview edge %d", m.splitterRect.Max.X, m.previewRect.Min.X)
	}
	if m.listRect.Overlaps(m.splitterRect) || m.previewRect.Overlaps(m.splitterRect) {
		t.Error("the handle must not overlap either column")
	}
}

// TestDragResizesTheList follows one click, one motion and one release.
func TestDragResizesTheList(t *testing.T) {
	rec := &saveRecorder{}
	m := splitterPanel(t, rec)

	start := m.listW
	x := m.splitterRect.Min.X
	m2, _ := m.Update(tea.MouseClickMsg{X: x, Y: m.splitterRect.Min.Y, Button: tea.MouseLeft})
	if !m2.dragging {
		t.Fatal("the click on the handle must start a drag")
	}

	m2, _ = m2.Update(motionAt(x+10, m2.splitterRect.Min.Y))
	if m2.listW <= start {
		t.Errorf("listW = %d, want more than %d", m2.listW, start)
	}
	if !m2.dragDirty {
		t.Error("dragDirty = false, want the motion recorded")
	}

	m2, cmd := m2.Update(releaseAt(x+10, m2.splitterRect.Min.Y))
	if m2.dragging {
		t.Error("dragging = true, want the release to finish it")
	}
	drainSaves(t, m2, cmd)

	if len(rec.widths) != 1 {
		t.Fatalf("save calls = %d, want 1", len(rec.widths))
	}
	if rec.widths[0] != m2.listW {
		t.Errorf("saved width = %d, want %d", rec.widths[0], m2.listW)
	}
}

// TestDragRespectsBothMinimums clamps the handle at both ends.
func TestDragRespectsBothMinimums(t *testing.T) {
	m := splitterPanel(t, &saveRecorder{})

	y := m.splitterRect.Min.Y
	m.dragging = true
	m.dragTo(0)
	if m.listW < minGitList {
		t.Errorf("listW = %d, want at least %d", m.listW, minGitList)
	}

	m.dragTo(m.width * 2)
	if m.previewW < minGitPreview {
		t.Errorf("previewW = %d, want at least %d", m.previewW, minGitPreview)
	}

	m.dragging = false
	if got := m.listW + m.previewW; got != m.width {
		t.Errorf("listW+previewW = %d, want %d", got, m.width)
	}
	_ = y
}

// TestClickAwayFromTheHandleCancelsTheDrag keeps a stray click harmless.
func TestClickAwayFromTheHandleCancelsTheDrag(t *testing.T) {
	m := splitterPanel(t, &saveRecorder{})
	m.dragging = true

	next := clickAt(t, m, m.listRect.Min.X+1, m.listRect.Min.Y)

	if next.dragging {
		t.Error("dragging = true, want a click on a row to cancel it")
	}
}

// TestKeyboardResizesTheList mirrors the drag with the keyboard actions.
func TestKeyboardResizesTheList(t *testing.T) {
	rec := &saveRecorder{}
	m := splitterPanel(t, rec)

	start := m.listW
	next, cmd := m.Update(shiftKey(tea.KeyRight))
	if next.listW != start+widthStep {
		t.Errorf("listW = %d, want %d", next.listW, start+widthStep)
	}
	drainSaves(t, next, cmd)

	next, cmd = next.Update(shiftKey(tea.KeyLeft))
	if next.listW != start {
		t.Errorf("listW = %d, want the original %d", next.listW, start)
	}
	drainSaves(t, next, cmd)
}

// TestPersistedListWidthWinsOnConstruction checks the option is honoured.
func TestPersistedListWidthWins(t *testing.T) {
	m := NewWithOptions(gitFake(t), keymap.Default(), Options{Layout: LayoutSplit, ListWidth: 50})
	m.SetSize(120, 20)

	if m.listW != 50 {
		t.Errorf("listW = %d, want the persisted 50", m.listW)
	}
	if m.previewW != 70 {
		t.Errorf("previewW = %d, want 70", m.previewW)
	}
}

// TestPersistedListWidthIsClamped keeps an out-of-range value usable.
func TestPersistedListWidthIsClamped(t *testing.T) {
	m := NewWithOptions(gitFake(t), keymap.Default(), Options{Layout: LayoutSplit, ListWidth: 5})
	m.SetSize(120, 20)

	if m.listW < minGitList {
		t.Errorf("listW = %d, want at least %d", m.listW, minGitList)
	}
}

// TestDragIsIgnoredInSingleMode keeps the fallback free of a handle.
func TestDragIsIgnoredInSingleMode(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(80, 16)

	if m.onSplitter(image.Pt(40, 5)) {
		t.Error("onSplitter() = true, want no handle in single mode")
	}

	m.dragging = true
	m.dragTo(60)
	if m.listW != 80 {
		t.Errorf("listW = %d, want the whole width", m.listW)
	}

	_, cmd := m.Update(releaseAt(60, 5))
	if cmd != nil {
		t.Errorf("release returned %v, want no save after a cancelled drag", cmd())
	}
}

// TestSplitterHandleIsDrawn marks the border while dragging.
func TestSplitterHandleIsDrawn(t *testing.T) {
	m := splitterPanel(t, &saveRecorder{})
	idle := strings.Join(m.body(), "\n")

	m.dragging = true
	dragging := strings.Join(m.body(), "\n")

	if plain(idle) == plain(dragging) {
		t.Error("the body must mark the handle while dragging")
	}
}
