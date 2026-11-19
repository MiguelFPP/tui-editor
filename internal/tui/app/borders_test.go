package app

import (
	"context"
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

// canvasSizes are the terminal sizes every frame has to lay out against.
var canvasSizes = []struct {
	name          string
	width, height int
}{
	{name: "minimum", width: minWidth, height: minHeight},
	{name: "default", width: defaultWidth, height: defaultHeight},
	{name: "wide", width: 120, height: 40},
}

// twoContainersDocker turns the Docker fake into a list of two named containers.
func twoContainersDocker(t *testing.T, deps Deps) {
	t.Helper()

	docker := deps.Docker.(*fakes.DockerService)
	docker.OnContainers = func(context.Context, domain.ContainerScope) ([]domain.ContainerSummary, error) {
		return []domain.ContainerSummary{
			{ID: "aaaaaaaaaaaaaaaa", Name: "web", Image: "nginx:latest", State: domain.ContainerRunning},
			{ID: "bbbbbbbbbbbbbbbb", Name: "db", Image: "postgres:16", State: domain.ContainerExited},
		}, nil
	}
}

// dockerTitle returns the plain text of the first row of the Docker panel.
func dockerTitle(m Model) string {
	rows := strings.Split(m.View().Content, "\n")
	if m.dockerRect.Min.Y < 0 || m.dockerRect.Min.Y >= len(rows) {
		return ""
	}

	return plainCells(rows[m.dockerRect.Min.Y])
}

// outerRect grows an inner panel rectangle by the frame its box draws around it.
func outerRect(inner image.Rectangle) image.Rectangle {
	return image.Rect(
		inner.Min.X-borderSide, inner.Min.Y-borderSide,
		inner.Max.X+borderSide, inner.Max.Y+borderSide,
	)
}

// panelRects returns the inner rectangle of every panel of the active tab.
func panelRects(m Model) map[string]image.Rectangle {
	switch m.tab {
	case GitTab:
		return map[string]image.Rectangle{"git": m.gitRect}
	case DockerTab:
		return map[string]image.Rectangle{"docker": m.dockerRect}
	default:
		return map[string]image.Rectangle{"tree": m.treeRect, "editor": m.editorRect}
	}
}

// frameEscape is the ANSI prefix the frame token of a style emits.
func frameEscape(t *testing.T, style lipgloss.Style) string {
	t.Helper()

	rendered := style.Render("x")
	end := strings.IndexByte(rendered, 'm')
	if end < 0 {
		t.Fatalf("token rendered %q, want an ANSI escape", rendered)
	}

	return rendered[:end+1]
}

// cellRange returns the rendered cells between two columns of a row, escapes included.
func cellRange(row string, from, to int) string {
	var b strings.Builder

	visible := 0
	inEscape := false

	for _, r := range row {
		switch {
		case r == 0x1b:
			inEscape = true
			if visible >= from && visible < to {
				b.WriteRune(r)
			}
		case inEscape:
			if visible >= from && visible < to {
				b.WriteRune(r)
			}
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
		default:
			if visible >= from && visible < to {
				b.WriteRune(r)
			}
			visible++
		}
	}

	return b.String()
}

// rowCells returns the plain cells of one rendered row.
func rowCells(t *testing.T, content string, y int) []rune {
	t.Helper()

	rows := strings.Split(content, "\n")
	if y < 0 || y >= len(rows) {
		t.Fatalf("row %d is outside the %d rendered rows", y, len(rows))
	}

	return []rune(plainCells(rows[y]))
}

// assertGlyph fails when the cell x of a row is not want.
func assertGlyph(t *testing.T, row []rune, x int, want rune) {
	t.Helper()

	if x < 0 || x >= len(row) {
		t.Fatalf("column %d is outside the %d rendered cells", x, len(row))
	}
	if row[x] != want {
		t.Errorf("cell %d = %q, want %q", x, row[x], want)
	}
}

// assertFrameSpan fails when the frame around inner does not cover the columns from..to.
func assertFrameSpan(t *testing.T, name string, inner image.Rectangle, from, to int) {
	t.Helper()

	outer := outerRect(inner)
	if outer.Min.X != from || outer.Max.X != to {
		t.Errorf("%s frame spans columns %d..%d, want %d..%d", name, outer.Min.X, outer.Max.X, from, to)
	}
}

// assertFrame reports whether the frame corner at column x of a row carries an escape.
func assertFrame(t *testing.T, row string, x int, escape string, want bool) {
	t.Helper()

	cell := cellRange(row, x, x+1)
	if got := strings.Contains(cell, escape); got != want {
		t.Errorf("frame corner at column %d = %q, want the escape %q present %v", x, cell, escape, want)
	}
}

func TestFramedPanelsFillTheCanvasExactly(t *testing.T) {
	for _, size := range canvasSizes {
		for _, tab := range []Tab{FilesTab, GitTab, DockerTab} {
			t.Run(size.name+"/"+tab.String(), func(t *testing.T) {
				m := newSized(t, testDeps(t), size.width, size.height)
				m, _ = press(t, m, tabKey(tab))

				content := m.View().Content
				rows := strings.Split(content, "\n")
				if len(rows) != size.height {
					t.Fatalf("rendered %d rows, want %d", len(rows), size.height)
				}

				// No row may overflow the canvas; the body and the statusbar fill it exactly.
				body := bodyHeight(size.height)
				bottom := headerRows + body - 1
				for i, row := range rows {
					width := lipgloss.Width(row)
					if width > size.width {
						t.Errorf("row %d = %d cells, want at most %d", i, width, size.width)
					}
					if i >= headerRows && width != size.width {
						t.Errorf("row %d = %d cells, want the full width %d", i, width, size.width)
					}
				}

				// Every frame stays one cell inside the canvas and covers the body from end to end.
				for name, inner := range panelRects(m) {
					outer := outerRect(inner)
					if outer.Min.Y != headerRows || outer.Max.Y != headerRows+body {
						t.Errorf("%s frame spans rows %d..%d, want the body %d..%d", name, outer.Min.Y, outer.Max.Y, headerRows, headerRows+body)
					}
					if inner.Dx() < 1 || inner.Dy() < 1 {
						t.Errorf("%s inner area = %dx%d, want at least one cell per side", name, inner.Dx(), inner.Dy())
					}
					if outer.Min.X < 0 || outer.Max.X > size.width {
						t.Errorf("%s frame spans columns %d..%d, want it inside the %d cells of the canvas", name, outer.Min.X, outer.Max.X, size.width)
					}
				}

				// The frames tile the body: no gap between them and no overflow past the canvas.
				treeW := splitWidth(size.width, 0)
				switch m.tab {
				case GitTab:
					assertFrameSpan(t, "git", m.gitRect, 0, size.width)
				case DockerTab:
					assertFrameSpan(t, "docker", m.dockerRect, 0, size.width)
				default:
					assertFrameSpan(t, "tree", m.treeRect, 0, treeW)
					assertFrameSpan(t, "editor", m.editorRect, treeW, size.width)
				}

				top := rowCells(t, content, headerRows)
				assertGlyph(t, top, 0, '╭')
				assertGlyph(t, top, size.width-1, '╮')
				assertGlyph(t, rowCells(t, content, bottom), 0, '╰')
				assertGlyph(t, rowCells(t, content, bottom), size.width-1, '╯')

				// The two frames of the Files tab meet one cell inside the canvas.
				if m.tab == FilesTab {
					assertGlyph(t, top, treeW-1, '╮')
					assertGlyph(t, top, treeW, '╭')
					assertGlyph(t, rowCells(t, content, headerRows+1), treeW-1, '│')
					assertGlyph(t, rowCells(t, content, headerRows+1), treeW, '│')
				}

				// The side frames sit on the first and the last column of every inner row.
				for y := headerRows + 1; y < bottom; y++ {
					row := rowCells(t, content, y)
					assertGlyph(t, row, 0, '│')
					assertGlyph(t, row, size.width-1, '│')
				}
			})
		}
	}
}

func TestFocusedPanelUsesTheAccentFrame(t *testing.T) {
	accent := frameEscape(t, styles.BorderFocused)
	muted := frameEscape(t, styles.Border)
	if accent == muted {
		t.Fatalf("the frame tokens render the same escape %q, want two distinct tokens", accent)
	}

	m := newSized(t, testDeps(t), 120, 40)

	if got := m.box("x", 5, 3, true); !strings.Contains(got, accent) {
		t.Errorf("box(focused) = %q, want the accent frame %q", got, accent)
	}
	if got := m.box("x", 5, 3, false); strings.Contains(got, accent) {
		t.Errorf("box(unfocused) = %q, want no accent frame", got)
	}
	if got := m.box("x", 5, 3, false); !strings.Contains(got, muted) {
		t.Errorf("box(unfocused) = %q, want the muted frame %q", got, muted)
	}

	// The tree owns the keyboard of the Files tab, so its frame carries the accent.
	rows := strings.Split(m.View().Content, "\n")
	assertFrame(t, rows[headerRows], m.treeRect.Min.X-borderSide, accent, true)
	assertFrame(t, rows[headerRows], m.treeRect.Min.X-borderSide, muted, false)
	assertFrame(t, rows[headerRows], m.editorRect.Min.X-borderSide, accent, false)
	assertFrame(t, rows[headerRows], m.editorRect.Min.X-borderSide, muted, true)

	// Opening a file hands the keyboard to the editor and swaps the two frames.
	m = openFile(t, m, "main.go")
	if m.Focus() != FocusEditor {
		t.Fatalf("Focus() = %v, want the editor", m.Focus())
	}

	rows = strings.Split(m.View().Content, "\n")
	assertFrame(t, rows[headerRows], m.treeRect.Min.X-borderSide, accent, false)
	assertFrame(t, rows[headerRows], m.treeRect.Min.X-borderSide, muted, true)
	assertFrame(t, rows[headerRows], m.editorRect.Min.X-borderSide, accent, true)
	assertFrame(t, rows[headerRows], m.editorRect.Min.X-borderSide, muted, false)

	// A tab with one panel frames it with the accent too.
	m, _ = press(t, m, typed('2'))
	rows = strings.Split(m.View().Content, "\n")
	assertFrame(t, rows[headerRows], m.gitRect.Min.X-borderSide, accent, true)
}

func TestClicksOnTheInnerEdgeSelectTheRowUnderTheCursor(t *testing.T) {
	t.Run("tree", func(t *testing.T) {
		m := newSized(t, testDeps(t), 120, 40)
		m = drain(t, m, m.Init())

		m = update(t, m, click(m.treeRect.Min.X, m.treeRect.Min.Y))
		if node, ok := m.tree.Selected(); !ok || node.Path != "main.go" {
			t.Errorf("Selected() = %q (%v), want main.go from the first row inside the frame", node.Path, ok)
		}

		m = update(t, m, click(m.treeRect.Min.X, m.treeRect.Min.Y+1))
		if node, ok := m.tree.Selected(); !ok || node.Path != "long.txt" {
			t.Errorf("Selected() = %q (%v), want long.txt from the second row inside the frame", node.Path, ok)
		}
	})

	t.Run("docker", func(t *testing.T) {
		deps := testDeps(t)
		twoContainersDocker(t, deps)

		m := newSized(t, deps, 120, 40)
		m, _ = press(t, m, typed('3'))
		m, cmd := press(t, m, typed('r')) // r loads the list without arming the ticker
		m = drain(t, m, cmd)

		if title := dockerTitle(m); !strings.Contains(title, "web") {
			t.Fatalf("docker title = %q, want the first container selected", title)
		}

		// Two header rows inside the frame, then one row per container.
		m = update(t, m, click(m.dockerRect.Min.X, m.dockerRect.Min.Y+3))
		if title := dockerTitle(m); !strings.Contains(title, "db") {
			t.Errorf("docker title = %q, want the container of the clicked row", title)
		}
	})

	t.Run("git", func(t *testing.T) {
		deps := testDeps(t)
		git := twoFilesGit(t, deps)

		m := newSized(t, deps, 120, 40)
		m = drain(t, m, m.Init())
		m = update(t, m, typed('2'))

		// The first clickable row of the framed list.
		list := m.git.ListBounds()
		m = update(t, m, click(list.Min.X, list.Min.Y+1))

		model, cmd := press(t, m, typed('s'))
		model = drain(t, model, cmd)

		if got := lastStagePaths(t, git); len(got) != 1 || got[0] != "long.txt" {
			t.Errorf("Stage paths = %q, want the row clicked at the inner edge", got)
		}
		if model.git.CapturesInput() {
			t.Error("CapturesInput() = true, want the stage done")
		}
	})
}

func TestClicksOnTheFrameNeverChangeTheSelection(t *testing.T) {
	t.Run("files", func(t *testing.T) {
		m := newSized(t, testDeps(t), 120, 40)
		m = drain(t, m, m.Init())

		m = update(t, m, click(m.treeRect.Min.X, m.treeRect.Min.Y+1))
		if node, ok := m.tree.Selected(); !ok || node.Path != "long.txt" {
			t.Fatalf("Selected() = %q (%v), want long.txt before the frame clicks", node.Path, ok)
		}

		tree, editor := m.treeRect, m.editorRect
		cases := []struct {
			name string
			x, y int
		}{
			{name: "left frame of the tree", x: tree.Min.X - borderSide, y: tree.Min.Y},
			{name: "right frame of the tree", x: tree.Max.X, y: tree.Min.Y},
			{name: "top frame of the tree", x: tree.Min.X, y: tree.Min.Y - borderSide},
			{name: "bottom frame of the tree", x: tree.Min.X, y: tree.Max.Y},
			{name: "left frame of the editor", x: editor.Min.X - borderSide, y: editor.Min.Y},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := update(t, m, click(tc.x, tc.y))
				if node, ok := got.tree.Selected(); !ok || node.Path != "long.txt" {
					t.Errorf("Selected() = %q (%v), want long.txt kept", node.Path, ok)
				}
				if got.Focus() != m.Focus() {
					t.Errorf("Focus() = %v, want %v kept", got.Focus(), m.Focus())
				}
			})
		}
	})

	t.Run("git", func(t *testing.T) {
		deps := testDeps(t)
		git := twoFilesGit(t, deps)

		m := newSized(t, deps, 120, 40)
		m = drain(t, m, m.Init())
		m = update(t, m, typed('2'))

		// The column frames are chrome: rows start inside the list bounds.
		list := m.git.ListBounds()
		m = update(t, m, click(list.Min.X+1, list.Min.Y+1))                // long.txt
		m = update(t, m, click(m.gitRect.Min.X-borderSide, gitBodyTop(m))) // the left frame

		model, cmd := press(t, m, typed('s'))
		model = drain(t, model, cmd)

		if got := lastStagePaths(t, git); len(got) != 1 || got[0] != "long.txt" {
			t.Errorf("Stage paths = %q, want the selection kept by the frame click", got)
		}
		if model.git.CapturesInput() {
			t.Error("CapturesInput() = true, want the stage done")
		}
	})

	t.Run("docker", func(t *testing.T) {
		deps := testDeps(t)
		twoContainersDocker(t, deps)

		m := newSized(t, deps, 120, 40)
		m, _ = press(t, m, typed('3'))
		m, cmd := press(t, m, typed('r'))
		m = drain(t, m, cmd)

		m = update(t, m, click(m.dockerRect.Min.X, m.dockerRect.Min.Y+3))          // db
		m = update(t, m, click(m.dockerRect.Min.X-borderSide, m.dockerRect.Min.Y)) // the left frame

		if title := dockerTitle(m); !strings.Contains(title, "db") {
			t.Errorf("docker title = %q, want the selection kept by the frame click", title)
		}
	})
}
