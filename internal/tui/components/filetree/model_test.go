package filetree

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
)

// tree is the fake repository layout shared by every test.
var tree = map[domain.RelPath][]domain.FileNode{
	"": {
		node("docs", domain.Dir),
		node("src", domain.Dir),
		node(".gitignore", domain.File),
		node("README.md", domain.File),
	},
	"docs": {
		node("docs/guide.md", domain.File),
	},
	"src": {
		node("src/main.go", domain.File),
		node("src/internal", domain.Dir),
	},
	"src/internal": {
		node("src/internal/logic.go", domain.File),
	},
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func node(path string, kind domain.FileKind) domain.FileNode {
	name := path
	if index := strings.LastIndex(path, "/"); index >= 0 {
		name = path[index+1:]
	}

	return domain.FileNode{Name: name, Path: domain.RelPath(path), Kind: kind}
}

func treeFake(t *testing.T) *fakes.FileService {
	t.Helper()

	files := fakes.NewFileService()
	files.OnReadDir = func(_ context.Context, dir domain.RelPath) (domain.DirListing, error) {
		entries, ok := tree[dir]
		if !ok {
			return domain.DirListing{}, fmt.Errorf("%w: %q", domain.ErrNotFound, dir)
		}

		return domain.DirListing{Dir: dir, Entries: entries}, nil
	}

	return files
}

// drain runs one command and feeds its message back, like the Bubble Tea runtime does.
func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		return m
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("command returned no message, want one")
	}

	next, nested := m.Update(msg)
	if nested != nil {
		t.Fatal("a loaded listing produced another command, want nil")
	}

	return next
}

func loadRoot(t *testing.T, files ports.FileService) Model {
	t.Helper()

	m := New(files, "")

	return drain(t, m, m.Init())
}

// expand presses a key that must load a listing and applies the result.
func expand(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()

	next, cmd := m.Update(msg)
	if cmd == nil {
		t.Fatalf("%q produced no command, want a load command", msg.String())
	}

	return drain(t, next, cmd)
}

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func typed(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func labels(items []Item) []string {
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.Node.Name)
	}

	return got
}

func depths(items []Item) []int {
	got := make([]int, 0, len(items))
	for _, item := range items {
		got = append(got, item.Depth)
	}

	return got
}

func plain(view string) string {
	return ansiPattern.ReplaceAllString(view, "")
}

func TestInitLoadsRootLazily(t *testing.T) {
	files := treeFake(t)
	m := loadRoot(t, files)

	if got, want := labels(m.Visible()), []string{"docs", "src", ".gitignore", "README.md"}; !slices.Equal(got, want) {
		t.Fatalf("Visible() = %v, want %v", got, want)
	}
	if got, want := depths(m.Visible()), []int{0, 0, 0, 0}; !slices.Equal(got, want) {
		t.Errorf("depths = %v, want %v", got, want)
	}
	if got := files.CallCount("ReadDir"); got != 1 {
		t.Errorf("ReadDir calls = %d, want 1", got)
	}

	node, ok := m.Selected()
	if !ok {
		t.Fatal("Selected() ok = false, want true")
	}
	if node.Path != "docs" {
		t.Errorf("Selected().Path = %q, want %q", node.Path, "docs")
	}
}

func TestExpandCollapseAndJumpToParent(t *testing.T) {
	m := loadRoot(t, treeFake(t))

	m = expand(t, m, key(tea.KeyRight))
	if got, want := labels(m.Visible()), []string{"docs", "guide.md", "src", ".gitignore", "README.md"}; !slices.Equal(got, want) {
		t.Fatalf("after expand Visible() = %v, want %v", got, want)
	}
	if got, want := depths(m.Visible()), []int{0, 1, 0, 0, 0}; !slices.Equal(got, want) {
		t.Errorf("after expand depths = %v, want %v", got, want)
	}

	m, cmd := m.Update(key(tea.KeyLeft))
	if cmd != nil {
		t.Error("collapsing produced a command, want nil")
	}
	if got, want := labels(m.Visible()), []string{"docs", "src", ".gitignore", "README.md"}; !slices.Equal(got, want) {
		t.Errorf("after collapse Visible() = %v, want %v", got, want)
	}

	// A closed directory moves the cursor to its parent instead of collapsing twice.
	m, _ = m.Update(key(tea.KeyDown))
	m = expand(t, m, key(tea.KeyRight))
	m, _ = m.Update(key(tea.KeyDown))
	m, _ = m.Update(key(tea.KeyDown))
	m = expand(t, m, key(tea.KeyRight))

	if node, _ := m.Selected(); node.Path != "src/internal" {
		t.Fatalf("Selected().Path = %q, want %q", node.Path, "src/internal")
	}
	if got, want := labels(m.Visible()), []string{"docs", "src", "main.go", "internal", "logic.go", ".gitignore", "README.md"}; !slices.Equal(got, want) {
		t.Fatalf("Visible() = %v, want %v", got, want)
	}

	m, _ = m.Update(key(tea.KeyLeft))
	if got, want := labels(m.Visible()), []string{"docs", "src", "main.go", "internal", ".gitignore", "README.md"}; !slices.Equal(got, want) {
		t.Errorf("after collapsing internal Visible() = %v, want %v", got, want)
	}
	if node, _ := m.Selected(); node.Path != "src/internal" {
		t.Errorf("Selected().Path = %q, want %q", node.Path, "src/internal")
	}

	m, _ = m.Update(key(tea.KeyLeft))
	if node, _ := m.Selected(); node.Path != "src" {
		t.Errorf("Selected().Path = %q, want the parent %q", node.Path, "src")
	}
}

func TestEnterTogglesDirectoriesAndChoosesFiles(t *testing.T) {
	m := loadRoot(t, treeFake(t))

	m, cmd := m.Update(key(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("enter on a directory produced no command, want a load command")
	}
	if !m.expanded["docs"] {
		t.Error("docs was not expanded")
	}

	m = drain(t, m, cmd)
	m, cmd = m.Update(key(tea.KeyEnter))
	if cmd != nil {
		t.Error("enter on an expanded directory produced a command, want nil")
	}
	if m.expanded["docs"] {
		t.Error("docs was not collapsed")
	}

	m, _ = m.Update(key(tea.KeyEnd))
	if node, _ := m.Selected(); node.Path != "README.md" {
		t.Fatalf("Selected().Path = %q, want %q", node.Path, "README.md")
	}

	for _, msg := range []tea.KeyPressMsg{key(tea.KeyEnter), key(tea.KeySpace)} {
		next, cmd := m.Update(msg)
		if cmd == nil {
			t.Fatalf("%q on a file produced no command, want FileChosenMsg", msg.String())
		}

		chosen, ok := cmd().(FileChosenMsg)
		if !ok {
			t.Fatalf("%q command returned %T, want FileChosenMsg", msg.String(), cmd())
		}
		if chosen.Path != "README.md" {
			t.Errorf("FileChosenMsg.Path = %q, want %q", chosen.Path, "README.md")
		}
		if _, nested := next.Update(cmd()); nested != nil {
			t.Error("FileChosenMsg produced another command, want nil")
		}
	}
}

func TestKeyboardNavigation(t *testing.T) {
	m := loadRoot(t, treeFake(t))
	m.SetBounds(0, 0, 30, 2)

	m, _ = m.Update(key(tea.KeyUp))
	if m.cursor != 0 {
		t.Errorf("up at the first row: cursor = %d, want 0", m.cursor)
	}

	m, _ = m.Update(key(tea.KeyDown))
	m, _ = m.Update(key(tea.KeyDown))
	m, _ = m.Update(key(tea.KeyDown))
	if node, _ := m.Selected(); node.Path != "README.md" {
		t.Errorf("Selected().Path = %q, want %q", node.Path, "README.md")
	}
	if m.offset != 2 {
		t.Errorf("offset = %d, want 2 to keep the cursor visible", m.offset)
	}

	m, _ = m.Update(key(tea.KeyDown))
	if node, _ := m.Selected(); node.Path != "README.md" {
		t.Errorf("down past the last row moved to %q, want %q", node.Path, "README.md")
	}

	m, _ = m.Update(key(tea.KeyPgUp))
	if m.cursor != 1 {
		t.Errorf("pgup: cursor = %d, want 1", m.cursor)
	}
	if m.offset != 0 {
		t.Errorf("pgup: offset = %d, want 0", m.offset)
	}

	m, _ = m.Update(key(tea.KeyHome))
	if m.cursor != 0 {
		t.Errorf("home: cursor = %d, want 0", m.cursor)
	}

	m, _ = m.Update(key(tea.KeyEnd))
	if m.cursor != 3 {
		t.Errorf("end: cursor = %d, want 3", m.cursor)
	}

	m, _ = m.Update(key(tea.KeyPgDown))
	if m.cursor != 3 {
		t.Errorf("pgdown past the end: cursor = %d, want 3", m.cursor)
	}
}

// selectRow moves the cursor with the down key until the wanted node is selected.
func selectRow(t *testing.T, m Model, path domain.RelPath) Model {
	t.Helper()

	for range len(m.Visible()) + 1 {
		if node, ok := m.Selected(); ok && node.Path == path {
			return m
		}

		m, _ = m.Update(key(tea.KeyDown))
	}

	t.Fatalf("node %q is not selectable in %v", path, labels(m.Visible()))

	return m
}

func TestFilterSearchesLoadedNodesAndFlattensMatches(t *testing.T) {
	m := loadRoot(t, treeFake(t))
	m = expand(t, m, key(tea.KeyRight))
	m = expand(t, selectRow(t, m, "src"), key(tea.KeyRight))
	m = expand(t, selectRow(t, m, "src/internal"), key(tea.KeyRight))

	m, _ = m.Update(typed('/'))
	if !m.filtering || m.Filter() != "" {
		t.Fatalf("filtering = %v, filter = %q, want true and empty", m.filtering, m.Filter())
	}

	for _, r := range "gox" {
		m, _ = m.Update(typed(r))
	}
	m, _ = m.Update(key(tea.KeyBackspace))
	if m.Filter() != "go" {
		t.Fatalf("filter = %q, want %q", m.Filter(), "go")
	}

	if got, want := labels(m.Visible()), []string{"logic.go", "main.go"}; !slices.Equal(got, want) {
		t.Fatalf("matches = %v, want %v", got, want)
	}
	if got, want := depths(m.Visible()), []int{0, 0}; !slices.Equal(got, want) {
		t.Errorf("match depths = %v, want %v", got, want)
	}

	m, cmd := m.Update(key(tea.KeyEnter))
	if cmd != nil {
		t.Error("applying the filter produced a command, want nil")
	}
	if m.filtering {
		t.Error("filtering = true after enter, want false")
	}
	if got, want := labels(m.Visible()), []string{"logic.go", "main.go"}; !slices.Equal(got, want) {
		t.Errorf("applied filter matches = %v, want %v", got, want)
	}
	if view := plain(m.View()); !strings.Contains(view, "filtro: /go") || !strings.Contains(view, "logic.go") {
		t.Errorf("View() = %q, want the active filter and its matches", view)
	}

	m, _ = m.Update(key(tea.KeyEscape))
	if m.Filter() != "" {
		t.Errorf("filter = %q after esc, want empty", m.Filter())
	}
	want := []string{"docs", "guide.md", "src", "main.go", "internal", "logic.go", ".gitignore", "README.md"}
	if got := labels(m.Visible()); !slices.Equal(got, want) {
		t.Errorf("Visible() = %v, want the tree back %v", got, want)
	}
}

func TestFilterWithoutMatches(t *testing.T) {
	m := loadRoot(t, treeFake(t))

	m, _ = m.Update(typed('/'))
	for _, r := range "zzz" {
		m, _ = m.Update(typed(r))
	}

	if len(m.Visible()) != 0 {
		t.Errorf("Visible() = %v, want no matches", labels(m.Visible()))
	}
	if node, ok := m.Selected(); ok || node.Path != "" {
		t.Errorf("Selected() = %+v, %v, want the zero node and false", node, ok)
	}
	if view := plain(m.View()); !strings.Contains(view, "sin coincidencias") {
		t.Errorf("View() = %q, want the empty-match message", view)
	}
}

func TestMouseSelectionAndDoubleClick(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	m := loadRoot(t, treeFake(t))
	m.SetBounds(5, 2, 30, 10)
	m.now = func() time.Time { return base }

	m, cmd := m.Update(click(6, 3))
	if cmd != nil {
		t.Error("a single click produced a command, want nil")
	}
	if node, _ := m.Selected(); node.Path != "src" {
		t.Fatalf("Selected().Path = %q, want %q", node.Path, "src")
	}

	m, cmd = m.Update(click(6, 3))
	if cmd == nil {
		t.Fatal("the second click produced no command, want a load command")
	}
	if !m.expanded["src"] {
		t.Error("the double click did not expand src")
	}
	m = drain(t, m, cmd)

	m.now = func() time.Time { return base.Add(2 * time.Second) }
	m, cmd = m.Update(click(6, 3))
	if cmd != nil {
		t.Error("a click outside the double-click window produced a command, want nil")
	}
	if !m.expanded["src"] {
		t.Error("a click outside the double-click window collapsed src")
	}

	m.now = func() time.Time { return base.Add(2*time.Second + 100*time.Millisecond) }
	m, cmd = m.Update(click(6, 3))
	if cmd != nil {
		t.Error("collapsing a directory produced a command, want nil")
	}
	if m.expanded["src"] {
		t.Error("the second double click did not collapse src")
	}

	m.SetBounds(5, 2, 30, 10)
	m, _ = m.Update(click(6, 5))
	if node, _ := m.Selected(); node.Path != "README.md" {
		t.Fatalf("Selected().Path = %q, want %q", node.Path, "README.md")
	}

	next, cmd := m.Update(click(6, 5))
	if cmd == nil {
		t.Fatal("double clicking a file produced no command, want FileChosenMsg")
	}
	chosen, ok := cmd().(FileChosenMsg)
	if !ok {
		t.Fatalf("command returned %T, want FileChosenMsg", cmd())
	}
	if chosen.Path != "README.md" {
		t.Errorf("FileChosenMsg.Path = %q, want %q", chosen.Path, "README.md")
	}
	if _, nested := next.Update(cmd()); nested != nil {
		t.Error("FileChosenMsg produced another command, want nil")
	}
}

func TestMouseIgnoresClicksOutsideTheList(t *testing.T) {
	m := loadRoot(t, treeFake(t))
	m.SetBounds(5, 2, 30, 4)

	m, _ = m.Update(key(tea.KeyDown))
	if node, _ := m.Selected(); node.Path != "src" {
		t.Fatalf("Selected().Path = %q, want %q", node.Path, "src")
	}

	cases := []struct {
		name string
		msg  tea.Msg
	}{
		{name: "outside bounds", msg: click(1, 1)},
		{name: "right button", msg: tea.MouseClickMsg{X: 6, Y: 2, Button: tea.MouseRight}},
		{name: "wheel left", msg: tea.MouseWheelMsg{X: 6, Y: 2, Button: tea.MouseWheelLeft}},
		{name: "row below the listing", msg: click(6, 9)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, cmd := m.Update(tc.msg)
			if cmd != nil {
				t.Fatalf("command = %v, want nil", cmd())
			}
			if node, _ := next.Selected(); node.Path != "src" {
				t.Errorf("Selected().Path = %q, want %q", node.Path, "src")
			}
			if next.offset != m.offset {
				t.Errorf("offset = %d, want %d", next.offset, m.offset)
			}
		})
	}
}

func TestMouseWheelScrollsOffset(t *testing.T) {
	m := loadRoot(t, treeFake(t))
	m.SetBounds(0, 0, 30, 3)

	m, cmd := m.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown})
	if cmd != nil {
		t.Error("the wheel produced a command, want nil")
	}
	if m.offset != 1 {
		t.Errorf("offset = %d, want 1", m.offset)
	}

	m, _ = m.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelUp})
	if m.offset != 0 {
		t.Errorf("offset = %d, want 0", m.offset)
	}

	m, _ = m.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelUp})
	if m.offset != 0 {
		t.Errorf("offset = %d, want the scroll clamped to 0", m.offset)
	}
}

func TestViewStates(t *testing.T) {
	t.Run("loading", func(t *testing.T) {
		view := plain(New(treeFake(t), "").View())
		if !strings.Contains(view, "cargando") {
			t.Errorf("View() = %q, want the loading state", view)
		}
	})

	t.Run("empty", func(t *testing.T) {
		empty := fakes.NewFileService()
		empty.OnReadDir = func(_ context.Context, dir domain.RelPath) (domain.DirListing, error) {
			return domain.DirListing{Dir: dir}, nil
		}

		view := plain(loadRoot(t, empty).View())
		if !strings.Contains(view, "vacío") {
			t.Errorf("View() = %q, want the empty state", view)
		}
	})

	t.Run("error", func(t *testing.T) {
		fail := fakes.NewFileService()
		fail.OnReadDir = func(_ context.Context, _ domain.RelPath) (domain.DirListing, error) {
			return domain.DirListing{}, fmt.Errorf("%w: no se pudo leer", domain.ErrPermissionDenied)
		}

		m := loadRoot(t, fail)
		if m.err == nil {
			t.Fatal("err = nil, want the load failure")
		}
		if len(m.Visible()) != 0 {
			t.Errorf("Visible() = %v, want no rows", labels(m.Visible()))
		}

		view := plain(m.View())
		if !strings.Contains(view, "error") || !strings.Contains(view, "permiso denegado") {
			t.Errorf("View() = %q, want the error state", view)
		}
	})

	t.Run("rows", func(t *testing.T) {
		m := loadRoot(t, treeFake(t))

		collapsed := plain(m.View())
		if !strings.Contains(collapsed, "▸ docs/") {
			t.Errorf("View() = %q, want the collapsed chevron on docs", collapsed)
		}

		m = expand(t, m, key(tea.KeyRight))
		view := plain(m.View())

		for _, want := range []string{"▾ docs/", "▸ src/", "guide.md", ".gitignore"} {
			if !strings.Contains(view, want) {
				t.Errorf("View() = %q, want %q", view, want)
			}
		}
	})
}

func TestMissingServiceReportsAnError(t *testing.T) {
	m := New(nil, "")

	m = drain(t, m, m.Init())

	if !errors.Is(m.err, errNoService) {
		t.Fatalf("err = %v, want %v", m.err, errNoService)
	}
	if view := plain(m.View()); !strings.Contains(view, "error") {
		t.Errorf("View() = %q, want the error state", view)
	}
}

func TestSetBoundsRecomputesViewport(t *testing.T) {
	m := loadRoot(t, treeFake(t))

	m.SetBounds(3, 4, 20, 2)
	if m.bounds.Min.X != 3 || m.bounds.Min.Y != 4 || m.bounds.Dx() != 20 || m.bounds.Dy() != 2 {
		t.Fatalf("bounds = %v, want the requested rectangle", m.bounds)
	}
	if len(m.rowRects) != 2 {
		t.Fatalf("len(rowRects) = %d, want 2", len(m.rowRects))
	}
	if m.rowRects[0].Min.Y != 4 || m.rowRects[1].Min.Y != 5 {
		t.Errorf("rowRects = %v, want rows at y=4 and y=5", m.rowRects)
	}
	if m.rowRects[0].Min.X != 3 || m.rowRects[0].Max.X != 23 {
		t.Errorf("rowRects[0] = %v, want x from 3 to 23", m.rowRects[0])
	}

	m.SetBounds(3, 4, 20, 1)
	if len(m.rowRects) != 1 {
		t.Errorf("len(rowRects) = %d, want 1", len(m.rowRects))
	}

	m.SetBounds(3, 4, 20, 10)
	if len(m.rowRects) != len(m.Visible()) {
		t.Errorf("len(rowRects) = %d, want one rect per visible row (%d)", len(m.rowRects), len(m.Visible()))
	}
}

func TestUpdateIgnoresUnrelatedMessages(t *testing.T) {
	m := loadRoot(t, treeFake(t))

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd != nil {
		t.Errorf("command = %v, want nil", cmd())
	}
	if len(next.Visible()) != len(m.Visible()) {
		t.Errorf("Visible() = %v, want it unchanged", labels(next.Visible()))
	}
}

func TestLoadingReportsPendingReads(t *testing.T) {
	files := treeFake(t)
	m := New(files, "")

	if !m.Loading() {
		t.Error("Loading() = false before Init, want true")
	}
	if m.Err() != nil {
		t.Errorf("Err() = %v, want nil", m.Err())
	}

	m = drain(t, m, m.Init())
	if m.Loading() {
		t.Error("Loading() = true after the listing, want false")
	}
	if m.Err() != nil {
		t.Errorf("Err() = %v, want nil", m.Err())
	}
}

func TestLoadingAndErrReportFailures(t *testing.T) {
	fail := fakes.NewFileService()
	fail.OnReadDir = func(_ context.Context, _ domain.RelPath) (domain.DirListing, error) {
		return domain.DirListing{}, fmt.Errorf("%w: no se pudo leer", domain.ErrPermissionDenied)
	}

	m := loadRoot(t, fail)
	if m.Loading() {
		t.Error("Loading() = true after a failure, want false")
	}
	if !errors.Is(m.Err(), domain.ErrPermissionDenied) {
		t.Errorf("Err() = %v, want %v", m.Err(), domain.ErrPermissionDenied)
	}
}
