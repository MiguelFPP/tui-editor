package gitpanel

import (
	"strconv"
	"strings"
)

// View composes the header, the section tabs and the body of the active section.
func (m Model) View() string {
	lines := make([]string, 0, m.bodyHeight()+headerRows+footerRows)

	lines = append(lines, styles.title.Render(truncate(m.titleText(), m.width)))
	lines = append(lines, m.tabs())

	if m.help {
		return strings.Join(append(lines, m.helpLines()...), "\n")
	}

	lines = append(lines, m.body()...)
	lines = append(lines, styles.meta.Render(truncate(m.footerText(), m.width)))

	return strings.Join(lines, "\n")
}

func (m Model) titleText() string {
	switch {
	case m.errs.status != nil:
		return "git · error: " + describeError(m.errs.status)
	case m.loading.status:
		return "git · cargando…"
	case m.status.Unborn:
		return "git · rama " + m.status.Branch + " · sin commits"
	case m.status.Detached:
		return "git · HEAD desacoplado · " + changeLabel(len(m.status.Files))
	}

	parts := []string{"git", "rama " + m.status.Branch}
	if m.status.Upstream != "" {
		parts = append(parts, m.status.Upstream)
	}
	if m.status.Ahead > 0 {
		parts = append(parts, "↑"+strconv.Itoa(m.status.Ahead))
	}
	if m.status.Behind > 0 {
		parts = append(parts, "↓"+strconv.Itoa(m.status.Behind))
	}

	return strings.Join(append(parts, changeLabel(len(m.status.Files))), " · ")
}

func changeLabel(count int) string {
	switch count {
	case 0:
		return "sin cambios"
	case 1:
		return "1 cambio"
	default:
		return strconv.Itoa(count) + " cambios"
	}
}

func (m Model) tabs() string {
	labels := make([]string, 0, sectionCount)
	for section := Section(0); section < sectionCount; section++ {
		labels = append(labels, section.String())
	}

	// The plain text decides the cut: ANSI escapes must never reach the rune limit.
	joined := strings.Join(labels, " │ ")
	if width := m.width; width > 0 && len([]rune(joined)) > width {
		return truncate(joined, width)
	}

	rendered := make([]string, 0, sectionCount)
	for index, label := range labels {
		if Section(index) == m.section {
			rendered = append(rendered, styles.tabOn.Render(label))

			continue
		}

		rendered = append(rendered, styles.tab.Render(label))
	}

	return strings.Join(rendered, " │ ")
}

// body renders the visible window of the active section.
func (m Model) body() []string {
	height := m.bodyHeight()
	rows := m.active()

	if state, ok := m.stateLine(); ok {
		return pad([]string{state}, height)
	}

	start := m.offsets[m.section]
	end := min(start+height, len(rows))

	window := make([]string, 0, height)
	for i := start; i < end; i++ {
		line := truncate(rows[i], m.width)
		if i == m.cursors[m.section] {
			line = styles.cursor.Render(line)
		}

		window = append(window, line)
	}

	return pad(window, height)
}

// stateLine is the message that replaces the rows while loading or when there is nothing to show.
func (m Model) stateLine() (string, bool) {
	if err := m.sectionError(m.section); err != nil {
		return styles.fail.Render(truncate("error: "+describeError(err), m.width)), true
	}
	if m.sectionLoading(m.section) && len(m.active()) == 0 {
		return styles.meta.Render("cargando…"), true
	}

	switch m.section {
	case Changes:
		switch {
		case m.status.Unborn:
			return styles.meta.Render(truncate("sin commits todavía", m.width)), true
		case len(m.status.Files) == 0:
			return styles.meta.Render(truncate("sin cambios", m.width)), true
		}
	case Diff:
		if !m.loaded {
			return styles.meta.Render(truncate("sin diff · enter en Cambios para cargarlo", m.width)), true
		}
	case Branches:
		if len(m.branches) == 0 {
			return styles.meta.Render(truncate("sin ramas", m.width)), true
		}
	case History:
		if len(m.history.Commits) == 0 {
			return styles.meta.Render(truncate("sin commits", m.width)), true
		}
	}

	return "", false
}

func (m Model) footerText() string {
	parts := []string{
		"tab: sección", "j/k: mover", "enter: diff del archivo",
		"d: " + m.modeLabel(), "r: refrescar", "?: ayuda",
	}

	switch {
	case m.section == Diff && m.diff.Truncated:
		parts = append(parts, "diff truncado")
	case m.section == History && m.history.Truncated:
		parts = append(parts, "historial truncado")
	case m.section == Changes && m.status.Truncated:
		parts = append(parts, "lista truncada")
	}

	return strings.Join(parts, " · ")
}

func (m Model) modeLabel() string {
	if m.staged {
		return "staged"
	}

	return "worktree"
}

func (m Model) helpLines() []string {
	return []string{
		styles.meta.Render("ayuda · panel de git (F5: solo lectura)"),
		"",
		"  tab / shift+tab   cambiar de sección",
		"  j / k, ↑ / ↓      mover el cursor",
		"  enter             cargar el diff del archivo seleccionado",
		"  d                 alternar entre staged (" + m.modeLabel() + ") y worktree",
		"  r                 refrescar estado, ramas e historial",
		"  esc               cerrar la ayuda o volver a Cambios",
		"  ?                 mostrar u ocultar esta ayuda",
	}
}

// pad fills the body up to the available height, so the layout stays stable.
func pad(lines []string, height int) []string {
	for len(lines) < height {
		lines = append(lines, "")
	}

	return lines
}
