package gitpanel

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

// TestToggleListCollapsesAndExpands keeps the state of the list across the toggle.
func TestToggleListCollapsesAndExpands(t *testing.T) {
	rec := &saveRecorder{}
	m := splitterPanel(t, rec)

	m.cursors[Changes] = 3
	m.scrollList = 1
	width := m.listW
	// sync rebuilds the metrics: the cursor is clamped to what the list really has.
	m.sync()
	wantCursor, wantScroll := m.cursors[Changes], m.scrollList

	next, cmd := m.Update(typed('H'))
	if !next.listCollapsed {
		t.Fatal("H did not collapse the list")
	}
	if next.split {
		t.Error("split = true, want the preview with the whole width")
	}
	if got := next.previewW; got != 0 {
		t.Errorf("previewW = %d, want 0 while collapsed", got)
	}
	if next.cursors[Changes] != wantCursor || next.scrollList != wantScroll {
		t.Errorf("cursor/offset = %d/%d, want %d/%d kept",
			next.cursors[Changes], next.scrollList, wantCursor, wantScroll)
	}
	if next.listWidth != m.listWidth {
		t.Errorf("listWidth = %d, want %d kept", next.listWidth, m.listWidth)
	}
	drainSaves(t, next, cmd)

	back, cmd := next.Update(typed('H'))
	if back.listCollapsed {
		t.Fatal("the second H did not expand the list")
	}
	if !back.split || back.listW != width {
		t.Errorf("split/listW = %v/%d, want true/%d", back.split, back.listW, width)
	}
	drainSaves(t, back, cmd)

	if len(rec.collapsed) != 2 || !rec.collapsed[0] || rec.collapsed[1] {
		t.Errorf("saved collapsed = %v, want [true false]", rec.collapsed)
	}
}

// TestCollapsedBodyShowsTheDiff checks the preview takes the whole body.
func TestCollapsedBodyShowsTheDiff(t *testing.T) {
	m := open(t, twoHunkFake(t))
	m.SetSize(120, 20)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))
	m = press(t, m, typed('H'))

	if !m.listCollapsed {
		t.Fatal("the list must be collapsed")
	}

	body := plain(strings.Join(m.body(), "\n"))
	if !strings.Contains(body, "@@ -1,1 +1,1 @@") {
		t.Errorf("body = %q, want the diff of the selected file", body)
	}
	if strings.Contains(body, "Cambios ·") {
		t.Errorf("body = %q, want no list header while collapsed", body)
	}
	for index, line := range m.body() {
		if got := len([]rune(plain(line))); got > m.width {
			t.Errorf("row %d width = %d, want at most %d", index, got, m.width)
		}
	}
}

// TestCollapsedWithoutDiffKeepsTheList avoids an empty body before a preview is loaded.
func TestCollapsedWithoutDiffKeepsTheList(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(120, 20)
	m = press(t, m, typed('H'))

	body := plain(strings.Join(m.body(), "\n"))
	if !strings.Contains(body, "normal.txt") {
		t.Errorf("body = %q, want the list kept when there is nothing to preview", body)
	}
}

// TestToggleListInSingleModeIsInert keeps the one-column mode untouched.
func TestToggleListInSingleModeIsInert(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(80, 16)

	next, cmd := m.Update(typed('H'))
	if next.listCollapsed {
		t.Error("listCollapsed = true, want it ignored in single mode")
	}
	if cmd != nil {
		t.Errorf("toggle returned %v, want no save", cmd())
	}
}

// TestCollapsedWithPersistedStateStartsHidden covers the option at construction.
func TestCollapsedWithPersistedStateStartsHidden(t *testing.T) {
	m := NewWithOptions(gitFake(t), keymap.Default(), Options{
		Layout:        LayoutSplit,
		ListCollapsed: true,
	})
	m.SetSize(120, 20)

	if m.split {
		t.Fatal("split = true, want the persisted collapsed list")
	}
	if m.listCollapsed != true {
		t.Error("listCollapsed = false, want true")
	}
}

// TestListToggleIsDocumented keeps the key discoverable.
func TestListToggleIsDocumented(t *testing.T) {
	m := splitterPanel(t, &saveRecorder{})
	m = press(t, m, key('?'))

	view := plain(m.View())
	if !strings.Contains(view, "H") || !strings.Contains(view, "minimizar la lista") {
		t.Errorf("View() = %q, want the toggle documented", view)
	}

	m = press(t, m, key('?'))
	if footer := m.footerText(); !strings.Contains(footer, "H: lista") {
		t.Errorf("footer = %q, want the toggle hint", footer)
	}
}

// TestCollapsedDragDoesNotResize keeps the handle inert while the list is hidden.
func TestCollapsedDragDoesNotResize(t *testing.T) {
	m := splitterPanel(t, &saveRecorder{})
	m.listCollapsed = true
	m.sync()
	before := m.listWidth

	if m.onSplitter(m.splitterRect.Min) && m.splitterRect.Dx() > 0 {
		t.Error("onSplitter() = true, want no handle while collapsed")
	}

	m.dragging = true
	m.dragTo(10)
	if m.listWidth != before {
		t.Errorf("listWidth = %d, want %d kept", m.listWidth, before)
	}
}
