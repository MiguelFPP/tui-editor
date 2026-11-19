package app

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/config"
	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
	"github.com/MiguelFPP/tui-editor/internal/tui/components/filetree"
	"github.com/MiguelFPP/tui-editor/internal/tui/components/gitpanel"
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

	deps := Deps{Config: config.Config{Repo: t.TempDir(), Editor: "vim"}, Files: files, Git: git, Editor: editors, Docker: docker}

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
	for _, want := range []string{"atajos", "1 / 2", "tab"} {
		if !strings.Contains(view, want) {
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
