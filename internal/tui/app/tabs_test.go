package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// tabRow is the row of the top labels, right below the title.
const tabRow = headerRows - 1

// tabMiddle returns the x of a cell inside the label of one tab.
func tabMiddle(m Model, tab Tab) int {
	return m.tabRects[tab].Min.X + m.tabRects[tab].Dx()/2
}

// tabKey is the shortcut that opens a tab.
func tabKey(tab Tab) tea.KeyPressMsg {
	switch tab {
	case GitTab:
		return typed('2')
	case DockerTab:
		return typed('3')
	default:
		return typed('1')
	}
}

// clickPress applies one click and returns the model with its command still to run.
func clickPress(t *testing.T, m Model, x, y int) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(click(x, y))
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want app.Model", next)
	}

	return model, cmd
}

// plainCells drops the ANSI escapes of a rendered row, keeping its visible text.
func plainCells(s string) string {
	var b strings.Builder

	inEscape := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEscape = true
		case inEscape && ((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')):
			inEscape = false
		case !inEscape:
			b.WriteRune(r)
		}
	}

	return b.String()
}

func TestTabsRowKeepsTheSamePlainTextForEveryTab(t *testing.T) {
	const labels = " Files │ Git │ Docker "

	for tab := Tab(0); tab < tabCount; tab++ {
		m := newSized(t, testDeps(t), 120, 40)
		m, _ = press(t, m, tabKey(tab))

		rows := strings.Split(m.View().Content, "\n")
		if len(rows) <= tabRow {
			t.Fatalf("rendered %d rows, want the tab row at %d", len(rows), tabRow)
		}

		row := plainCells(rows[tabRow])
		if !strings.HasPrefix(row, labels) {
			t.Errorf("tab row with %v active = %q, want the prefix %q", tab, row, labels)
		}
		if got := lipgloss.Width(row); got != 120 {
			t.Errorf("tab row width = %d, want the canvas width 120", got)
		}
	}
}

func TestTabsClickLikeTheNumberKeys(t *testing.T) {
	cases := []struct {
		name string
		from Tab
		to   Tab
	}{
		{name: "files to git", from: FilesTab, to: GitTab},
		{name: "files to docker", from: FilesTab, to: DockerTab},
		{name: "git to files", from: GitTab, to: FilesTab},
		{name: "git to docker", from: GitTab, to: DockerTab},
		{name: "docker to files", from: DockerTab, to: FilesTab},
		{name: "docker to git", from: DockerTab, to: GitTab},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newSized(t, testDeps(t), 120, 40)
			m, _ = press(t, m, tabKey(tc.from))

			want := newSized(t, testDeps(t), 120, 40)
			want, _ = press(t, want, tabKey(tc.to))

			m = update(t, m, click(tabMiddle(m, tc.to), tabRow))
			if m.Tab() != want.Tab() || m.Focus() != want.Focus() {
				t.Errorf("tab/focus = %v/%v, want %v/%v as with the number keys",
					m.Tab(), m.Focus(), want.Tab(), want.Focus())
			}
		})
	}
}

func TestTabsClickOverEveryCellOfALabel(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)

	for tab := Tab(0); tab < tabCount; tab++ {
		rect := m.tabRects[tab]
		if rect.Min.Y != tabRow || rect.Max.Y != tabRow+1 {
			t.Fatalf("tabRects[%v] = %v, want the label on the tab row", tab, rect)
		}
		if rect.Dx() != lipgloss.Width(tabLabels[tab]) {
			t.Fatalf("tabRects[%v] width = %d, want the label width %d", tab, rect.Dx(), lipgloss.Width(tabLabels[tab]))
		}

		for x := rect.Min.X; x < rect.Max.X; x++ {
			if got := update(t, m, click(x, tabRow)); got.Tab() != tab {
				t.Fatalf("click(%d, %d) = %v, want %v", x, tabRow, got.Tab(), tab)
			}
		}
	}
}

func TestTabsClicksOnSeparatorsGapsAndTitleAreInert(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m, _ = press(t, m, typed('2')) // Git is the marker: any switch shows up

	cases := []struct {
		name string
		x, y int
	}{
		{name: "separator after the files label", x: m.tabRects[FilesTab].Max.X, y: tabRow},
		{name: "separator after the git label", x: m.tabRects[GitTab].Max.X, y: tabRow},
		{name: "empty space after the docker label", x: m.tabRects[DockerTab].Max.X, y: tabRow},
		{name: "last cell of the row", x: m.width - 1, y: tabRow},
		{name: "title row over the docker label", x: tabMiddle(m, DockerTab), y: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := update(t, m, click(tc.x, tc.y))
			if got.Tab() != GitTab || got.Focus() != FocusGit {
				t.Errorf("tab/focus = %v/%v, want Git/git kept", got.Tab(), got.Focus())
			}
		})
	}
}

func TestTabsClickIsIgnoredWhileTheHelpIsOpen(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = update(t, m, typed('?'))

	m = update(t, m, click(tabMiddle(m, DockerTab), tabRow))
	if m.Tab() != FilesTab {
		t.Errorf("Tab() = %v, want Files kept while the help is open", m.Tab())
	}
}

func TestTabsClickOnDockerMirrorsTheKeyboard(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)

	// The Docker ticker belongs to the runtime, so the test keeps the command instead of running it.
	m, cmd := clickPress(t, m, tabMiddle(m, DockerTab), tabRow)
	if cmd == nil {
		t.Fatal("no command, want the Docker load and ticker started by the click")
	}
	if m.Tab() != DockerTab || m.Focus() != FocusDocker {
		t.Fatalf("tab/focus = %v/%v, want Docker/docker", m.Tab(), m.Focus())
	}
	if got, want := m.dockerRect.Dx()+2*borderSide, 120; got != want {
		t.Errorf("docker frame width = %d, want the full width %d", got, want)
	}

	// Leaving the tab stops the ticker: the click returns what the shortcut returns, no more.
	reference, referenceCmd := press(t, m, tabKey(FilesTab))
	back, backCmd := clickPress(t, m, tabMiddle(m, FilesTab), tabRow)
	if back.Tab() != reference.Tab() || back.Focus() != reference.Focus() {
		t.Errorf("tab/focus = %v/%v, want %v/%v as with the number keys",
			back.Tab(), back.Focus(), reference.Tab(), reference.Focus())
	}
	if (backCmd == nil) != (referenceCmd == nil) {
		t.Errorf("click command = %v, want the %v of the key path", backCmd != nil, referenceCmd != nil)
	}
}

func TestMouseClickOnTheFirstBodyRowStillFocusesThePanel(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m = openFile(t, m, "main.go")

	if m.Focus() != FocusEditor {
		t.Fatalf("Focus() = %v, want editor after choosing a file", m.Focus())
	}

	// The Files label is already active: clicking it keeps the editor focus.
	m = update(t, m, click(tabMiddle(m, FilesTab), tabRow))
	if m.Tab() != FilesTab || m.Focus() != FocusEditor {
		t.Errorf("tab/focus = %v/%v, want Files/editor kept", m.Tab(), m.Focus())
	}

	// The first row inside the frame is the first body row of the tree.
	m = update(t, m, click(tabMiddle(m, FilesTab), headerRows+borderSide))
	if m.Tab() != FilesTab {
		t.Errorf("Tab() = %v, want the tab unchanged by a body click", m.Tab())
	}
	if m.Focus() != FocusTree {
		t.Errorf("Focus() = %v, want tree on a click in the first body row", m.Focus())
	}
}
