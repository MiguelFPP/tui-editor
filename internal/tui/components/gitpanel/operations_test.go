package gitpanel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

// opsFake answers the reads of the branch, stash and sync sections.
func opsFake(t *testing.T) *fakes.GitService {
	t.Helper()

	git := gitFake(t)
	git.OnStashes = func(context.Context) ([]domain.StashEntry, error) {
		return []domain.StashEntry{
			{Index: 0, Message: "trabajo a medias", Branch: "main"},
			{Index: 1, Message: "otro", Branch: "main"},
		}, nil
	}
	git.OnMerge = func(context.Context, string) (domain.MergeResult, error) {
		return domain.MergeResult{Status: domain.FastForward}, nil
	}
	git.OnPull = func(context.Context) (domain.MergeResult, error) {
		return domain.MergeResult{Status: domain.FastForward}, nil
	}

	return git
}

// cleanStatusFake answers without conflicts, so the destructive keys stay inert.
func cleanStatusFake(t *testing.T) *fakes.GitService {
	t.Helper()

	git := opsFake(t)
	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		return domain.GitStatus{Branch: "main", Files: []domain.GitFileStatus{
			{Path: "normal.txt", Index: domain.GitUnmodified, Worktree: domain.GitModified},
		}}, nil
	}

	return git
}

// selectStash moves the cursor of the Stash section to one entry.
func selectStash(t *testing.T, m Model, index int) Model {
	t.Helper()

	for range index {
		m = settle(t, m, key(tea.KeyDown))
	}

	if got, ok := m.selectedStashIndex(); !ok || got != index {
		t.Fatalf("selectedStashIndex() = %d, %v, want %d", got, ok, index)
	}

	return m
}

// selectConflict moves the cursor of the Changes section to the conflicted file.
func selectConflict(t *testing.T, m Model) Model {
	t.Helper()

	for range len(m.status.Files) {
		if file, ok := m.selectedFile(); ok && file.Conflicted {
			return m
		}

		m = settle(t, m, key(tea.KeyDown))
	}

	t.Fatal("no conflicted file reached")

	return m
}

func TestBranchKeysCheckoutCreateAndDelete(t *testing.T) {
	git := opsFake(t)
	m := goTo(t, open(t, git), Branches)

	// The current branch is already checked out, so enter does nothing.
	m = settle(t, m, key(tea.KeyEnter))
	if got := git.CallCount("Checkout"); got != 0 {
		t.Errorf("Checkout calls = %d, want 0 on the current branch", got)
	}

	m = settle(t, m, key(tea.KeyDown))
	m = settle(t, m, key(tea.KeyEnter))
	if got := git.CallCount("Checkout"); got != 1 {
		t.Fatalf("Checkout calls = %d, want 1", got)
	}
	if got := lastArgs(t, git, "Checkout")[0]; got != "feature-x" {
		t.Errorf("Checkout branch = %v, want feature-x", got)
	}
	if got := git.CallCount("Status"); got != 2 {
		t.Errorf("Status calls = %d, want 2: a checkout refreshes the panel", got)
	}
	if got := git.CallCount("Commits"); got != 1 {
		t.Errorf("Commits calls = %d, want 1: a checkout forces the history", got)
	}

	// n opens the prompt and creates the branch at its optional start point.
	m = settle(t, m, typed('n'))
	if !m.CapturesInput() {
		t.Fatal("CapturesInput() = false after n")
	}
	if view := plain(m.View()); !strings.Contains(view, "rama nueva") {
		t.Errorf("View() = %q, want the branch prompt", view)
	}

	m = typeText(t, m, "nueva main")
	m = settle(t, m, key(tea.KeyEnter))

	args := lastArgs(t, git, "CreateBranch")
	if args[0] != "nueva" || args[1] != "main" {
		t.Errorf("CreateBranch args = %#v, want the name and the start point", args)
	}
	if m.CapturesInput() {
		t.Error("CapturesInput() = true after the branch was created")
	}

	// x asks first and never forces the delete.
	m = settle(t, m, typed('x'))
	if !m.CapturesInput() {
		t.Fatal("CapturesInput() = false after x")
	}
	if view := plain(m.View()); !strings.Contains(view, "¿borrar la rama feature-x?") {
		t.Errorf("View() = %q, want the delete confirmation", view)
	}

	m = settle(t, m, typed('n'))
	if got := git.CallCount("DeleteBranch"); got != 0 {
		t.Errorf("DeleteBranch calls = %d, want 0 after a cancelled confirmation", got)
	}
	if m.CapturesInput() {
		t.Error("CapturesInput() = true after n, want the confirmation closed")
	}

	m = settle(t, m, typed('x'))
	m = settle(t, m, typed('y'))

	if got := git.CallCount("DeleteBranch"); got != 1 {
		t.Fatalf("DeleteBranch calls = %d, want 1", got)
	}
	del := lastArgs(t, git, "DeleteBranch")
	if del[0] != "feature-x" || del[1] != false {
		t.Errorf("DeleteBranch args = %#v, want feature-x without force", del)
	}
	if m.CapturesInput() {
		t.Error("CapturesInput() = true after the delete")
	}
}

func TestBranchKeysBelongToTheBranchesSection(t *testing.T) {
	git := opsFake(t)
	m := open(t, git)

	for _, msg := range []tea.KeyPressMsg{typed('n'), typed('x'), typed('m')} {
		m = settle(t, m, msg)
	}

	if m.CapturesInput() {
		t.Error("a modal opened outside the Branches section")
	}
	if got := git.CallCount("CreateBranch") + git.CallCount("DeleteBranch") + git.CallCount("Merge"); got != 0 {
		t.Errorf("branch writes = %d, want none outside the Branches section", got)
	}

	// The same keys stay inert without a service.
	empty := New(nil, keymap.Default())
	empty.SetSize(72, 14)
	empty = goTo(t, empty, Branches)
	empty = settle(t, empty, typed('n'))
	if empty.CapturesInput() {
		t.Error("the prompt opened without a git service")
	}
}

func TestMergeKeyReportsTheOutcome(t *testing.T) {
	cases := []struct {
		name   string
		result domain.MergeResult
		notice string
	}{
		{
			name:   "avance rápido",
			result: domain.MergeResult{Status: domain.FastForward},
			notice: "merge: avance rápido",
		},
		{
			name:   "conflictos",
			result: domain.MergeResult{Status: domain.Conflicted, Conflicts: []domain.Conflict{{Path: "a.txt"}}},
			notice: "merge: 1 conflicto",
		},
		{
			name:   "commit de fusión",
			result: domain.MergeResult{Status: domain.Clean},
			notice: "merge: commit de fusión",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			git := opsFake(t)
			git.OnMerge = func(context.Context, string) (domain.MergeResult, error) { return tc.result, nil }

			m := goTo(t, open(t, git), Branches)
			m = settle(t, m, key(tea.KeyDown))
			m = settle(t, m, typed('m'))

			if got := git.CallCount("Merge"); got != 1 {
				t.Fatalf("Merge calls = %d, want 1", got)
			}
			if got := lastArgs(t, git, "Merge")[0]; got != "feature-x" {
				t.Errorf("Merge branch = %v, want feature-x", got)
			}
			if !strings.Contains(m.footerText(), tc.notice) {
				t.Errorf("footer = %q, want %q", m.footerText(), tc.notice)
			}
			if got := git.CallCount("Commits"); got != 1 {
				t.Errorf("Commits calls = %d, want 1: a merge forces the history", got)
			}
		})
	}
}

func TestStashSectionLoadsLazilyAndDrivesTheEntry(t *testing.T) {
	git := opsFake(t)
	m := open(t, git)

	if got := git.CallCount("Stashes"); got != 0 {
		t.Fatalf("Stashes calls = %d, want 0 before visiting the section", got)
	}

	m = goTo(t, m, Stash)
	if got := git.CallCount("Stashes"); got != 1 {
		t.Fatalf("Stashes calls = %d, want 1", got)
	}

	view := plain(m.View())
	for _, want := range []string{"stash@{0} · main · trabajo a medias", "stash@{1} · main · otro"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
	for _, want := range []string{"enter: aplicar", "z: pop", "x: descartar"} {
		if !strings.Contains(m.footerText(), want) {
			t.Errorf("footer = %q, want %q", m.footerText(), want)
		}
	}

	m = settle(t, m, key(tea.KeyEnter))
	if got := lastArgs(t, git, "StashApply")[0]; got != 0 {
		t.Errorf("StashApply index = %v, want 0", got)
	}
	if got := git.CallCount("Stashes"); got != 2 {
		t.Errorf("Stashes calls = %d, want 2: a write reloads the list", got)
	}

	m = settle(t, m, typed('z'))
	if got := lastArgs(t, git, "StashPop")[0]; got != 0 {
		t.Errorf("StashPop index = %v, want 0", got)
	}

	m = selectStash(t, m, 1)
	m = settle(t, m, typed('x'))
	if view := plain(m.View()); !strings.Contains(view, "¿descartar stash@{1}?") {
		t.Errorf("View() = %q, want the drop confirmation of the selected entry", view)
	}

	m = settle(t, m, key(tea.KeyEnter))
	if got := git.CallCount("StashDrop"); got != 1 {
		t.Fatalf("StashDrop calls = %d, want 1", got)
	}
	if got := lastArgs(t, git, "StashDrop")[0]; got != 1 {
		t.Errorf("StashDrop index = %v, want 1", got)
	}
}

func TestStashSectionStatesAndErrors(t *testing.T) {
	t.Run("sin stash", func(t *testing.T) {
		git := opsFake(t)
		git.OnStashes = func(context.Context) ([]domain.StashEntry, error) { return nil, nil }

		m := goTo(t, open(t, git), Stash)
		if view := plain(m.View()); !strings.Contains(view, "sin stash") {
			t.Errorf("View() = %q, want the empty stash state", view)
		}
	})

	t.Run("error", func(t *testing.T) {
		git := opsFake(t)
		git.OnStashes = func(context.Context) ([]domain.StashEntry, error) {
			return nil, domain.ErrNotRepository
		}

		m := goTo(t, open(t, git), Stash)
		if view := plain(m.View()); !strings.Contains(view, "no es un repositorio git") {
			t.Errorf("View() = %q, want the failure", view)
		}
		if !errors.Is(m.Err(), domain.ErrNotRepository) {
			t.Errorf("Err() = %v, want %v", m.Err(), domain.ErrNotRepository)
		}
	})
}

func TestStashPushPromptFromTheChangesSection(t *testing.T) {
	git := opsFake(t)
	m := open(t, git)

	m = settle(t, m, typed('z'))
	if !m.CapturesInput() {
		t.Fatal("CapturesInput() = false after z")
	}
	if view := plain(m.View()); !strings.Contains(view, "stash · mensaje (opcional)") {
		t.Errorf("View() = %q, want the stash prompt", view)
	}

	m = typeText(t, m, "wip")
	m = settle(t, m, key(tea.KeyEnter))

	if got := lastArgs(t, git, "StashPush")[0]; got != "wip" {
		t.Errorf("StashPush message = %v, want wip", got)
	}
	if m.CapturesInput() {
		t.Error("CapturesInput() = true after the stash")
	}

	// An empty message is valid: the adapter sends it without -m.
	m = settle(t, m, typed('z'))
	m = settle(t, m, key(tea.KeyEnter))

	if got := lastArgs(t, git, "StashPush")[0]; got != "" {
		t.Errorf("StashPush message = %v, want the empty message", got)
	}
	if got := git.CallCount("StashPush"); got != 2 {
		t.Errorf("StashPush calls = %d, want 2", got)
	}
}

func TestPromptAndConfirmValidationAndEscape(t *testing.T) {
	git := opsFake(t)
	m := goTo(t, open(t, git), Branches)

	m = settle(t, m, typed('n'))
	m = settle(t, m, key(tea.KeyEnter))

	if !errors.Is(m.prompt.err, domain.ErrInvalidArgument) {
		t.Fatalf("prompt.err = %v, want %v", m.prompt.err, domain.ErrInvalidArgument)
	}
	if !m.CapturesInput() {
		t.Error("CapturesInput() = false, want the prompt kept open")
	}
	if view := plain(m.View()); !strings.Contains(view, "argumento inválido") {
		t.Errorf("View() = %q, want the empty name error", view)
	}
	if got := git.CallCount("CreateBranch"); got != 0 {
		t.Errorf("CreateBranch calls = %d, want 0: an empty name never reaches git", got)
	}

	m = typeText(t, m, "borrador")
	m = settle(t, m, key(tea.KeyEscape))
	if m.CapturesInput() {
		t.Error("CapturesInput() = true after esc")
	}
	if got := m.prompt.input.Value(); got != "" {
		t.Errorf("draft = %q, want it cleared by esc", got)
	}
	if got := git.CallCount("CreateBranch"); got != 0 {
		t.Errorf("CreateBranch calls = %d, want 0", got)
	}

	m = settle(t, m, key(tea.KeyDown))
	m = settle(t, m, typed('x'))
	m = settle(t, m, key(tea.KeyEscape))
	if m.CapturesInput() {
		t.Error("CapturesInput() = true after escaping the confirmation")
	}
	if got := git.CallCount("DeleteBranch"); got != 0 {
		t.Errorf("DeleteBranch calls = %d, want 0", got)
	}
}

func TestPromptFitsThePanel(t *testing.T) {
	git := opsFake(t)
	m := goTo(t, open(t, git), Branches)
	m.SetSize(12, 8)

	m = settle(t, m, typed('n'))

	for i, line := range strings.Split(plain(m.View()), "\n") {
		if got := len([]rune(line)); got > 12 {
			t.Errorf("line %d = %q is %d runes wide, want at most 12", i, line, got)
		}
	}

	m.SetSize(72, 14)
	if !m.CapturesInput() {
		t.Error("a resize closed the prompt")
	}
	for i, line := range strings.Split(plain(m.View()), "\n") {
		if got := len([]rune(line)); got > 72 {
			t.Errorf("line %d = %q is %d runes wide, want at most 72 after the resize", i, line, got)
		}
	}
}

func TestConflictKeysResolveOursAndTheirs(t *testing.T) {
	git := opsFake(t)
	m := selectConflict(t, open(t, git))
	if m.Section() != Changes {
		t.Fatalf("Section() = %v, want %v", m.Section(), Changes)
	}

	m = settle(t, m, typed('o'))
	resolved := lastArgs(t, git, "ResolveConflict")
	if resolved[0] != domain.RelPath("conflict.txt") || resolved[1] != domain.Ours {
		t.Errorf("ResolveConflict args = %#v, want conflict.txt with ours", resolved)
	}

	m = settle(t, m, typed('t'))
	resolved = lastArgs(t, git, "ResolveConflict")
	if resolved[0] != domain.RelPath("conflict.txt") || resolved[1] != domain.Theirs {
		t.Errorf("ResolveConflict args = %#v, want conflict.txt with theirs", resolved)
	}
	if got := git.CallCount("ResolveConflict"); got != 2 {
		t.Errorf("ResolveConflict calls = %d, want 2", got)
	}

	// A file without conflicts ignores the resolution keys.
	m = settle(t, m, key(tea.KeyUp))
	m = settle(t, m, typed('o'))
	if got := git.CallCount("ResolveConflict"); got != 2 {
		t.Errorf("ResolveConflict calls = %d, want 2: no conflict, no resolution", got)
	}
}

func TestConflictBannerAndAbortMerge(t *testing.T) {
	git := opsFake(t)
	m := open(t, git)

	if got := m.titleText(); !strings.Contains(got, "1 conflicto · o/t/A") {
		t.Errorf("titleText() = %q, want the conflict banner", got)
	}
	if view := plain(m.View()); !strings.Contains(view, "1 conflicto") {
		t.Errorf("View() = %q, want the conflict count", view)
	}
	if !strings.Contains(m.footerText(), "o/t: resolver") || !strings.Contains(m.footerText(), "A: abortar merge") {
		t.Errorf("footer = %q, want the conflict keys", m.footerText())
	}

	m = settle(t, m, typed('A'))
	if !m.CapturesInput() {
		t.Fatal("CapturesInput() = false after A")
	}
	if view := plain(m.View()); !strings.Contains(view, "¿abortar el merge en curso?") {
		t.Errorf("View() = %q, want the abort confirmation", view)
	}

	m = settle(t, m, typed('y'))
	if got := git.CallCount("AbortMerge"); got != 1 {
		t.Fatalf("AbortMerge calls = %d, want 1", got)
	}
	if m.CapturesInput() {
		t.Error("CapturesInput() = true after the abort")
	}

	// Without conflicts there is nothing to abort.
	clean := cleanStatusFake(t)
	fresh := open(t, clean)
	if strings.Contains(fresh.titleText(), "conflicto") {
		t.Errorf("titleText() = %q, want no banner on a clean tree", fresh.titleText())
	}
	fresh = settle(t, fresh, typed('A'))
	if fresh.CapturesInput() {
		t.Error("A opened a confirmation without conflicts")
	}
	if got := clean.CallCount("AbortMerge"); got != 0 {
		t.Errorf("AbortMerge calls = %d, want 0", got)
	}
}

func TestSyncKeysFetchPullAndPush(t *testing.T) {
	git := opsFake(t)
	m := open(t, git)

	m = settle(t, m, typed('f'))
	if got := git.CallCount("Fetch"); got != 1 {
		t.Fatalf("Fetch calls = %d, want 1", got)
	}
	if !strings.Contains(m.footerText(), noticeFetch) {
		t.Errorf("footer = %q, want %q", m.footerText(), noticeFetch)
	}

	m = settle(t, m, typed('p'))
	if got := git.CallCount("Pull"); got != 1 {
		t.Fatalf("Pull calls = %d, want 1", got)
	}
	if !strings.Contains(m.footerText(), "merge: avance rápido") {
		t.Errorf("footer = %q, want the pull notice", m.footerText())
	}
	if got := git.CallCount("Commits"); got != 1 {
		t.Errorf("Commits calls = %d, want 1: a pull forces the history", got)
	}

	m = settle(t, m, typed('P'))
	if got := git.CallCount("Push"); got != 1 {
		t.Fatalf("Push calls = %d, want 1", got)
	}
	if !strings.Contains(m.footerText(), noticePush) {
		t.Errorf("footer = %q, want %q", m.footerText(), noticePush)
	}

	if got := git.CallCount("Status"); got < 4 {
		t.Errorf("Status calls = %d, want one refresh per write", got)
	}
}

func TestSyncErrorsReachThePanel(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "sin upstream", err: domain.ErrNoUpstream, want: "la rama no tiene upstream"},
		{name: "non fast forward", err: domain.ErrNonFastForward, want: "el remoto tiene commits nuevos: haz pull antes de push"},
		{name: "autenticación", err: domain.ErrAuthRequired, want: "el remoto pide autenticación"},
		{name: "remoto inalcanzable", err: domain.ErrRemoteUnreachable, want: "no se pudo alcanzar el remoto"},
		{name: "sin remoto", err: domain.ErrNoRemote, want: "no hay remoto configurado"},
		{name: "árbol sucio", err: domain.ErrDirtyWorktree, want: "hay cambios sin guardar en el árbol de trabajo"},
		{name: "merge en curso", err: domain.ErrMergeInProgress, want: "hay un merge en curso"},
		{name: "sin merge", err: domain.ErrNoMergeInProgress, want: "no hay ningún merge en curso"},
		{name: "conflictos", err: fmt.Errorf("git: StashApply: %w", domain.ErrConflict), want: "quedan conflictos sin resolver"},
		{name: "no encontrado", err: domain.ErrNotFound, want: "no encontrado"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeError(tc.err); got != tc.want {
				t.Errorf("describeError() = %q, want %q", got, tc.want)
			}
		})
	}

	git := opsFake(t)
	git.OnPush = func(context.Context) error { return domain.ErrNonFastForward }

	m := open(t, git)
	m = settle(t, m, typed('P'))

	if !errors.Is(m.writeErr, domain.ErrNonFastForward) {
		t.Fatalf("writeErr = %v, want %v", m.writeErr, domain.ErrNonFastForward)
	}
	if !errors.Is(m.Err(), domain.ErrNonFastForward) {
		t.Errorf("Err() = %v, want the push failure", m.Err())
	}
	if view := plain(m.View()); !strings.Contains(view, "el remoto tiene commits nuevos") {
		t.Errorf("View() = %q, want the translated failure", view)
	}
	if got := git.CallCount("Status"); got != 1 {
		t.Errorf("Status calls = %d, want 1: a failed write does not reload", got)
	}
}

func TestWriteKeysDoNotStack(t *testing.T) {
	git := opsFake(t)
	m := open(t, git)
	m.writing = true

	for _, msg := range []tea.KeyPressMsg{typed('s'), typed('u'), typed('z'), typed('f'), typed('p'), typed('P'), typed('A')} {
		m = settle(t, m, msg)
	}

	if got := git.CallCount("Stage") + git.CallCount("Unstage") + git.CallCount("StashPush") +
		git.CallCount("Fetch") + git.CallCount("Pull") + git.CallCount("Push") + git.CallCount("AbortMerge"); got != 0 {
		t.Errorf("writes = %d, want none while another one is in flight", got)
	}

	// A prompt that is already open does not start a second write either.
	m.writing = true
	m = settle(t, m, typed('z'))
	m = typeText(t, m, "wip")
	m = settle(t, m, key(tea.KeyEnter))

	if got := git.CallCount("StashPush"); got != 0 {
		t.Errorf("StashPush calls = %d, want none while writing", got)
	}
}

func TestWriteFailuresKeepTheModalOpen(t *testing.T) {
	git := opsFake(t)
	git.OnCreateBranch = func(context.Context, string, string) error {
		return fmt.Errorf("git: CreateBranch: %w", domain.ErrInvalidArgument)
	}
	git.OnDeleteBranch = func(context.Context, string, bool) error {
		return fmt.Errorf("git: DeleteBranch: %w", domain.ErrDirtyWorktree)
	}

	m := goTo(t, open(t, git), Branches)

	m = settle(t, m, typed('n'))
	m = typeText(t, m, "nueva")
	m = settle(t, m, key(tea.KeyEnter))

	if !errors.Is(m.prompt.err, domain.ErrInvalidArgument) {
		t.Fatalf("prompt.err = %v, want %v", m.prompt.err, domain.ErrInvalidArgument)
	}
	if !m.CapturesInput() {
		t.Error("the prompt closed after a failure")
	}
	if m.writing {
		t.Error("writing = true after the failure")
	}
	if view := plain(m.View()); !strings.Contains(view, "argumento inválido") {
		t.Errorf("View() = %q, want the failure inside the prompt", view)
	}

	m = settle(t, m, key(tea.KeyEscape))
	m = settle(t, m, key(tea.KeyDown))
	m = settle(t, m, typed('x'))
	m = settle(t, m, typed('y'))

	if !errors.Is(m.confirm.err, domain.ErrDirtyWorktree) {
		t.Fatalf("confirm.err = %v, want %v", m.confirm.err, domain.ErrDirtyWorktree)
	}
	if !m.CapturesInput() {
		t.Error("the confirmation closed after a failure")
	}
	if !errors.Is(m.Err(), domain.ErrDirtyWorktree) {
		t.Errorf("Err() = %v, want the failure", m.Err())
	}
	if view := plain(m.View()); !strings.Contains(view, "hay cambios sin guardar") {
		t.Errorf("View() = %q, want the failure inside the confirmation", view)
	}
}

func TestHelpDocumentsTheNewKeys(t *testing.T) {
	m := open(t, opsFake(t))
	m = settle(t, m, key('?'))

	view := plain(m.View())
	for _, want := range []string{
		"crear una rama", "borrar la rama", "fusionar la rama", "stash", "ours", "abortar el merge",
		"fetch, pull y push", "prompt y confirmación",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
}
