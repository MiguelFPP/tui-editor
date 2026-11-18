package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
	"github.com/MiguelFPP/tui-editor/internal/tui/components/editor"
	"github.com/MiguelFPP/tui-editor/internal/tui/components/filetree"
)

func ctrlC() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
}

func ctrlS() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
}

// openDraft opens path and enters the quick edit mode through the root keymap.
func openDraft(t *testing.T, m Model, path domain.RelPath, width int) Model {
	t.Helper()

	m = openFile(t, m, path)

	next, _ := press(t, m, typed('e'))
	if !next.editor.CapturesInput() {
		t.Fatal("CapturesInput() = false after e, want the draft open")
	}

	return next
}

func TestEditorDraftOwnsTheRootKeyboard(t *testing.T) {
	m := newSized(t, testDeps(t), 200, 40)
	m = openDraft(t, m, "main.go", 200)

	// q, the numbers, the help and both tab keys must reach the draft, never the root keymap.
	for _, msg := range []tea.KeyPressMsg{typed('q'), typed('1'), typed('2'), typed('?'), key(tea.KeyTab), shiftTab()} {
		m, _ = press(t, m, msg)
	}

	if m.Tab() != FilesTab || m.HelpVisible() {
		t.Errorf("tab = %v, help = %v, want the Files tab and no help", m.Tab(), m.HelpVisible())
	}
	if !m.editor.CapturesInput() {
		t.Error("CapturesInput() = false, want the draft kept open")
	}
	if got := m.editor.Err(); got != nil {
		t.Errorf("Err() = %v, want no error from typing", got)
	}
	if !m.editor.Dirty() {
		t.Error("Dirty() = false, want the typed draft")
	}
}

func TestEditorDraftWarnsBeforeQuitting(t *testing.T) {
	m := newSized(t, testDeps(t), 200, 40)
	m = openDraft(t, m, "main.go", 200)

	// A clean draft does not block the quit shortcut.
	if _, quit := press(t, m, ctrlC()); quit == nil {
		t.Fatal("no command, want quit")
	} else if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatalf("command returned %T, want tea.QuitMsg", quit())
	}

	m = update(t, m, typed('x'))
	if !m.editor.Dirty() {
		t.Fatal("Dirty() = false after typing, want true")
	}

	next, quit := press(t, m, ctrlC())
	if quit == nil {
		t.Fatal("no command, want the toast of the unsaved draft")
	}
	if next.toast != toastPrefix+dirtyDraft {
		t.Errorf("toast = %q, want %q", next.toast, toastPrefix+dirtyDraft)
	}
	if !strings.Contains(next.View().Content, "cambios sin guardar") {
		t.Errorf("View() = %q, want the unsaved-changes toast", next.View().Content)
	}
	if !next.editor.CapturesInput() {
		t.Error("CapturesInput() = false, want the draft kept")
	}
}

func TestFileChoiceIsBlockedByTheDraft(t *testing.T) {
	m := newSized(t, testDeps(t), 200, 40)
	m = openDraft(t, m, "main.go", 200)
	m, _ = press(t, m, typed('x'))

	next, cmd := m.Update(filetree.FileChosenMsg{Path: "long.txt"})
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want app.Model", next)
	}

	if got := model.editor.View(); !strings.Contains(got, "main.go") {
		t.Errorf("the open file changed to another one: %q", got)
	}
	if !model.editor.Dirty() {
		t.Error("Dirty() = false, want the draft kept")
	}
	if model.toast != toastPrefix+dirtyDraft {
		t.Errorf("toast = %q, want %q", model.toast, toastPrefix+dirtyDraft)
	}
	if cmd == nil {
		t.Error("no command, want the toast timer")
	}
}

func TestFileChoiceWorksWithoutADraft(t *testing.T) {
	m := newSized(t, testDeps(t), 200, 40)
	m = openFile(t, m, "main.go")

	next, cmd := m.Update(filetree.FileChosenMsg{Path: "long.txt"})
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want app.Model", next)
	}
	model = drain(t, model, cmd)

	if got := model.editor.View(); !strings.Contains(got, "long.txt") {
		t.Errorf("editor = %q, want the chosen file", got)
	}
	if model.toast != "" {
		t.Errorf("toast = %q, want no notice", model.toast)
	}
}

func TestEditorSaveWritesThroughTheRoot(t *testing.T) {
	deps := testDeps(t)
	files := deps.Files.(*fakes.FileService)

	m := newSized(t, deps, 200, 40)
	m = openDraft(t, m, "main.go", 200)
	m, _ = press(t, m, typed('x'))

	model, cmd := press(t, m, ctrlS())
	if cmd == nil {
		t.Fatal("no command, want the write")
	}

	model = drain(t, model, cmd)

	if got := files.CallCount("WriteFile"); got != 1 {
		t.Fatalf("WriteFile calls = %d, want 1", got)
	}
	if model.editor.Dirty() {
		t.Error("Dirty() = true after the save, want false")
	}
	if got := model.editor.Err(); got != nil {
		t.Errorf("Err() = %v, want nil", got)
	}
	if model.toast != "" {
		t.Errorf("toast = %q, want no notice", model.toast)
	}
}

func TestFileChangedRefreshesTheGitPanel(t *testing.T) {
	deps := testDeps(t)
	git := deps.Git.(*fakes.GitService)

	m := newSized(t, deps, 120, 40)
	m = drain(t, m, m.Init())

	status, branches := git.CallCount("Status"), git.CallCount("Branches")

	next, cmd := m.Update(editor.FileChangedMsg{Path: "main.go"})
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want app.Model", next)
	}
	if cmd == nil {
		t.Fatal("no command, want the Git refresh")
	}

	model = drain(t, model, cmd)

	if got := git.CallCount("Status"); got != status+1 {
		t.Errorf("Status calls = %d, want %d", got, status+1)
	}
	if got := git.CallCount("Branches"); got != branches+1 {
		t.Errorf("Branches calls = %d, want %d", got, branches+1)
	}
}
