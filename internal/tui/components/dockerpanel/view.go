package dockerpanel

import (
	"strings"

	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

// View composes the header, the container table or the logs, and the footer.
func (m Model) View() string {
	if m.logsOpen {
		return m.logsView()
	}

	lines := make([]string, 0, m.height)
	lines = append(lines, styles.TitlePlain.Render(truncate(m.titleText(), m.width)))
	lines = append(lines, styles.Header.Render(truncate(columnHeader(), m.width)))

	switch {
	case m.confirm != confirmNone:
		lines = append(lines, m.confirmLines()...)
	default:
		if state, ok := m.stateLine(); ok {
			lines = append(lines, state)
		} else {
			lines = append(lines, m.bodyRows()...)
		}
		lines = append(lines, styles.MetaPlain.Render(truncate(m.footerText(), m.width)))
	}

	return strings.Join(lines, "\n")
}

// stateLine is the message that replaces the rows while loading, on failure or when empty.
func (m Model) stateLine() (string, bool) {
	switch {
	case m.actionErr != nil:
		return styles.FailPlain.Render(truncate("error al actuar: "+describeError(m.actionErr), m.width)), true
	case m.unavailable():
		return styles.FailPlain.Render(truncate(unavailableText, m.width)), true
	case m.loadingContainers && len(m.containers) == 0:
		return styles.MetaPlain.Render("cargando…"), true
	case m.err != nil && len(m.containers) == 0:
		return styles.FailPlain.Render(truncate("error: "+describeError(m.err), m.width)), true
	case len(m.containers) == 0:
		return styles.MetaPlain.Render(truncate(m.emptyText(), m.width)), true
	}

	return "", false
}

// emptyText explains an empty list, naming the scope and the key that widens it.
func (m Model) emptyText() string {
	if m.showAll || m.repoRoot == "" {
		return "sin contenedores"
	}

	return "sin contenedores de este proyecto · a: ver todos"
}

// bodyRows renders the visible window of the container table.
func (m Model) bodyRows() []string {
	height := m.bodyHeight()
	start := m.offset
	end := min(start+height, len(m.rows))

	window := make([]string, 0, height)
	for i := start; i < end; i++ {
		line := truncate(m.rows[i], m.width)
		if i == m.cursor {
			line = styles.CursorPlain.Render(line)
		}

		window = append(window, line)
	}

	return pad(window, height)
}

// footerText lists the keys of the panel and the notice of the last action.
func (m Model) footerText() string {
	parts := make([]string, 0, 10)
	if m.notice != "" {
		parts = append(parts, m.notice)
	}
	if m.err != nil && len(m.containers) > 0 {
		parts = append(parts, "error: "+describeError(m.err))
	}

	parts = append(parts,
		m.keys.Label(keymap.ActionDockerUp)+"/"+m.keys.Label(keymap.ActionDockerDown)+": mover",
		m.keys.Label(keymap.ActionDockerStats)+": stats",
		m.keys.Label(keymap.ActionDockerLogs)+": logs",
		m.scopeHint(),
		m.keys.Label(keymap.ActionDockerStart)+": start",
		m.keys.Label(keymap.ActionDockerStop)+": stop",
		m.keys.Label(keymap.ActionDockerRestart)+": restart",
		m.keys.Label(keymap.ActionDockerRefresh)+": refrescar",
	)

	return strings.Join(parts, columnSeparator)
}

// confirmLines renders the confirmation over the body of the panel.
func (m Model) confirmLines() []string {
	lines := make([]string, 0, m.bodyHeight()+footerRows)
	lines = append(lines, styles.TitlePlain.Render(truncate("confirmar · "+m.confirm.question(m.confirmName), m.width)))

	if m.writing {
		lines = append(lines, styles.MetaPlain.Render(truncate("actuando…", m.width)))
	}

	lines = append(lines, styles.MetaPlain.Render(truncate(m.keys.Label(keymap.ActionDockerConfirmYes)+": confirmar · "+
		m.keys.Label(keymap.ActionDockerConfirmNo)+": cancelar", m.width)))

	return pad(lines, m.bodyHeight()+footerRows)
}

// logsView renders the logs title, the scrollable viewport and its hints.
func (m Model) logsView() string {
	footer := m.keys.Label(keymap.ActionDockerLogsClose) + ": cerrar · " +
		m.keys.Label(keymap.ActionDockerLogsReload) + ": recargar · ↑/↓, rueda: desplazar"

	switch {
	case m.loadingLogs && len(m.logLines) == 0:
		footer = "cargando logs…"
	case m.logsErr != nil:
		footer = "error: " + describeError(m.logsErr)
	}

	return strings.Join([]string{
		styles.TitlePlain.Render(truncate(m.logsTitle(), m.width)),
		m.viewport.View(),
		styles.MetaPlain.Render(truncate(footer, m.width)),
	}, "\n")
}

// logsTitle names the container of the open logs and how much history is loaded.
func (m Model) logsTitle() string {
	parts := []string{"logs", m.logsName, lineLabel(len(m.logLines))}
	if m.logs.Truncated {
		parts = append(parts, "truncado")
	}
	if !m.logs.NextBefore.IsZero() {
		parts = append(parts, "↑ más antiguos")
	}

	return strings.Join(parts, columnSeparator)
}

// pad fills the body up to the available height, so the layout stays stable.
func pad(lines []string, height int) []string {
	for len(lines) < height {
		lines = append(lines, "")
	}

	return lines
}
