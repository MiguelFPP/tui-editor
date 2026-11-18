package gitpanel

import (
	"strconv"
	"strings"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// tabSeparator joins the section labels; the mouse hit-testing relies on it.
const tabSeparator = " │ "

// titleSeparator joins the fields of the header row.
const titleSeparator = " · "

// View composes the header, the section tabs and the body of the active section.
func (m Model) View() string {
	lines := make([]string, 0, m.bodyHeight()+headerRows+footerRows)

	lines = append(lines, styles.title.Render(truncate(m.titleText(), m.width)))
	lines = append(lines, m.tabs())

	switch {
	case m.help:
		for _, line := range m.helpLines() {
			lines = append(lines, truncate(line, m.width))
		}
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

// titleText is the header of the panel: the repository identity, the state of the panel and the
// conflict banner, always in the same row.
func (m Model) titleText() string {
	parts := []string{"git"}
	if state := m.stateLabel(); state != "" {
		parts = append(parts, state) // the state goes first: a narrow panel must still show it
	}

	title := strings.Join(append(parts, m.identityLabels()...), titleSeparator)

	return title + conflictLabel(len(m.status.Conflicts()))
}

// identityLabels names the branch, its upstream and how far it is from it.
func (m Model) identityLabels() []string {
	switch {
	case m.status.Unborn:
		return []string{"rama " + m.status.Branch, "sin commits"}
	case m.status.Detached:
		return []string{"HEAD desacoplado", changeLabel(len(m.status.Files))}
	case m.status.Branch == "":
		return nil
	}

	parts := []string{"rama " + m.status.Branch}
	if m.status.Upstream != "" {
		parts = append(parts, m.status.Upstream)
	}
	if m.status.Ahead > 0 {
		parts = append(parts, "↑"+strconv.Itoa(m.status.Ahead))
	}
	if m.status.Behind > 0 {
		parts = append(parts, "↓"+strconv.Itoa(m.status.Behind))
	}

	return append(parts, changeLabel(len(m.status.Files)))
}

// stateLabel reports the operation in flight or the failure that stopped the panel; the actionable
// message stays in the body, the header only marks that something went wrong.
func (m Model) stateLabel() string {
	switch {
	case m.Err() != nil:
		return "error"
	case m.writing:
		return m.writeLabel()
	case m.loading.status:
		return "cargando…"
	default:
		return ""
	}
}

// writeLabel names the write in flight, also when the key that started it is unknown.
func (m Model) writeLabel() string {
	if m.writeOp == "" {
		return "escribiendo…"
	}

	return "escribiendo: " + m.writeOp
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

	return titleSeparator + conflictCount(count) + titleSeparator + "o/t/A"
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
	joined := strings.Join(labels, tabSeparator)
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

	return strings.Join(rendered, tabSeparator)
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
	if m.writeErr != nil {
		return styles.fail.Render(truncate("error al escribir: "+describeError(m.writeErr), m.width)), true
	}
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

// helpLines documents the panel by section, plus the keys every modal owns.
func (m Model) helpLines() []string {
	lines := []string{
		"ayuda · panel de git",
		"",
		" general",
		"  tab / shift+tab   cambiar de sección (también con clic en la pestaña)",
		"  j / k, ↑ / ↓      mover el cursor; la rueda desplaza la lista",
		"  r                 refrescar estado, ramas, stash e historial",
		"  f / p / P         fetch, pull y push",
		"  esc               cerrar la ayuda o volver a Cambios",
		"  ?                 mostrar u ocultar esta ayuda",
		"",
	}
	lines = append(lines, m.helpSection(Changes,
		"  enter             cargar el diff del archivo",
		"  s / u             preparar el archivo o quitar el archivo seleccionado",
		"  c                 abrir el editor del mensaje del commit",
		"  z                 guardar los cambios en el stash",
		"  o / t             resolver el conflicto con ours o con theirs",
		"  A                 abortar el merge en curso",
	)...)
	lines = append(lines, m.helpSection(Diff,
		"  d                 alternar entre worktree y staged",
		"  [ / ]             mover el cursor entre hunks",
		"  espacio           marcar o desmarcar el hunk",
		"  s / u             preparar o quitar los hunks marcados",
	)...)
	lines = append(lines, m.helpSection(Branches,
		"  enter             checkout de la rama seleccionada",
		"  n                 crear una rama: nombre y punto de partida opcional",
		"  x                 borrar la rama seleccionada",
		"  m                 fusionar la rama seleccionada en la actual",
	)...)
	lines = append(lines, m.helpSection(Stash,
		"  enter             aplicar la entrada seleccionada",
		"  z                 pop de la entrada seleccionada",
		"  x                 descartar la entrada seleccionada",
	)...)
	lines = append(lines, m.helpSection(History,
		"  j / k, ↑ / ↓      recorrer los commits (solo lectura)",
	)...)

	return append(lines,
		"",
		" modales",
		"  commit: ctrl+s confirma, esc cancela, enter añade una línea",
		"  prompt y confirmación: enter confirma, esc cancela",
		"  confirmar un borrado: y o enter confirma, n o esc cancela",
	)
}

// helpSection renders the heading of one section plus its keys; the active one is marked.
func (m Model) helpSection(section Section, keys ...string) []string {
	heading := " " + section.String()
	if section == m.section {
		heading += " · sección actual"
	}

	return append([]string{heading}, append(keys, "")...)
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
