package app

import (
	"image"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

// toastPrefix marks the statusbar toast, distinct from the panel error lines.
const toastPrefix = "aviso: "

// tabSeparator joins the top labels; the mouse hit-testing relies on its width.
const tabSeparator = "│"

// resize recomputes the canvas and the panel rectangles; the root owns hit-testing.
func (m *Model) resize() {
	width, height := m.canvas()
	body := bodyHeight(height)
	treeW := splitWidth(width)
	editorW := max(width-treeW, 0)

	m.tree.SetBounds(0, headerRows, treeW, body)
	if m.hasEditor {
		m.editor.SetSize(editorW, body)
	}
	m.git.SetBounds(0, headerRows, width, body)
	m.docker.SetBounds(0, headerRows, width, body)

	m.treeRect = image.Rect(0, headerRows, treeW, headerRows+body)
	m.editorRect = image.Rect(treeW, headerRows, width, headerRows+body)
	m.gitRect = image.Rect(0, headerRows, width, headerRows+body)
	m.dockerRect = image.Rect(0, headerRows, width, headerRows+body)
	m.markTabs(width)
}

// markTabs stores the rectangle of every top tab, on the row between the title and the body.
func (m *Model) markTabs(width int) {
	m.tabRects = [tabCount]image.Rectangle{}

	column := 0
	for tab := Tab(0); tab < tabCount; tab++ {
		size := lipgloss.Width(tabLabels[tab])
		if column+size > width {
			return // the labels a narrow row cannot show are not clickable
		}

		m.tabRects[tab] = image.Rect(column, headerRows-1, column+size, headerRows)
		column += size + lipgloss.Width(tabSeparator)
	}
}

func (m Model) canvas() (int, int) {
	return max(m.width, minWidth), max(m.height, minHeight)
}

func bodyHeight(height int) int {
	return max(height-headerRows-footerRows, 0)
}

// splitWidth reserves the left column for the tree, capped so the editor keeps a minimum.
func splitWidth(total int) int {
	tree := clamp(total*treePercent/100, minTree, max(total-minEditorBox, minTree))

	return clamp(tree, 0, total)
}

// render composes the header, the active tab and the statusbar.
func (m Model) render() string {
	width, height := m.canvas()

	lines := make([]string, 0, height)
	lines = append(lines, styles.Title.Render(clipCells(" tui-editor ", width)))
	lines = append(lines, m.tabsLine(width))
	lines = append(lines, m.body(width, height)...)
	lines = append(lines, m.statusLine(width))

	content := strings.Join(lines, "\n")
	if m.help {
		return m.helpOverlay(content, width, height)
	}

	return content
}

// body renders the panels of the active tab, always exactly bodyHeight rows high.
func (m Model) body(width, height int) []string {
	rows := bodyHeight(height)

	switch m.tab {
	case GitTab:
		return fitBox(m.git.View(), width, rows)
	case DockerTab:
		return fitBox(m.docker.View(), width, rows)
	}

	treeW := splitWidth(width)
	editorW := max(width-treeW, 0)

	treeLines := fitBox(m.tree.View(), treeW, rows)
	editorLines := fitBox(m.editorBox(editorW, rows), editorW, rows)

	out := make([]string, 0, rows)
	for i := range treeLines {
		out = append(out, treeLines[i]+editorLines[i])
	}

	return out
}

// editorBox renders the preview, or the placeholder before a file is chosen.
func (m Model) editorBox(width, height int) string {
	if m.hasEditor {
		return m.editor.View()
	}

	return lipgloss.Place(max(width, 1), max(height, 1), lipgloss.Center, lipgloss.Center,
		styles.Placeholder.Render("selecciona un archivo"))
}

func (m Model) tabsLine(width int) string {
	parts := make([]string, 0, tabCount*2-1)

	for tab := Tab(0); tab < tabCount; tab++ {
		if tab > 0 {
			parts = append(parts, styles.TabSeparator.Render(tabSeparator))
		}

		style := styles.Tab
		if tab == m.tab {
			style = styles.TabActive
		}

		parts = append(parts, style.Render(tabLabels[tab]))
	}

	return padLine(strings.Join(parts, ""), width)
}

// statusLine shows the repo, the tab, the focus, the hints, the spinner and the toast.
func (m Model) statusLine(width int) string {
	parts := []string{
		"repo: " + m.repoLabel(),
		"tab: " + m.tab.String(),
		"foco: " + m.focusLabel(),
		"1/2/3 tabs",
		"tab foco",
		"? ayuda",
		"q salir",
	}

	text := strings.Join(parts, " · ")
	if m.toast != "" {
		text = m.toast + " · " + text
	}
	if m.loading() {
		text = m.spinner.View() + " " + text
	}

	return styles.Status.Render(clipCells(text, width))
}

// helpOverlay centers the help box over the layout without changing its size.
func (m Model) helpOverlay(base string, width, height int) string {
	box := styles.Help.Render(strings.Join(helpLines(), "\n"))
	boxLines := strings.Split(box, "\n")

	left := max((width-lipgloss.Width(box))/2, 0)
	top := max((height-len(boxLines))/2, 0)

	lines := strings.Split(base, "\n")
	for i, line := range boxLines {
		row := top + i
		if row < 0 || row >= len(lines) {
			continue
		}

		lines[row] = strings.Repeat(" ", left) + line
	}

	return strings.Join(lines, "\n")
}

// fitBox splits a rendered view into exactly height rows, each padded to width cells.
func fitBox(view string, width, height int) []string {
	lines := strings.Split(view, "\n")
	out := make([]string, 0, max(height, 0))

	for i := 0; i < height; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}

		out = append(out, padLine(line, width))
	}

	return out
}

// padLine clips line to width cells and fills the rest with spaces.
func padLine(line string, width int) string {
	if width <= 0 {
		return ""
	}

	line = clipCells(line, width)
	if gap := width - lipgloss.Width(line); gap > 0 {
		return line + strings.Repeat(" ", gap)
	}

	return line
}

// clipCells cuts a styled string to width visible cells, keeping the ANSI intact.
func clipCells(s string, width int) string {
	if width <= 0 {
		return ""
	}

	var b strings.Builder
	visible := 0
	inEscape := false

	for _, r := range s {
		if r == 0x1b {
			inEscape = true
			b.WriteRune(r)

			continue
		}
		if inEscape {
			b.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}

			continue
		}
		if visible >= width {
			break
		}

		b.WriteRune(r)
		visible++
	}

	return b.String()
}

func clamp(value, low, high int) int {
	if high < low {
		return low
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}

	return value
}
