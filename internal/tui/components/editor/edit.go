package editor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

// FileChangedMsg tells the root that the file on disk changed and Git must refresh.
type FileChangedMsg struct {
	Path domain.RelPath
}

// savedMsg carries the outcome of one WriteFile.
type savedMsg struct {
	path  domain.RelPath
	stamp domain.FileStamp
	text  string
	err   error
}

// externalReadyMsg carries the editor command resolved for the open file.
type externalReadyMsg struct {
	command domain.EditorCommand
	err     error
}

// externalDoneMsg reports that the external editor session finished.
type externalDoneMsg struct {
	err error
}

var errNoEditor = errors.New("editor: servicio de editor no configurado")

// draftHint is the confirmation shown before dropping unsaved changes.
func (m Model) draftHint() string {
	return "¿descartar los cambios sin guardar? (" + m.keys.Label(keymap.ActionEditorDiscardYes) +
		" descarta, " + m.keys.Label(keymap.ActionEditorDiscardNo) + " cancela)"
}

// The caret reverses its cell; the reset keeps the colour of the token around it.
const (
	cursorOn  = "\x1b[7m"
	cursorOff = "\x1b[27m"
)

// handleKey drives the discard confirmation, the draft and the shortcuts of the preview.
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case m.confirmDiscard:
		return m.handleDiscardKey(msg)
	case m.editing:
		return m.handleDraftKey(msg)
	}

	if action, ok := m.keys.Match(keymap.ScopeEditor, msg.String()); ok {
		switch action {
		case keymap.ActionEditorEdit:
			return m, m.startEditing()
		case keymap.ActionEditorExternal:
			return m, m.startExternal()
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	return m, cmd
}

// handleDraftKey edits the draft, saves it with the save shortcut and leaves it with cancel.
func (m Model) handleDraftKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if action, ok := m.keys.Match(keymap.ScopeEditorDraft, msg.String()); ok {
		switch action {
		case keymap.ActionEditorCancel:
			if m.dirty {
				m.confirmDiscard = true

				return m, nil
			}

			return m.stopEditing(), nil
		case keymap.ActionEditorSave:
			return m, m.save()
		}
	}

	area, cmd := m.area.Update(msg)
	m.area = area
	m.dirty = m.area.Value() != m.raw
	m.refreshDraftLines()
	m.followDraftCursor()

	return m, cmd
}

// handleDiscardKey answers the confirmation: yes drops the draft, no keeps it.
func (m Model) handleDiscardKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	action, ok := m.keys.Match(keymap.ScopeEditorDiscard, msg.String())
	if !ok {
		return m, nil
	}

	switch action {
	case keymap.ActionEditorDiscardYes:
		return m.discardDraft(), nil
	case keymap.ActionEditorDiscardNo:
		m.confirmDiscard = false
	}

	return m, nil
}

// editable reports whether the open preview is a plain, complete text file.
func (m Model) editable() bool {
	return m.mode == modeFile && !m.path.IsZero() && m.err == nil && !m.loading && !m.truncated
}

// startEditing opens the draft of the file; a diff, a binary or a truncated preview is not editable.
func (m *Model) startEditing() tea.Cmd {
	if !m.editable() {
		return nil
	}

	m.editing = true
	m.confirmDiscard = false
	m.saveErr = nil
	m.dirty = false
	m.refreshDraftLines()
	m.area.MoveToBegin() // the draft opens on the first line, not at the end of the file
	m.followDraftCursor()

	return m.area.Focus()
}

// stopEditing leaves the draft and returns to the preview of the file.
func (m Model) stopEditing() Model {
	m.editing = false
	m.confirmDiscard = false
	m.saveErr = nil
	m.clearDraftLines()
	m.area.Blur()

	return m
}

// discardDraft throws the draft away and restores the preview of the file.
func (m Model) discardDraft() Model {
	m.area.SetValue(m.raw)
	m.dirty = false
	m.clearDraftLines()

	return m.stopEditing()
}

// refreshDraftLines re-highlights the draft only when its text changed.
func (m *Model) refreshDraftLines() {
	source := m.area.Value()
	if m.draftLines != nil && source == m.draftSource {
		return
	}

	m.draftSource = source
	m.draftLines = HighlightLines(m.path, source)
}

// clearDraftLines drops the highlighted draft of the previous text.
func (m *Model) clearDraftLines() {
	m.draftLines, m.draftSource = nil, ""
}

// draftBody renders the draft behind the preview gutter, above it the discard
// confirmation and the last failure.
func (m Model) draftBody() string {
	lines := make([]string, 0, 3)
	if m.confirmDiscard {
		lines = append(lines, styles.FailPlain.Render(m.draftHint()))
	}
	if m.saveErr != nil {
		lines = append(lines, styles.FailPlain.Render("error: "+describeError(m.saveErr)))
	}

	return strings.Join(append(lines, m.draftWindow(len(lines))...), "\n")
}

// draftWindow renders the visible rows of the draft, numbered and coloured like
// the preview; the rows below the draft keep the height of the body.
func (m Model) draftWindow(extra int) []string {
	rows := max(m.bodyHeight()-extra, 0)
	if rows == 0 {
		return nil
	}

	lines := m.draftLines
	if lines == nil {
		// The cache is filled when the draft opens; a reload drops it first.
		lines = strings.Split(trimFinalNewline(m.area.Value()), "\n")
	}

	total := len(lines)
	digits := gutterDigits(total)
	cut := lipgloss.NewStyle().MaxWidth(m.width - digits - lipgloss.Width(gutterSeparator))
	offset := clamp(m.draftTop, 0, max(total-rows, 0))
	caret := m.area.Line()

	window := make([]string, 0, rows)
	for row := range rows {
		index := offset + row

		switch {
		case index < total && index == caret:
			window = append(window, gutterNumber(index, digits)+cut.Render(cursorRow(lines[index], m.area.Column())))
		case index < total:
			window = append(window, gutterNumber(index, digits)+cut.Render(expandTabs(lines[index])))
		case index == caret:
			// The caret waits on the empty row a trailing newline leaves behind.
			window = append(window, gutterBlank(digits)+cursorOn+" "+cursorOff)
		default:
			window = append(window, gutterBlank(digits))
		}
	}

	return window
}

// draftHints counts the rows the discard confirmation and the last failure take.
func (m Model) draftHints() int {
	hints := 0
	if m.confirmDiscard {
		hints++
	}
	if m.saveErr != nil {
		hints++
	}

	return hints
}

// draftRows is the number of draft rows the body can show.
func (m Model) draftRows() int {
	return max(m.bodyHeight()-m.draftHints(), 0)
}

// draftTotal is the number of lines of the draft, from the cache or the textarea.
func (m Model) draftTotal() int {
	if m.draftLines != nil {
		return len(m.draftLines)
	}

	return len(strings.Split(trimFinalNewline(m.area.Value()), "\n"))
}

// gutterCells is the width of the gutter the draft window draws.
func (m Model) gutterCells() int {
	return gutterDigits(m.draftTotal()) + lipgloss.Width(gutterSeparator)
}

// followDraftCursor scrolls the window just enough to keep the caret inside it.
func (m *Model) followDraftCursor() {
	rows := m.draftRows()
	line := m.area.Line()

	if line < m.draftTop {
		m.draftTop = line
	}
	if line >= m.draftTop+rows {
		m.draftTop = line - rows + 1
	}

	m.draftTop = clamp(m.draftTop, 0, max(m.draftTotal()-rows, 0))
}

// scrollDraft moves the window of the draft without moving the caret.
func (m *Model) scrollDraft(delta int) {
	m.draftTop = clamp(m.draftTop+delta, 0, max(m.draftTotal()-m.draftRows(), 0))
}

// handleDraftClick moves the caret to the clicked cell, without a selection.
func (m *Model) handleDraftClick(msg tea.MouseClickMsg) {
	if !m.editing || msg.Button != tea.MouseLeft {
		return
	}
	if msg.X < 0 || msg.X >= m.width || msg.Y < headerRows || msg.Y >= headerRows+m.bodyHeight() {
		return // the header and the padding around the body are not clickable
	}

	column := max(msg.X-m.gutterCells(), 0)
	row := msg.Y - headerRows - m.draftHints() + m.draftTop

	// The textarea adds its own gutter and scroll inside PositionAt.
	m.area.BeginSelection(column, row-m.area.ScrollYOffset())
	m.area.ClearSelection()
	m.area.SetCursorColumn(column)
	m.followDraftCursor()
}

// cursorRow reverse-videos the cell of the caret on one draft row.
func cursorRow(row string, column int) string {
	text := expandTabs(row)

	return insertCell(text, cellOf(text, column))
}

// cellOf is the visible cell of the given column of a styled row.
func cellOf(row string, column int) int {
	cell, runes := 0, 0

	for index := 0; index < len(row) && runes < column; {
		if row[index] == 0x1b {
			index += ansiEnd(row[index:])

			continue
		}

		r, size := utf8.DecodeRuneInString(row[index:])
		cell += lipgloss.Width(string(r))
		runes++
		index += size
	}

	return cell
}

// insertCell wraps the rune at the given cell in reverse video; past the end of
// the row it draws a space, so the caret is always visible.
func insertCell(row string, cell int) string {
	var out strings.Builder

	visible := 0
	for index := 0; index < len(row); {
		if row[index] == 0x1b {
			width := ansiEnd(row[index:])
			out.WriteString(row[index : index+width])
			index += width

			continue
		}

		r, size := utf8.DecodeRuneInString(row[index:])
		if visible == cell {
			// Reverse video around the rune keeps the colour of its token.
			out.WriteString(cursorOn)
			out.WriteRune(r)
			out.WriteString(cursorOff)
			out.WriteString(row[index+size:])

			return out.String()
		}

		out.WriteString(row[index : index+size])
		visible += lipgloss.Width(string(r))
		index += size
	}

	if visible <= cell {
		out.WriteString(cursorOn + " " + cursorOff)
	}

	return out.String()
}

// ansiEnd is the length of the CSI sequence that starts at s[0].
func ansiEnd(s string) int {
	for i := 1; i < len(s); i++ {
		if c := s[i]; (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return i + 1
		}
	}

	return len(s)
}

// clamp limits value to the [low, high] range.
func clamp(value, low, high int) int {
	if high < low {
		return low
	}

	return min(max(value, low), high)
}

// save writes the draft over the file, claiming the stamp of the last read.
func (m *Model) save() tea.Cmd {
	if !m.editing || m.saving {
		return nil
	}

	files := m.files
	if files == nil {
		m.saveErr = errNoService

		return nil
	}

	m.saving = true
	m.saveErr = nil

	path, stamp, text := m.path, m.stamp, m.area.Value()

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		defer cancel()

		next, err := files.WriteFile(ctx, path, text, stamp)
		if err != nil {
			return savedMsg{path: path, err: err}
		}

		return savedMsg{path: path, stamp: next, text: text}
	}
}

// applySaved adopts the outcome of a save; a failure keeps the draft and its changes.
func (m *Model) applySaved(msg savedMsg) tea.Cmd {
	if msg.path != m.path {
		return nil // a late answer of a file that is no longer open
	}

	m.saving = false

	if msg.err != nil {
		m.saveErr = msg.err
		m.dirty = m.area.Value() != m.raw

		return nil
	}

	m.saveErr = nil
	m.stamp = msg.stamp
	m.raw = msg.text
	m.content = trimFinalNewline(Highlight(m.path, m.raw))
	m.dirty = m.area.Value() != m.raw
	m.sync()

	return changed(m.path)
}

// startExternal resolves the external editor of the configuration for the open file.
func (m *Model) startExternal() tea.Cmd {
	if m.editing || m.external || m.loading || m.mode != modeFile || m.path.IsZero() {
		return nil
	}

	editors := m.editors
	if editors == nil {
		m.saveErr = errNoEditor

		return nil
	}

	m.saveErr = nil
	m.external = true

	path := m.path

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()

		command, err := editors.Resolve(ctx, domain.EditorTarget{Path: path})

		return externalReadyMsg{command: command, err: err}
	}
}

// launchExternal hands the terminal to the resolved editor until it exits.
func (m *Model) launchExternal(msg externalReadyMsg) tea.Cmd {
	if msg.err != nil || msg.command.IsZero() {
		m.external = false

		if msg.err != nil {
			m.saveErr = msg.err
		} else {
			m.saveErr = errNoEditor
		}

		return nil
	}

	process := exec.Command(msg.command.Binary, msg.command.Args...)

	return tea.ExecProcess(process, func(err error) tea.Msg {
		return externalDoneMsg{err: err}
	})
}

// applyExternalDone reloads the file the external editor closed on.
func (m *Model) applyExternalDone(msg externalDoneMsg) tea.Cmd {
	m.external = false
	if msg.err != nil {
		m.saveErr = fmt.Errorf("editor externo: %w", msg.err)
	}

	m.loading = true

	return tea.Batch(m.load(m.path), changed(m.path))
}

// changed announces the new content of path to the root model.
func changed(path domain.RelPath) tea.Cmd {
	return func() tea.Msg { return FileChangedMsg{Path: path} }
}
