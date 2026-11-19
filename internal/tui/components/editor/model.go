// Package editor previews one file or one diff, with syntax highlighting, scroll and a quick
// edit mode that saves through the file port.
package editor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/MiguelFPP/tui-editor/internal/tui/common"
)

const (
	defaultWidth  = 80
	defaultHeight = 20
	headerRows    = 1
	tabWidth      = 4

	loadTimeout = 5 * time.Second
	saveTimeout = 10 * time.Second

	gutterSeparator = " │ "
	headerSeparator = " · "
)

// mode selects the body source: a file read through the port or an existing diff.
type mode uint8

const (
	modeFile mode = iota
	modeDiff
)

func (m mode) label() string {
	if m == modeDiff {
		return "diff"
	}

	return "archivo"
}

// previewLoadedMsg carries the result of one ReadFile, tagged with its path.
type previewLoadedMsg struct {
	path    domain.RelPath
	content domain.FileContent
	err     error
}

var errNoService = errors.New("editor: servicio de archivos no configurado")

var styles = struct {
	title  lipgloss.Style
	meta   lipgloss.Style
	gutter lipgloss.Style
	fail   lipgloss.Style
}{
	title:  plainCells(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))),
	meta:   plainCells(lipgloss.NewStyle().Faint(true)),
	gutter: plainCells(lipgloss.NewStyle().Faint(true)),
	fail:   plainCells(lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))),
}

// plainCells keeps tabs untouched: the viewport measures cells, not expanded tabs.
func plainCells(style lipgloss.Style) lipgloss.Style {
	return style.TabWidth(lipgloss.NoTabConversion)
}

// Model is the editor/preview component; it never performs I/O outside a command.
type Model struct {
	files   ports.FileService
	editors ports.EditorService
	path    domain.RelPath
	mode    mode
	diff    domain.Diff

	viewport viewport.Model
	content  string
	raw      string

	area textarea.Model

	editing        bool
	dirty          bool
	stamp          domain.FileStamp
	saveErr        error
	saving         bool
	confirmDiscard bool
	external       bool

	err       error
	loading   bool
	truncated bool

	width  int
	height int
}

// New opens path in preview mode; the only read happens in Init.
func New(files ports.FileService, editors ports.EditorService, path domain.RelPath) Model {
	m := Model{
		files:    files,
		editors:  editors,
		path:     path,
		mode:     modeFile,
		viewport: viewport.New(),
		area:     newArea(),
		loading:  true,
	}
	m.SetSize(defaultWidth, defaultHeight)

	return m
}

// newArea builds the draft editor of the quick edit mode.
func newArea() textarea.Model {
	area := textarea.New()
	area.ShowLineNumbers = true
	area.SetVirtualCursor(true)

	return area
}

// NewDiff builds the diff preview from data already in memory, without I/O.
func NewDiff(d domain.Diff) Model {
	m := Model{
		path:     d.Path,
		mode:     modeDiff,
		diff:     d,
		viewport: viewport.New(),
		area:     newArea(),
	}
	m.SetSize(defaultWidth, defaultHeight)

	return m
}

// Init requests the file preview.
func (m Model) Init() tea.Cmd {
	if m.mode != modeFile {
		return nil
	}

	return m.load(m.path)
}

// Loading reports whether the preview is still being read or a save is in flight.
func (m Model) Loading() bool {
	return m.loading || m.saving
}

// Err returns the last failure of the open file, if any.
func (m Model) Err() error {
	if m.saveErr != nil {
		return m.saveErr
	}

	return m.err
}

// CapturesInput reports that the draft owns the keyboard of the component.
func (m Model) CapturesInput() bool {
	return m.editing || m.confirmDiscard
}

// Dirty reports whether the draft holds changes that are not on disk yet.
func (m Model) Dirty() bool {
	return m.dirty
}

// Update applies one message; scrolling and resizing stay off the I/O path.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)

		return m, nil
	case previewLoadedMsg:
		m.applyPreview(msg)

		return m, nil
	case savedMsg:
		return m, m.applySaved(msg)
	case externalReadyMsg:
		return m, m.launchExternal(msg)
	case externalDoneMsg:
		return m, m.applyExternalDone(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseWheelMsg:
		if m.editing {
			return m, nil // the draft covers the preview
		}

		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)

		return m, cmd
	default:
		return m, nil
	}
}

// View renders the header plus the body: the draft, a state line or the preview.
func (m Model) View() string {
	lines := []string{m.header()}

	switch {
	case m.editing:
		lines = append(lines, m.draftBody())
	default:
		if state, ok := m.stateLine(); ok {
			lines = append(lines, state)

			break
		}
		if m.saveErr != nil {
			lines = append(lines, styles.fail.Render("error: "+describeError(m.saveErr)))
		}

		lines = append(lines, m.viewport.View())
	}

	return strings.Join(lines, "\n")
}

// SetSize resizes the draft and the viewport, and keeps the scroll position the content allows.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)

	offset, xOffset := m.viewport.YOffset(), m.viewport.XOffset()

	m.viewport.SetWidth(m.width)
	m.viewport.SetHeight(m.bodyHeight())
	m.area.SetWidth(m.width)
	m.area.SetHeight(m.bodyHeight())
	m.sync()

	m.viewport.SetYOffset(offset)
	m.viewport.SetXOffset(xOffset)
}

// load asks the port for one preview under a bounded context.
func (m Model) load(path domain.RelPath) tea.Cmd {
	files := m.files
	if files == nil {
		return func() tea.Msg { return previewLoadedMsg{path: path, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()

		content, err := files.ReadFile(ctx, path)
		if err != nil {
			return previewLoadedMsg{path: path, err: err}
		}

		return previewLoadedMsg{path: path, content: content}
	}
}

func (m *Model) applyPreview(msg previewLoadedMsg) {
	if msg.path != m.path {
		return // a late result of a file that is no longer open
	}

	m.loading = false

	if msg.err != nil {
		m.err = msg.err
		m.raw, m.content, m.truncated = "", "", false
		m.sync()

		return
	}

	m.err = nil
	m.truncated = msg.content.Truncated
	m.stamp = msg.content.Stamp
	m.raw = msg.content.Text
	m.content = trimFinalNewline(Highlight(m.path, m.raw))
	m.syncDraft()
	m.sync()
}

// syncDraft mirrors the file on disk into the draft and drops the previous draft.
func (m *Model) syncDraft() {
	m.area.SetValue(m.raw)
	m.dirty = false
}

// sync rebuilds the rendered body and pushes it into the viewport.
func (m *Model) sync() {
	if m.mode == modeDiff {
		m.content = common.RenderDiff(m.diff, m.width, true)
	}

	m.viewport.LeftGutterFunc = m.gutter()
	m.viewport.SetContent(expandTabs(m.content))
}

// expandTabs measures indentation in cells, so the viewport never expands tabs on its own.
func expandTabs(text string) string {
	return strings.ReplaceAll(text, "\t", strings.Repeat(" ", tabWidth))
}

// gutter numbers the file preview; the diff already carries its own numbers.
func (m Model) gutter() viewport.GutterFunc {
	if m.mode != modeFile || m.content == "" {
		return viewport.NoGutter
	}

	return lineNumbers(lineCount(m.content))
}

// lineNumbers renders a fixed-width, right-aligned gutter that keeps ANSI intact.
func lineNumbers(total int) viewport.GutterFunc {
	digits := len(strconv.Itoa(max(total, 1)))

	return func(ctx viewport.GutterContext) string {
		if ctx.Soft {
			return styles.gutter.Render(strings.Repeat(" ", digits) + gutterSeparator)
		}

		return styles.gutter.Render(fmt.Sprintf("%*d", digits, ctx.Index+1) + gutterSeparator)
	}
}

// header is one line with the path, the mode and the state flags, cut to width.
func (m Model) header() string {
	return styles.title.Render(fitHeader(strings.Join(m.headerParts(), headerSeparator), m.width))
}

// headerParts keeps the path first, then the mode, then the state flags.
func (m Model) headerParts() []string {
	parts := []string{m.pathLabel(), m.mode.label()}

	if m.mode == modeDiff {
		if m.diff.Staged {
			parts = append(parts, "staged")
		}
		if m.diff.Binary {
			parts = append(parts, "binario")
		}

		return parts
	}

	if m.saveErr != nil {
		parts = append(parts, "error")
	}
	if m.editing {
		parts = append(parts, "editando")
	}
	if m.dirty {
		parts = append(parts, "sin guardar")
	}
	if m.external {
		parts = append(parts, "editor externo")
	}
	if m.err == nil && !m.loading {
		parts = append(parts, countLabel(lineCount(m.bodySource())))
	}
	if m.truncated {
		parts = append(parts, "truncado")
	}

	return parts
}

// bodySource counts the lines of what is on screen: the draft while editing, the preview otherwise.
func (m Model) bodySource() string {
	if m.editing {
		return trimFinalNewline(m.area.Value())
	}

	return m.content
}

func (m Model) pathLabel() string {
	if m.path.IsZero() {
		return "(sin ruta)"
	}

	return m.path.String()
}

// stateLine replaces the body while loading or when there is nothing to show.
func (m Model) stateLine() (string, bool) {
	switch {
	case m.err != nil:
		return styles.fail.Render("error: " + describeError(m.err)), true
	case m.loading:
		return styles.meta.Render("cargando…"), true
	case m.content == "":
		return styles.meta.Render("vacío"), true
	default:
		return "", false
	}
}

func (m Model) bodyHeight() int {
	return max(m.height-headerRows, 0)
}

func describeError(err error) string {
	switch {
	case errors.Is(err, errNoService):
		return "servicio de archivos no configurado"
	case errors.Is(err, errNoEditor):
		return "servicio de editor no configurado"
	case errors.Is(err, domain.ErrBinary):
		return "archivo binario"
	case errors.Is(err, domain.ErrTooLarge):
		return "demasiado grande"
	case errors.Is(err, domain.ErrConflict):
		return "el archivo cambió en disco"
	case errors.Is(err, domain.ErrEditorUnavailable):
		return "el editor externo no está disponible"
	case errors.Is(err, domain.ErrPermissionDenied):
		return "permiso denegado"
	case errors.Is(err, domain.ErrNotFound):
		return "no encontrado"
	case errors.Is(err, domain.ErrInvalidPath), errors.Is(err, domain.ErrInvalidArgument):
		return "ruta inválida"
	case errors.Is(err, context.DeadlineExceeded):
		return "tiempo de lectura agotado"
	default:
		return err.Error()
	}
}

func countLabel(lines int) string {
	if lines == 1 {
		return "1 línea"
	}

	return strconv.Itoa(lines) + " líneas"
}

// trimFinalNewline drops the empty last line a trailing newline would render.
func trimFinalNewline(text string) string {
	return strings.TrimSuffix(text, "\n")
}

func lineCount(text string) int {
	if text == "" {
		return 0
	}

	return strings.Count(text, "\n") + 1
}

// fitHeader cuts text to width, marking the cut; width <= 0 means "no limit".
func fitHeader(text string, width int) string {
	if width <= 0 {
		return text
	}

	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}

	return string(runes[:width-1]) + "…"
}
