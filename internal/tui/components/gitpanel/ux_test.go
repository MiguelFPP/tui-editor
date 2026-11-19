package gitpanel

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// bodyRowY is the terminal row of the n-th body row of a panel whose view starts at top.
func bodyRowY(top, row int) int {
	return top + headerRows + row
}

// clickAt applies one left click and drains the command it produces, if any.
func clickAt(t *testing.T, m Model, x, y int) Model {
	t.Helper()

	next, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if cmd == nil {
		return next
	}

	return loadAll(t, next, cmd)
}

func TestClickSelectsTheRowUnderTheCursor(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetBounds(4, 6, 72, 14)

	m = clickAt(t, m, 10, bodyRowY(6, 2))
	if got := m.cursors[Changes]; got != 2 {
		t.Errorf("cursor after the click = %d, want 2", got)
	}

	m = clickAt(t, m, 4, bodyRowY(6, 0))
	if got := m.cursors[Changes]; got != 0 {
		t.Errorf("cursor after the first row = %d, want 0", got)
	}

	m = clickAt(t, m, 10, bodyRowY(6, 4))
	if got := m.cursors[Changes]; got != 4 {
		t.Errorf("cursor on the last row = %d, want 4", got)
	}

	// Below the last row, on the header and outside the panel the selection does not move.
	for _, point := range [][2]int{{10, bodyRowY(6, 5)}, {10, 6}, {10, bodyRowY(6, 1) + 40}} {
		m = clickAt(t, m, point[0], point[1])
		if got := m.cursors[Changes]; got != 4 {
			t.Errorf("cursor after a click at %v = %d, want 4", point, got)
		}
	}
}

func TestClickSelectsTheRowOfEverySection(t *testing.T) {
	m := open(t, opsFake(t))
	m.SetBounds(0, 0, 72, 14)

	cases := []struct {
		section Section
		row     int
		want    int
	}{
		{section: Branches, row: 2, want: 2},
		{section: Stash, row: 1, want: 1},
		{section: History, row: 1, want: 1},
	}

	for _, tc := range cases {
		m = goTo(t, m, tc.section)
		m = clickAt(t, m, 5, bodyRowY(0, tc.row))

		if got := m.cursors[tc.section]; got != tc.want {
			t.Errorf("%v: cursor = %d, want %d", tc.section, got, tc.want)
		}
	}
}

func TestClickUsesTheVisibleWindowAfterScrolling(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetBounds(0, 0, 72, 6)

	if got := m.bodyHeight(); got != 3 {
		t.Fatalf("bodyHeight() = %d, want 3", got)
	}

	next, cmd := m.Update(tea.MouseWheelMsg{X: 4, Y: 4, Button: tea.MouseWheelDown})
	if cmd != nil {
		t.Errorf("the wheel produced a command, want nil")
	}
	if next.offsets[Changes] == 0 {
		t.Fatal("offsets[Changes] = 0 after the wheel, want the list scrolled")
	}

	offset := next.offsets[Changes]

	next = clickAt(t, next, 4, bodyRowY(0, 0))
	if got := next.cursors[Changes]; got != offset {
		t.Errorf("cursor after the first visible row = %d, want %d", got, offset)
	}

	next = clickAt(t, next, 4, bodyRowY(0, 2))
	if got := next.cursors[Changes]; got != offset+2 {
		t.Errorf("cursor after the third visible row = %d, want %d", got, offset+2)
	}
}

func TestClickSelectsTheHunkUnderTheCursorWithoutStaging(t *testing.T) {
	git := twoHunkFake(t)
	m := crossToDiff(t, git)
	m.SetBounds(0, 0, 72, 16)

	// 72 cells keep the preview below the aligned-diff threshold: two hunks, each with its title.
	if len(m.hunkRows) != 2 {
		t.Fatalf("hunkRows = %d, want 2", len(m.hunkRows))
	}
	if len(m.diffLines) < 2 {
		t.Fatalf("diffLines = %d, want at least the two hunk titles", len(m.diffLines))
	}

	// The spans must cover the rendered title of every hunk, or the click would pick the wrong one.
	for index, span := range m.hunkRows {
		if !strings.HasPrefix(plain(m.diffLines[span.start-1]), "@@") {
			t.Fatalf("row %d = %q, want the title of the hunk %d", span.start, plain(m.diffLines[span.start-1]), index)
		}
	}

	m = clickAt(t, m, 10, bodyRowY(0, m.hunkRows[1].start))
	if got := m.hunkCursor; got != 1 {
		t.Errorf("hunkCursor = %d, want 1", got)
	}
	if got := m.cursors[Diff]; got != m.hunkRows[1].start {
		t.Errorf("cursor = %d, want the clicked row %d", got, m.hunkRows[1].start)
	}

	m = clickAt(t, m, 10, bodyRowY(0, m.hunkRows[0].start+1))
	if got := m.hunkCursor; got != 0 {
		t.Errorf("hunkCursor = %d, want 0", got)
	}

	// The header of the diff belongs to no hunk, and a click never stages anything.
	m = clickAt(t, m, 10, bodyRowY(0, 0))
	if got := m.hunkCursor; got != 0 {
		t.Errorf("hunkCursor = %d, want it kept", got)
	}
	if got := git.CallCount("StageHunks") + git.CallCount("UnstageHunks"); got != 0 {
		t.Errorf("hunk writes = %d, want a click to stage nothing", got)
	}

	// The hunk chosen with the mouse is the one the keys stage.
	m = clickAt(t, m, 10, bodyRowY(0, m.hunkRows[1].start))
	m = settle(t, m, spaceKey())
	m = settle(t, m, key('s'))

	hunks, ok := lastArgs(t, git, "StageHunks")[1].([]int)
	if !ok || len(hunks) != 1 || hunks[0] != 1 {
		t.Errorf("StageHunks = %#v, want the clicked hunk", hunks)
	}
}

func TestClickOnASectionTabSwitchesSection(t *testing.T) {
	git := opsFake(t)
	m := open(t, git)
	m.SetBounds(0, 0, 72, 14)

	for _, section := range []Section{Stash, History, Changes} {
		rect := m.tabRects[section]
		if rect.Dx() == 0 {
			t.Fatalf("tabRects[%v] is empty, want the label of a visible tab", section)
		}

		m = clickAt(t, m, rect.Min.X+1, rect.Min.Y)
		if got := m.Section(); got != section {
			t.Fatalf("Section() = %v, want %v", got, section)
		}
	}

	if got := git.CallCount("Stashes"); got != 1 {
		t.Errorf("Stashes calls = %d, want 1: the tab loads its section", got)
	}

	// The cell between two labels belongs to no tab.
	separator := m.tabRects[Diff].Max.X
	m = clickAt(t, m, separator, m.tabRects[Diff].Min.Y)
	if got := m.Section(); got != Changes {
		t.Errorf("Section() = %v, want the separator click ignored", got)
	}
}

func TestClicksAreIgnoredWhileAModalOrTheHelpIsOpen(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetBounds(0, 0, 72, 14)

	m = clickAt(t, m, 5, bodyRowY(0, 3))
	selected := m.cursors[Changes]

	m = editKey(t, m, key('c'))
	m = clickAt(t, m, 5, bodyRowY(0, 1))
	if !m.CapturesInput() {
		t.Fatal("the click closed the commit modal")
	}
	if got := m.cursors[Changes]; got != selected {
		t.Errorf("cursor behind the modal = %d, want %d", got, selected)
	}

	m = press(t, m, key(tea.KeyEscape))
	if m.CapturesInput() {
		t.Fatal("CapturesInput() = true after esc")
	}

	m = press(t, m, key('?'))
	m = clickAt(t, m, 5, bodyRowY(0, 1))
	if !m.help {
		t.Fatal("the click closed the help")
	}
	if got := m.cursors[Changes]; got != selected {
		t.Errorf("cursor behind the help = %d, want %d", got, selected)
	}
}

func TestModalsTrapTheFocusKeys(t *testing.T) {
	cases := []struct {
		name string
		open func(Model) Model
	}{
		{name: "commit", open: func(m Model) Model { return editKey(t, m, key('c')) }},
		{name: "prompt", open: func(m Model) Model { return settle(t, goTo(t, m, Branches), typed('n')) }},
		{name: "confirmación", open: func(m Model) Model {
			m = settle(t, goTo(t, m, Branches), key(tea.KeyDown))

			return settle(t, m, typed('x'))
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.open(open(t, opsFake(t)))

			if !m.CapturesInput() {
				t.Fatal("CapturesInput() = false, want the modal open")
			}

			section := m.Section()

			for _, msg := range []tea.KeyPressMsg{
				key(tea.KeyTab),
				{Code: tea.KeyTab, Mod: tea.ModShift},
				typed('j'),
				typed('k'),
				typed('?'),
			} {
				m = editKey(t, m, msg)
			}

			if !m.CapturesInput() {
				t.Error("CapturesInput() = false, want the modal kept open")
			}
			if m.Section() != section {
				t.Errorf("Section() = %v, want %v: tab left the modal", m.Section(), section)
			}
			if m.help {
				t.Error("the help opened behind a modal")
			}
		})
	}
}

func TestCommitModalDoesNotAcceptTabsInTheDraft(t *testing.T) {
	m := open(t, gitFake(t))

	m = editKey(t, m, key('c'))
	m = typeText(t, m, "titulo")
	m = editKey(t, m, key(tea.KeyTab))
	m = editKey(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = typeText(t, m, "fin")

	if got, want := m.commit.area.Value(), "titulofin"; got != want {
		t.Errorf("draft = %q, want %q", got, want)
	}
	if !m.CapturesInput() {
		t.Error("CapturesInput() = false, want the modal kept open")
	}
}

func TestPromptModalKeepsTheFocusAndCancels(t *testing.T) {
	git := opsFake(t)
	m := goTo(t, open(t, git), Branches)

	m = settle(t, m, typed('n'))
	m = typeText(t, m, "nueva")
	m = editKey(t, m, key(tea.KeyTab))

	if !m.CapturesInput() {
		t.Fatal("tab closed the prompt")
	}
	if got, want := m.prompt.input.Value(), "nueva"; got != want {
		t.Errorf("draft = %q, want %q: tab must not reach the input", got, want)
	}

	m = settle(t, m, key(tea.KeyEscape))
	if m.CapturesInput() || git.CallCount("CreateBranch") != 0 {
		t.Errorf("CapturesInput() = %v, CreateBranch calls = %d, want the prompt cancelled",
			m.CapturesInput(), git.CallCount("CreateBranch"))
	}
}

func TestConfirmModalAnswersWithN(t *testing.T) {
	git := opsFake(t)
	m := goTo(t, open(t, git), Branches)

	m = settle(t, m, key(tea.KeyDown))
	m = settle(t, m, typed('x'))
	if !m.CapturesInput() {
		t.Fatal("CapturesInput() = false, want the confirmation open")
	}

	m = settle(t, m, typed('n'))
	if m.CapturesInput() {
		t.Error("CapturesInput() = true after n, want the confirmation cancelled")
	}
	if got := git.CallCount("DeleteBranch"); got != 0 {
		t.Errorf("DeleteBranch calls = %d, want 0", got)
	}
}

func TestTheModalKeepsItsErrorAndDraftUntilTheWriteAnswers(t *testing.T) {
	git := gitFake(t)
	git.OnCommit = func(context.Context, string) error { return domain.ErrPermissionDenied }

	m := open(t, git)
	m = editKey(t, m, key('c'))
	m = typeText(t, m, "borrador")

	started, cmd := m.Update(ctrlKey('s'))
	if cmd == nil {
		t.Fatal("command = nil, want the commit")
	}
	if !started.writing {
		t.Fatal("writing = false, want the commit in flight")
	}
	if view := plain(started.View()); !strings.Contains(view, "escribiendo: commit") {
		t.Errorf("View() = %q, want the operation in flight", view)
	}

	// The modal owns the write: esc cannot leave it before the answer arrives.
	started = editKey(t, started, key(tea.KeyEscape))
	if !started.CapturesInput() {
		t.Fatal("esc closed the modal while the commit was in flight")
	}

	failed, _ := started.Update(cmd())
	if failed.writing {
		t.Error("writing = true after the answer, want the write finished")
	}
	if !errors.Is(failed.commit.err, domain.ErrPermissionDenied) {
		t.Errorf("commit.err = %v, want the failure inside the modal", failed.commit.err)
	}
	if got, want := failed.commit.area.Value(), "borrador"; got != want {
		t.Errorf("draft = %q, want %q", got, want)
	}
	if !failed.CapturesInput() {
		t.Error("CapturesInput() = false, want the modal kept open after a failure")
	}

	view := plain(failed.View())
	for _, want := range []string{"error", "permiso denegado", "borrador", "esc: cancelar"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
}

func TestTitleReportsTheStateOfThePanel(t *testing.T) {
	m := open(t, opsFake(t))

	want := "git · rama main · origin/main · ↑2 · ↓1 · 5 cambios · 1 conflicto · o/t/A"
	if got := m.titleText(); got != want {
		t.Errorf("titleText() = %q, want %q", got, want)
	}

	m.writing = true
	if got := m.titleText(); !strings.Contains(got, "git · escribiendo… · rama main") {
		t.Errorf("titleText() = %q, want the write without an operation, before the identity", got)
	}

	m.writeOp = opCommit
	if got := m.titleText(); !strings.Contains(got, "escribiendo: commit") {
		t.Errorf("titleText() = %q, want the operation named", got)
	}

	m.writing, m.writeOp = false, ""
	m.writeErr = domain.ErrGitIndexLocked
	if got := m.titleText(); !strings.Contains(got, "git · error · rama main") {
		t.Errorf("titleText() = %q, want the failure marked", got)
	}
	if view := plain(m.View()); !strings.Contains(view, "error al escribir: el índice de git está bloqueado") {
		t.Errorf("View() = %q, want the actionable failure", view)
	}
}

func TestUnbornTitleKeepsTheBranch(t *testing.T) {
	git := opsFake(t)
	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		return domain.GitStatus{Branch: "main", Unborn: true}, nil
	}

	m := open(t, git)
	if got, want := m.titleText(), "git · rama main · sin commits"; got != want {
		t.Errorf("titleText() = %q, want %q", got, want)
	}
}

func TestUnknownFailuresLoseTheRawOutput(t *testing.T) {
	raw := errors.New("git push origin main: salida 128: ! [rejected]\nfatal: could not read Username\nhint: revisa el remoto")

	if got, want := describeError(raw), "git push origin main: salida 128: ! [rejected]"; got != want {
		t.Errorf("describeError() = %q, want %q", got, want)
	}

	control := errors.New("fallo\x1b[31m\x07 raro\rmás")
	got := describeError(control)
	if strings.ContainsAny(got, "\x1b\x07\r\n") {
		t.Errorf("describeError() = %q, want no control characters", got)
	}

	long := errors.New(strings.Repeat("x", maxErrorCells+40))
	if got := len([]rune(describeError(long))); got != maxErrorCells {
		t.Errorf("len(describeError()) = %d, want %d", got, maxErrorCells)
	}

	// The failure reaches the body of the panel, one line long.
	git := opsFake(t)
	git.OnPush = func(context.Context) error { return raw }

	m := open(t, git)
	m = settle(t, m, typed('P'))

	view := plain(m.View())
	for _, want := range []string{"error al escribir", "! [rejected]"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
	if strings.Contains(view, "could not read Username") {
		t.Errorf("View() = %q, want the raw stderr left out", view)
	}
}

func TestRefreshClearsTheWriteFailure(t *testing.T) {
	git := opsFake(t)
	git.OnPush = func(context.Context) error { return domain.ErrNonFastForward }

	m := open(t, git)
	m = settle(t, m, typed('P'))
	if m.writeErr == nil {
		t.Fatal("writeErr = nil, want the failure of the push")
	}

	m = settle(t, m, key('r'))
	if m.writeErr != nil {
		t.Errorf("writeErr = %v, want the refresh to clear it", m.writeErr)
	}
	if view := plain(m.View()); strings.Contains(view, "error al escribir") {
		t.Errorf("View() = %q, want the failure gone", view)
	}
}

func TestHelpIsGroupedBySection(t *testing.T) {
	m := open(t, opsFake(t))
	m = press(t, m, key('?'))

	view := plain(m.View())
	for _, want := range []string{
		"ayuda · panel de git",
		" general",
		" Cambios · sección actual",
		" Diff",
		" Ramas",
		" Stash",
		" Historial",
		" modales",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
	if strings.Contains(view, "Cambios · sección actual · sección actual") {
		t.Error("the section marker is duplicated")
	}

	m = press(t, m, key(tea.KeyTab))
	if m.Section() != Diff || !m.help {
		t.Fatalf("section/help = %v/%v, want Diff with the help open", m.Section(), m.help)
	}
	if got := plain(m.View()); !strings.Contains(got, " Diff · sección actual") {
		t.Errorf("View() = %q, want the marker to follow the section", got)
	}
}

func TestHelpFitsThePanelWidth(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(24, 12)
	m = press(t, m, key('?'))

	for i, line := range strings.Split(plain(m.View()), "\n") {
		if got := len([]rune(line)); got > 24 {
			t.Errorf("line %d = %q is %d runes wide, want at most 24", i, line, got)
		}
	}
}
