package app

import (
	"fmt"
	"image"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

// toastPrefix marks the statusbar toast, distinct from the panel error lines.
const toastPrefix = "aviso: "

// tabSeparator joins the top labels; the mouse hit-testing relies on its width.
const tabSeparator = "│"

// borderSide is the chrome one panel box spends on each of its four sides.
const borderSide = 1

// helpPadding is the horizontal padding styles.Help adds on each side.
const helpPadding = 2

// overlayBoxChrome is the cells the shared overlay box spends: a frame plus the
// padding on both sides. Long lines are clipped to what is left of the canvas.
const overlayBoxChrome = 2 * (borderSide + helpPadding)

// resetStyle closes the attributes a clipped panel row may leave open.
const resetStyle = "\x1b[m"

// resize recomputes the canvas and the panel rectangles; the root owns hit-testing.
func (m *Model) resize() {
	width, height := m.canvas()
	body := bodyHeight(height)
	treeW := splitWidth(width)
	editorW := max(width-treeW, 0)

	// The frames are chrome: every rectangle is the inner area of its box, one cell inside.
	treeInner := innerRect(0, headerRows, treeW, body)
	editorInner := innerRect(treeW, headerRows, editorW, body)
	panelInner := innerRect(0, headerRows, width, body)

	m.tree.SetBounds(treeInner.Min.X, treeInner.Min.Y, treeInner.Dx(), treeInner.Dy())
	if m.hasEditor {
		m.editor.SetSize(editorInner.Dx(), editorInner.Dy())
	}
	m.git.SetBounds(panelInner.Min.X, panelInner.Min.Y, panelInner.Dx(), panelInner.Dy())
	m.docker.SetBounds(panelInner.Min.X, panelInner.Min.Y, panelInner.Dx(), panelInner.Dy())

	m.treeRect, m.editorRect = treeInner, editorInner
	m.gitRect, m.dockerRect = panelInner, panelInner
	m.markTabs(width)
}

// innerRect is the area left inside the frame of a box at x, y with the given outer size.
func innerRect(x, y, width, height int) image.Rectangle {
	x, y = x+borderSide, y+borderSide

	return image.Rect(x, y, x+innerCells(width), y+innerCells(height))
}

// innerCells returns the cells left by the frame of a box, never negative.
func innerCells(size int) int {
	return max(size-2*borderSide, 0)
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
	switch {
	case m.help:
		return m.helpOverlay(content, width, height)
	case m.configOpen:
		return m.configOverlay(content, width, height)
	}

	return content
}

// body renders the framed panels of the active tab, always exactly bodyHeight rows high.
func (m Model) body(width, height int) []string {
	rows := bodyHeight(height)

	switch m.tab {
	case GitTab:
		return fitBox(m.box(m.git.View(), width, rows, m.focus == FocusGit), width, rows)
	case DockerTab:
		return fitBox(m.box(m.docker.View(), width, rows, m.focus == FocusDocker), width, rows)
	}

	treeW := splitWidth(width)
	editorW := max(width-treeW, 0)

	treeLines := fitBox(m.box(m.tree.View(), treeW, rows, m.focus == FocusTree), treeW, rows)
	editorLines := fitBox(m.box(m.editorBox(innerCells(editorW), innerCells(rows)), editorW, rows, m.focus == FocusEditor), editorW, rows)

	out := make([]string, 0, rows)
	for i := range treeLines {
		out = append(out, treeLines[i]+editorLines[i])
	}

	return out
}

// box draws content inside a framed box of exactly width x height cells; focused picks the token.
func (m Model) box(content string, width, height int, focused bool) string {
	innerWidth, innerHeight := innerCells(width), innerCells(height)
	if innerWidth < 1 || innerHeight < 1 {
		// No room for a frame: the bare content keeps the layout tiling exactly.
		return strings.Join(fitBox(content, width, height), "\n")
	}

	lines := fitBox(content, innerWidth, innerHeight)
	for i, line := range lines {
		lines[i] = line + resetStyle // an attribute left open must not reach the frame
	}

	style := styles.Border
	if focused {
		style = styles.BorderFocused
	}

	return style.Render(strings.Join(lines, "\n"))
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
		m.tabsHint(),
		m.keys.Label(keymap.ActionFocusNext) + " foco",
		m.keys.Label(keymap.ActionHelp) + " ayuda",
		m.keys.Label(keymap.ActionQuit) + " salir",
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

// tabsHint names the keys that switch the top tabs, in tab order.
func (m Model) tabsHint() string {
	actions := [...]keymap.Action{keymap.ActionTabFiles, keymap.ActionTabGit, keymap.ActionTabDocker}

	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		labels = append(labels, m.keys.Label(action))
	}

	return strings.Join(labels, "/") + " tabs"
}

// helpOverlay centers the help box over the layout without changing its size.
func (m Model) helpOverlay(base string, width, height int) string {
	box := styles.Help.Render(strings.Join(helpLines(m.keys), "\n"))

	return overlayBox(base, box, width, height)
}

// configOverlay centers the read-only configuration box over the layout.
func (m Model) configOverlay(base string, width, height int) string {
	box := styles.Help.Render(strings.Join(m.configLines(width), "\n"))

	return overlayBox(base, box, width, height)
}

// configLines renders the configuration summary: the file, the theme, the key
// overrides and the restart notice. Long lines are clipped to the canvas.
func (m Model) configLines(width int) []string {
	actions := m.overrideActions()

	lines := make([]string, 0, len(actions)+6)
	lines = append(lines, styles.HelpTitle.Render("configuración"))
	lines = append(lines, "archivo: "+m.configFileLabel())
	lines = append(lines, "tema: "+styles.CurrentTheme().Name)
	lines = append(lines, fmt.Sprintf("overrides: %d", len(actions)))

	if len(actions) == 0 {
		lines = append(lines, "sin overrides")
	} else {
		for _, action := range actions {
			lines = append(lines, "  "+action+": "+strings.Join(m.cfg.Keys[action], " / "))
		}
	}

	lines = append(lines, "")
	lines = append(lines, "edita el archivo y reinicia para aplicar")
	lines = append(lines, m.keys.Label(keymap.ActionConfig)+" / esc: cerrar")

	// A long path must not widen the box past the canvas and break the layout.
	limit := max(width-overlayBoxChrome, 1)
	for index, line := range lines {
		lines[index] = clipCells(line, limit)
	}

	return lines
}

// configFileLabel names the loaded file, or reports the built-in defaults.
func (m Model) configFileLabel() string {
	if strings.TrimSpace(m.cfg.ConfigPath) == "" {
		return "sin archivo (defaults)"
	}

	return m.cfg.ConfigPath
}

// overrideActions returns the actions with at least one override key, sorted.
func (m Model) overrideActions() []string {
	actions := make([]string, 0, len(m.cfg.Keys))
	for action, keys := range m.cfg.Keys {
		if len(keys) > 0 {
			actions = append(actions, action)
		}
	}

	sort.Strings(actions)

	return actions
}

// overlayBox centers an already rendered box over base without resizing base.
func overlayBox(base, box string, width, height int) string {
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
