package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/config"
	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
	"github.com/MiguelFPP/tui-editor/internal/tui/components/filetree"
	"github.com/MiguelFPP/tui-editor/internal/tui/components/gitpanel"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

const treeLines = 30

func longFile() string {
	var out strings.Builder
	for i := 1; i <= treeLines; i++ {
		fmt.Fprintf(&out, "line-%d\n", i)
	}

	return out.String()
}

func testDeps(t *testing.T) Deps {
	t.Helper()

	files := fakes.NewFileService()
	files.OnReadDir = func(_ context.Context, dir domain.RelPath) (domain.DirListing, error) {
		if dir != "" {
			return domain.DirListing{Dir: dir}, nil
		}

		return domain.DirListing{Dir: dir, Entries: []domain.FileNode{
			{Name: "main.go", Path: "main.go", Kind: domain.File},
			{Name: "long.txt", Path: "long.txt", Kind: domain.File},
		}}, nil
	}
	files.OnReadFile = func(_ context.Context, path domain.RelPath) (domain.FileContent, error) {
		if path == "long.txt" {
			return domain.FileContent{Path: path, Text: longFile()}, nil
		}

		return domain.FileContent{Path: path, Text: "package main\n"}, nil
	}
	files.OnWriteFile = func(_ context.Context, _ domain.RelPath, content string, _ domain.FileStamp) (domain.FileStamp, error) {
		return domain.FileStamp{Size: int64(len(content)), ModTime: time.Now()}, nil
	}

	git := fakes.NewGitService()
	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		return domain.GitStatus{Branch: "main"}, nil
	}
	git.OnBranches = func(context.Context) ([]domain.Branch, error) {
		return []domain.Branch{{Name: "main", Current: true}}, nil
	}

	editors := fakes.NewEditorService()
	editors.OnResolve = func(_ context.Context, target domain.EditorTarget) (domain.EditorCommand, error) {
		return domain.EditorCommand{Binary: "vim", Args: []string{target.Path.String()}}, nil
	}

	docker := fakes.NewDockerService()
	docker.OnContainers = func(context.Context, domain.ContainerScope) ([]domain.ContainerSummary, error) {
		return []domain.ContainerSummary{
			{ID: "aaaaaaaaaaaaaaaa", Name: "web", Image: "nginx:latest", State: domain.ContainerRunning},
		}, nil
	}

	deps := Deps{
		Config: config.Config{Repo: t.TempDir(), Editor: "vim"},
		Files:  files,
		Git:    git,
		Editor: editors,
		Docker: docker,
		Keys:   keymap.Default(),
	}

	return deps
}

func newSized(t *testing.T, deps Deps, width, height int) Model {
	t.Helper()

	return update(t, New(deps), tea.WindowSizeMsg{Width: width, Height: height})
}

// update feeds one message and returns the concrete root model.
func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()

	next, _ := m.Update(msg)
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want app.Model", next)
	}

	return model
}

// drain runs one command tree, feeding its messages back like the runtime does.
func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		return m
	}

	return applyMsg(t, m, cmd())
}

func applyMsg(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()

	if msg == nil {
		return m
	}

	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, nested := range batch {
			if nested == nil {
				continue
			}

			m = applyMsg(t, m, nested())
		}

		return m
	}

	return update(t, m, msg)
}

func openFile(t *testing.T, m Model, path domain.RelPath) Model {
	t.Helper()

	next, cmd := m.Update(filetree.FileChosenMsg{Path: path})
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want app.Model", next)
	}
	if !model.hasEditor {
		t.Fatal("hasEditor = false after choosing a file")
	}

	return drain(t, model, cmd)
}

func typed(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func shiftTab() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
}

func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func wheel(button tea.MouseButton, x, y int) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: x, Y: y, Button: button}
}

// press applies one key and returns the model with its command still to run.
func press(t *testing.T, m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want app.Model", next)
	}

	return model, cmd
}

// stagedGit turns the Git fake into a repository with one modified file.
func stagedGit(t *testing.T, deps Deps) *fakes.GitService {
	t.Helper()

	git := deps.Git.(*fakes.GitService)
	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		return domain.GitStatus{Branch: "main", Files: []domain.GitFileStatus{
			{Path: "main.go", Index: domain.GitModified, Worktree: domain.GitUnmodified},
		}}, nil
	}

	return git
}

func TestNewKeepsConfigAndDefaults(t *testing.T) {
	deps := testDeps(t)
	m := New(deps)

	if !reflect.DeepEqual(m.cfg, deps.Config) {
		t.Errorf("cfg = %+v, want %+v", m.cfg, deps.Config)
	}
	if m.Tab() != FilesTab || m.Focus() != FocusTree {
		t.Errorf("tab/focus = %v/%v, want Files/tree", m.Tab(), m.Focus())
	}
	if m.HelpVisible() {
		t.Error("HelpVisible() = true on a fresh model, want false")
	}
	if m.hasEditor {
		t.Error("hasEditor = true on a fresh model, want false")
	}
}

func TestInitReturnsACommand(t *testing.T) {
	if cmd := New(testDeps(t)).Init(); cmd == nil {
		t.Fatal("Init() = nil, want the load and spinner commands")
	}
}

func TestModelWindowSize(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)

	if m.width != 120 || m.height != 40 {
		t.Fatalf("size = %dx%d, want 120x40", m.width, m.height)
	}
	if m.treeRect.Dx() == 0 || m.editorRect.Dx() == 0 {
		t.Errorf("panel rects = %v / %v, want both non-empty", m.treeRect, m.editorRect)
	}
	if got, want := m.gitRect.Dx()+2*borderSide, 120; got != want {
		t.Errorf("git frame width = %d, want the full width %d", got, want)
	}
	if m.treeRect.Max.Y > m.height {
		t.Errorf("tree rect %v exceeds the canvas", m.treeRect)
	}
}

func TestQuitKeys(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{name: "q", msg: typed('q')},
		{name: "ctrl+c", msg: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, cmd := New(testDeps(t)).Update(tc.msg)
			if _, ok := next.(Model); !ok {
				t.Fatalf("Update returned %T, want app.Model", next)
			}
			if cmd == nil {
				t.Fatal("no command, want quit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("command returned %T, want tea.QuitMsg", cmd())
			}
		})
	}
}

// withKeys returns the default registry with the overrides applied on top of it.
func withKeys(t *testing.T, overrides map[string][]string) keymap.Registry {
	t.Helper()

	keys, err := keymap.Default().Apply(overrides)
	if err != nil {
		t.Fatalf("Apply(%v) = %v, want no error", overrides, err)
	}

	return keys
}

func TestDefaultKeymapIsUsedWithoutOne(t *testing.T) {
	deps := testDeps(t)
	deps.Keys = keymap.Registry{}

	m := newSized(t, deps, 120, 40)
	m = update(t, m, typed('2'))

	if m.Tab() != GitTab {
		t.Errorf("Tab() = %v, want the built-in keymap to switch the tab", m.Tab())
	}
}

// TestHelpAndStatusbarShowTheEffectiveKeys checks that the help overlay and the statusbar render the
// keys of the injected keymap, never the built-in ones.
func TestHelpAndStatusbarShowTheEffectiveKeys(t *testing.T) {
	deps := testDeps(t)
	deps.Keys = withKeys(t, map[string][]string{"help": {"h"}, "quit": {"x"}, "tab_git": {"g"}})

	m := newSized(t, deps, 200, 40)
	m = update(t, m, typed('h'))
	if !m.HelpVisible() {
		t.Fatal("HelpVisible() = false, want the reassigned key to open the help")
	}

	view := plainCells(m.View().Content)
	for _, want := range []string{
		"h  abrir la ayuda",
		"x / ctrl+c  salir de la aplicación",
		"g  pestaña de Git",
		"esc  cerrar la ayuda o reenviarla al panel",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
	if strings.Contains(view, "?  abrir la ayuda") {
		t.Errorf("View() = %q, want the default help key replaced", view)
	}

	m = update(t, m, key(tea.KeyEscape))
	if m.HelpVisible() {
		t.Fatal("HelpVisible() = true, want esc to close the help")
	}

	status := plainCells(m.View().Content)
	for _, want := range []string{"1/g/3 tabs", "tab foco", "h ayuda", "x / ctrl+c salir"} {
		if !strings.Contains(status, want) {
			t.Errorf("View() = %q, want %q", status, want)
		}
	}
}

func TestTabsSwitchWithTheOverriddenKey(t *testing.T) {
	deps := testDeps(t)
	deps.Keys = withKeys(t, map[string][]string{"tab_git": {"g"}})

	m := newSized(t, deps, 120, 40)
	m = update(t, m, typed('g'))
	if m.Tab() != GitTab || m.Focus() != FocusGit {
		t.Errorf("tab/focus = %v/%v after g, want Git/git", m.Tab(), m.Focus())
	}

	// The default key is replaced, not added.
	fresh := newSized(t, deps, 120, 40)
	fresh = update(t, fresh, typed('2'))
	if fresh.Tab() != FilesTab {
		t.Errorf("Tab() = %v after 2, want the default key replaced", fresh.Tab())
	}
}

func TestQuitKeyCanBeReassigned(t *testing.T) {
	deps := testDeps(t)
	deps.Keys = withKeys(t, map[string][]string{"quit": {"x"}})

	m := newSized(t, deps, 120, 40)

	cmd, stop := m.handleKey(typed('x'))
	if !stop {
		t.Fatal("x did not quit, want the reassigned key to quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("command returned %T, want tea.QuitMsg", cmd())
	}

	// q is no longer a quit key: it falls through to the focused panel.
	if _, stop := m.handleKey(typed('q')); stop {
		t.Error("q still quits, want the default key replaced")
	}

	// ctrl+c is fixed: reassigning quit never disables it.
	cmd, stop = m.handleKey(ctrlC())
	if !stop {
		t.Fatal("ctrl+c did not quit, want the fixed key always active")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("command returned %T, want tea.QuitMsg", cmd())
	}
}

func TestDraftCapturesAReassignedQuitKey(t *testing.T) {
	deps := testDeps(t)
	deps.Keys = withKeys(t, map[string][]string{"quit": {"x"}})

	m := newSized(t, deps, 120, 40)
	m = openDraft(t, m, "main.go", 120)

	// ctrl+c stays fixed: it quits while the draft is still clean.
	if _, stop := m.handleKey(ctrlC()); !stop {
		t.Error("ctrl+c did not quit, want the fixed key always active")
	}

	// The reassigned key is written into the draft instead of leaving the program.
	if _, stop := m.handleKey(typed('x')); stop {
		t.Error("x quit the program, want the draft to capture the reassigned key")
	}
	if !m.editor.CapturesInput() {
		t.Error("CapturesInput() = false, want the draft kept open")
	}
	if got := m.editor.Err(); got != nil {
		t.Errorf("Err() = %v, want no error from typing", got)
	}
	if !m.editor.Dirty() {
		t.Error("Dirty() = false, want the reassigned key typed into the draft")
	}
}

func TestTabsSwitchWithTheNumberKeys(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)

	m = update(t, m, typed('2'))
	if m.Tab() != GitTab {
		t.Fatalf("Tab() = %v, want Git", m.Tab())
	}
	if m.Focus() != FocusGit {
		t.Errorf("Focus() = %v, want git", m.Focus())
	}

	m = update(t, m, typed('1'))
	if m.Tab() != FilesTab {
		t.Fatalf("Tab() = %v, want Files", m.Tab())
	}
	if m.Focus() != FocusTree {
		t.Errorf("Focus() = %v, want tree", m.Focus())
	}
}

func TestTabsRenderBothLabels(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)

	view := m.View().Content
	for _, want := range []string{"Files", "Git"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
}

func TestFocusTabCyclesTreeAndEditor(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = openFile(t, m, "main.go")

	if m.Focus() != FocusEditor {
		t.Fatalf("Focus() = %v, want editor after choosing a file", m.Focus())
	}

	m = update(t, m, key(tea.KeyTab))
	if m.Focus() != FocusTree {
		t.Errorf("tab: Focus() = %v, want tree", m.Focus())
	}

	m = update(t, m, key(tea.KeyTab))
	if m.Focus() != FocusEditor {
		t.Errorf("tab again: Focus() = %v, want editor", m.Focus())
	}

	m = update(t, m, shiftTab())
	if m.Focus() != FocusTree {
		t.Errorf("shift+tab: Focus() = %v, want tree", m.Focus())
	}
}

func TestFocusTabWithoutEditorStaysInTheTree(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)

	m = update(t, m, key(tea.KeyTab))
	if m.Focus() != FocusTree {
		t.Errorf("Focus() = %v, want tree without an editor", m.Focus())
	}
}

func TestFocusTabInGitIsForwardedToThePanel(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = drain(t, m, m.Init())
	m = update(t, m, typed('2'))

	if got := m.git.Section(); got != gitpanel.Changes {
		t.Fatalf("Section() = %v, want the first section", got)
	}

	m = update(t, m, key(tea.KeyTab))
	if got := m.git.Section(); got == gitpanel.Changes {
		t.Error("tab in the Git tab did not move the section")
	}
	if m.Tab() != GitTab {
		t.Errorf("Tab() = %v, want the tab unchanged", m.Tab())
	}
}

func TestMouseClickChangesFocusInFiles(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = openFile(t, m, "main.go")

	m = update(t, m, click(5, 5))
	if m.Focus() != FocusTree {
		t.Fatalf("click on the tree: Focus() = %v, want tree", m.Focus())
	}

	m = update(t, m, click(60, 5))
	if m.Focus() != FocusEditor {
		t.Fatalf("click on the editor: Focus() = %v, want editor", m.Focus())
	}
}

func TestMouseClickOutsideThePanelsKeepsTheFocus(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = openFile(t, m, "main.go")

	m = update(t, m, click(5, 0))
	if m.Focus() != FocusEditor {
		t.Errorf("Focus() = %v, want editor kept on a header click", m.Focus())
	}
}

func TestMouseClickInGitFocusesGit(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = update(t, m, typed('2'))

	m = update(t, m, click(10, 5))
	if m.Focus() != FocusGit {
		t.Errorf("Focus() = %v, want git", m.Focus())
	}
}

func TestMouseWheelScrollsThePanelUnderTheCursor(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 12)
	m = openFile(t, m, "long.txt")

	if content := m.View().Content; !strings.Contains(content, "line-1") {
		t.Fatalf("View() = %q, want the first line before scrolling", content)
	}

	m = update(t, m, wheel(tea.MouseWheelDown, 60, 5))
	if content := m.View().Content; !strings.Contains(content, "line-4") {
		t.Errorf("View() = %q, want the scrolled body", content)
	}
}

func TestMouseIsIgnoredWhileTheHelpIsOpen(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = openFile(t, m, "main.go")
	m = update(t, m, typed('?'))

	m = update(t, m, click(5, 5))
	if m.Focus() != FocusEditor {
		t.Errorf("Focus() = %v, want editor kept while the help is open", m.Focus())
	}
}

func TestHelpTogglesWithTheQuestionMark(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)

	m = update(t, m, typed('?'))
	if !m.HelpVisible() {
		t.Fatal("HelpVisible() = false, want true")
	}

	view := m.View().Content
	for _, want := range []string{
		"tui-editor · atajos", "1  pestaña de archivos", "tab  siguiente panel",
		"esc  cerrar la ayuda", ",  ver la configuración",
	} {
		if !strings.Contains(plainCells(view), want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}

	m = update(t, m, typed('?'))
	if m.HelpVisible() {
		t.Error("HelpVisible() = true, want the second ? to close it")
	}
}

func TestHelpClosesWithEscape(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = update(t, m, typed('?'))

	m = update(t, m, key(tea.KeyEscape))
	if m.HelpVisible() {
		t.Error("HelpVisible() = true, want esc to close help")
	}
}

func TestHelpSwallowsOtherKeys(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = openFile(t, m, "main.go")
	m = update(t, m, typed('?'))

	m = update(t, m, typed('2'))
	if m.Tab() != FilesTab || !m.HelpVisible() {
		t.Errorf("tab/help changed while the help was open: tab=%v help=%v", m.Tab(), m.HelpVisible())
	}
}

func TestDefaultKeymapIncludesConfig(t *testing.T) {
	if action, ok := keymap.Default().Match(keymap.ScopeRoot, ","); !ok || action != keymap.ActionConfig {
		t.Errorf("Match(root, %q) = (%q, %v), want (%q, true)", ",", action, ok, keymap.ActionConfig)
	}
}

func TestConfigOverlayOpensClosesAndShowsData(t *testing.T) {
	deps := testDeps(t)
	deps.Config.ConfigPath = "/tmp/tui-editor/config.json"
	deps.Config.Keys = map[string][]string{"tree_down": {"n"}, "docker_logs": {"o"}}

	m := New(deps)

	m, _ = press(t, m, typed(','))
	if !m.ConfigVisible() {
		t.Fatal("ConfigVisible() = false, want the , key to open the overlay")
	}

	view := plainCells(m.View().Content)
	for _, want := range []string{"/tmp/tui-editor/config.json", "overrides: 2", "tree_down", "docker_logs", "reinicia"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}

	m, _ = press(t, m, key(tea.KeyEscape))
	if m.ConfigVisible() {
		t.Fatal("ConfigVisible() = true, want esc to close the overlay")
	}
}

func TestConfigOverlayWithoutFile(t *testing.T) {
	deps := testDeps(t)
	deps.Config.ConfigPath = ""
	deps.Config.Keys = nil

	m := New(deps)

	m, _ = press(t, m, typed(','))
	if !m.ConfigVisible() {
		t.Fatal("ConfigVisible() = false, want the overlay open")
	}

	view := plainCells(m.View().Content)
	for _, want := range []string{"sin archivo (defaults)", "sin overrides"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
}

func TestConfigOverlayIgnoresMouse(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = openFile(t, m, "main.go")

	m, _ = press(t, m, typed(','))
	if !m.ConfigVisible() {
		t.Fatal("ConfigVisible() = false, want the overlay open")
	}

	m = update(t, m, click(tabMiddle(m, DockerTab), tabRow))
	if m.Tab() != FilesTab || m.Focus() != FocusEditor {
		t.Errorf("tab/focus = %v/%v, want Files/editor kept while the overlay is open", m.Tab(), m.Focus())
	}

	m = update(t, m, wheel(tea.MouseWheelDown, m.editorRect.Min.X+2, m.editorRect.Min.Y+2))
	if m.Focus() != FocusEditor {
		t.Errorf("Focus() = %v, want editor kept on a wheel while the overlay is open", m.Focus())
	}
}

func TestConfigActionCanBeReassigned(t *testing.T) {
	deps := testDeps(t)
	// f2 already belongs to the tree toggle of the default keymap.
	deps.Keys = withKeys(t, map[string][]string{"config": {"F3"}})

	m := newSized(t, deps, 120, 40)
	m, _ = press(t, m, key(tea.KeyF3))
	if !m.ConfigVisible() {
		t.Fatal("ConfigVisible() = false, want the reassigned F3 to open the overlay")
	}

	// The default key is replaced, not added.
	fresh := newSized(t, deps, 120, 40)
	fresh, _ = press(t, fresh, typed(','))
	if fresh.ConfigVisible() {
		t.Error("ConfigVisible() = true after ,, want the default key replaced")
	}
}

func TestEditorPlaceholderBeforeChoosingAFile(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)

	if view := m.View().Content; !strings.Contains(view, "selecciona un archivo") {
		t.Errorf("View() = %q, want the placeholder", view)
	}
}

func TestChoosingAFileOpensTheEditor(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = openFile(t, m, "main.go")

	view := m.View().Content
	if strings.Contains(view, "selecciona un archivo") {
		t.Errorf("View() = %q, want the editor instead of the placeholder", view)
	}
	if !strings.Contains(view, "main.go") {
		t.Errorf("View() = %q, want the open path", view)
	}
	if !strings.Contains(view, "1 línea") {
		t.Errorf("View() = %q, want the loaded preview", view)
	}
}

func TestStatusbarShowsRepoAndHints(t *testing.T) {
	deps := testDeps(t)
	m := newSized(t, deps, 200, 40)

	view := m.View().Content
	for _, want := range []string{deps.Config.Repo, "tab:", "foco:", "?", "q"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
}

func TestGitStatusMessageReachesThePanel(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = drain(t, m, m.Init())
	m = update(t, m, typed('2'))

	if view := m.View().Content; !strings.Contains(view, "rama main") {
		t.Errorf("View() = %q, want the Git status routed to the panel", view)
	}
}

func TestStatusbarToastShowsTheFirstError(t *testing.T) {
	deps := testDeps(t)
	deps.Git.(*fakes.GitService).OnStatus = func(context.Context) (domain.GitStatus, error) {
		return domain.GitStatus{}, domain.ErrNotRepository
	}

	m := newSized(t, deps, 200, 40)
	m = drain(t, m, m.Init())

	view := m.View().Content
	if !strings.Contains(view, toastPrefix) {
		t.Fatalf("View() = %q, want the error toast", view)
	}
	if !strings.Contains(view, "no es un repositorio") {
		t.Errorf("View() = %q, want the error detail", view)
	}
	if got := strings.Count(view, toastPrefix); got != 1 {
		t.Errorf("toast count = %d, want 1 (no duplicates)", got)
	}
}

func TestToastExpiresOnlyWithTheCurrentSequence(t *testing.T) {
	deps := testDeps(t)
	deps.Git.(*fakes.GitService).OnStatus = func(context.Context) (domain.GitStatus, error) {
		return domain.GitStatus{}, domain.ErrNotRepository
	}

	m := newSized(t, deps, 200, 40)
	m = drain(t, m, m.Init())

	if !strings.Contains(m.View().Content, toastPrefix) {
		t.Fatal("no toast after the error")
	}

	m = update(t, m, toastExpiredMsg{seq: m.toastSeq + 1})
	if !strings.Contains(m.View().Content, toastPrefix) {
		t.Error("a stale timer cleared the toast")
	}

	m = update(t, m, toastExpiredMsg{seq: m.toastSeq})
	if strings.Contains(m.View().Content, toastPrefix) {
		t.Error("the current timer did not clear the toast")
	}
}

func TestViewConfiguresAltScreenAndMouse(t *testing.T) {
	view := newSized(t, testDeps(t), 120, 40).View()

	if !view.AltScreen {
		t.Error("AltScreen = false, want true")
	}
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("MouseMode = %v, want cell motion", view.MouseMode)
	}
	if !strings.Contains(view.Content, "tui-editor") {
		t.Errorf("View() = %q, want the program name", view.Content)
	}
}

func TestLayoutFitsTheCanvas(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)

	content := m.View().Content
	if got, want := strings.Count(content, "\n")+1, 40; got != want {
		t.Errorf("rendered %d rows, want %d", got, want)
	}
}

func TestGitCommitModalOwnsTheKeyboard(t *testing.T) {
	deps := testDeps(t)
	m := newSized(t, deps, 120, 40)
	m = drain(t, m, m.Init())
	m = update(t, m, typed('2'))

	m, cmd := press(t, m, typed('c'))
	if cmd == nil {
		t.Fatal("no command, want the editor focus")
	}
	if !m.git.CapturesInput() {
		t.Fatal("CapturesInput() = false after c, want the modal open")
	}

	// q, 1 and ? must reach the editor instead of the root keymap.
	for _, msg := range []tea.KeyPressMsg{typed('q'), typed('1'), typed('?'), typed('2')} {
		m, _ = press(t, m, msg)
	}
	if m.Tab() != GitTab || m.HelpVisible() {
		t.Errorf("tab = %v, help = %v, want the Git tab and no help", m.Tab(), m.HelpVisible())
	}
	if !m.git.CapturesInput() {
		t.Error("CapturesInput() = false, want the modal kept open")
	}
	if got := m.git.Err(); got != nil {
		t.Errorf("Err() = %v, want no error from typing", got)
	}

	if _, quit := press(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); quit == nil {
		t.Fatal("no command, want quit")
	} else if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatalf("command returned %T, want tea.QuitMsg", quit())
	}
}

func TestGitPromptOwnsTheKeyboard(t *testing.T) {
	deps := testDeps(t)
	m := newSized(t, deps, 120, 40)
	m = drain(t, m, m.Init())
	m = update(t, m, typed('2'))
	m = update(t, m, key(tea.KeyTab))
	m = update(t, m, key(tea.KeyTab))

	model, cmd := press(t, m, typed('n'))
	if cmd == nil {
		t.Fatal("no command, want the focus of the prompt")
	}
	if !model.git.CapturesInput() {
		t.Fatal("CapturesInput() = false after n, want the prompt open")
	}

	// q, 1 and ? must reach the prompt instead of the root keymap.
	for _, msg := range []tea.KeyPressMsg{typed('q'), typed('1'), typed('?'), typed('2')} {
		model, _ = press(t, model, msg)
	}
	if model.Tab() != GitTab || model.HelpVisible() {
		t.Errorf("tab = %v, help = %v, want the Git tab and no help", model.Tab(), model.HelpVisible())
	}
	if !model.git.CapturesInput() {
		t.Error("CapturesInput() = false, want the prompt kept open")
	}
	if got := model.git.Err(); got != nil {
		t.Errorf("Err() = %v, want no error from typing", got)
	}

	if _, quit := press(t, model, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); quit == nil {
		t.Fatal("no command, want quit")
	} else if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatalf("command returned %T, want tea.QuitMsg", quit())
	}
}

func TestGitStageAndCommitThroughTheRoot(t *testing.T) {
	deps := testDeps(t)
	git := stagedGit(t, deps)
	committed := ""
	git.OnCommit = func(_ context.Context, message string) error {
		committed = message

		return nil
	}

	m := newSized(t, deps, 120, 40)
	m = drain(t, m, m.Init())
	m = update(t, m, typed('2'))

	model, cmd := press(t, m, typed('s'))
	model = drain(t, model, cmd)
	if got := git.CallCount("Stage"); got != 1 {
		t.Fatalf("Stage calls = %d, want 1", got)
	}
	if model.git.CapturesInput() {
		t.Error("CapturesInput() = true, want the s key to stage a file")
	}

	model, _ = press(t, model, typed('c'))
	for _, r := range "hola" {
		model, _ = press(t, model, typed(r))
	}

	next, cmd := press(t, model, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("no command, want the commit")
	}

	next = drain(t, next, cmd)
	if committed != "hola" {
		t.Errorf("Commit message = %q, want %q", committed, "hola")
	}
	if next.git.CapturesInput() {
		t.Error("CapturesInput() = true after the commit, want the modal closed")
	}

	// The tab switching keys work again once the modal is closed.
	next = update(t, next, typed('1'))
	if next.Tab() != FilesTab {
		t.Errorf("Tab() = %v, want the root shortcuts back", next.Tab())
	}
}

func TestMouseClickOnTheEditorIsRebasedOntoThePanel(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = openFile(t, m, "long.txt") // enough lines for an absolute click to miss the row
	m = update(t, m, typed('e'))

	if !m.editor.CapturesInput() {
		t.Fatal("CapturesInput() = false, want the draft")
	}

	// An absolute click on the second body row must land there, not two rows below it.
	m = update(t, m, click(m.editorRect.Min.X+2, m.editorRect.Min.Y+2))

	if got := m.Focus(); got != FocusEditor {
		t.Errorf("Focus() = %v, want the editor", got)
	}
	if got := m.editor.CaretLine(); got != 1 {
		t.Errorf("CaretLine() = %d, want row 1 of the draft", got)
	}
}

func TestMouseWheelOnTheEditorKeepsWorkingWithoutCoordinates(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = openFile(t, m, "main.go")
	m = update(t, m, typed('e'))

	m = update(t, m, wheel(tea.MouseWheelDown, m.editorRect.Min.X+2, m.editorRect.Min.Y+2))

	if got := m.Focus(); got != FocusEditor {
		t.Errorf("Focus() = %v, want the editor", got)
	}
}

// sizedDeps returns the standard deps with the persisted tree layout applied.
func sizedDeps(t *testing.T, cfg config.Config) Deps {
	t.Helper()

	deps := testDeps(t)
	deps.Config.TreeWidth, deps.Config.TreeCollapsed = cfg.TreeWidth, cfg.TreeCollapsed

	return deps
}

// layoutCall is one recorded call to the injected SaveLayout.
type layoutCall struct {
	width     int
	collapsed bool
}

// layoutRecorder returns a SaveLayout stub and the calls it records.
func layoutRecorder() (func(int, bool) error, *[]layoutCall) {
	calls := make([]layoutCall, 0, 4)

	return func(width int, collapsed bool) error {
		calls = append(calls, layoutCall{width: width, collapsed: collapsed})

		return nil
	}, &calls
}

// tabRowText returns the plain text of the rendered row of the top labels.
func tabRowText(t *testing.T, m Model) string {
	t.Helper()

	rows := strings.Split(m.View().Content, "\n")
	if len(rows) <= tabRow {
		t.Fatalf("rendered %d rows, want the tab row at %d", len(rows), tabRow)
	}

	return plainCells(rows[tabRow])
}

func TestNewWiresTheLayoutSaver(t *testing.T) {
	recorder, calls := layoutRecorder()

	deps := testDeps(t)
	deps.SaveLayout = recorder

	m := New(deps)
	if m.saveLayout == nil {
		t.Fatal("saveLayout = nil, want the injected saver")
	}
	if err := m.saveLayout(42, true); err != nil {
		t.Fatalf("saveLayout(42, true) = %v, want no error", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("saveLayout calls = %d, want 1", len(*calls))
	}
	if got := (*calls)[0]; got.width != 42 || !got.collapsed {
		t.Errorf("call = %+v, want {width:42 collapsed:true}", got)
	}

	// A test or an embed without a config file passes no saver at all.
	if plain := New(testDeps(t)); plain.saveLayout != nil {
		t.Error("saveLayout != nil, want nil without an injected saver")
	}
}

func TestLayoutStartsFromTheConfig(t *testing.T) {
	cases := []struct {
		name      string
		width     int
		treeWidth int
		want      int
	}{
		{name: "explicit", width: 120, treeWidth: 42, want: 42},
		{name: "explicit ignores the percentage", width: 200, treeWidth: 42, want: 42},
		{name: "automatic", width: 120, treeWidth: 0, want: 42},
		{name: "automatic on a wide canvas", width: 200, treeWidth: 0, want: 70},
		{name: "below the minimum", width: 120, treeWidth: 5, want: minTree},
		{name: "above the maximum", width: 120, treeWidth: 9999, want: 120 - minEditorBox},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newSized(t, sizedDeps(t, config.Config{TreeWidth: tc.treeWidth}), tc.width, 40)

			if got := m.treeWidthNow(tc.width); got != tc.want {
				t.Errorf("treeWidthNow(%d) = %d, want %d", tc.width, got, tc.want)
			}
			if got := outerRect(m.treeRect).Dx(); got != tc.want {
				t.Errorf("tree frame = %d cells wide, want %d", got, tc.want)
			}
			if got, want := m.editorRect.Min.X, tc.want+borderSide; got != want {
				t.Errorf("editorRect.Min.X = %d, want %d right of the tree", got, want)
			}
		})
	}
}

func TestCollapsedTreeHandsTheCanvasToTheEditor(t *testing.T) {
	m := newSized(t, sizedDeps(t, config.Config{TreeCollapsed: true}), 120, 40)

	if m.treeRect != (image.Rectangle{}) {
		t.Errorf("treeRect = %v, want it empty while the tree is collapsed", m.treeRect)
	}
	if m.splitterRect != (image.Rectangle{}) {
		t.Errorf("splitterRect = %v, want it empty while the tree is collapsed", m.splitterRect)
	}
	if got := m.treeWidthNow(120); got != 0 {
		t.Errorf("treeWidthNow(120) = %d, want 0", got)
	}
	if m.canSplit() {
		t.Error("canSplit() = true, want false while the tree is collapsed")
	}
	if got := outerRect(m.editorRect).Dx(); got != 120 {
		t.Errorf("editor frame = %d cells wide, want the whole canvas 120", got)
	}
	if got := m.Focus(); got != FocusEditor {
		t.Errorf("Focus() = %v, want the editor with a collapsed tree", got)
	}
}

func TestSplitterCoversTheSharedBorder(t *testing.T) {
	m := newSized(t, sizedDeps(t, config.Config{TreeWidth: 42}), 120, 40)
	treeW := m.treeWidthNow(120)

	if !m.canSplit() {
		t.Fatal("canSplit() = false, want a splitter in the Files tab")
	}
	if got, want := m.splitterRect.Min.X, treeW-1; got != want {
		t.Errorf("splitterRect.Min.X = %d, want %d", got, want)
	}
	if got, want := m.splitterRect.Max.X, treeW+1; got != want {
		t.Errorf("splitterRect.Max.X = %d, want %d", got, want)
	}
	if got, want := m.splitterRect.Dx(), 2; got != want {
		t.Errorf("splitterRect = %d cells wide, want %d", got, want)
	}
	if got, want := m.splitterRect.Min.Y, headerRows; got != want {
		t.Errorf("splitterRect.Min.Y = %d, want the first body row %d", got, want)
	}
	if got, want := m.splitterRect.Max.Y, headerRows+bodyHeight(40); got != want {
		t.Errorf("splitterRect.Max.Y = %d, want the end of the body %d", got, want)
	}
}

func TestTreeButtonBelongsToTheFilesTab(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)

	if m.collapseRect.Dx() == 0 {
		t.Fatal("collapseRect is empty in the Files tab, want the tree button")
	}
	if got, want := m.collapseRect.Max.X, 120; got != want {
		t.Errorf("button ends at column %d, want the canvas edge %d", got, want)
	}
	if got := m.collapseRect.Min.X; got < m.tabRects[DockerTab].Max.X {
		t.Errorf("button starts at column %d, want it after the last tab", got)
	}
	if got := m.collapseRect.Min.Y; got != tabRow {
		t.Errorf("button row = %d, want the tab row %d", got, tabRow)
	}
	if row := tabRowText(t, m); !strings.HasSuffix(row, "[<]") {
		t.Errorf("tab row = %q, want the button of an expanded tree", row)
	}

	// The other tabs have no tree to resize.
	for _, tab := range []Tab{GitTab, DockerTab} {
		other, _ := press(t, newSized(t, testDeps(t), 120, 40), tabKey(tab))
		if other.collapseRect != (image.Rectangle{}) {
			t.Errorf("collapseRect with %v = %v, want it empty", tab, other.collapseRect)
		}
	}
}

func TestTreeButtonFollowsTheCollapsedState(t *testing.T) {
	collapsed := newSized(t, sizedDeps(t, config.Config{TreeCollapsed: true}), 120, 40)
	if row := tabRowText(t, collapsed); !strings.HasSuffix(row, "[>]") {
		t.Errorf("tab row = %q, want the button of a hidden tree", row)
	}

	if got := collapsed.collapseRect.Dx(); got == 0 {
		t.Error("collapseRect is empty while collapsed, want the button to bring the tree back")
	}
}

func TestTreeButtonOnTheMinimumCanvas(t *testing.T) {
	m := newSized(t, testDeps(t), minWidth, minHeight)

	// A narrow canvas may drop the button, but the row keeps its exact width either way.
	if got := lipgloss.Width(tabRowText(t, m)); got != minWidth {
		t.Errorf("tab row = %d cells, want %d", got, minWidth)
	}
	if m.collapseRect.Dx() > 0 && m.collapseRect.Min.X < m.tabRects[DockerTab].Max.X {
		t.Errorf("collapseRect = %v, want it right of the last tab", m.collapseRect)
	}

	// The button never overlaps the labels.
	if rect := m.collapseBounds(m.tabLabelsWidth()); rect != (image.Rectangle{}) {
		t.Errorf("collapseBounds(%d) = %v, want it empty without room", m.tabLabelsWidth(), rect)
	}
}

func TestCollapsedRenderFillsTheCanvas(t *testing.T) {
	for _, size := range canvasSizes {
		t.Run(size.name, func(t *testing.T) {
			m := newSized(t, sizedDeps(t, config.Config{TreeCollapsed: true}), size.width, size.height)
			content := m.View().Content

			rows := strings.Split(content, "\n")
			if len(rows) != size.height {
				t.Fatalf("rendered %d rows, want %d", len(rows), size.height)
			}
			for i, row := range rows {
				width := lipgloss.Width(row)
				if width > size.width {
					t.Errorf("row %d = %d cells, want at most %d", i, width, size.width)
				}
				if i >= headerRows && width != size.width {
					t.Errorf("row %d = %d cells, want the full width %d", i, width, size.width)
				}
			}

			// The editor is the only box: it covers the body from end to end.
			body := bodyHeight(size.height)
			outer := outerRect(m.editorRect)
			if outer.Min.X != 0 || outer.Max.X != size.width {
				t.Errorf("editor frame spans columns %d..%d, want 0..%d", outer.Min.X, outer.Max.X, size.width)
			}
			if outer.Min.Y != headerRows || outer.Max.Y != headerRows+body {
				t.Errorf("editor frame spans rows %d..%d, want the body %d..%d", outer.Min.Y, outer.Max.Y, headerRows, headerRows+body)
			}
			if inner := m.treeRect; inner.Dx() != 0 || inner.Dy() != 0 {
				t.Errorf("tree inner area = %v, want it empty while collapsed", inner)
			}

			bottom := headerRows + body - 1
			top := rowCells(t, content, headerRows)
			assertGlyph(t, top, 0, '╭')
			assertGlyph(t, top, size.width-1, '╮')
			assertGlyph(t, rowCells(t, content, bottom), 0, '╰')
			assertGlyph(t, rowCells(t, content, bottom), size.width-1, '╯')
		})
	}
}

func TestLayoutSaveErrorShowsTheToast(t *testing.T) {
	m := newSized(t, testDeps(t), 200, 40)

	const detail = "no se pudo guardar el layout"
	m = update(t, m, layoutSavedMsg{err: errors.New(detail)})
	if view := m.View().Content; !strings.Contains(view, toastPrefix+detail) {
		t.Fatalf("View() = %q, want the %q toast", view, toastPrefix+detail)
	}

	// The toast expires with the timer of its own sequence.
	m = update(t, m, toastExpiredMsg{seq: m.toastSeq})
	if strings.Contains(m.View().Content, toastPrefix) {
		t.Error("the toast survived its timer")
	}

	// A successful save is silent.
	quiet := update(t, newSized(t, testDeps(t), 200, 40), layoutSavedMsg{})
	if strings.Contains(quiet.View().Content, toastPrefix) {
		t.Errorf("View() = %q, want no toast after a successful save", quiet.View().Content)
	}
}
