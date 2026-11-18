package gitpanel

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func plain(view string) string {
	return ansiPattern.ReplaceAllString(view, "")
}

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// status builds a snapshot with one staged, one unstaged, one renamed and one untracked file.
func status() domain.GitStatus {
	return domain.GitStatus{
		Branch:   "main",
		Upstream: "origin/main",
		Ahead:    2,
		Behind:   1,
		Files: []domain.GitFileStatus{
			{Path: "staged.txt", Index: domain.GitAdded, Worktree: domain.GitUnmodified},
			{Path: "normal.txt", Index: domain.GitUnmodified, Worktree: domain.GitModified},
			{Path: "nuevo.txt", Index: domain.GitUntracked, Worktree: domain.GitUntracked},
			{Path: "renamed space.txt", OrigPath: "with space.txt", Index: domain.GitRenamed, Worktree: domain.GitUnmodified},
			{Path: "conflict.txt", Index: domain.GitUnmerged, Worktree: domain.GitUnmerged, Conflicted: true},
		},
	}
}

// sampleDiff is the diff the fake answers with for normal.txt.
func sampleDiff() domain.Diff {
	return domain.Diff{
		Path:      "normal.txt",
		Additions: 1,
		Deletions: 1,
		Hunks: []domain.DiffHunk{{
			Header:   "@@ -1,2 +1,2 @@",
			OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
			Lines: []domain.DiffLine{
				{Kind: domain.Context, Text: "uno", OldLine: 1, NewLine: 1},
				{Kind: domain.Removed, Text: "dos", OldLine: 2},
				{Kind: domain.Added, Text: "DOS", NewLine: 2},
			},
		}},
	}
}

func gitFake(t *testing.T) *fakes.GitService {
	t.Helper()

	git := fakes.NewGitService()
	git.OnStatus = func(context.Context) (domain.GitStatus, error) { return status(), nil }
	git.OnBranches = func(context.Context) ([]domain.Branch, error) {
		return []domain.Branch{
			{Name: "main", Upstream: "origin/main", Current: true, Ahead: 2, Behind: 1},
			{Name: "feature-x", Upstream: "origin/feature-x", Gone: true},
			{Name: "suelta"},
		}, nil
	}
	git.OnCommits = func(_ context.Context, query domain.CommitQuery) (domain.CommitPage, error) {
		if query.Limit != domain.DefaultCommits {
			return domain.CommitPage{}, fmt.Errorf("límite inesperado: %d", query.Limit)
		}

		return domain.CommitPage{Commits: []domain.Commit{
			{Hash: strings.Repeat("a", 40), ShortHash: "aaaaaaa", Author: "Test User", Subject: "tercer commit"},
			{Hash: strings.Repeat("b", 40), ShortHash: "bbbbbbb", Author: "Test User", Subject: "segundo commit"},
		}}, nil
	}
	git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
		if req.Path != "normal.txt" {
			return domain.Diff{}, fmt.Errorf("%w: %q", domain.ErrNotFound, req.Path)
		}

		diff := sampleDiff()
		diff.Staged = req.Staged

		return diff, nil
	}

	return git
}

// load runs one command and feeds its message back, like the Bubble Tea runtime does.
func load(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		t.Fatal("command = nil, want a load command")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("command returned no message, want one")
	}

	next, nested := m.Update(msg)
	if nested != nil {
		t.Fatal("a loaded answer produced another command, want nil")
	}

	return next
}

// open builds the panel and applies the status and the branches Init asks for.
func open(t *testing.T, git *fakes.GitService) Model {
	t.Helper()

	m := New(git)
	m.SetSize(72, 14)

	return loadAll(t, m, m.Init())
}

// loadAll drains a batch of commands, feeding every message back in order.
func loadAll(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		t.Fatal("batch = nil, want commands")
	}

	msg := cmd()
	switch batch := msg.(type) {
	case tea.BatchMsg:
		for _, nested := range batch {
			m = loadAll(t, m, nested)
		}

		return m
	default:
		next, nested := m.Update(msg)
		if nested != nil {
			t.Fatal("a loaded answer produced another command, want nil")
		}

		return next
	}
}

// press applies one key and drains the command it returns.
func press(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()

	next, cmd := m.Update(msg)
	if cmd == nil {
		return next
	}

	return loadAll(t, next, cmd)
}

func TestNewLoadsStatusAndBranchesOnInit(t *testing.T) {
	git := gitFake(t)
	m := New(git)
	m.SetSize(72, 14)

	if m.Section() != Changes {
		t.Errorf("Section() = %v, want %v", m.Section(), Changes)
	}
	if view := plain(m.View()); !strings.Contains(view, "cargando") {
		t.Errorf("View() = %q, want the loading state before Init", view)
	}
	if got := git.CallCount("Status"); got != 0 {
		t.Fatalf("Status calls = %d before Init, want 0", got)
	}

	m = loadAll(t, m, m.Init())

	if got := git.CallCount("Status"); got != 1 {
		t.Errorf("Status calls = %d, want 1", got)
	}
	if got := git.CallCount("Branches"); got != 1 {
		t.Errorf("Branches calls = %d, want 1", got)
	}
	if got := git.CallCount("Commits"); got != 0 {
		t.Errorf("Commits calls = %d, want 0: the history is lazy", got)
	}
	if m.loading.status || m.loading.branches {
		t.Errorf("loading = %+v, want both loaded", m.loading)
	}

	view := plain(m.View())
	for _, want := range []string{
		"git · rama main · origin/main · ↑2 · ↓1 · 5 cambios",
		"Cambios │ Diff │ Ramas │ Historial",
		"tab: sección",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
}

func TestChangesSectionListsFilesWithMarkers(t *testing.T) {
	m := open(t, gitFake(t))

	view := plain(m.View())
	for _, want := range []string{
		"A. staged.txt",
		".M normal.txt",
		"?? nuevo.txt · sin seguimiento",
		"R. renamed space.txt · renombrado desde with space.txt",
		"UU conflict.txt · conflicto",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}

	if got, want := m.cursors[Changes], 0; got != want {
		t.Errorf("cursor = %d, want %d", got, want)
	}
}

func TestCursorNavigationAndSections(t *testing.T) {
	m := open(t, gitFake(t))

	m = press(t, m, key('j'))
	if got := m.cursors[Changes]; got != 1 {
		t.Errorf("cursor after j = %d, want 1", got)
	}
	m = press(t, m, key(tea.KeyDown))
	if got := m.cursors[Changes]; got != 2 {
		t.Errorf("cursor after down = %d, want 2", got)
	}
	m = press(t, m, key('k'))
	m = press(t, m, key(tea.KeyUp))
	if got := m.cursors[Changes]; got != 0 {
		t.Errorf("cursor after k and up = %d, want 0", got)
	}
	m = press(t, m, key(tea.KeyUp))
	if got := m.cursors[Changes]; got != 0 {
		t.Errorf("cursor at the top = %d, want 0", got)
	}

	m = press(t, m, key(tea.KeyTab))
	if m.Section() != Diff {
		t.Fatalf("after tab Section() = %v, want %v", m.Section(), Diff)
	}
	if got := plain(m.View()); !strings.Contains(got, "sin diff") {
		t.Errorf("View() = %q, want the empty diff state", got)
	}

	for _, section := range []Section{Branches, History} {
		m = press(t, m, key(tea.KeyTab))
		if m.Section() != section {
			t.Fatalf("after tab Section() = %v, want %v", m.Section(), section)
		}
	}

	m = press(t, m, key(tea.KeyTab))
	if m.Section() != Changes {
		t.Errorf("tab wraps to %v, want %v", m.Section(), Changes)
	}

	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.Section() != History {
		t.Errorf("shift+tab goes back to %v, want %v", m.Section(), History)
	}
	if got := plain(m.View()); !strings.Contains(got, "tercer commit") {
		t.Errorf("View() = %q, want the loaded history", got)
	}
}

func TestHistoryLoadsLazilyWithTheDefaultLimit(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	if got := git.CallCount("Commits"); got != 0 {
		t.Fatalf("Commits calls = %d, want 0", got)
	}

	m = press(t, m, key(tea.KeyTab))
	m = press(t, m, key(tea.KeyTab))
	m = press(t, m, key(tea.KeyTab))

	if m.Section() != History {
		t.Fatalf("Section() = %v, want %v", m.Section(), History)
	}
	if got := git.CallCount("Commits"); got != 1 {
		t.Fatalf("Commits calls = %d, want 1", got)
	}
	if len(m.history.Commits) != 2 {
		t.Errorf("history = %+v, want the fake page", m.history.Commits)
	}

	view := plain(m.View())
	for _, want := range []string{"aaaaaaa", "tercer commit", "Test User", "bbbbbbb"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}

	// Leaving and coming back does not reload an already loaded page.
	m = press(t, m, key(tea.KeyTab))
	m = press(t, m, key(tea.KeyTab))
	if got := git.CallCount("Commits"); got != 1 {
		t.Errorf("Commits calls = %d, want 1: the page is cached", got)
	}
}

func TestEnterLoadsTheDiffOfTheSelectedFile(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	if got := git.CallCount("Diff"); got != 1 {
		t.Fatalf("Diff calls = %d, want 1", got)
	}
	calls := git.Calls()
	request, ok := calls[len(calls)-1].Args[0].(domain.DiffRequest)
	if !ok || request.Path != "normal.txt" || request.Staged {
		t.Fatalf("Diff request = %#v, want the worktree diff of normal.txt", request)
	}

	m = press(t, m, key(tea.KeyTab))
	if m.Section() != Diff {
		t.Fatalf("Section() = %v, want %v", m.Section(), Diff)
	}

	view := plain(m.View())
	for _, want := range []string{"normal.txt · diff", "@@ -1,2 +1,2 @@", "-dos", "+DOS"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
	if strings.Contains(view, "cargando") {
		t.Errorf("View() = %q, want the loaded diff", view)
	}
}

func TestToggleStagedReloadsTheLoadedDiff(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	m = press(t, m, key('d'))
	if !m.staged {
		t.Error("staged = false after d, want true")
	}
	if got := git.CallCount("Diff"); got != 2 {
		t.Fatalf("Diff calls = %d, want 2: the loaded diff is reloaded", got)
	}
	if !m.diff.Staged {
		t.Error("the reloaded diff must be the staged one")
	}

	m = press(t, m, key(tea.KeyTab))
	if view := plain(m.View()); !strings.Contains(view, "d: staged") {
		t.Errorf("View() = %q, want the staged marker in the footer", view)
	}

	m = press(t, m, key('d'))
	if m.staged || m.diff.Staged {
		t.Error("d must toggle back to the worktree")
	}
	if got := git.CallCount("Diff"); got != 3 {
		t.Errorf("Diff calls = %d, want 3", got)
	}

	// Without a loaded diff, d only flips the flag.
	fresh := open(t, git)
	fresh = press(t, fresh, key('d'))
	if !fresh.staged || fresh.loaded {
		t.Errorf("staged = %v, loaded = %v, want the flag flipped without a request", fresh.staged, fresh.loaded)
	}
}

func TestRefreshReloadsEverythingLoaded(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))
	m = press(t, m, key(tea.KeyTab))
	m = press(t, m, key(tea.KeyTab))
	m = press(t, m, key(tea.KeyTab))

	before := map[string]int{
		"Status":   git.CallCount("Status"),
		"Branches": git.CallCount("Branches"),
		"Commits":  git.CallCount("Commits"),
		"Diff":     git.CallCount("Diff"),
	}

	m = press(t, m, key('r'))

	for method, want := range map[string]int{"Status": 1, "Branches": 1, "Commits": 1, "Diff": 1} {
		if got := git.CallCount(method) - before[method]; got != want {
			t.Errorf("%s calls = %d, want %d more", method, got, want)
		}
	}
	if m.loading.status || m.loading.branches || m.loading.history || m.loading.diff {
		t.Errorf("loading = %+v, want every refresh applied", m.loading)
	}
}

func TestStaleAnswersAreDiscarded(t *testing.T) {
	m := open(t, gitFake(t))
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	if m.request.Path != "normal.txt" || !m.loaded {
		t.Fatalf("request = %+v, want the loaded diff of normal.txt", m.request)
	}

	// A refresh bumps the sequence number, so answers of the previous one must be ignored.
	m.seqs.status++
	m.seqs.diff++

	stale := statusLoadedMsg{seq: m.seqs.status - 1, status: domain.GitStatus{Branch: "otra"}}
	next, cmd := m.Update(stale)
	if cmd != nil {
		t.Errorf("command = %v, want nil", cmd())
	}
	if next.status.Branch != "main" {
		t.Errorf("branch = %q, want the current status kept", next.status.Branch)
	}

	staleDiff := diffLoadedMsg{seq: m.seqs.diff - 1, req: domain.DiffRequest{Path: "old.txt"}, diff: domain.Diff{Path: "old.txt"}}
	next, _ = next.Update(staleDiff)
	if next.request.Path != "normal.txt" || next.diff.Path != "normal.txt" {
		t.Errorf("diff = %q/%q, want the current diff kept", next.request.Path, next.diff.Path)
	}
	if next.loading.diff {
		t.Error("loading.diff = true, want the stale answer to be dropped before applying")
	}
}

func TestErrorsAreDescribedInSpanish(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "no es un repositorio", err: domain.ErrNotRepository, want: "no es un repositorio git"},
		{name: "git ausente", err: fmt.Errorf("git: Status: %w", domain.ErrGitUnavailable), want: "git no está disponible"},
		{name: "índice bloqueado", err: fmt.Errorf("git: Stage: %w", domain.ErrGitIndexLocked), want: "el índice de git está bloqueado"},
		{name: "permiso", err: domain.ErrPermissionDenied, want: "permiso denegado"},
		{name: "timeout", err: fmt.Errorf("git: Status: %w", context.DeadlineExceeded), want: "tiempo de lectura agotado"},
		{name: "otro", err: errors.New("algo raro"), want: "algo raro"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeError(tc.err); got != tc.want {
				t.Errorf("describeError() = %q, want %q", got, tc.want)
			}
		})
	}

	git := gitFake(t)
	git.OnStatus = func(context.Context) (domain.GitStatus, error) { return domain.GitStatus{}, domain.ErrNotRepository }

	m := open(t, git)
	if m.errs.status == nil {
		t.Fatal("status error = nil, want the failure")
	}

	view := plain(m.View())
	for _, want := range []string{"error", "no es un repositorio git", "Cambios"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
	if strings.Contains(view, "A. staged.txt") {
		t.Errorf("View() = %q, want the stale rows dropped", view)
	}
}

func TestStatesWhenThereIsNothingToShow(t *testing.T) {
	t.Run("sin cambios", func(t *testing.T) {
		git := gitFake(t)
		git.OnStatus = func(context.Context) (domain.GitStatus, error) {
			return domain.GitStatus{Branch: "main"}, nil
		}

		view := plain(open(t, git).View())
		if !strings.Contains(view, "sin cambios") {
			t.Errorf("View() = %q, want the clean state", view)
		}
	})

	t.Run("sin ramas", func(t *testing.T) {
		git := gitFake(t)
		git.OnBranches = func(context.Context) ([]domain.Branch, error) { return nil, nil }

		m := open(t, git)
		m = press(t, m, key(tea.KeyTab))
		m = press(t, m, key(tea.KeyTab))

		if view := plain(m.View()); !strings.Contains(view, "sin ramas") {
			t.Errorf("View() = %q, want the empty branch state", view)
		}
	})

	t.Run("sin commits", func(t *testing.T) {
		git := gitFake(t)
		git.OnCommits = func(context.Context, domain.CommitQuery) (domain.CommitPage, error) {
			return domain.CommitPage{}, nil
		}

		m := open(t, git)
		for range 3 {
			m = press(t, m, key(tea.KeyTab))
		}

		if view := plain(m.View()); !strings.Contains(view, "sin commits") {
			t.Errorf("View() = %q, want the empty history state", view)
		}

		m = press(t, m, key('r'))
		if got := git.CallCount("Commits"); got != 2 {
			t.Errorf("Commits calls = %d, want 2: a refresh reloads an empty history too", got)
		}
	})

	t.Run("unborn", func(t *testing.T) {
		git := gitFake(t)
		git.OnStatus = func(context.Context) (domain.GitStatus, error) {
			return domain.GitStatus{Branch: "main", Unborn: true}, nil
		}

		view := plain(open(t, git).View())
		for _, want := range []string{"sin commits todavía", "rama main"} {
			if !strings.Contains(view, want) {
				t.Errorf("View() = %q, want %q", view, want)
			}
		}
	})

	t.Run("diff vacío", func(t *testing.T) {
		git := gitFake(t)
		git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
			return domain.Diff{Path: req.Path}, nil
		}

		m := open(t, git)
		m = press(t, m, key(tea.KeyEnter))
		m = press(t, m, key(tea.KeyTab))

		if view := plain(m.View()); !strings.Contains(view, "sin cambios") {
			t.Errorf("View() = %q, want the empty diff state", view)
		}
	})

	t.Run("diff binario", func(t *testing.T) {
		git := gitFake(t)
		git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
			return domain.Diff{Path: req.Path, Binary: true}, nil
		}

		m := open(t, git)
		m = press(t, m, key(tea.KeyEnter))
		m = press(t, m, key(tea.KeyTab))

		if view := plain(m.View()); !strings.Contains(view, "diferencia binaria") {
			t.Errorf("View() = %q, want the binary diff state", view)
		}
	})
}

func TestMissingServiceReportsAnError(t *testing.T) {
	m := New(nil)
	m.SetSize(72, 14)

	m = loadAll(t, m, m.Init())
	if !errors.Is(m.errs.status, errNoService) {
		t.Fatalf("status error = %v, want %v", m.errs.status, errNoService)
	}

	view := plain(m.View())
	for _, want := range []string{"error", "servicio de git no configurado"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}

	// The other sections keep the same error without panicking.
	m = press(t, m, key(tea.KeyTab))
	m = press(t, m, key(tea.KeyEnter))
	if view := plain(m.View()); !strings.Contains(view, "error") {
		t.Errorf("View() = %q, want the diff error state", view)
	}
}

func TestHelpAndEscape(t *testing.T) {
	m := open(t, gitFake(t))

	m = press(t, m, key('?'))
	view := plain(m.View())
	for _, want := range []string{"ayuda", "tab / shift+tab", "enter", "d ", "esc", "?"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}

	m = press(t, m, key(tea.KeyEscape))
	if m.help {
		t.Error("esc did not close the help")
	}
	if m.Section() != Changes {
		t.Errorf("Section() = %v, want %v", m.Section(), Changes)
	}

	m = press(t, m, key(tea.KeyTab))
	m = press(t, m, key(tea.KeyEscape))
	if m.Section() != Changes {
		t.Errorf("esc from Branches: Section() = %v, want %v", m.Section(), Changes)
	}
}

func TestSetSizeClipsRowsAndKeepsTheCursorVisible(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)
	m.SetSize(12, 6)

	for i, line := range strings.Split(plain(m.View()), "\n") {
		if got := len([]rune(line)); got > 12 {
			t.Errorf("line %d = %q is %d runes wide, want at most 12", i, line, got)
		}
	}

	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyDown))
	if m.offsets[Changes] != 1 {
		t.Errorf("offset = %d, want 1 to keep the cursor inside the viewport", m.offsets[Changes])
	}

	m = press(t, m, key(tea.KeyUp))
	m = press(t, m, key(tea.KeyUp))
	m = press(t, m, key(tea.KeyUp))
	if m.offsets[Changes] != 0 || m.cursors[Changes] != 0 {
		t.Errorf("cursor/offset = %d/%d, want 0/0", m.cursors[Changes], m.offsets[Changes])
	}

	size := tea.WindowSizeMsg{Width: 30, Height: 8}
	next, cmd := m.Update(size)
	if cmd != nil {
		t.Errorf("WindowSizeMsg command = %v, want nil", cmd())
	}
	if next.width != 30 || next.height != 8 {
		t.Errorf("size = %dx%d, want 30x8", next.width, next.height)
	}
}

func TestViewKeepsTheDiffBodyCached(t *testing.T) {
	m := open(t, gitFake(t))
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))
	m = press(t, m, key(tea.KeyTab))

	first := m.diffBody
	if first == "" {
		t.Fatal("diffBody is empty, want the rendered diff")
	}

	view := plain(m.View())
	for _, want := range []string{"normal.txt · diff", "@@ -1,2 +1,2 @@", "-dos"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want the cached body %q", view, want)
		}
	}
	if m.diffBody != first {
		t.Error("View() rebuilt the diff body, want it cached")
	}

	m.SetSize(12, 10)
	if m.diffBody == first {
		t.Error("SetSize must re-render the cached body with the new width")
	}
}

func TestUpdateIgnoresUnrelatedMessages(t *testing.T) {
	m := open(t, gitFake(t))

	next, cmd := m.Update(struct{ A int }{A: 1})
	if cmd != nil {
		t.Errorf("command = %v, want nil", cmd())
	}
	if len(next.status.Files) != len(m.status.Files) || next.width != m.width {
		t.Error("an unrelated message changed the model")
	}
}

func TestSectionLabels(t *testing.T) {
	labels := []string{Changes.String(), Diff.String(), Branches.String(), History.String()}
	if want := []string{"Cambios", "Diff", "Ramas", "Historial"}; !slices.Equal(labels, want) {
		t.Errorf("labels = %v, want %v", labels, want)
	}
	if got := Section(9).String(); got != "Cambios" {
		t.Errorf("Section(9).String() = %q, want the first label", got)
	}
}
