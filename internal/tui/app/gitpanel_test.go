package app

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
	"github.com/MiguelFPP/tui-editor/internal/tui/components/gitpanel"
)

// panelHeaderRows are the title and the section tabs the Git panel renders above its body.
const panelHeaderRows = 2

// gitBodyTop is the first terminal row of the Git body: the app header plus the panel header.
func gitBodyTop(m Model) int {
	return m.gitRect.Min.Y + panelHeaderRows
}

// brokenStatus reports one conflicted file, which enables the abort-merge confirmation.
func brokenStatus(context.Context) (domain.GitStatus, error) {
	return domain.GitStatus{Branch: "main", Files: []domain.GitFileStatus{
		{Path: "main.go", Index: domain.GitUnmerged, Worktree: domain.GitUnmerged, Conflicted: true},
	}}, nil
}

// twoFilesGit answers with a clean tree and two modified files.
func twoFilesGit(t *testing.T, deps Deps) *fakes.GitService {
	t.Helper()

	git := deps.Git.(*fakes.GitService)
	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		return domain.GitStatus{Branch: "main", Files: []domain.GitFileStatus{
			{Path: "main.go", Index: domain.GitUnmodified, Worktree: domain.GitModified},
			{Path: "long.txt", Index: domain.GitUnmodified, Worktree: domain.GitModified},
		}}, nil
	}

	return git
}

// twoHunkDiff is a unified diff with two hunks of one change each.
func twoHunkDiff(path domain.RelPath) domain.Diff {
	hunk := func(start int, old, changed string) domain.DiffHunk {
		return domain.DiffHunk{
			OldStart: start, OldLines: 1, NewStart: start, NewLines: 1,
			Lines: []domain.DiffLine{
				{Kind: domain.Removed, Text: old, OldLine: start},
				{Kind: domain.Added, Text: changed, NewLine: start},
			},
		}
	}

	return domain.Diff{
		Path: path,
		Hunks: []domain.DiffHunk{
			hunk(1, "uno", "UNO"),
			hunk(20, "veinte", "VEINTE"),
		},
	}
}

// lastStagePaths returns the paths of the last Stage call of the fake.
func lastStagePaths(t *testing.T, git *fakes.GitService) []domain.RelPath {
	t.Helper()

	calls := git.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method != "Stage" {
			continue
		}

		paths, ok := calls[i].Args[0].([]domain.RelPath)
		if !ok {
			t.Fatalf("Stage argument = %T, want []domain.RelPath", calls[i].Args[0])
		}

		return paths
	}

	t.Fatal("no call to Stage was recorded")

	return nil
}

// lastStageHunks returns the hunk indices of the last StageHunks call of the fake.
func lastStageHunks(t *testing.T, git *fakes.GitService) []int {
	t.Helper()

	calls := git.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method != "StageHunks" {
			continue
		}

		hunks, ok := calls[i].Args[1].([]int)
		if !ok {
			t.Fatalf("StageHunks argument = %T, want []int", calls[i].Args[1])
		}

		return hunks
	}

	t.Fatal("no call to StageHunks was recorded")

	return nil
}

func TestGitPanelClickSelectsTheRowUnderTheCursor(t *testing.T) {
	deps := testDeps(t)
	git := twoFilesGit(t, deps)

	m := newSized(t, deps, 120, 40)
	m = drain(t, m, m.Init())
	m = update(t, m, typed('2'))

	// Each column draws its own frame: the row rectangles start inside the list bounds.
	list := m.git.ListBounds()
	m = update(t, m, click(list.Min.X+1, list.Min.Y))
	if m.Focus() != FocusGit {
		t.Fatalf("Focus() = %v, want git", m.Focus())
	}

	m = update(t, m, click(list.Min.X+1, list.Min.Y+1))

	model, cmd := press(t, m, typed('s'))
	model = drain(t, model, cmd)

	if got := lastStagePaths(t, git); len(got) != 1 || got[0] != "long.txt" {
		t.Errorf("Stage paths = %q, want the row clicked with the mouse", got)
	}
	if model.git.CapturesInput() {
		t.Error("CapturesInput() = true, want the stage done")
	}
}

func TestGitPanelClickPicksTheHunkUnderTheCursor(t *testing.T) {
	deps := testDeps(t)
	git := stagedGit(t, deps)
	git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
		return twoHunkDiff(req.Path), nil
	}

	// Below the split threshold the list and the diff share one column, so a click on a row maps
	// directly to the hunk under it.
	m := newSized(t, deps, 99, 40)
	m = drain(t, m, m.Init())
	m = update(t, m, typed('2'))

	model, cmd := press(t, m, key(tea.KeyEnter))
	model = drain(t, model, cmd)
	model = update(t, model, key(tea.KeyTab))

	if got := model.git.Section(); got != gitpanel.Diff {
		t.Fatalf("Section() = %v, want the diff", got)
	}

	// The body starts with the row that names the diff, then the title and the two lines of the
	// first hunk, so the second hunk starts four rows further down.
	top := gitBodyTop(model)
	model = update(t, model, click(5, top+4))

	model, cmd = press(t, model, key(tea.KeySpace))
	model = drain(t, model, cmd)
	model, cmd = press(t, model, typed('s'))
	model = drain(t, model, cmd)

	if got := lastStageHunks(t, git); len(got) != 1 || got[0] != 1 {
		t.Errorf("StageHunks = %v, want the hunk clicked with the mouse", got)
	}
}

func TestGitPanelClickOnATabSwitchesSection(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = drain(t, m, m.Init())
	m = update(t, m, typed('2'))

	if got := m.git.Section(); got != gitpanel.Changes {
		t.Fatalf("Section() = %v, want the first section", got)
	}

	// The labels are "Cambios │ Diff │ Ramas │ Stash │ Historial", so "Ramas" starts at column 17.
	m = update(t, m, click(18, m.gitRect.Min.Y+1))

	if got := m.git.Section(); got != gitpanel.Branches {
		t.Errorf("Section() = %v, want the clicked tab", got)
	}
}

func TestGitConfirmOwnsTheKeyboard(t *testing.T) {
	deps := testDeps(t)
	git := deps.Git.(*fakes.GitService)
	git.OnStatus = brokenStatus

	m := newSized(t, deps, 120, 40)
	m = drain(t, m, m.Init())
	m = update(t, m, typed('2'))

	m, _ = press(t, m, typed('A'))
	if !m.git.CapturesInput() {
		t.Fatal("CapturesInput() = false after A, want the confirmation open")
	}

	// q, the numbers, the help and both tab keys must reach the panel, never the root keymap.
	for _, msg := range []tea.KeyPressMsg{typed('q'), typed('1'), typed('2'), typed('?'), key(tea.KeyTab), shiftTab()} {
		m, _ = press(t, m, msg)
	}
	if m.Tab() != GitTab || m.HelpVisible() {
		t.Errorf("tab = %v, help = %v, want the Git tab and no help", m.Tab(), m.HelpVisible())
	}
	if !m.git.CapturesInput() {
		t.Error("CapturesInput() = false, want the confirmation kept open")
	}
	if got := m.git.Err(); got != nil {
		t.Errorf("Err() = %v, want no error from the trapped keys", got)
	}

	// n cancels without touching the repository.
	m, _ = press(t, m, typed('n'))
	if m.git.CapturesInput() {
		t.Error("CapturesInput() = true after n, want the confirmation cancelled")
	}
	if got := git.CallCount("AbortMerge"); got != 0 {
		t.Errorf("AbortMerge calls = %d, want 0", got)
	}

	// ctrl+c keeps quitting while a modal owns the keyboard.
	m, _ = press(t, m, typed('A'))
	if !m.git.CapturesInput() {
		t.Fatal("CapturesInput() = false, want the confirmation open again")
	}

	if _, quit := press(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); quit == nil {
		t.Fatal("no command, want quit")
	} else if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatalf("command returned %T, want tea.QuitMsg", quit())
	}

	// y confirms and the modal closes when the write succeeds.
	m, _ = press(t, m, typed('A'))
	model, cmd := press(t, m, typed('y'))
	model = drain(t, model, cmd)

	if got := git.CallCount("AbortMerge"); got != 1 {
		t.Errorf("AbortMerge calls = %d, want 1", got)
	}
	if model.git.CapturesInput() {
		t.Error("CapturesInput() = true after the merge was aborted")
	}
}
