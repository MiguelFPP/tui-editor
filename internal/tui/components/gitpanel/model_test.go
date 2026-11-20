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
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func plain(view string) string {
	return ansiPattern.ReplaceAllString(view, "")
}

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// typed builds a printable key, the way the runtime delivers one.
func typed(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func ctrlKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

func spaceKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
}

// settle applies one key and feeds back the answers the panel owns.
func settle(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()

	next, cmd := m.Update(msg)

	return answers(t, next, cmd)
}

// editKey applies one key of the commit modal and drops the commands of the editor, which are
// cursor timers that would make the test wait for them.
func editKey(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()

	next, _ := m.Update(msg)

	return next
}

// typeText writes every rune of text into the commit modal.
func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()

	for _, r := range text {
		m = editKey(t, m, typed(r))
	}

	return m
}

// answers drains a command tree, applying only the messages the panel produces: the rest belong to
// the commit editor or to another component.
func answers(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		return m
	}

	msg := cmd()
	if msg == nil {
		return m
	}

	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, nested := range batch {
			m = answers(t, m, nested)
		}

		return m
	}
	if !panelMessage(msg) {
		return m
	}

	next, nested := m.Update(msg)

	return answers(t, next, nested)
}

func panelMessage(msg tea.Msg) bool {
	switch msg.(type) {
	case statusLoadedMsg, branchesLoadedMsg, stashesLoadedMsg, historyLoadedMsg, diffLoadedMsg, sidesLoadedMsg, writeDoneMsg, pollStatusMsg:
		return true
	default:
		return false
	}
}

// twoHunkFake answers the diff of any path with two separate hunks.
func twoHunkFake(t *testing.T) *fakes.GitService {
	t.Helper()

	git := gitFake(t)
	git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
		return domain.Diff{
			Path: req.Path, Staged: req.Staged,
			Hunks: []domain.DiffHunk{
				changeAt(1, "uno", "UNO"),
				changeAt(20, "veinte", "VEINTE"),
			},
		}, nil
	}

	return git
}

func changeAt(start int, old, changed string) domain.DiffHunk {
	return domain.DiffHunk{
		Header:   fmt.Sprintf("@@ -%d,1 +%d,1 @@", start, start),
		OldStart: start, OldLines: 1, NewStart: start, NewLines: 1,
		Lines: []domain.DiffLine{
			{Kind: domain.Removed, Text: old, OldLine: start},
			{Kind: domain.Added, Text: changed, NewLine: start},
		},
	}
}

// withDiff opens the panel with a loaded diff of normal.txt.
func withDiff(t *testing.T, git *fakes.GitService) Model {
	t.Helper()

	m := open(t, git)
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))

	return m
}

// crossToDiff loads a diff and moves to the Diff section.
func crossToDiff(t *testing.T, git *fakes.GitService) Model {
	t.Helper()

	m := withDiff(t, git)
	m = press(t, m, key(tea.KeyTab))
	if m.Section() != Diff {
		t.Fatalf("Section() = %v, want %v", m.Section(), Diff)
	}

	return m
}

// lastArgs returns the arguments of the last recorded call of one method.
func lastArgs(t *testing.T, git *fakes.GitService, method string) []any {
	t.Helper()

	calls := git.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method == method {
			return calls[i].Args
		}
	}

	t.Fatalf("no call to %s was recorded", method)

	return nil
}

// stagedPaths asserts the paths of the last Stage or Unstage call.
func stagedPaths(t *testing.T, git *fakes.GitService, method string) []domain.RelPath {
	t.Helper()

	args := lastArgs(t, git, method)
	paths, ok := args[0].([]domain.RelPath)
	if !ok {
		t.Fatalf("%s argument = %T, want []domain.RelPath", method, args[0])
	}

	return paths
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

	return openWith(t, git, keymap.Default())
}

// openWith opens the panel driven by a custom keymap.
func openWith(t *testing.T, git *fakes.GitService, keys keymap.Registry) Model {
	t.Helper()

	m := New(git, keys)
	m.SetSize(72, 14)

	return loadAll(t, m, m.Init())
}

// overrideKeys returns the default registry with the overrides applied on top of it.
func overrideKeys(t *testing.T, overrides map[string][]string) keymap.Registry {
	t.Helper()

	keys, err := keymap.Default().Apply(overrides)
	if err != nil {
		t.Fatalf("Apply(%v) = %v, want no error", overrides, err)
	}

	return keys
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

// goTo moves the panel to one section with the tab key, loading what is read on demand.
func goTo(t *testing.T, m Model, section Section) Model {
	t.Helper()

	for steps := 0; m.Section() != section; steps++ {
		if steps > sectionCount {
			t.Fatalf("Section() = %v, want %v", m.Section(), section)
		}

		m = press(t, m, key(tea.KeyTab))
	}

	return m
}

func TestNewLoadsStatusAndBranchesOnInit(t *testing.T) {
	git := gitFake(t)
	m := New(git, keymap.Default())
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
	if got := git.CallCount("Stashes"); got != 0 {
		t.Errorf("Stashes calls = %d, want 0: the stash list is lazy", got)
	}
	if m.loading.status || m.loading.branches {
		t.Errorf("loading = %+v, want both loaded", m.loading)
	}

	view := plain(m.View())
	for _, want := range []string{
		"git · rama main · origin/main · ↑2 · ↓1 · 5 cambios",
		"Cambios │ Diff │ Ramas │ Stash │ Historial",
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

	for _, section := range []Section{Branches, Stash, History} {
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

	m = goTo(t, m, History)

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
	m = goTo(t, m, Changes)
	m = goTo(t, m, History)
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
	if footer := m.footerText(); !strings.Contains(footer, "d: staged") {
		t.Errorf("footer = %q, want the staged marker", footer)
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
	m = goTo(t, m, History)

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
		{name: "timeout", err: fmt.Errorf("git: Status: %w", context.DeadlineExceeded), want: "tiempo agotado"},
		{name: "sin cambios", err: fmt.Errorf("git: Commit: %w", domain.ErrNoChanges), want: "no hay cambios preparados"},
		{name: "demasiado grande", err: fmt.Errorf("git: Commit: %w", domain.ErrTooLarge), want: "contenido demasiado grande"},
		{name: "ruta", err: domain.ErrInvalidPath, want: "ruta inválida"},
		{name: "argumento", err: fmt.Errorf("git: Commit: %w", domain.ErrInvalidArgument), want: "argumento inválido"},
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

		m := goTo(t, open(t, git), History)

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
	m := New(nil, keymap.Default())
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
	labels := []string{Changes.String(), Diff.String(), Branches.String(), Stash.String(), History.String()}
	if want := []string{"Cambios", "Diff", "Ramas", "Stash", "Historial"}; !slices.Equal(labels, want) {
		t.Errorf("labels = %v, want %v", labels, want)
	}
	if got := Section(9).String(); got != "Cambios" {
		t.Errorf("Section(9).String() = %q, want the first label", got)
	}
}

func TestLoadingAndErrReportTheSectionState(t *testing.T) {
	git := gitFake(t)
	m := New(git, keymap.Default())
	m.SetSize(72, 14)

	if !m.Loading() {
		t.Error("Loading() = false before Init, want true")
	}
	if m.Err() != nil {
		t.Errorf("Err() = %v, want nil", m.Err())
	}

	m = loadAll(t, m, m.Init())
	if m.Loading() {
		t.Error("Loading() = true after the loads, want false")
	}
	if m.Err() != nil {
		t.Errorf("Err() = %v, want nil", m.Err())
	}

	git.OnBranches = func(context.Context) ([]domain.Branch, error) {
		return nil, domain.ErrNotRepository
	}
	m = press(t, m, key('r'))

	if m.Loading() {
		t.Error("Loading() = true after the refresh, want false")
	}
	if !errors.Is(m.Err(), domain.ErrNotRepository) {
		t.Errorf("Err() = %v, want %v", m.Err(), domain.ErrNotRepository)
	}
}

func TestWheelScrollsTheActiveSection(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(72, 5)

	if got := m.offsets[Changes]; got != 0 {
		t.Fatalf("offsets[Changes] = %d, want 0", got)
	}

	next, cmd := m.Update(tea.MouseWheelMsg{X: 4, Y: 4, Button: tea.MouseWheelDown})
	if cmd != nil {
		t.Errorf("the wheel produced a command, want nil")
	}
	if next.offsets[Changes] != wheelStep {
		t.Errorf("offsets[Changes] = %d, want %d", next.offsets[Changes], wheelStep)
	}

	next, _ = next.Update(tea.MouseWheelMsg{X: 4, Y: 4, Button: tea.MouseWheelUp})
	if next.offsets[Changes] != 0 {
		t.Errorf("offsets[Changes] = %d, want 0", next.offsets[Changes])
	}

	next, _ = next.Update(tea.MouseWheelMsg{X: 4, Y: 4, Button: tea.MouseWheelUp})
	if next.offsets[Changes] != 0 {
		t.Errorf("offsets[Changes] = %d, want the scroll clamped at 0", next.offsets[Changes])
	}
}

func TestStageAndUnstageKeysOnTheChangesSection(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)
	m = press(t, m, key(tea.KeyDown))

	m = settle(t, m, key('s'))
	if got := git.CallCount("Stage"); got != 1 {
		t.Fatalf("Stage calls = %d, want 1", got)
	}
	if got, want := stagedPaths(t, git, "Stage"), []domain.RelPath{"normal.txt"}; !slices.Equal(got, want) {
		t.Errorf("Stage paths = %q, want %q", got, want)
	}
	if m.writing || m.writeErr != nil {
		t.Errorf("writing = %v, writeErr = %v, want the write done", m.writing, m.writeErr)
	}
	if got := git.CallCount("Status"); got != 2 {
		t.Errorf("Status calls = %d, want 2: a write refreshes the panel", got)
	}
	if got := git.CallCount("Branches"); got != 2 {
		t.Errorf("Branches calls = %d, want 2", got)
	}

	m = settle(t, m, key('u'))
	if got := git.CallCount("Unstage"); got != 1 {
		t.Fatalf("Unstage calls = %d, want 1", got)
	}
	if got, want := stagedPaths(t, git, "Unstage"), []domain.RelPath{"normal.txt"}; !slices.Equal(got, want) {
		t.Errorf("Unstage paths = %q, want %q", got, want)
	}
}

func TestStageOfARenameCarriesThePreImage(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)
	for range 3 {
		m = press(t, m, key(tea.KeyDown))
	}

	if file, ok := m.selectedFile(); !ok || file.Path != "renamed space.txt" {
		t.Fatalf("selected file = %+v, want the renamed entry", file)
	}

	m = settle(t, m, key('s'))

	want := []domain.RelPath{"renamed space.txt", "with space.txt"}
	if got := stagedPaths(t, git, "Stage"); !slices.Equal(got, want) {
		t.Errorf("Stage paths = %q, want %q", got, want)
	}
}

func TestStageKeysAreIgnoredWithoutAFile(t *testing.T) {
	git := gitFake(t)
	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		return domain.GitStatus{Branch: "main"}, nil
	}

	m := open(t, git)
	m = settle(t, m, key('s'))
	m = settle(t, m, key('u'))

	if got := git.CallCount("Stage") + git.CallCount("Unstage"); got != 0 {
		t.Errorf("write calls = %d, want none without a selected file", got)
	}
	if m.writing {
		t.Error("writing = true, want no write without a selected file")
	}
}

func TestWriteFailureIsReportedAndDoesNotReload(t *testing.T) {
	git := gitFake(t)
	git.OnStage = func(context.Context, []domain.RelPath) error {
		return fmt.Errorf("git: Stage: %w", domain.ErrGitIndexLocked)
	}

	m := open(t, git)
	before := git.CallCount("Status")

	m = settle(t, m, key('s'))
	if !errors.Is(m.writeErr, domain.ErrGitIndexLocked) {
		t.Fatalf("writeErr = %v, want %v", m.writeErr, domain.ErrGitIndexLocked)
	}
	if !errors.Is(m.Err(), domain.ErrGitIndexLocked) {
		t.Errorf("Err() = %v, want the write failure", m.Err())
	}
	if got := git.CallCount("Status"); got != before {
		t.Errorf("Status calls = %d, want %d: a failed write does not reload", got, before)
	}
	if view := plain(m.View()); !strings.Contains(view, "el índice de git está bloqueado") {
		t.Errorf("View() = %q, want the write failure", view)
	}
	if m.loading.status || m.Loading() {
		t.Errorf("Loading() = true and status loading = %v, want the write finished", m.loading.status)
	}
}

func TestWriteAnswersOfAnOldSequenceAreDropped(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	next, _ := m.Update(writeDoneMsg{seq: m.seqs.write + 1, op: opStage, err: domain.ErrNotRepository})
	if next.writeErr != nil {
		t.Errorf("writeErr = %v, want the stale answer dropped", next.writeErr)
	}
	if next.writing {
		t.Error("writing = true, want the panel untouched")
	}
}

func TestLoadingCoversTheWrite(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	m.writing = true
	if !m.Loading() {
		t.Error("Loading() = false while a write is in flight, want true")
	}

	m.writing = false
	if m.Loading() {
		t.Error("Loading() = true after the write, want false")
	}
}

func TestCommitModalCapturesInputAndCommits(t *testing.T) {
	git := gitFake(t)
	committed := ""
	git.OnCommit = func(_ context.Context, message string) error {
		committed = message

		return nil
	}

	m := open(t, git)
	if m.CapturesInput() {
		t.Fatal("CapturesInput() = true before opening the modal")
	}

	m = editKey(t, m, key('c'))
	if !m.CapturesInput() {
		t.Fatal("CapturesInput() = false after c")
	}

	view := plain(m.View())
	for _, want := range []string{"commit · mensaje", "ctrl+s: confirmar"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}

	m = typeText(t, m, "hola")
	m = settle(t, m, ctrlKey('s'))

	if committed != "hola" {
		t.Errorf("Commit message = %q, want %q", committed, "hola")
	}
	if m.CapturesInput() {
		t.Error("CapturesInput() = true after a successful commit, want the modal closed")
	}
	if got := m.commit.area.Value(); got != "" {
		t.Errorf("draft = %q, want it cleared after the commit", got)
	}
	if m.writeErr != nil || m.commit.err != nil {
		t.Errorf("writeErr = %v, commit.err = %v, want no error", m.writeErr, m.commit.err)
	}
	if got := git.CallCount("Commits"); got != 1 {
		t.Errorf("Commits calls = %d, want 1: a commit forces the history", got)
	}
}

func TestCommitModalInsertsNewlinesAndKeepsTheDraftOnEscape(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	m = editKey(t, m, key('c'))
	m = typeText(t, m, "titulo")
	m = editKey(t, m, key(tea.KeyEnter))
	m = typeText(t, m, "cuerpo")

	if got, want := m.commit.area.Value(), "titulo\ncuerpo"; got != want {
		t.Fatalf("draft = %q, want %q", got, want)
	}

	m = editKey(t, m, key(tea.KeyEscape))
	if m.CapturesInput() {
		t.Fatal("CapturesInput() = true after esc, want the modal closed")
	}
	if got, want := m.commit.area.Value(), "titulo\ncuerpo"; got != want {
		t.Errorf("draft after esc = %q, want the draft kept %q", got, want)
	}

	m = editKey(t, m, key('c'))
	if got, want := m.commit.area.Value(), "titulo\ncuerpo"; got != want {
		t.Errorf("draft after reopening = %q, want %q", got, want)
	}
	if got := git.CallCount("Commit"); got != 0 {
		t.Errorf("Commit calls = %d, want 0", got)
	}
}

func TestCommitModalRejectsAnEmptyMessage(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	m = editKey(t, m, key('c'))
	m = settle(t, m, ctrlKey('s'))

	if !errors.Is(m.commit.err, domain.ErrInvalidArgument) {
		t.Fatalf("commit.err = %v, want %v", m.commit.err, domain.ErrInvalidArgument)
	}
	if !m.CapturesInput() {
		t.Error("CapturesInput() = false, want the modal to stay open")
	}
	if got := git.CallCount("Commit"); got != 0 {
		t.Errorf("Commit calls = %d, want 0: an empty message never reaches git", got)
	}
	if view := plain(m.View()); !strings.Contains(view, "argumento inválido") {
		t.Errorf("View() = %q, want the empty message error", view)
	}
}

func TestCommitFailureKeepsTheModalOpen(t *testing.T) {
	git := gitFake(t)
	git.OnCommit = func(context.Context, string) error {
		return fmt.Errorf("git: Commit: %w", domain.ErrPermissionDenied)
	}

	m := open(t, git)
	before := git.CallCount("Status")

	m = editKey(t, m, key('c'))
	m = typeText(t, m, "x")
	m = settle(t, m, ctrlKey('s'))

	if !errors.Is(m.writeErr, domain.ErrPermissionDenied) {
		t.Fatalf("writeErr = %v, want %v", m.writeErr, domain.ErrPermissionDenied)
	}
	if !errors.Is(m.commit.err, domain.ErrPermissionDenied) {
		t.Errorf("commit.err = %v, want the failure shown in the modal", m.commit.err)
	}
	if !m.CapturesInput() {
		t.Error("CapturesInput() = false, want the modal kept open after a failure")
	}
	if got, want := m.commit.area.Value(), "x"; got != want {
		t.Errorf("draft = %q, want the draft kept %q", got, want)
	}
	if got := git.CallCount("Status"); got != before {
		t.Errorf("Status calls = %d, want %d: a failed commit does not reload", got, before)
	}
}

func TestCommitWithoutStagedChangesDoesNotReload(t *testing.T) {
	git := gitFake(t)
	git.OnCommit = func(context.Context, string) error { return domain.ErrNoChanges }

	m := open(t, git)
	before := git.CallCount("Status")

	m = editKey(t, m, key('c'))
	m = typeText(t, m, "x")
	m = settle(t, m, ctrlKey('s'))

	if !errors.Is(m.writeErr, domain.ErrNoChanges) {
		t.Fatalf("writeErr = %v, want %v", m.writeErr, domain.ErrNoChanges)
	}
	if got := git.CallCount("Status"); got != before {
		t.Errorf("Status calls = %d, want %d: nothing changed", got, before)
	}
	if view := plain(m.View()); !strings.Contains(view, "no hay cambios preparados") {
		t.Errorf("View() = %q, want the empty index message", view)
	}
}

func TestCommitNeedsAService(t *testing.T) {
	m := New(nil, keymap.Default())
	m.SetSize(72, 14)

	next, cmd := m.Update(key('c'))
	if cmd != nil {
		t.Errorf("command = %v, want nil without a service", cmd())
	}
	if next.CapturesInput() {
		t.Error("CapturesInput() = true without a service, want the modal closed")
	}
}

func TestHunkKeysMoveAndMark(t *testing.T) {
	git := twoHunkFake(t)
	m := crossToDiff(t, git)

	if got := len(m.diff.Hunks); got != 2 {
		t.Fatalf("hunks = %d, want 2", got)
	}
	if got := m.hunkCursor; got != 0 {
		t.Fatalf("hunkCursor = %d, want 0", got)
	}

	m = settle(t, m, key(']'))
	if got := m.hunkCursor; got != 1 {
		t.Errorf("hunkCursor after ] = %d, want 1", got)
	}
	m = settle(t, m, key(']'))
	if got := m.hunkCursor; got != 1 {
		t.Errorf("hunkCursor = %d, want it clamped at the last hunk", got)
	}
	m = settle(t, m, key('['))
	m = settle(t, m, key('['))
	if got := m.hunkCursor; got != 0 {
		t.Errorf("hunkCursor after [ = %d, want 0", got)
	}

	if view := plain(m.View()); !strings.Contains(view, "hunk 1/2") {
		t.Errorf("View() = %q, want the hunk cursor in the header", view)
	}

	m = settle(t, m, spaceKey())
	if !m.marked[0] {
		t.Error("the space key did not mark the hunk under the cursor")
	}
	if view := plain(m.View()); !strings.Contains(view, "1 marcados") {
		t.Errorf("View() = %q, want the mark count", view)
	}

	m = settle(t, m, spaceKey())
	if m.marked[0] {
		t.Error("the second space did not unmark the hunk")
	}
}

func TestStageHunksFromTheWorktreeDiff(t *testing.T) {
	git := twoHunkFake(t)
	m := crossToDiff(t, git)

	m = settle(t, m, spaceKey())
	m = settle(t, m, key(']'))
	m = settle(t, m, spaceKey())

	m = settle(t, m, key('s'))
	if got := git.CallCount("StageHunks"); got != 1 {
		t.Fatalf("StageHunks calls = %d, want 1", got)
	}

	args := lastArgs(t, git, "StageHunks")
	if diff, ok := args[0].(domain.Diff); !ok || diff.Path != "normal.txt" {
		t.Fatalf("StageHunks diff = %#v, want the loaded diff", args[0])
	}
	hunks, ok := args[1].([]int)
	if !ok || !slices.Equal(hunks, []int{0, 1}) {
		t.Errorf("StageHunks = %#v, want both hunks in order", args[1])
	}
	if got := git.CallCount("UnstageHunks"); got != 0 {
		t.Errorf("UnstageHunks calls = %d, want 0", got)
	}
	if len(m.selectedHunks()) != 0 {
		t.Error("the marks survived a successful write")
	}
	if got := git.CallCount("Diff"); got != 2 {
		t.Errorf("Diff calls = %d, want 2: the loaded diff is reloaded", got)
	}
}

func TestStagingHunksNeedsAMarkedHunk(t *testing.T) {
	git := twoHunkFake(t)
	m := crossToDiff(t, git)

	m = settle(t, m, key('s'))
	if got := git.CallCount("StageHunks"); got != 0 {
		t.Errorf("StageHunks calls = %d, want 0 without a marked hunk", got)
	}
	if m.writing {
		t.Error("writing = true, want no write without a marked hunk")
	}
}

func TestHunkKeysRespectTheDiffMode(t *testing.T) {
	git := twoHunkFake(t)
	m := crossToDiff(t, git)

	// The worktree diff cannot be unstaged and the staged diff cannot be staged.
	m = settle(t, m, spaceKey())
	m = settle(t, m, key('u'))
	if got := git.CallCount("UnstageHunks"); got != 0 {
		t.Errorf("UnstageHunks calls = %d, want 0 on the worktree diff", got)
	}

	m = press(t, m, key('d'))
	if !m.staged {
		t.Fatal("staged = false after d, want the staged diff")
	}
	if footer := m.footerText(); !strings.Contains(footer, "u: quitar hunks") {
		t.Errorf("footer = %q, want the unstage hint", footer)
	}

	m = settle(t, m, key('s'))
	if got := git.CallCount("StageHunks"); got != 0 {
		t.Errorf("StageHunks calls = %d, want 0 on the staged diff", got)
	}

	m = settle(t, m, spaceKey())
	m = settle(t, m, key('u'))
	if got := git.CallCount("UnstageHunks"); got != 1 {
		t.Fatalf("UnstageHunks calls = %d, want 1", got)
	}
	hunks, ok := lastArgs(t, git, "UnstageHunks")[1].([]int)
	if !ok || !slices.Equal(hunks, []int{0}) {
		t.Errorf("UnstageHunks = %#v, want the marked hunk", hunks)
	}
}

func TestLoadingADiffResetsTheHunkSelection(t *testing.T) {
	git := twoHunkFake(t)
	m := crossToDiff(t, git)

	m = settle(t, m, key(']'))
	m = settle(t, m, spaceKey())
	if len(m.selectedHunks()) != 1 {
		t.Fatalf("selected hunks = %v, want one", m.selectedHunks())
	}

	m = press(t, m, key('d'))
	if m.hunkCursor != 0 {
		t.Errorf("hunkCursor = %d, want 0 after reloading the diff", m.hunkCursor)
	}
	if len(m.selectedHunks()) != 0 {
		t.Errorf("selected hunks = %v, want the marks cleared", m.selectedHunks())
	}
}

func TestHunkKeysAreInertWithoutADiff(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)
	m = press(t, m, key(tea.KeyTab))

	m = settle(t, m, key(']'))
	m = settle(t, m, key('['))
	m = settle(t, m, spaceKey())

	if m.hunkCursor != 0 || len(m.selectedHunks()) != 0 {
		t.Errorf("cursor = %d, marks = %v, want both untouched", m.hunkCursor, m.selectedHunks())
	}

	m = settle(t, m, key('s'))
	m = settle(t, m, key('u'))
	if got := git.CallCount("StageHunks") + git.CallCount("UnstageHunks"); got != 0 {
		t.Errorf("hunk writes = %d, want none without a diff", got)
	}
}

func TestHunkKeysBelongToTheDiffSection(t *testing.T) {
	git := twoHunkFake(t)
	m := withDiff(t, git)

	if m.Section() != Changes {
		t.Fatalf("Section() = %v, want %v", m.Section(), Changes)
	}

	m = settle(t, m, key(']'))
	m = settle(t, m, key(']'))
	m = settle(t, m, spaceKey())

	if m.hunkCursor != 0 || len(m.selectedHunks()) != 0 {
		t.Errorf("cursor = %d, marks = %v, want the file list to ignore the hunk keys", m.hunkCursor, m.selectedHunks())
	}

	m = settle(t, m, key('s'))
	if got := git.CallCount("StageHunks"); got != 0 {
		t.Errorf("StageHunks calls = %d, want the file to be staged instead", got)
	}
	if got := git.CallCount("Stage"); got != 1 {
		t.Errorf("Stage calls = %d, want 1", got)
	}
}

func TestHelpAndFooterDocumentTheWriteKeys(t *testing.T) {
	m := open(t, gitFake(t))

	m = press(t, m, key('?'))
	view := plain(m.View())
	for _, want := range []string{"preparar el archivo", "quitar el archivo", "abrir el editor", "space", "commit: ctrl+s"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
	if strings.Contains(view, "F5: solo lectura") {
		t.Errorf("View() = %q, want the read-only note gone", view)
	}

	m = press(t, m, key(tea.KeyEscape))
	footer := m.footerText()
	for _, want := range []string{"s: preparar", "u: quitar", "c: commit"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer = %q, want %q", footer, want)
		}
	}
}

func TestCommitModalFitsThePanel(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(12, 8)
	m = editKey(t, m, key('c'))

	for i, line := range strings.Split(plain(m.View()), "\n") {
		if got := len([]rune(line)); got > 12 {
			t.Errorf("line %d = %q is %d runes wide, want at most 12", i, line, got)
		}
	}

	m.SetSize(72, 14)
	if !m.CapturesInput() {
		t.Error("a resize closed the modal")
	}
	for i, line := range strings.Split(plain(m.View()), "\n") {
		if got := len([]rune(line)); got > 72 {
			t.Errorf("line %d = %q is %d runes wide, want at most 72 after the resize", i, line, got)
		}
	}
}

func TestWritingStateIsRendered(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	m.writing = true
	if view := plain(m.View()); !strings.Contains(view, "escribiendo") {
		t.Errorf("View() = %q, want the writing state", view)
	}

	m.writing = false
	if view := plain(m.View()); strings.Contains(view, "escribiendo") {
		t.Errorf("View() = %q, want the writing state gone", view)
	}
}

func TestCommitModalIgnoresUnrelatedMessages(t *testing.T) {
	m := open(t, gitFake(t))
	m = settle(t, m, key('c'))

	next, cmd := m.Update(struct{ A int }{A: 1})
	if cmd != nil {
		t.Errorf("command = %v, want nil", cmd())
	}
	if !next.CapturesInput() || next.commit.area.Value() != "" {
		t.Error("an unrelated message changed the modal")
	}
}

func TestEmptyKeymapFallsBackToTheDefaults(t *testing.T) {
	m := openWith(t, gitFake(t), keymap.Registry{})

	m = press(t, m, key(tea.KeyTab))
	if m.Section() != Diff {
		t.Errorf("Section() = %v after tab, want %v with the built-in defaults", m.Section(), Diff)
	}
}

func TestKeymapOverridesDriveTheGitScope(t *testing.T) {
	git := gitFake(t)
	m := openWith(t, git, overrideKeys(t, map[string][]string{"git_stage": {"w"}}))
	m = press(t, m, key(tea.KeyDown))

	m = settle(t, m, typed('w'))
	if got := git.CallCount("Stage"); got != 1 {
		t.Fatalf("Stage calls after w = %d, want 1", got)
	}
	if got, want := stagedPaths(t, git, "Stage"), []domain.RelPath{"normal.txt"}; !slices.Equal(got, want) {
		t.Errorf("Stage paths = %q, want %q", got, want)
	}

	// The overridden key replaces the default one, it does not add to it.
	before := git.CallCount("Stage")
	m = settle(t, m, typed('s'))
	if got := git.CallCount("Stage"); got != before {
		t.Errorf("Stage calls after s = %d, want %d: the default key was replaced", got, before)
	}
}

func TestKeymapOverridesTheSectionKeys(t *testing.T) {
	keys := overrideKeys(t, map[string][]string{
		"git_section_next": {"n"},
		"git_new_branch":   {"b"}, // frees n for the section key
	})
	m := openWith(t, gitFake(t), keys)

	m = press(t, m, typed('n'))
	if m.Section() != Diff {
		t.Fatalf("Section() = %v after n, want %v", m.Section(), Diff)
	}

	// tab no longer advances the section.
	m = press(t, m, key(tea.KeyTab))
	if m.Section() != Diff {
		t.Errorf("Section() = %v after tab, want %v: the default key was replaced", m.Section(), Diff)
	}

	// The section that keeps its default key still answers to it.
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.Section() != Changes {
		t.Errorf("Section() = %v after shift+tab, want %v", m.Section(), Changes)
	}
}

// TestKeymapOverridesKeepTheModalKeysFixed checks that reassigning the configurable keys of the
// panel never changes the fixed keys of the commit, prompt and confirmation modals.
func TestKeymapOverridesKeepTheModalKeysFixed(t *testing.T) {
	keys := overrideKeys(t, map[string][]string{
		"git_commit": {"w"},
		"git_stage":  {"c"}, // the swapped keys stay configurable
	})

	t.Run("commit: ctrl+s saves and esc cancels", func(t *testing.T) {
		git := gitFake(t)
		m := openWith(t, git, keys)

		m = editKey(t, m, typed('w'))
		if !m.CapturesInput() {
			t.Fatal("w did not open the commit modal")
		}
		m = typeText(t, m, "mensaje")

		m = editKey(t, m, key(tea.KeyTab))
		m = editKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		if !m.CapturesInput() || m.commit.area.Value() != "mensaje" {
			t.Fatalf("tab left the commit modal or reached the draft")
		}

		m = settle(t, m, ctrlKey('s'))
		if got := git.CallCount("Commit"); got != 1 {
			t.Fatalf("Commit calls after ctrl+s = %d, want 1", got)
		}
		if m.CapturesInput() {
			t.Error("the commit modal stayed open after a successful commit")
		}

		m = editKey(t, m, typed('w'))
		m = settle(t, m, key(tea.KeyEscape))
		if m.CapturesInput() {
			t.Error("esc did not close the commit modal")
		}
	})

	t.Run("prompt: enter confirms and esc cancels", func(t *testing.T) {
		git := opsFake(t)
		m := goTo(t, openWith(t, git, keys), Branches)

		m = settle(t, m, typed('n'))
		m = typeText(t, m, "nueva")
		m = editKey(t, m, key(tea.KeyTab))
		if !m.CapturesInput() || m.prompt.input.Value() != "nueva" {
			t.Fatalf("tab left the prompt or reached the input")
		}

		m = settle(t, m, key(tea.KeyEnter))
		if got := git.CallCount("CreateBranch"); got != 1 {
			t.Fatalf("CreateBranch calls after enter = %d, want 1", got)
		}

		m = settle(t, m, typed('n'))
		m = settle(t, m, key(tea.KeyEscape))
		if m.CapturesInput() {
			t.Error("esc did not close the prompt")
		}
	})

	t.Run("confirmation: y accepts and n cancels", func(t *testing.T) {
		git := opsFake(t)
		m := goTo(t, openWith(t, git, keys), Branches)

		m = settle(t, m, key(tea.KeyDown))
		m = settle(t, m, typed('x'))
		if !m.CapturesInput() {
			t.Fatal("x did not open the confirmation")
		}
		m = editKey(t, m, key(tea.KeyTab))
		if !m.CapturesInput() {
			t.Fatal("tab closed the confirmation")
		}

		m = settle(t, m, typed('n'))
		if m.CapturesInput() || git.CallCount("DeleteBranch") != 0 {
			t.Fatal("n did not cancel the confirmation")
		}

		m = settle(t, m, typed('x'))
		m = settle(t, m, typed('y'))
		if got := git.CallCount("DeleteBranch"); got != 1 {
			t.Fatalf("DeleteBranch calls after y = %d, want 1", got)
		}
	})
}

// TestKeymapOverridesReachTheFooterAndTheHelp checks that the footer and the help of the panel
// document the effective keys, never the built-in ones.
func TestKeymapOverridesReachTheFooterAndTheHelp(t *testing.T) {
	m := openWith(t, gitFake(t), overrideKeys(t, map[string][]string{"git_stage": {"w"}}))
	m.SetSize(100, 20)

	footer := m.footerText()
	if !strings.Contains(footer, "w: preparar") {
		t.Errorf("footer = %q, want the reassigned stage key", footer)
	}
	if strings.Contains(footer, "s: preparar") {
		t.Errorf("footer = %q, want the default stage key replaced", footer)
	}

	m = press(t, m, key('?'))
	view := plain(m.View())
	if !strings.Contains(view, "w / u") || strings.Contains(view, "s / u") {
		t.Errorf("View() = %q, want the reassigned stage key in the help", view)
	}
	if !strings.Contains(view, "preparar el archivo") {
		t.Errorf("View() = %q, want the description of the reassigned action kept", view)
	}
}
