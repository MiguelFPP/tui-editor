package editor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
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
const draftHint = "¿descartar los cambios sin guardar? (y descarta, n cancela)"

// handleKey drives the discard confirmation, the draft and the shortcuts of the preview.
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case m.confirmDiscard:
		return m.handleDiscardKey(msg)
	case m.editing:
		return m.handleDraftKey(msg)
	}

	switch msg.String() {
	case "e":
		return m, m.startEditing()
	case "E", "ctrl+e":
		return m, m.startExternal()
	default:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)

		return m, cmd
	}
}

// handleDraftKey edits the draft, saves it with ctrl+s and leaves it with esc.
func (m Model) handleDraftKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.dirty {
			m.confirmDiscard = true

			return m, nil
		}

		return m.stopEditing(), nil
	case "ctrl+s":
		return m, m.save()
	}

	area, cmd := m.area.Update(msg)
	m.area = area
	m.dirty = m.area.Value() != m.raw

	return m, cmd
}

// handleDiscardKey answers the confirmation: y drops the draft, n and esc keep it.
func (m Model) handleDiscardKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		return m.discardDraft(), nil
	case "n", "esc":
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

	return m.area.Focus()
}

// stopEditing leaves the draft and returns to the preview of the file.
func (m Model) stopEditing() Model {
	m.editing = false
	m.confirmDiscard = false
	m.saveErr = nil
	m.area.Blur()

	return m
}

// discardDraft throws the draft away and restores the preview of the file.
func (m Model) discardDraft() Model {
	m.area.SetValue(m.raw)
	m.dirty = false

	return m.stopEditing()
}

// draftBody renders the draft, above it the discard confirmation and the last failure.
func (m Model) draftBody() string {
	lines := make([]string, 0, 3)
	if m.confirmDiscard {
		lines = append(lines, styles.fail.Render(draftHint))
	}
	if m.saveErr != nil {
		lines = append(lines, styles.fail.Render("error: "+describeError(m.saveErr)))
	}

	return strings.Join(append(lines, m.area.View()), "\n")
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
