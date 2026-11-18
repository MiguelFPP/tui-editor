// Package gitpanel renders the read-only Git sections of one repository.
package gitpanel

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/MiguelFPP/tui-editor/internal/tui/common"
)

const (
	defaultWidth  = 80
	defaultHeight = 20

	headerRows = 2
	footerRows = 1
	minRows    = 1

	loadTimeout = 5 * time.Second
	wheelStep   = 3
)

// Section is one navigable block of the panel.
type Section uint8

const (
	Changes Section = iota
	Diff
	Branches
	History

	sectionCount = 4
)

var sectionLabels = [sectionCount]string{"Cambios", "Diff", "Ramas", "Historial"}

func (s Section) String() string {
	if s >= sectionCount {
		return sectionLabels[0]
	}

	return sectionLabels[s]
}

// statusLoadedMsg carries one refresh of the working tree.
type statusLoadedMsg struct {
	seq    int
	status domain.GitStatus
	err    error
}

// branchesLoadedMsg carries the local branches.
type branchesLoadedMsg struct {
	seq      int
	branches []domain.Branch
	err      error
}

// historyLoadedMsg carries one page of history.
type historyLoadedMsg struct {
	seq  int
	page domain.CommitPage
	err  error
}

// diffLoadedMsg carries the diff of the requested path.
type diffLoadedMsg struct {
	seq  int
	req  domain.DiffRequest
	diff domain.Diff
	err  error
}

var errNoService = errors.New("gitpanel: servicio de git no configurado")

var styles = struct {
	title  lipgloss.Style
	tab    lipgloss.Style
	tabOn  lipgloss.Style
	cursor lipgloss.Style
	meta   lipgloss.Style
	fail   lipgloss.Style
}{
	title:  plainCells(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))),
	tab:    plainCells(lipgloss.NewStyle().Faint(true)),
	tabOn:  plainCells(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))),
	cursor: plainCells(lipgloss.NewStyle().Bold(true).Reverse(true)),
	meta:   plainCells(lipgloss.NewStyle().Faint(true)),
	fail:   plainCells(lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))),
}

// plainCells keeps tabs untouched, so the rendered rows match the parsed text.
func plainCells(style lipgloss.Style) lipgloss.Style {
	return style.TabWidth(lipgloss.NoTabConversion)
}

// Model is the Git panel; it never performs I/O outside a command.
type Model struct {
	git ports.GitReader

	section Section
	cursors [sectionCount]int
	offsets [sectionCount]int

	status        domain.GitStatus
	branches      []domain.Branch
	history       domain.CommitPage
	historyLoaded bool

	diff      domain.Diff
	request   domain.DiffRequest
	loaded    bool
	staged    bool
	diffBody  string
	diffLines []string

	loading loadingState
	errs    errorState
	seqs    seqState

	width  int
	height int
	help   bool
}

type loadingState struct {
	status   bool
	branches bool
	history  bool
	diff     bool
}

type errorState struct {
	status   error
	branches error
	history  error
	diff     error
}

type seqState struct {
	status   int
	branches int
	history  int
	diff     int
}

// New builds the panel; Init asks for the status and the branches, nothing else.
func New(git ports.GitReader) Model {
	m := Model{
		git:     git,
		loading: loadingState{status: true, branches: true},
		seqs:    seqState{status: 1, branches: 1},
	}
	m.SetSize(defaultWidth, defaultHeight)

	return m
}

// Init requests the status and the branch list of the repository.
func (m Model) Init() tea.Cmd {
	return tea.Batch(loadStatus(m.git, m.seqs.status), loadBranches(m.git, m.seqs.branches))
}

// Section returns the section currently displayed.
func (m Model) Section() Section {
	return m.section
}

// Loading reports whether any section is still reading.
func (m Model) Loading() bool {
	return m.loading.status || m.loading.branches || m.loading.history || m.loading.diff
}

// Err returns the first active section error, if any.
func (m Model) Err() error {
	switch {
	case m.errs.status != nil:
		return m.errs.status
	case m.errs.diff != nil:
		return m.errs.diff
	case m.errs.branches != nil:
		return m.errs.branches
	default:
		return m.errs.history
	}
}

// SetSize stores the terminal size and re-renders the cached diff.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)

	m.sync()
}

// sync rebuilds the cached diff body; View only composes it.
func (m *Model) sync() {
	if !m.loaded {
		m.diffBody, m.diffLines = "", nil

		return
	}

	m.diffBody = common.RenderDiff(m.diff, m.width, true)
	m.diffLines = strings.Split(m.diffBody, "\n")
	m.clamp(m.section)
}

// rows returns the body lines of one section, already formatted.
func (m Model) rows(section Section) []string {
	switch section {
	case Changes:
		return m.changeRows()
	case Diff:
		return m.diffRows()
	case Branches:
		return m.branchRows()
	default:
		return m.historyRows()
	}
}

// active returns the rows of the section on screen.
func (m Model) active() []string {
	return m.rows(m.section)
}

func (m Model) changeRows() []string {
	rows := make([]string, 0, len(m.status.Files))
	for _, file := range m.status.Files {
		rows = append(rows, fileRow(file))
	}

	return rows
}

func (m Model) branchRows() []string {
	rows := make([]string, 0, len(m.branches))
	for _, branch := range m.branches {
		rows = append(rows, branchRow(branch))
	}

	return rows
}

func (m Model) historyRows() []string {
	rows := make([]string, 0, len(m.history.Commits))
	for _, commit := range m.history.Commits {
		rows = append(rows, commitRow(commit))
	}

	return rows
}

func (m Model) diffRows() []string {
	if !m.loaded {
		return nil
	}

	header := display(m.request.Path) + " · diff"
	if m.request.Staged {
		header += " · staged"
	}
	if m.diff.Binary {
		header += " · binario"
	}

	return append([]string{header}, m.diffLines...)
}

// clamp keeps the cursor and the offset of one section inside its rows.
func (m *Model) clamp(section Section) {
	if section >= sectionCount {
		return
	}

	rows := len(m.rows(section))
	m.cursors[section] = min(max(m.cursors[section], 0), max(rows-1, 0))
	m.offsets[section] = min(max(m.offsets[section], 0), max(rows-m.bodyHeight(), 0))
	m.scrollToCursor(section)
}

// scrollToCursor moves the offset so the cursor stays visible.
func (m *Model) scrollToCursor(section Section) {
	height := m.bodyHeight()
	if height <= 0 {
		return
	}

	if m.cursors[section] < m.offsets[section] {
		m.offsets[section] = m.cursors[section]
	}
	if m.cursors[section] >= m.offsets[section]+height {
		m.offsets[section] = m.cursors[section] - height + 1
	}
}

func (m Model) bodyHeight() int {
	return max(m.height-headerRows-footerRows, minRows)
}

// selectedFile returns the file under the cursor of the Changes section.
func (m Model) selectedFile() (domain.GitFileStatus, bool) {
	if m.cursors[Changes] < 0 || m.cursors[Changes] >= len(m.status.Files) {
		return domain.GitFileStatus{}, false
	}

	return m.status.Files[m.cursors[Changes]], true
}

// sectionError returns the failure of one section, if any.
func (m Model) sectionError(section Section) error {
	switch section {
	case Changes:
		return m.errs.status
	case Diff:
		return m.errs.diff
	case Branches:
		return m.errs.branches
	default:
		return m.errs.history
	}
}

func (m Model) sectionLoading(section Section) bool {
	switch section {
	case Changes:
		return m.loading.status
	case Diff:
		return m.loading.diff
	case Branches:
		return m.loading.branches
	default:
		return m.loading.history
	}
}

// describeError turns an adapter failure into the message the panel shows.
func describeError(err error) string {
	switch {
	case errors.Is(err, errNoService):
		return "servicio de git no configurado"
	case errors.Is(err, domain.ErrNotRepository):
		return "no es un repositorio git"
	case errors.Is(err, domain.ErrGitUnavailable):
		return "git no está disponible"
	case errors.Is(err, domain.ErrGitIndexLocked):
		return "el índice de git está bloqueado"
	case errors.Is(err, domain.ErrPermissionDenied):
		return "permiso denegado"
	case errors.Is(err, domain.ErrInvalidPath), errors.Is(err, domain.ErrInvalidArgument):
		return "ruta inválida"
	case errors.Is(err, context.DeadlineExceeded):
		return "tiempo de lectura agotado"
	case errors.Is(err, context.Canceled):
		return "cancelado"
	default:
		return err.Error()
	}
}

// fileRow renders one porcelain status entry with its conflict and rename markers.
func fileRow(file domain.GitFileStatus) string {
	label := fmt.Sprintf("%s%s %s", file.Index, file.Worktree, display(file.Path))

	switch {
	case file.Conflicted:
		label += " · conflicto"
	case file.Renamed():
		label += " · renombrado desde " + display(file.OrigPath)
	case file.Untracked():
		label += " · sin seguimiento"
	}

	return label
}

func branchRow(branch domain.Branch) string {
	label := "  " + branch.Name
	if branch.Current {
		label = "* " + branch.Name
	}

	if branch.Gone {
		label += " (desaparecida)"
	} else if branch.Upstream != "" {
		label += " (" + branch.Upstream + ")"
	}

	if branch.Ahead > 0 {
		label += " ↑" + strconv.Itoa(branch.Ahead)
	}
	if branch.Behind > 0 {
		label += " ↓" + strconv.Itoa(branch.Behind)
	}

	return label
}

func commitRow(commit domain.Commit) string {
	label := commit.ShortHash
	if label == "" {
		label = truncateHash(commit.Hash)
	}

	parts := []string{label, commit.Subject}
	if commit.Author != "" {
		parts = append(parts, commit.Author)
	}
	if !commit.Date.IsZero() {
		parts = append(parts, commit.Date.Format("2006-01-02 15:04"))
	}

	return strings.Join(parts, " · ")
}

func truncateHash(hash string) string {
	if len(hash) <= 7 {
		return hash
	}

	return hash[:7]
}

func display(path domain.RelPath) string {
	if path.IsZero() {
		return "."
	}

	return string(path)
}

func truncate(text string, width int) string {
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
