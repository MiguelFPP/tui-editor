// Package gitpanel renders the Git sections of one repository, stages and commits them, and drives
// the branch, stash, merge and sync operations.
package gitpanel

import (
	"context"
	"errors"
	"fmt"
	"image"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/MiguelFPP/tui-editor/internal/tui/common"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

const (
	defaultWidth  = 80
	defaultHeight = 20

	headerRows = 2
	footerRows = 1
	minRows    = 1

	loadTimeout = 5 * time.Second
	// writeTimeout bounds a stage or commit, including the repository hooks.
	writeTimeout = 60 * time.Second
	// networkTimeout bounds a fetch, a pull or a push, which talk to a remote.
	networkTimeout = 2 * time.Minute
	wheelStep      = 3

	// commitRows is the height of the commit message editor.
	commitRows = 6
	// promptCells leaves room for the "> " prompt of the text input inside the panel.
	promptCells = 3

	// The write operations, used to label their result.
	opStage        = "preparar"
	opUnstage      = "quitar"
	opStageHunks   = "preparar hunks"
	opUnstageHunks = "quitar hunks"
	opCommit       = "commit"
	opCheckout     = "checkout"
	opCreateBranch = "crear rama"
	opDeleteBranch = "borrar rama"
	opStashPush    = "stash"
	opStashApply   = "aplicar stash"
	opStashPop     = "pop stash"
	opStashDrop    = "descartar stash"
	opMerge        = "merge"
	opAbortMerge   = "abortar merge"
	opResolve      = "resolver conflicto"
	opFetch        = "fetch"
	opPull         = "pull"
	opPush         = "push"

	// The notices shown when a network write succeeds.
	noticeFetch = "fetch completado"
	noticePush  = "push completado"

	// maxErrorCells bounds the line the panel shows for a failure it does not recognise.
	maxErrorCells = 160

	// gitSplitMinWidth is the panel width below which the list and the preview share one column.
	gitSplitMinWidth = 100
	minGitList       = 28
	minGitPreview    = 34
	listPercent      = 40
)

// Section is one navigable block of the panel.
type Section uint8

const (
	Changes Section = iota
	Diff
	Branches
	Stash
	History

	sectionCount = 5
)

var sectionLabels = [sectionCount]string{"Cambios", "Diff", "Ramas", "Stash", "Historial"}

func (s Section) String() string {
	if s >= sectionCount {
		return sectionLabels[0]
	}

	return sectionLabels[s]
}

// Layout is the preferred shape of the Changes and Diff sections.
type Layout uint8

const (
	LayoutSplit Layout = iota
	LayoutSingle
)

func (l Layout) String() string {
	if l == LayoutSingle {
		return "single"
	}

	return "split"
}

// ParseLayout reads the persisted preference; an empty or unknown value keeps split.
func ParseLayout(raw string) Layout {
	if strings.EqualFold(strings.TrimSpace(raw), "single") {
		return LayoutSingle
	}

	return LayoutSplit
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

// stashesLoadedMsg carries the stash list.
type stashesLoadedMsg struct {
	seq     int
	stashes []domain.StashEntry
	err     error
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

// writeDoneMsg carries the outcome of one write, plus the notice a network write leaves behind.
type writeDoneMsg struct {
	seq    int
	op     string
	notice string
	err    error
}

var errNoService = errors.New("gitpanel: servicio de git no configurado")

// Model is the Git panel; it never performs I/O outside a command.
type Model struct {
	git  ports.GitService
	keys keymap.Registry

	section Section
	cursors [sectionCount]int
	offsets [sectionCount]int

	status        domain.GitStatus
	branches      []domain.Branch
	stashes       []domain.StashEntry
	stashesLoaded bool
	history       domain.CommitPage
	historyLoaded bool

	diff       domain.Diff
	request    domain.DiffRequest
	loaded     bool
	staged     bool
	diffBody   string
	diffLines  []string
	hunkCursor int
	marked     map[int]bool

	writing  bool
	writeOp  string
	writeErr error
	notice   string

	commit  commitModal
	prompt  promptModal
	confirm confirmModal

	loading loadingState
	errs    errorState
	seqs    seqState

	width  int
	height int
	bounds image.Rectangle

	// rowRects holds the rows the active section can show, tabRects the section labels of the
	// header and hunkRows the row span of every hunk of the loaded diff.
	rowRects [sectionCount][]image.Rectangle
	tabRects [sectionCount]image.Rectangle
	hunkRows []rowSpan

	// Layout of the Changes and Diff sections: the persisted preference, the mode in force and
	// the inner rectangles of both columns.
	layout      Layout
	split       bool
	listW       int
	previewW    int
	listRect    image.Rectangle
	previewRect image.Rectangle

	// scrollList and scrollPreview are the independent offsets of the two columns.
	scrollList    int
	scrollPreview int

	// highlighted is the cached, coloured preview body; key tells which diff and width it belongs to.
	highlighted  []string
	highlightKey string

	help bool
}

// rowSpan is a half-open range of rows of the diff section.
type rowSpan struct {
	start int
	end   int
}

// commitModal is the commit message editor, kept across an escaped modal so the draft survives.
type commitModal struct {
	open bool
	err  error
	area textarea.Model
}

// promptKind selects what the one-line prompt asks for.
type promptKind uint8

const (
	promptNone promptKind = iota
	promptNewBranch
	promptStashMessage
)

// promptModal is the one-line input of a branch name or a stash message.
type promptModal struct {
	open  bool
	kind  promptKind
	err   error
	input textinput.Model
}

// confirmKind selects what the confirmation would do.
type confirmKind uint8

const (
	confirmNone confirmKind = iota
	confirmDeleteBranch
	confirmStashDrop
	confirmAbortMerge
)

// confirmModal asks for a yes or a no before a destructive operation.
type confirmModal struct {
	open   bool
	kind   confirmKind
	branch string
	index  int
	err    error
}

type loadingState struct {
	status   bool
	branches bool
	stashes  bool
	history  bool
	diff     bool
}

type errorState struct {
	status   error
	branches error
	stashes  error
	history  error
	diff     error
}

type seqState struct {
	status   int
	branches int
	stashes  int
	history  int
	diff     int
	write    int
}

// New builds the panel; Init asks for the status and the branches, nothing else. An empty keymap
// falls back to the built-in defaults.
func New(git ports.GitService, keys keymap.Registry) Model {
	return NewWithLayout(git, keys, LayoutSplit)
}

// NewWithLayout builds the panel with the persisted layout preference.
func NewWithLayout(git ports.GitService, keys keymap.Registry, layout Layout) Model {
	if len(keys.All()) == 0 {
		keys = keymap.Default()
	}

	m := Model{
		git:     git,
		keys:    keys,
		layout:  layout,
		loading: loadingState{status: true, branches: true},
		seqs:    seqState{status: 1, branches: 1},
		marked:  make(map[int]bool),
		commit:  newCommitModal(defaultWidth, defaultHeight),
		prompt:  newPromptModal(promptNone, defaultWidth),
	}
	m.SetSize(defaultWidth, defaultHeight)

	return m
}

// newCommitModal builds the message editor with an empty draft.
func newCommitModal(width, height int) commitModal {
	area := textarea.New()
	area.Placeholder = "mensaje del commit"
	area.ShowLineNumbers = false
	area.CharLimit = domain.MaxCommitMessageBytes
	area.SetVirtualCursor(true)
	area.SetWidth(modalWidth(width))
	area.SetHeight(modalRows(height))

	return commitModal{area: area}
}

// newPromptModal builds the text input of one prompt kind.
func newPromptModal(kind promptKind, width int) promptModal {
	input := textinput.New()
	input.Prompt = promptMark
	input.Placeholder = promptPlaceholder(kind)
	input.CharLimit = promptCharLimit(kind)
	input.SetWidth(promptWidth(width))

	return promptModal{kind: kind, input: input}
}

const promptMark = "> "

// promptPlaceholder is the hint shown while the prompt is empty.
func promptPlaceholder(kind promptKind) string {
	if kind == promptStashMessage {
		return "mensaje del stash (opcional)"
	}

	return "nombre [punto de partida]"
}

// promptCharLimit bounds what the input accepts, mirroring the domain limits.
func promptCharLimit(kind promptKind) int {
	if kind == promptStashMessage {
		return domain.MaxCommitMessageBytes
	}

	return domain.MaxBranchNameBytes
}

// CapturesInput reports that a modal owns the keyboard of the panel.
func (m Model) CapturesInput() bool {
	return m.commit.open || m.prompt.open || m.confirm.open
}

// modalWidth keeps the editor one cell inside the panel.
func modalWidth(width int) int {
	return max(width-1, 4)
}

// modalRows keeps the editor inside the body of the panel.
func modalRows(height int) int {
	body := max(height-headerRows-footerRows, minRows)

	return min(max(body-2, 1), commitRows)
}

// promptWidth keeps the input, plus its two-cell prompt, inside the panel.
func promptWidth(width int) int {
	return max(width-promptCells, 1)
}

// Init requests the status and the branch list of the repository.
func (m Model) Init() tea.Cmd {
	return tea.Batch(loadStatus(m.git, m.seqs.status), loadBranches(m.git, m.seqs.branches))
}

// Section returns the section currently displayed.
func (m Model) Section() Section {
	return m.section
}

// Loading reports whether any section is still reading or a write is in flight.
func (m Model) Loading() bool {
	return m.writing || m.loading.status || m.loading.branches || m.loading.stashes ||
		m.loading.history || m.loading.diff
}

// Err returns the first active error, if any.
func (m Model) Err() error {
	switch {
	case m.errs.status != nil:
		return m.errs.status
	case m.writeErr != nil:
		return m.writeErr
	case m.commit.err != nil:
		return m.commit.err
	case m.prompt.err != nil:
		return m.prompt.err
	case m.confirm.err != nil:
		return m.confirm.err
	case m.errs.diff != nil:
		return m.errs.diff
	case m.errs.branches != nil:
		return m.errs.branches
	case m.errs.stashes != nil:
		return m.errs.stashes
	default:
		return m.errs.history
	}
}

// SetSize stores the terminal size, keeping the origin of the panel.
func (m *Model) SetSize(width, height int) {
	m.SetBounds(m.bounds.Min.X, m.bounds.Min.Y, width, height)
}

// SetBounds places the panel in the terminal, resizes the modals and re-renders the cached diff.
func (m *Model) SetBounds(x, y, width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.bounds = image.Rect(x, y, x+m.width, y+m.height)
	m.commit.area.SetWidth(modalWidth(m.width))
	m.commit.area.SetHeight(modalRows(m.height))
	m.prompt.input.SetWidth(promptWidth(m.width))

	m.markTabs()
	m.sync()
}

// sync rebuilds the cached diff body, the column geometry and the hit-test rectangles; View only
// composes them.
func (m *Model) sync() {
	if !m.loaded {
		m.diffBody, m.diffLines, m.hunkRows = "", nil, nil
		m.highlighted, m.highlightKey = nil, ""
	} else {
		m.diffBody = common.RenderDiff(m.diff, m.width, true)
		m.diffLines = strings.Split(m.diffBody, "\n")
		m.hunkRows = diffSpans(m.diff)
		m.highlighted, m.highlightKey = m.highlightedPreview(), m.highlightKeyFor()
	}

	m.split = m.splitNow()
	m.listW, m.previewW = m.splitWidths(m.width)
	m.markColumns()

	// Both column offsets survive a resize, clamped to the rows they now have.
	m.scrollList = m.clampOffset(m.scrollList, len(m.listRows()))
	m.scrollPreview = m.clampOffset(m.scrollPreview, len(m.previewRows()))

	m.clamp(m.section)
}

// highlightKeyFor identifies the cached preview: the path, the mode and the width decide the lexer.
func (m Model) highlightKeyFor() string {
	return fmt.Sprintf("%s|%t|%d", m.request.Path, m.request.Staged, m.width)
}

// highlightedPreview colours the body of the diff once per key; a repeated render reuses it.
func (m *Model) highlightedPreview() []string {
	key := m.highlightKeyFor()
	if m.highlightKey == key && len(m.highlighted) == len(m.diffLines) {
		return m.highlighted
	}

	return common.HighlightDiffLines(m.request.Path, m.diffLines)
}

// splitNow reports whether the active section shows the list and the preview side by side.
func (m Model) splitNow() bool {
	if m.layout == LayoutSingle || m.width < gitSplitMinWidth {
		return false
	}

	return m.section == Changes || m.section == Diff
}

// previewSection reports whether the right column has content to draw.
func (m Model) previewSection() bool {
	return m.section == Diff || m.loaded
}

// listRows returns the rows of the left column: the changes, or the active section when the
// preview is not shown.
func (m Model) listRows() []string {
	if m.section == Diff {
		return m.changeRows()
	}

	return m.active()
}

// previewRows returns the rows of the right column, already formatted.
func (m Model) previewRows() []string {
	return m.diffRows()
}

// listColumns reports the width of each column; the preview gets nothing outside split mode.
func (m Model) listColumns() (int, int) {
	if !m.split {
		return m.width, 0
	}

	return m.listW, m.previewW
}

// scrollListBy and scrollPreviewBy move one column's offset without touching the other.
func (m *Model) scrollListBy(delta int) {
	m.scrollList = m.clampOffset(m.scrollList+delta, len(m.listRows()))
}

func (m *Model) scrollPreviewBy(delta int) {
	m.scrollPreview = m.clampOffset(m.scrollPreview+delta, len(m.previewRows()))
}

// clampOffset keeps one column offset inside its rows.
func (m Model) clampOffset(offset, rows int) int {
	return min(max(offset, 0), max(rows-m.bodyHeight(), 0))
}

// scrollListToCursor moves the list offset so the Changes cursor stays visible.
func (m *Model) scrollListToCursor() {
	height := m.bodyHeight()
	if height <= 0 {
		return
	}

	cursor := m.cursors[Changes]
	if cursor < m.scrollList {
		m.scrollList = cursor
	}
	if cursor >= m.scrollList+height {
		m.scrollList = cursor - height + 1
	}

	m.scrollList = m.clampOffset(m.scrollList, len(m.listRows()))
}

// splitWidths divides the panel width between both columns; in single mode the list takes it all.
func (m Model) splitWidths(panelW int) (int, int) {
	if !m.split {
		return max(panelW, 0), 0
	}

	high := max(panelW-minGitPreview, minGitList)
	list := min(max(panelW*listPercent/100, minGitList), high)

	return list, max(panelW-list, 0)
}

// markColumns stores the inner rectangle of each column, or a single one when they share the body.
func (m *Model) markColumns() {
	m.listRect, m.previewRect = image.Rectangle{}, image.Rectangle{}
	if m.width <= 0 {
		return
	}

	top := m.bounds.Min.Y + headerRows
	height := m.bodyHeight()
	if height <= 0 {
		return
	}

	if !m.split {
		m.listRect = image.Rect(m.bounds.Min.X, top, m.bounds.Min.X+m.width, top+height)

		return
	}

	m.listRect = image.Rect(m.bounds.Min.X, top, m.bounds.Min.X+m.listW, top+height)
	m.previewRect = image.Rect(m.bounds.Min.X+m.listW, top, m.bounds.Min.X+m.listW+m.previewW, top+height)
}

// diffSpans locates every hunk inside the rows of the diff section, its title line included.
func diffSpans(diff domain.Diff) []rowSpan {
	if diff.IsEmpty() || diff.Binary {
		return nil
	}

	spans := make([]rowSpan, 0, len(diff.Hunks))
	row := 1 // the first row of the section names the diff

	for _, hunk := range diff.Hunks {
		rows := len(hunk.Lines) + 1
		spans = append(spans, rowSpan{start: row, end: row + rows})
		row += rows
	}

	return spans
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
	case Stash:
		return m.stashRows()
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

func (m Model) stashRows() []string {
	rows := make([]string, 0, len(m.stashes))
	for _, entry := range m.stashes {
		rows = append(rows, stashRow(entry))
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
	if hunk := m.hunkLabel(); hunk != "" {
		header += " · " + hunk
	}

	body := m.diffLines
	if len(m.highlighted) == len(m.diffLines) {
		body = m.highlighted
	}

	return append([]string{header}, body...)
}

// hunkLabel describes the hunk cursor and the marks of the loaded diff.
func (m Model) hunkLabel() string {
	if len(m.diff.Hunks) == 0 {
		return ""
	}

	label := fmt.Sprintf("hunk %d/%d", m.hunkCursor+1, len(m.diff.Hunks))
	if count := len(m.selectedHunks()); count > 0 {
		label += fmt.Sprintf(" · %d marcados", count)
	}

	return label
}

// selectedHunks returns the marked hunk indices in order, ignoring the ones out of range.
func (m Model) selectedHunks() []int {
	var indices []int

	for index, marked := range m.marked {
		if marked && index >= 0 && index < len(m.diff.Hunks) {
			indices = append(indices, index)
		}
	}

	slices.Sort(indices)

	return indices
}

// moveHunk moves the hunk cursor of the loaded diff.
func (m *Model) moveHunk(delta int) {
	if m.section != Diff || len(m.diff.Hunks) == 0 {
		return
	}

	m.hunkCursor = min(max(m.hunkCursor+delta, 0), len(m.diff.Hunks)-1)
}

// toggleHunk marks or unmarks the hunk under the cursor.
func (m *Model) toggleHunk() {
	if m.section != Diff || m.hunkCursor < 0 || m.hunkCursor >= len(m.diff.Hunks) {
		return
	}
	if m.marked == nil {
		m.marked = make(map[int]bool)
	}

	m.marked[m.hunkCursor] = !m.marked[m.hunkCursor]
}

// resetHunks clears the hunk cursor and the marks of the previous diff.
func (m *Model) resetHunks() {
	m.hunkCursor = 0
	m.marked = make(map[int]bool)
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
	m.markRows(section, rows)
}

// markRows stores the rectangle of every row the section can show; the offset maps them at click
// time, so scrolling does not invalidate them.
func (m *Model) markRows(section Section, rows int) {
	if section >= sectionCount {
		return
	}

	m.rowRects[section] = nil

	count := min(m.bodyHeight(), rows)
	if count <= 0 || m.width <= 0 {
		return
	}

	top := m.bounds.Min.Y + headerRows
	rects := make([]image.Rectangle, 0, count)
	for row := 0; row < count; row++ {
		rects = append(rects, image.Rect(m.bounds.Min.X, top+row, m.bounds.Max.X, top+row+1))
	}

	m.rowRects[section] = rects
}

// markTabs stores the rectangle of every section tab the header can show.
func (m *Model) markTabs() {
	m.tabRects = [sectionCount]image.Rectangle{}
	if m.width <= 0 || m.height < headerRows {
		return
	}

	row := m.bounds.Min.Y + 1
	column := m.bounds.Min.X

	for section := Section(0); section < sectionCount; section++ {
		label := section.String()
		if column-m.bounds.Min.X+len([]rune(label)) > m.width {
			return // the cut labels of a narrow panel are not clickable
		}

		m.tabRects[section] = image.Rect(column, row, column+len([]rune(label)), row+1)
		column += len([]rune(label)) + len([]rune(tabSeparator))
	}
}

// rowAt maps terminal coordinates to the row index of the active section. In split mode it only
// answers for the list column: the preview has its own router.
func (m Model) rowAt(point image.Point) (int, bool) {
	if m.split {
		return m.listRowAt(point)
	}

	for index, rect := range m.rowRects[m.section] {
		if !point.In(rect) {
			continue
		}

		row := m.offsets[m.section] + index
		if row >= len(m.active()) {
			return 0, false
		}

		return row, true
	}

	return 0, false
}

// listRowAt maps a point inside the left column to an index of its rows.
func (m Model) listRowAt(point image.Point) (int, bool) {
	if !point.In(m.listRect) {
		return 0, false
	}

	row := m.scrollList + point.Y - m.listRect.Min.Y
	if row < 0 || row >= len(m.listRows()) {
		return 0, false
	}

	return row, true
}

// previewRowAt maps a point inside the right column to an index of the cached diff rows.
func (m Model) previewRowAt(point image.Point) (int, bool) {
	if !m.split || !point.In(m.previewRect) {
		return 0, false
	}

	row := m.scrollPreview + point.Y - m.previewRect.Min.Y
	if row < 0 || row >= len(m.previewRows()) {
		return 0, false
	}

	return row, true
}

// tabAt maps terminal coordinates to one of the visible section tabs.
func (m Model) tabAt(point image.Point) (Section, bool) {
	for section := Section(0); section < sectionCount; section++ {
		if m.tabRects[section].Dx() > 0 && point.In(m.tabRects[section]) {
			return section, true
		}
	}

	return m.section, false
}

// hunkAt maps a row of the diff section to the hunk that covers it.
func (m Model) hunkAt(row int) (int, bool) {
	for index, span := range m.hunkRows {
		if row >= span.start && row < span.end {
			return index, true
		}
	}

	return 0, false
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

// selectedBranch returns the branch under the cursor of the Branches section.
func (m Model) selectedBranch() (domain.Branch, bool) {
	if m.cursors[Branches] < 0 || m.cursors[Branches] >= len(m.branches) {
		return domain.Branch{}, false
	}

	return m.branches[m.cursors[Branches]], true
}

// selectedStashIndex returns the position of the stash under the cursor of its section.
func (m Model) selectedStashIndex() (int, bool) {
	if m.cursors[Stash] < 0 || m.cursors[Stash] >= len(m.stashes) {
		return 0, false
	}

	return m.stashes[m.cursors[Stash]].Index, true
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
	case Stash:
		return m.errs.stashes
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
	case Stash:
		return m.loading.stashes
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
	case errors.Is(err, domain.ErrNoChanges):
		return "no hay cambios preparados"
	case errors.Is(err, domain.ErrConflict):
		return "quedan conflictos sin resolver"
	case errors.Is(err, domain.ErrMergeInProgress):
		return "hay un merge en curso"
	case errors.Is(err, domain.ErrNoMergeInProgress):
		return "no hay ningún merge en curso"
	case errors.Is(err, domain.ErrDirtyWorktree):
		return "hay cambios sin guardar en el árbol de trabajo"
	case errors.Is(err, domain.ErrNoUpstream):
		return "la rama no tiene upstream"
	case errors.Is(err, domain.ErrNonFastForward):
		return "el remoto tiene commits nuevos: haz pull antes de push"
	case errors.Is(err, domain.ErrAuthRequired):
		return "el remoto pide autenticación"
	case errors.Is(err, domain.ErrRemoteUnreachable):
		return "no se pudo alcanzar el remoto"
	case errors.Is(err, domain.ErrNoRemote):
		return "no hay remoto configurado"
	case errors.Is(err, domain.ErrTooLarge):
		return "contenido demasiado grande"
	case errors.Is(err, domain.ErrPermissionDenied):
		return "permiso denegado"
	case errors.Is(err, domain.ErrInvalidPath):
		return "ruta inválida"
	case errors.Is(err, domain.ErrInvalidArgument):
		return "argumento inválido"
	case errors.Is(err, domain.ErrNotFound):
		return "no encontrado"
	case errors.Is(err, context.DeadlineExceeded):
		return "tiempo agotado"
	case errors.Is(err, context.Canceled):
		return "cancelado"
	default:
		return summarize(err)
	}
}

// summarize reduces an unknown failure to one line without control characters: the raw stderr of
// git never reaches the panel, it stays in the log.
func summarize(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")

	return truncate(strings.TrimSpace(strings.Map(dropControl, line)), maxErrorCells)
}

// dropControl keeps the printable runes, plus the tab the renderer already converts.
func dropControl(r rune) rune {
	if r == '\t' {
		return r
	}
	if r < ' ' || r == 0x7f {
		return -1
	}

	return r
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

func stashRow(entry domain.StashEntry) string {
	label := entry.Ref()
	if entry.Branch != "" {
		label += " · " + entry.Branch
	}
	if entry.Message != "" {
		label += " · " + entry.Message
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

// mergeNotice summarizes the outcome of a merge or a pull.
func mergeNotice(result domain.MergeResult) string {
	switch result.Status {
	case domain.UpToDate:
		return "merge: ya al día"
	case domain.FastForward:
		return "merge: avance rápido"
	case domain.Conflicted:
		return "merge: " + conflictCount(len(result.Conflicts))
	default:
		return "merge: commit de fusión"
	}
}

// forcesHistory reports which writes reload the history even if it was never shown.
func forcesHistory(op string) bool {
	switch op {
	case opCommit, opCheckout, opCreateBranch, opDeleteBranch, opMerge, opPull:
		return true
	default:
		return false
	}
}
