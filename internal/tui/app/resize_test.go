package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/config"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

// resizeTreeWidth is the explicit tree width the keyboard resize tests start from.
const resizeTreeWidth = 40

// shiftLeft and shiftRight are the resize keys, built as the runtime reports them.
func shiftLeft() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift}
}

func shiftRight() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift}
}

// maxTreeWidth is the widest tree the canvas allows before the editor hits its minimum.
func maxTreeWidth(width int) int {
	return max(width-minEditorBox, minTree)
}

// resizeStep presses one resize key and returns the model with its command already run.
func resizeStep(t *testing.T, m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()

	model, cmd := press(t, m, msg)

	return drain(t, model, cmd), cmd
}

func TestTreeResizeMovesTheBorderByTwoCells(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := sizedDeps(t, config.Config{TreeWidth: resizeTreeWidth})
	deps.SaveLayout = recorder

	m := newSized(t, deps, dragWidth, dragHeight)
	if got := m.treeWidthNow(dragWidth); got != resizeTreeWidth {
		t.Fatalf("treeWidthNow(%d) = %d, want the explicit %d", dragWidth, got, resizeTreeWidth)
	}

	model, cmd := resizeStep(t, m, shiftRight())
	if cmd == nil {
		t.Fatal("no command, want the layout saved by shift+right")
	}

	want := resizeTreeWidth + treeStep
	if got := model.treeWidthNow(dragWidth); got != want {
		t.Errorf("treeWidthNow(%d) = %d, want %d", dragWidth, got, want)
	}
	if got := outerRect(model.treeRect).Dx(); got != want {
		t.Errorf("tree frame = %d cells wide, want %d", got, want)
	}
	if got := outerRect(model.editorRect).Dx(); got != dragWidth-want {
		t.Errorf("editor frame = %d cells wide, want %d", got, dragWidth-want)
	}
	if got := model.splitterRect.Min.X; got != want-1 {
		t.Errorf("splitterRect.Min.X = %d, want the handle at %d", got, want-1)
	}
	if len(*calls) != 1 {
		t.Fatalf("SaveLayout calls = %d after shift+right, want exactly 1", len(*calls))
	}
	if got := (*calls)[0]; got.width != want || got.collapsed {
		t.Errorf("call = %+v, want {width:%d collapsed:false}", got, want)
	}

	// The opposite key walks the border back to where it started.
	model, cmd = resizeStep(t, model, shiftLeft())
	if cmd == nil {
		t.Fatal("no command, want the layout saved by shift+left")
	}

	if got := model.treeWidthNow(dragWidth); got != resizeTreeWidth {
		t.Errorf("treeWidthNow(%d) = %d, want the previous %d", dragWidth, got, resizeTreeWidth)
	}
	if got := outerRect(model.editorRect).Dx(); got != dragWidth-resizeTreeWidth {
		t.Errorf("editor frame = %d cells wide, want %d", got, dragWidth-resizeTreeWidth)
	}
	if len(*calls) != 2 {
		t.Fatalf("SaveLayout calls = %d after shift+left, want exactly 2", len(*calls))
	}
	if got := (*calls)[1]; got.width != resizeTreeWidth || got.collapsed {
		t.Errorf("call = %+v, want {width:%d collapsed:false}", got, resizeTreeWidth)
	}
}

func TestTreeResizeIsClampedAtBothEnds(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := sizedDeps(t, config.Config{TreeWidth: resizeTreeWidth})
	deps.SaveLayout = recorder

	m := newSized(t, deps, dragWidth, dragHeight)

	// Shrinking stops at the tree minimum, saving one step per real move.
	model, saves := m, 0
	for i := 0; i < 40; i++ {
		var cmd tea.Cmd
		model, cmd = resizeStep(t, model, shiftLeft())
		if cmd != nil {
			saves++
		}
	}

	shrunk := (resizeTreeWidth - minTree) / treeStep
	if got := model.treeWidthNow(dragWidth); got != minTree {
		t.Errorf("treeWidthNow(%d) = %d after shrinking, want the minTree %d", dragWidth, got, minTree)
	}
	if saves != shrunk || len(*calls) != shrunk {
		t.Fatalf("saves/calls = %d/%d, want %d", saves, len(*calls), shrunk)
	}

	// On the minimum the key changes nothing and saves nothing.
	model, cmd := resizeStep(t, model, shiftLeft())
	if cmd != nil {
		t.Errorf("command = %v, want nil on the minimum", cmd)
	}
	if got := model.treeWidthNow(dragWidth); got != minTree {
		t.Errorf("treeWidthNow(%d) = %d, want the minimum %d kept", dragWidth, got, minTree)
	}
	if len(*calls) != shrunk {
		t.Errorf("SaveLayout calls = %d, want no save on the minimum", len(*calls))
	}

	// Growing stops where the editor reaches its minimum.
	saves = 0
	for i := 0; i < 40; i++ {
		var grow tea.Cmd
		model, grow = resizeStep(t, model, shiftRight())
		if grow != nil {
			saves++
		}
	}

	limit := maxTreeWidth(dragWidth)
	grown := (limit - minTree) / treeStep
	if got := model.treeWidthNow(dragWidth); got != limit {
		t.Errorf("treeWidthNow(%d) = %d after growing, want the limit %d", dragWidth, got, limit)
	}
	if saves != grown || len(*calls) != shrunk+grown {
		t.Fatalf("saves/calls = %d/%d, want %d/%d", saves, len(*calls), grown, shrunk+grown)
	}
	if got := outerRect(model.editorRect).Dx(); got != dragWidth-limit {
		t.Errorf("editor frame = %d cells wide, want %d", got, dragWidth-limit)
	}

	// On the limit the key changes nothing and saves nothing.
	model, cmd = resizeStep(t, model, shiftRight())
	if cmd != nil {
		t.Errorf("command = %v, want nil on the limit", cmd)
	}
	if got := model.treeWidthNow(dragWidth); got != limit {
		t.Errorf("treeWidthNow(%d) = %d, want the limit %d kept", dragWidth, got, limit)
	}
	if len(*calls) != shrunk+grown {
		t.Errorf("SaveLayout calls = %d, want no save on the limit", len(*calls))
	}
}

func TestTreeResizeStartsFromTheAutomaticWidth(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := testDeps(t) // TreeWidth 0 leaves the tree on the automatic percentage
	deps.SaveLayout = recorder

	m := newSized(t, deps, dragWidth, dragHeight)
	if m.treeWidth != 0 {
		t.Fatalf("treeWidth = %d, want the automatic 0", m.treeWidth)
	}

	auto := dragWidth * treePercent / 100
	if got := m.treeWidthNow(dragWidth); got != auto {
		t.Fatalf("treeWidthNow(%d) = %d, want the automatic %d", dragWidth, got, auto)
	}

	model, cmd := resizeStep(t, m, shiftRight())
	if cmd == nil {
		t.Fatal("no command, want the first step saved")
	}

	want := auto + treeStep
	if got := model.treeWidthNow(dragWidth); got != want {
		t.Errorf("treeWidthNow(%d) = %d, want %d", dragWidth, got, want)
	}
	if got := model.treeWidth; got != want {
		t.Errorf("treeWidth = %d, want the explicit %d", got, want)
	}
	if len(*calls) != 1 {
		t.Fatalf("SaveLayout calls = %d, want exactly 1", len(*calls))
	}
	if got := (*calls)[0]; got.width != want || got.collapsed {
		t.Errorf("call = %+v, want {width:%d collapsed:false}", got, want)
	}
}

func TestTreeResizeIgnoresACollapsedTree(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := sizedDeps(t, config.Config{TreeWidth: resizeTreeWidth})
	deps.SaveLayout = recorder

	m := newSized(t, deps, dragWidth, dragHeight)

	model, cmd := press(t, m, toggleKey())
	model = drain(t, model, cmd)
	if !model.treeCollapsed {
		t.Fatal("treeCollapsed = false, want the tree hidden by f2")
	}
	if len(*calls) != 1 {
		t.Fatalf("SaveLayout calls = %d, want only the toggle to save", len(*calls))
	}

	for _, msg := range []tea.KeyPressMsg{shiftLeft(), shiftRight()} {
		next, resize := press(t, model, msg)
		if resize != nil {
			t.Errorf("command = %v, want nil with the tree hidden", resize)
		}
		next = drain(t, next, resize)

		if !next.treeCollapsed {
			t.Error("treeCollapsed = false, want the hidden tree kept")
		}
		if next.treeWidth != resizeTreeWidth {
			t.Errorf("treeWidth = %d, want %d kept", next.treeWidth, resizeTreeWidth)
		}
		if got := next.treeWidthNow(dragWidth); got != 0 {
			t.Errorf("treeWidthNow(%d) = %d, want 0 while collapsed", dragWidth, got)
		}

		model = next
	}
	if len(*calls) != 1 {
		t.Errorf("SaveLayout calls = %d, want the hidden tree to save nothing", len(*calls))
	}

	// The hidden tree keeps its width: showing it again restores the same split.
	model, cmd = press(t, model, toggleKey())
	model = drain(t, model, cmd)

	if model.treeCollapsed {
		t.Fatal("treeCollapsed = true, want the tree back")
	}
	if got := model.treeWidthNow(dragWidth); got != resizeTreeWidth {
		t.Errorf("treeWidthNow(%d) = %d, want %d restored", dragWidth, got, resizeTreeWidth)
	}
}

func TestTreeResizeOnlyActsInTheFilesTab(t *testing.T) {
	for _, tab := range []Tab{GitTab, DockerTab} {
		t.Run(tab.String(), func(t *testing.T) {
			recorder, calls := layoutRecorder()

			deps := sizedDeps(t, config.Config{TreeWidth: resizeTreeWidth})
			deps.SaveLayout = recorder

			m := newSized(t, deps, dragWidth, dragHeight)
			m, _ = press(t, m, tabKey(tab))

			for _, msg := range []tea.KeyPressMsg{shiftLeft(), shiftRight()} {
				model, cmd := press(t, m, msg)
				if cmd != nil {
					t.Errorf("command = %v, want nil in the %v tab", cmd, tab)
				}
				model = drain(t, model, cmd)

				if model.Tab() != tab {
					t.Errorf("Tab() = %v, want %v kept", model.Tab(), tab)
				}
				if model.treeWidth != resizeTreeWidth {
					t.Errorf("treeWidth = %d, want %d kept", model.treeWidth, resizeTreeWidth)
				}
				if got := model.treeWidthNow(dragWidth); got != resizeTreeWidth {
					t.Errorf("treeWidthNow(%d) = %d, want %d kept", dragWidth, got, resizeTreeWidth)
				}
			}
			if len(*calls) != 0 {
				t.Errorf("SaveLayout calls = %d, want the other tabs to save nothing", len(*calls))
			}
		})
	}
}

func TestTreeResizeBelongsToTheOpenDraft(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := sizedDeps(t, config.Config{TreeWidth: resizeTreeWidth})
	deps.SaveLayout = recorder

	m := newSized(t, deps, dragWidth, dragHeight)
	m = openDraft(t, m, "main.go", dragWidth)

	for _, msg := range []tea.KeyPressMsg{shiftLeft(), shiftRight()} {
		model, cmd := press(t, m, msg)
		model = drain(t, model, cmd)

		if !model.editor.CapturesInput() {
			t.Error("CapturesInput() = false, want the draft kept open")
		}
		if model.treeWidth != resizeTreeWidth {
			t.Errorf("treeWidth = %d, want %d kept", model.treeWidth, resizeTreeWidth)
		}
		if got := model.treeWidthNow(dragWidth); got != resizeTreeWidth {
			t.Errorf("treeWidthNow(%d) = %d, want %d kept", dragWidth, got, resizeTreeWidth)
		}
		if got := model.editor.Err(); got != nil {
			t.Errorf("editor.Err() = %v, want no error from a resize key", got)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("SaveLayout calls = %d, want nothing saved while editing", len(*calls))
	}
}

func TestTreeResizeIsSwallowedByTheOverlays(t *testing.T) {
	cases := []struct {
		name    string
		open    tea.KeyPressMsg
		visible func(Model) bool
	}{
		{name: "help", open: typed('?'), visible: Model.HelpVisible},
		{name: "config", open: typed(','), visible: Model.ConfigVisible},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder, calls := layoutRecorder()

			deps := sizedDeps(t, config.Config{TreeWidth: resizeTreeWidth})
			deps.SaveLayout = recorder

			m := newSized(t, deps, dragWidth, dragHeight)
			m = update(t, m, tc.open)
			if !tc.visible(m) {
				t.Fatalf("the %s overlay is closed, want it open", tc.name)
			}

			for _, msg := range []tea.KeyPressMsg{shiftLeft(), shiftRight()} {
				model, cmd := press(t, m, msg)
				if cmd != nil {
					t.Errorf("command = %v, want nil with the %s overlay open", cmd, tc.name)
				}
				model = drain(t, model, cmd)

				if !tc.visible(model) {
					t.Errorf("the %s overlay closed on a resize key", tc.name)
				}
				if model.treeWidth != resizeTreeWidth {
					t.Errorf("treeWidth = %d, want %d kept", model.treeWidth, resizeTreeWidth)
				}
				if got := model.treeWidthNow(dragWidth); got != resizeTreeWidth {
					t.Errorf("treeWidthNow(%d) = %d, want %d kept", dragWidth, got, resizeTreeWidth)
				}

				m = model
			}
			if len(*calls) != 0 {
				t.Errorf("SaveLayout calls = %d, want nothing saved by the overlay", len(*calls))
			}

			// Closing the overlay gives the key back to the layout.
			m = update(t, m, key(tea.KeyEscape))
			if tc.visible(m) {
				t.Fatalf("esc did not close the %s overlay", tc.name)
			}

			model, cmd := resizeStep(t, m, shiftRight())
			if cmd == nil {
				t.Fatal("no command, want the resize back once the overlay is closed")
			}
			if got := model.treeWidthNow(dragWidth); got != resizeTreeWidth+treeStep {
				t.Errorf("treeWidthNow(%d) = %d, want %d", dragWidth, got, resizeTreeWidth+treeStep)
			}
			if len(*calls) != 1 {
				t.Errorf("SaveLayout calls = %d, want exactly 1 after closing", len(*calls))
			}
		})
	}
}

func TestEachResizeStepSavesTheLayoutOnce(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := sizedDeps(t, config.Config{TreeWidth: resizeTreeWidth})
	deps.SaveLayout = recorder

	m := newSized(t, deps, dragWidth, dragHeight)

	want := resizeTreeWidth
	for step := 1; step <= 3; step++ {
		want += treeStep

		model, cmd := press(t, m, shiftRight())
		if cmd == nil {
			t.Fatalf("step %d: no command, want the layout saved", step)
		}
		model = drain(t, model, cmd)

		if len(*calls) != step {
			t.Fatalf("SaveLayout calls = %d after %d steps, want %d", len(*calls), step, step)
		}
		if got := (*calls)[step-1]; got.width != want || got.collapsed {
			t.Errorf("call %d = %+v, want {width:%d collapsed:false}", step, got, want)
		}
		if got := model.treeWidthNow(dragWidth); got != want {
			t.Errorf("step %d: treeWidthNow(%d) = %d, want %d", step, dragWidth, got, want)
		}

		m = model
	}
}

func TestTreeResizeWithoutASaverIsSafe(t *testing.T) {
	m := newSized(t, sizedDeps(t, config.Config{TreeWidth: resizeTreeWidth}), dragWidth, dragHeight)
	if m.saveLayout != nil {
		t.Fatal("saveLayout != nil, want none injected")
	}

	model, cmd := resizeStep(t, m, shiftRight())
	if cmd != nil {
		t.Errorf("command = %v, want nil without a saver", cmd)
	}
	if got := model.treeWidthNow(dragWidth); got != resizeTreeWidth+treeStep {
		t.Errorf("treeWidthNow(%d) = %d, want the resize applied without a saver", dragWidth, got)
	}
}

func TestHelpLinesDocumentTheLayout(t *testing.T) {
	keys := keymap.Default()

	layout := ""
	for _, line := range helpLines(keys) {
		if plain := plainCells(line); strings.HasPrefix(plain, "layout: ") {
			layout = plain
		}
	}
	if layout == "" {
		t.Fatal("helpLines() has no layout section, want the tree layout documented")
	}

	actions := []keymap.Action{keymap.ActionToggleTree, keymap.ActionTreeShrink, keymap.ActionTreeGrow}
	for _, action := range actions {
		if label := keys.Label(action); !strings.Contains(layout, label) {
			t.Errorf("layout section = %q, want the %s label %q", layout, action, label)
		}
	}

	// The panel sections keep their line.
	help := plainCells(strings.Join(helpLines(keys), "\n"))
	for _, section := range []string{"files: ", "git: ", "docker: "} {
		if !strings.Contains(help, "\n"+section) {
			t.Errorf("helpLines() = %q, want the %q section", help, section)
		}
	}
}
