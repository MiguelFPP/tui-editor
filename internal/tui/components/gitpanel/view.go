package gitpanel

import (
	"strconv"
	"strings"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// View composes the header, the section tabs and the body of the active section.
func (m Model) View() string {
	lines := make([]string, 0, m.bodyHeight()+headerRows+footerRows)

	lines = append(lines, styles.title.Render(truncate(m.titleText(), m.width)))
	lines = append(lines, m.tabs())

	switch {
	case m.help:
		lines = append(lines, m.helpLines()...)
	case m.confirm.open:
		lines = append(lines, m.confirmLines()...)
	case m.prompt.open:
		lines = append(lines, m.promptLines()...)
	case m.commit.open:
		lines = append(lines, m.commitLines()...)
	default:
		lines = append(lines, m.body()...)
		lines = append(lines, styles.meta.Render(truncate(m.footerText(), m.width)))
	}

	return strings.Join(lines, "\n")
}

// titleText is the header of the panel, with the conflict banner appended in the same row.
func (m Model) titleText() string {
	return m.baseTitle() + conflictLabel(len(m.status.Conflicts()))
}

func (m Model) baseTitle() string {
	switch {
	case m.writeErr != nil:
		return "git · error al escribir: " + describeError(m.writeErr)
	case m.errs.status != nil:
		return "git · error: " + describeError(m.errs.status)
	case m.writing:
		return "git · escribiendo…"
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

// conflictCount names the pending conflicts.
func conflictCount(count int) string {
	if count == 1 {
		return "1 conflicto"
	}

	return strconv.Itoa(count) + " conflictos"
}

// conflictLabel is the banner that documents the keys of a conflicted working tree.
func conflictLabel(count int) string {
	if count == 0 {
		return ""
	}

	return " · " + conflictCount(count) + " · o/t/A"
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
	case Stash:
		if len(m.stashes) == 0 {
			return styles.meta.Render(truncate("sin stash · z en Cambios para guardar cambios", m.width)), true
		}
	case History:
		if len(m.history.Commits) == 0 {
			return styles.meta.Render(truncate("sin commits", m.width)), true
		}
	}

	return "", false
}

func (m Model) footerText() string {
	parts := make([]string, 0, 12)
	if m.notice != "" {
		parts = append(parts, m.notice)
	}

	parts = append(parts, "tab: sección", "j/k: mover")

	switch m.section {
	case Changes:
		parts = append(parts, "enter: diff del archivo", "s: preparar", "u: quitar", "c: commit", "z: stash")
		if len(m.status.Conflicts()) > 0 {
			parts = append(parts, "o/t: resolver", "A: abortar merge")
		}
	case Diff:
		parts = append(parts, "[ ]: hunk", "espacio: marcar", "d: "+m.modeLabel())

		switch {
		case m.staged:
			parts = append(parts, "u: quitar hunks")
		case m.loaded:
			parts = append(parts, "s: preparar hunks")
		}
	case Branches:
		parts = append(parts, "enter: checkout", "n: nueva rama", "x: borrar", "m: merge")
	case Stash:
		parts = append(parts, "enter: aplicar", "z: pop", "x: descartar")
	}

	parts = append(parts, "f: fetch", "p: pull", "P: push", "r: refrescar", "?: ayuda")

	switch {
	case m.section == Diff && m.diff.Truncated:
		parts = append(parts, "diff truncado")
	case m.section == History && m.history.Truncated:
		parts = append(parts, "historial truncado")
	case m.section == Stash && len(m.stashes) >= domain.MaxStashes:
		parts = append(parts, "lista truncada")
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
		styles.meta.Render("ayuda · panel de git"),
		"",
		"  tab / shift+tab   cambiar de sección",
		"  j / k, ↑ / ↓      mover el cursor",
		"  enter             cargar el diff; en Ramas hace checkout y en Stash aplica la entrada",
		"  d                 alternar entre staged (" + m.modeLabel() + ") y worktree",
		"  s                 preparar el archivo; en Diff, los hunks marcados",
		"  u                 quitar el archivo; en Diff, los hunks marcados",
		"  c                 abrir el editor del mensaje del commit",
		"  n                 crear una rama: nombre y punto de partida opcional",
		"  x                 borrar la rama o descartar el stash seleccionados",
		"  m                 fusionar la rama seleccionada en la actual",
		"  z                 guardar los cambios en el stash; en Stash, pop de la entrada",
		"  o / t             resolver el conflicto con ours o con theirs",
		"  A                 abortar el merge en curso",
		"  f / p / P         fetch, pull y push",
		"  [ / ]             mover el cursor entre hunks (Diff)",
		"  espacio           marcar o desmarcar el hunk (Diff)",
		"  r                 refrescar estado, ramas, stash e historial",
		"  esc               cerrar la ayuda o volver a Cambios",
		"  ?                 mostrar u ocultar esta ayuda",
		"",
		styles.meta.Render("  commit: ctrl+s confirma, esc cancela y enter añade una línea"),
		styles.meta.Render("  prompt y confirmación: enter confirma, esc cancela"),
	}
}

// commitLines renders the commit editor over the whole body of the panel.
func (m Model) commitLines() []string {
	lines := make([]string, 0, m.bodyHeight()+footerRows)

	lines = append(lines, styles.title.Render(truncate("commit · mensaje", m.width)))
	lines = append(lines, strings.Split(m.commit.area.View(), "\n")...)

	if m.writing {
		lines = append(lines, styles.meta.Render(truncate("escribiendo el commit…", m.width)))
	}
	if m.commit.err != nil {
		lines = append(lines, styles.fail.Render(truncate("error: "+describeError(m.commit.err), m.width)))
	}

	lines = append(lines, styles.meta.Render(truncate("ctrl+s: confirmar · esc: cancelar · enter: nueva línea", m.width)))

	return pad(lines, m.bodyHeight()+footerRows)
}

// promptLines renders the one-line prompt over the whole body of the panel.
func (m Model) promptLines() []string {
	lines := make([]string, 0, m.bodyHeight()+footerRows)

	lines = append(lines, styles.title.Render(truncate(m.promptTitle(), m.width)))
	lines = append(lines, m.prompt.input.View())

	if m.writing {
		lines = append(lines, styles.meta.Render(truncate("escribiendo…", m.width)))
	}
	if m.prompt.err != nil {
		lines = append(lines, styles.fail.Render(truncate("error: "+describeError(m.prompt.err), m.width)))
	}

	lines = append(lines, styles.meta.Render(truncate("enter: confirmar · esc: cancelar", m.width)))

	return pad(lines, m.bodyHeight()+footerRows)
}

func (m Model) promptTitle() string {
	if m.prompt.kind == promptStashMessage {
		return "stash · mensaje (opcional)"
	}

	return "rama nueva · nombre [punto de partida]"
}

// confirmLines renders the confirmation over the whole body of the panel.
func (m Model) confirmLines() []string {
	lines := make([]string, 0, m.bodyHeight()+footerRows)

	lines = append(lines, styles.title.Render(truncate("confirmar · "+m.confirmQuestion(), m.width)))

	if m.writing {
		lines = append(lines, styles.meta.Render(truncate("escribiendo…", m.width)))
	}
	if m.confirm.err != nil {
		lines = append(lines, styles.fail.Render(truncate("error: "+describeError(m.confirm.err), m.width)))
	}

	lines = append(lines, styles.meta.Render(truncate("y/enter: confirmar · n/esc: cancelar", m.width)))

	return pad(lines, m.bodyHeight()+footerRows)
}

// confirmQuestion describes what the pending confirmation would do.
func (m Model) confirmQuestion() string {
	switch m.confirm.kind {
	case confirmDeleteBranch:
		return "¿borrar la rama " + m.confirm.branch + "?"
	case confirmStashDrop:
		return "¿descartar stash@{" + strconv.Itoa(m.confirm.index) + "}?"
	case confirmAbortMerge:
		return "¿abortar el merge en curso?"
	default:
		return ""
	}
}

// pad fills the body up to the available height, so the layout stays stable.
func pad(lines []string, height int) []string {
	for len(lines) < height {
		lines = append(lines, "")
	}

	return lines
}
