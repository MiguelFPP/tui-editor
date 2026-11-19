package app

import (
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/config"
)

// collapseTreeWidth is the explicit tree width the toggle tests persist.
const collapseTreeWidth = 42

// toggleKey is the default key of the tree toggle.
func toggleKey() tea.KeyPressMsg {
	return key(tea.KeyF2)
}

// toggleButton clicks the tree button of the tab row.
func toggleButton(t *testing.T, m Model) Model {
	t.Helper()

	if m.collapseRect.Dx() == 0 {
		t.Fatal("collapseRect is empty, want the tree button")
	}

	return update(t, m, click(m.collapseRect.Min.X, m.collapseRect.Min.Y))
}

// assertCollapsedFillsTheCanvas checks a hidden tree leaves the body to the editor alone.
func assertCollapsedFillsTheCanvas(t *testing.T, m Model, width, height int) {
	t.Helper()

	if m.treeRect != (image.Rectangle{}) {
		t.Errorf("treeRect = %v, want it empty while collapsed", m.treeRect)
	}
	if m.splitterRect != (image.Rectangle{}) {
		t.Errorf("splitterRect = %v, want it empty while collapsed", m.splitterRect)
	}
	if m.canSplit() {
		t.Error("canSplit() = true, want no handle while collapsed")
	}
	if got := outerRect(m.editorRect).Dx(); got != width {
		t.Errorf("editor frame = %d cells wide, want the whole canvas %d", got, width)
	}
	if got := outerRect(m.editorRect).Dy(); got != bodyHeight(height) {
		t.Errorf("editor frame = %d rows high, want the body %d", got, bodyHeight(height))
	}
}

func TestF2CollapsesAndExpandsTheTree(t *testing.T) {
	m := newSized(t, testDeps(t), dragWidth, dragHeight)
	m = openFile(t, m, "main.go")

	expanded := m.treeRect
	if expanded.Dx() == 0 || !m.canSplit() {
		t.Fatalf("treeRect = %v, want an expanded tree with a splitter", expanded)
	}

	model := update(t, m, toggleKey())

	if !model.treeCollapsed {
		t.Fatal("treeCollapsed = false after f2, want true")
	}
	assertCollapsedFillsTheCanvas(t, model, dragWidth, dragHeight)
	if got := model.Focus(); got != FocusEditor {
		t.Errorf("Focus() = %v, want the editor", got)
	}
	if row := tabRowText(t, model); !strings.HasSuffix(row, "[>]") {
		t.Errorf("tab row = %q, want the button of a hidden tree", row)
	}

	model = update(t, model, toggleKey())

	if model.treeCollapsed {
		t.Fatal("treeCollapsed = true after the second f2, want false")
	}
	if model.treeRect != expanded {
		t.Errorf("treeRect = %v, want the previous %v restored", model.treeRect, expanded)
	}
	if !model.canSplit() {
		t.Error("canSplit() = false, want the splitter back")
	}
	if got := model.Focus(); got != FocusEditor {
		t.Errorf("Focus() = %v, want the editor kept on expansion", got)
	}
	if row := tabRowText(t, model); !strings.HasSuffix(row, "[<]") {
		t.Errorf("tab row = %q, want the button of a shown tree", row)
	}
}

func TestTreeButtonTogglesLikeF2(t *testing.T) {
	byKey := newSized(t, testDeps(t), dragWidth, dragHeight)
	byKey = openFile(t, byKey, "main.go")
	expanded := byKey.treeRect
	byKey = update(t, byKey, toggleKey())

	byClick := newSized(t, testDeps(t), dragWidth, dragHeight)
	byClick = openFile(t, byClick, "main.go")
	byClick = toggleButton(t, byClick)

	if !byClick.treeCollapsed {
		t.Error("treeCollapsed = false, want the button to hide the tree")
	}
	if byClick.treeCollapsed != byKey.treeCollapsed || byClick.treeRect != byKey.treeRect ||
		byClick.splitterRect != byKey.splitterRect || byClick.editorRect != byKey.editorRect {
		t.Errorf("button state = %v/%v/%v, want %v/%v/%v as with f2",
			byClick.treeCollapsed, byClick.treeRect, byClick.editorRect,
			byKey.treeCollapsed, byKey.treeRect, byKey.editorRect)
	}
	if got := byClick.Focus(); got != byKey.Focus() {
		t.Errorf("Focus() = %v, want %v as with f2", got, byKey.Focus())
	}

	// A second click brings the tree back with the width it had before.
	back := toggleButton(t, byClick)
	if back.treeCollapsed {
		t.Error("treeCollapsed = true, want the second click to show the tree again")
	}
	if back.treeRect != expanded {
		t.Errorf("treeRect = %v, want the previous %v restored", back.treeRect, expanded)
	}
	if got := back.Focus(); got != FocusEditor {
		t.Errorf("Focus() = %v, want the editor kept", got)
	}
}

func TestEachToggleSavesTheLayoutOnce(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := sizedDeps(t, config.Config{TreeWidth: collapseTreeWidth})
	deps.SaveLayout = recorder

	m := newSized(t, deps, dragWidth, dragHeight)

	model, cmd := press(t, m, toggleKey())
	if cmd == nil {
		t.Fatal("no command, want the layout saved by f2")
	}
	model = drain(t, model, cmd)

	if len(*calls) != 1 {
		t.Fatalf("SaveLayout calls = %d after f2, want exactly 1", len(*calls))
	}
	if got := (*calls)[0]; got.width != collapseTreeWidth || !got.collapsed {
		t.Errorf("call = %+v, want {width:%d collapsed:true}", got, collapseTreeWidth)
	}
	if !model.treeCollapsed {
		t.Fatal("treeCollapsed = false, want the hidden tree")
	}

	// The expansion persists the width it kept while hidden.
	model, cmd = press(t, model, toggleKey())
	if cmd == nil {
		t.Fatal("no command, want the layout saved on expansion")
	}
	model = drain(t, model, cmd)

	if len(*calls) != 2 {
		t.Fatalf("SaveLayout calls = %d after the expansion, want exactly 2", len(*calls))
	}
	if got := (*calls)[1]; got.width != collapseTreeWidth || got.collapsed {
		t.Errorf("call = %+v, want {width:%d collapsed:false}", got, collapseTreeWidth)
	}

	// The button persists the same pair as the key.
	model, cmd = pressMsg(t, model, click(model.collapseRect.Min.X, model.collapseRect.Min.Y))
	if cmd == nil {
		t.Fatal("no command, want the button to save the layout")
	}
	model = drain(t, model, cmd)

	if len(*calls) != 3 {
		t.Fatalf("SaveLayout calls = %d after the button, want exactly 3", len(*calls))
	}
	if got := (*calls)[2]; got.width != collapseTreeWidth || !got.collapsed {
		t.Errorf("call = %+v, want {width:%d collapsed:true}", got, collapseTreeWidth)
	}
}

func TestToggleWithoutASaverIsSafe(t *testing.T) {
	m := newSized(t, testDeps(t), dragWidth, dragHeight) // no SaveLayout injected
	if m.saveLayout != nil {
		t.Fatal("saveLayout != nil, want none injected")
	}

	model, cmd := press(t, m, toggleKey())
	if cmd != nil {
		t.Errorf("command = %v, want nil without a saver", cmd)
	}
	model = drain(t, model, cmd)
	if !model.treeCollapsed {
		t.Error("treeCollapsed = false, want the toggle to work without a saver")
	}

	model, cmd = pressMsg(t, model, click(model.collapseRect.Min.X, model.collapseRect.Min.Y))
	if cmd != nil {
		t.Errorf("command = %v, want nil without a saver", cmd)
	}
	if model.treeCollapsed {
		t.Error("treeCollapsed = true, want the button to show the tree again")
	}
}

func TestToggleOnlyActsInTheFilesTab(t *testing.T) {
	for _, tab := range []Tab{GitTab, DockerTab} {
		t.Run(tab.String(), func(t *testing.T) {
			recorder, calls := layoutRecorder()

			deps := testDeps(t)
			deps.SaveLayout = recorder

			m := newSized(t, deps, dragWidth, dragHeight)
			m, _ = press(t, m, tabKey(tab))

			width := m.treeWidth
			model, cmd := press(t, m, toggleKey())
			model = drain(t, model, cmd)

			if model.treeCollapsed {
				t.Errorf("treeCollapsed = true in the %v tab, want it unchanged", tab)
			}
			if model.treeWidth != width {
				t.Errorf("treeWidth = %d, want %d kept", model.treeWidth, width)
			}
			if model.Tab() != tab || model.Focus() != m.Focus() {
				t.Errorf("tab/focus = %v/%v, want %v/%v kept", model.Tab(), model.Focus(), tab, m.Focus())
			}
			if len(*calls) != 0 {
				t.Errorf("SaveLayout calls = %d, want the other tabs to save nothing", len(*calls))
			}
		})
	}
}

func TestF2BelongsToTheOpenDraft(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := testDeps(t)
	deps.SaveLayout = recorder

	m := newSized(t, deps, dragWidth, dragHeight)
	m = openDraft(t, m, "main.go", dragWidth)

	model, _ := press(t, m, toggleKey())

	if model.treeCollapsed {
		t.Error("treeCollapsed = true, want the draft to capture f2")
	}
	if !model.editor.CapturesInput() {
		t.Error("CapturesInput() = false, want the draft kept open")
	}
	if got := model.editor.Err(); got != nil {
		t.Errorf("editor.Err() = %v, want no error from f2", got)
	}
	if len(*calls) != 0 {
		t.Errorf("SaveLayout calls = %d, want nothing saved while editing", len(*calls))
	}
}

func TestToggleKeepsTheTreeState(t *testing.T) {
	m := newSized(t, testDeps(t), dragWidth, dragHeight)
	m = drain(t, m, m.Init())

	m = update(t, m, click(m.treeRect.Min.X, m.treeRect.Min.Y+1)) // the second row
	selected, ok := m.tree.Selected()
	if !ok || selected.Path != "long.txt" {
		t.Fatalf("Selected() = %q (%v), want long.txt before the toggles", selected.Path, ok)
	}
	rows := len(m.tree.Visible())
	if rows == 0 {
		t.Fatal("Visible() = 0 rows, want the loaded listing")
	}

	model := update(t, m, toggleKey())
	if !model.treeCollapsed {
		t.Fatal("treeCollapsed = false, want the tree hidden")
	}
	model = update(t, model, toggleKey())

	if got, ok := model.tree.Selected(); !ok || got.Path != selected.Path {
		t.Errorf("Selected() = %q (%v), want %q kept", got.Path, ok, selected.Path)
	}
	if got := len(model.tree.Visible()); got != rows {
		t.Errorf("Visible() = %d rows, want %d kept", got, rows)
	}
}

func TestF2CollapseDropsTheActiveDrag(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := testDeps(t)
	deps.SaveLayout = recorder

	m := startDrag(t, newSized(t, deps, dragWidth, dragHeight))
	m, _ = pressMsg(t, m, motion(59, splitterRow))
	if !m.dragDirty {
		t.Fatal("dragDirty = false, want a moved handle")
	}

	model, cmd := press(t, m, toggleKey())
	model = drain(t, model, cmd)

	if model.dragging || model.dragDirty {
		t.Errorf("dragging/dirty = %v/%v, want both cleared by the collapse", model.dragging, model.dragDirty)
	}
	if !model.treeCollapsed {
		t.Error("treeCollapsed = false, want the tree hidden")
	}
	if len(*calls) != 1 {
		t.Fatalf("SaveLayout calls = %d, want only the toggle to save", len(*calls))
	}
	if got := (*calls)[0]; got.width != 60 || !got.collapsed {
		t.Errorf("call = %+v, want {width:60 collapsed:true}", got)
	}

	// The orphan drag is gone: its release saves nothing more.
	model, again := pressMsg(t, model, release(59, splitterRow))
	if again != nil || len(*calls) != 1 {
		t.Errorf("command/calls = %v/%d, want no extra save", again, len(*calls))
	}
}

func TestCollapsedWithoutAnEditorShowsThePlaceholder(t *testing.T) {
	m := newSized(t, testDeps(t), dragWidth, dragHeight)
	if m.hasEditor {
		t.Fatal("hasEditor = true, want no file open")
	}

	model := update(t, m, toggleKey())
	if !model.treeCollapsed {
		t.Fatal("treeCollapsed = false, want the tree hidden")
	}

	content := model.View().Content
	if !strings.Contains(content, "selecciona un archivo") {
		t.Errorf("View() = %q, want the editor placeholder", content)
	}

	rows := strings.Split(content, "\n")
	if len(rows) != dragHeight {
		t.Fatalf("rendered %d rows, want %d", len(rows), dragHeight)
	}
	for i, row := range rows {
		width := lipgloss.Width(row)
		if width > dragWidth {
			t.Errorf("row %d = %d cells, want at most %d", i, width, dragWidth)
		}
		if i >= headerRows && width != dragWidth {
			t.Errorf("row %d = %d cells, want the full width %d", i, width, dragWidth)
		}
	}

	assertCollapsedFillsTheCanvas(t, model, dragWidth, dragHeight)
	if got := model.Focus(); got != FocusEditor {
		t.Errorf("Focus() = %v, want the editor even without an open file", got)
	}
}

func TestF2IsSwallowedByTheHelpOverlay(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := testDeps(t)
	deps.SaveLayout = recorder

	m := newSized(t, deps, dragWidth, dragHeight)
	m = update(t, m, typed('?'))

	model, _ := press(t, m, toggleKey())

	if !model.HelpVisible() {
		t.Error("HelpVisible() = false, want the overlay still open")
	}
	if model.treeCollapsed {
		t.Error("treeCollapsed = true, want the overlay to capture f2")
	}
	if len(*calls) != 0 {
		t.Errorf("SaveLayout calls = %d, want nothing saved", len(*calls))
	}

	// esc closes the overlay and still leaves the tree alone.
	model = update(t, model, key(tea.KeyEscape))
	if model.HelpVisible() {
		t.Error("HelpVisible() = true, want esc to close the overlay")
	}
	if model.treeCollapsed {
		t.Error("treeCollapsed = true, want esc to leave the tree alone")
	}
}
