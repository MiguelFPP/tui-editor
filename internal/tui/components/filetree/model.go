// Package filetree renders the repository tree as a flattened, lazy-loaded list.
package filetree

import (
	"context"
	"errors"
	"image"
	"sort"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

const (
	defaultWidth  = 40
	defaultHeight = 20
	minRows       = 1

	loadTimeout      = 5 * time.Second
	doubleClickDelay = 400 * time.Millisecond
	wheelStep        = 3
)

// Item is one row of the flattened tree.
type Item struct {
	Node  domain.FileNode
	Depth int
}

// FileChosenMsg asks the root model to open Path.
type FileChosenMsg struct {
	Path domain.RelPath
}

// dirLoadedMsg carries the result of one lazy ReadDir.
type dirLoadedMsg struct {
	Dir     domain.RelPath
	Listing domain.DirListing
	Err     error
}

var errNoService = errors.New("filetree: servicio de archivos no configurado")

// Model is the filetree component: it owns no I/O, only the loaded tree state.
type Model struct {
	files ports.FileService
	root  domain.RelPath
	keys  keymap.Registry

	entries   map[domain.RelPath][]domain.FileNode
	expanded  map[domain.RelPath]bool
	loading   map[domain.RelPath]bool
	truncated map[domain.RelPath]bool

	visible  []Item
	matches  []Item
	rowRects []image.Rectangle

	cursor int
	offset int

	bounds image.Rectangle
	size   image.Point

	filter    string
	filtering bool

	err error

	clickRow  int
	clickedAt time.Time
	now       func() time.Time
}

// New builds the component for one root; nothing is read until Init runs. An
// empty keymap falls back to the built-in defaults.
func New(files ports.FileService, root domain.RelPath, keys keymap.Registry) Model {
	if len(keys.All()) == 0 {
		keys = keymap.Default()
	}

	return Model{
		files:     files,
		root:      root,
		keys:      keys,
		entries:   map[domain.RelPath][]domain.FileNode{},
		expanded:  map[domain.RelPath]bool{},
		loading:   map[domain.RelPath]bool{},
		truncated: map[domain.RelPath]bool{},
		bounds:    image.Rect(0, 0, defaultWidth, defaultHeight),
		size:      image.Pt(defaultWidth, defaultHeight),
		clickRow:  -1,
		now:       time.Now,
	}
}

// Init loads the root listing.
func (m Model) Init() tea.Cmd {
	return m.load(m.root)
}

// Update applies one message; directories are loaded through commands, never inline.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case dirLoadedMsg:
		m.applyListing(msg)
	case tea.KeyPressMsg:
		cmd = m.handleKey(msg)
	case tea.MouseClickMsg:
		cmd = m.handleClick(msg.Mouse())
	case tea.MouseWheelMsg:
		m.handleWheel(msg.Mouse())
	}

	m.sync()

	return m, cmd
}

// View renders the header (filter or error) plus the visible rows.
func (m Model) View() string {
	items := m.displayed()

	lines := make([]string, 0, m.rows()+1)
	if line, ok := m.headerLine(); ok {
		lines = append(lines, line)
	}

	if len(items) == 0 {
		if line, ok := m.emptyLine(); ok {
			lines = append(lines, line)
		}

		return strings.Join(lines, "\n")
	}

	end := min(m.offset+m.rows(), len(items))
	for i := m.offset; i < end; i++ {
		lines = append(lines, m.row(items[i], i == m.cursor))
	}

	return strings.Join(lines, "\n")
}

// SetBounds places the component in the terminal and recomputes the viewport.
func (m *Model) SetBounds(x, y, width, height int) {
	width, height = max(width, 0), max(height, 0)

	m.bounds = image.Rect(x, y, x+width, y+height)
	m.size = image.Pt(width, height)

	m.sync()
}

// Visible returns the rows currently displayed, filter included.
func (m Model) Visible() []Item {
	return m.displayed()
}

// Selected returns the node under the cursor.
func (m Model) Selected() (domain.FileNode, bool) {
	items := m.displayed()
	if m.cursor < 0 || m.cursor >= len(items) {
		return domain.FileNode{}, false
	}

	return items[m.cursor].Node, true
}

// Filter returns the active filter, empty when none.
func (m Model) Filter() string {
	return m.filter
}

// Loading reports whether a listing is still pending.
func (m Model) Loading() bool {
	return m.loadingNow()
}

// Err returns the active listing error, if any.
func (m Model) Err() error {
	return m.err
}

// load asks the port for one listing under a bounded context.
func (m Model) load(dir domain.RelPath) tea.Cmd {
	files := m.files
	if files == nil {
		return func() tea.Msg { return dirLoadedMsg{Dir: dir, Err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()

		listing, err := files.ReadDir(ctx, dir)
		if err != nil {
			return dirLoadedMsg{Dir: dir, Err: err}
		}

		return dirLoadedMsg{Dir: dir, Listing: listing}
	}
}

func (m *Model) applyListing(msg dirLoadedMsg) {
	delete(m.loading, msg.Dir)

	if msg.Err != nil {
		m.err = msg.Err

		return
	}

	m.err = nil
	m.entries[msg.Dir] = msg.Listing.Entries
	m.truncated[msg.Dir] = msg.Listing.Truncated
}

// handleKey resolves one key against the tree scope; the text filter owns its own keys.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.filtering && m.filterKey(msg) {
		return nil
	}

	action, ok := m.keys.Match(keymap.ScopeTree, msg.String())
	if !ok {
		return nil
	}

	switch action {
	case keymap.ActionTreeUp:
		m.setCursor(m.cursor - 1)
	case keymap.ActionTreeDown:
		m.setCursor(m.cursor + 1)
	case keymap.ActionTreePageUp:
		m.pageBy(-m.rows())
	case keymap.ActionTreePageDown:
		m.pageBy(m.rows())
	case keymap.ActionTreeHome:
		m.setCursor(0)
	case keymap.ActionTreeEnd:
		m.setCursor(len(m.displayed()) - 1)
	case keymap.ActionTreeExpand:
		if node, ok := m.Selected(); ok && node.Kind == domain.Dir {
			return m.expand(node)
		}
	case keymap.ActionTreeCollapse:
		return m.collapse()
	case keymap.ActionTreeActivate:
		return m.activate()
	case keymap.ActionTreeFilter:
		m.startFilter()
	case keymap.ActionTreeClearFilter:
		if m.filter != "" {
			m.filter = ""
			m.resetCursor()
		}
	}

	return nil
}

// filterKey handles the keys consumed while editing the filter.
func (m *Model) filterKey(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "enter":
		m.filtering = false

		return true
	case "esc":
		m.filtering = false
		m.filter = ""
		m.resetCursor()

		return true
	case "backspace":
		if m.filter != "" {
			m.filter = trimLastRune(m.filter)
			m.resetCursor()
		}

		return true
	}

	text := typedText(msg)
	if text == "" {
		return false
	}

	m.filter += text
	m.resetCursor()

	return true
}

func (m *Model) startFilter() {
	m.filtering = true
	m.filter = ""
	m.resetCursor()
}

func (m *Model) activate() tea.Cmd {
	node, ok := m.Selected()
	if !ok {
		return nil
	}

	if node.Kind != domain.Dir {
		return chosenCmd(node.Path)
	}
	if m.expanded[node.Path] {
		delete(m.expanded, node.Path)

		return nil
	}

	return m.expand(node)
}

// collapse closes the selected directory, or jumps to its parent when it is closed.
func (m *Model) collapse() tea.Cmd {
	items := m.displayed()
	if m.cursor < 0 || m.cursor >= len(items) {
		return nil
	}

	item := items[m.cursor]
	if item.Node.Kind == domain.Dir && m.expanded[item.Node.Path] {
		delete(m.expanded, item.Node.Path)

		return nil
	}

	for i := m.cursor - 1; i >= 0; i-- {
		if items[i].Depth < item.Depth {
			m.setCursor(i)

			return nil
		}
	}

	return nil
}

// expand shows dir and loads it on demand.
func (m *Model) expand(node domain.FileNode) tea.Cmd {
	if node.Kind != domain.Dir {
		return nil
	}

	m.expanded[node.Path] = true
	if _, ok := m.entries[node.Path]; ok || m.loading[node.Path] {
		return nil
	}

	m.loading[node.Path] = true

	return m.load(node.Path)
}

func (m *Model) handleClick(ev tea.Mouse) tea.Cmd {
	if ev.Button != tea.MouseLeft {
		return nil
	}

	index, ok := m.rowAt(ev.X, ev.Y)
	if !ok {
		return nil
	}
	m.cursor = index

	now := m.clock()
	doubleClick := m.clickRow == index && now.Sub(m.clickedAt) <= doubleClickDelay
	m.clickRow, m.clickedAt = index, now
	if !doubleClick {
		return nil
	}

	m.clickRow = -1

	return m.activate()
}

func (m *Model) handleWheel(ev tea.Mouse) {
	switch ev.Button {
	case tea.MouseWheelUp:
		m.offset -= wheelStep
	case tea.MouseWheelDown:
		m.offset += wheelStep
	}
}

// rowAt maps absolute terminal coordinates to an index of the displayed list.
func (m Model) rowAt(x, y int) (int, bool) {
	point := image.Pt(x, y)
	if !point.In(m.bounds) {
		return 0, false
	}

	for i, rect := range m.rowRects {
		if point.In(rect) {
			return m.offset + i, true
		}
	}

	return 0, false
}

func (m *Model) pageBy(delta int) {
	m.offset += delta
	m.setCursor(m.cursor + delta)
}

// setCursor clamps the cursor and scrolls the viewport to keep it visible.
func (m *Model) setCursor(index int) {
	m.cursor = clamp(index, 0, len(m.displayed())-1)
	m.scrollToCursor()
}

// scrollToCursor moves the offset so the cursor stays inside the viewport.
func (m *Model) scrollToCursor() {
	items, rows := m.displayed(), m.rows()

	m.offset = clamp(m.offset, 0, max(len(items)-rows, 0))
	if len(items) == 0 {
		m.offset = 0

		return
	}

	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
}

func (m *Model) resetCursor() {
	m.setCursor(0)
	m.offset = 0
}

// sync rebuilds the flattened list, the matches and the hit-test rectangles.
func (m *Model) sync() {
	m.visible = m.flatten()
	if m.filter == "" {
		m.matches = nil
	} else {
		m.matches = m.flattenMatches()
	}

	items := m.displayed()
	rows := m.rows()

	// The offset wins over the cursor here, so wheel scrolling is not undone.
	m.offset = clamp(m.offset, 0, max(len(items)-rows, 0))
	m.cursor = clamp(m.cursor, 0, len(items)-1)
	if m.cursor < m.offset {
		m.cursor = m.offset
	}
	if m.cursor >= m.offset+rows {
		m.cursor = min(m.offset+rows-1, len(items)-1)
	}

	rects := make([]image.Rectangle, 0, rows)
	top := m.bounds.Min.Y + m.headerRows()
	for i := m.offset; i < len(items) && len(rects) < rows; i++ {
		rects = append(rects, image.Rect(m.bounds.Min.X, top+len(rects), m.bounds.Max.X, top+len(rects)+1))
	}

	m.rowRects = rects
}

func (m Model) flatten() []Item {
	return m.flattenDir(m.root, 0, make([]Item, 0, 16))
}

func (m Model) flattenDir(dir domain.RelPath, depth int, out []Item) []Item {
	if depth >= domain.MaxTreeDepth {
		return out
	}

	for _, node := range m.entries[dir] {
		out = append(out, Item{Node: node, Depth: depth})
		if node.Kind == domain.Dir && m.expanded[node.Path] {
			out = m.flattenDir(node.Path, depth+1, out)
		}
	}

	return out
}

// flattenMatches searches every loaded node by name and path, then sorts by path.
func (m Model) flattenMatches() []Item {
	needle := strings.ToLower(m.filter)

	matches := make([]Item, 0, 16)
	for _, nodes := range m.entries {
		for _, node := range nodes {
			if strings.Contains(strings.ToLower(node.Name), needle) || strings.Contains(strings.ToLower(string(node.Path)), needle) {
				matches = append(matches, Item{Node: node})
			}
		}
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].Node.Path < matches[j].Node.Path })

	return matches
}

func (m Model) displayed() []Item {
	if m.filter == "" {
		return m.visible
	}

	return m.matches
}

func (m Model) rows() int {
	return max(m.size.Y-m.headerRows(), minRows)
}

func (m Model) headerRows() int {
	if m.err != nil || m.filtering || m.filter != "" {
		return 1
	}

	return 0
}

func (m Model) headerLine() (string, bool) {
	switch {
	case m.err != nil:
		return styles.Fail.Render("error: " + m.err.Error()), true
	case m.filtering:
		return styles.Meta.Render(m.keys.Label(keymap.ActionTreeFilter) + m.filter + "▏"), true
	case m.filter != "":
		return styles.Meta.Render("filtro: " + m.keys.Label(keymap.ActionTreeFilter) + m.filter), true
	default:
		return "", false
	}
}

func (m Model) emptyLine() (string, bool) {
	switch {
	case m.err != nil:
		return "", false
	case m.loadingNow():
		return styles.Meta.Render("cargando…"), true
	case m.filter != "":
		return styles.Meta.Render("sin coincidencias"), true
	default:
		return styles.Meta.Render("vacío"), true
	}
}

func (m Model) loadingNow() bool {
	return len(m.loading) > 0 || (len(m.entries) == 0 && m.err == nil)
}

func (m Model) row(item Item, active bool) string {
	text := m.rowText(item)

	switch {
	case active:
		return styles.Cursor.Render(text)
	case item.Node.Kind == domain.Dir:
		return styles.Dir.Render(text)
	default:
		return text
	}
}

func (m Model) rowText(item Item) string {
	label := item.Node.Name
	prefix := strings.Repeat("  ", item.Depth)

	if item.Node.Kind == domain.Dir {
		label += "/"
		if m.truncated[item.Node.Path] {
			label += "…"
		}
		if m.expanded[item.Node.Path] {
			prefix += "▾ "
		} else {
			prefix += "▸ "
		}
	} else {
		prefix += "  "
	}

	return truncate(prefix+label, m.size.X)
}

func (m Model) clock() time.Time {
	if m.now == nil {
		return time.Now()
	}

	return m.now()
}

func chosenCmd(path domain.RelPath) tea.Cmd {
	return func() tea.Msg { return FileChosenMsg{Path: path} }
}

func typedText(msg tea.KeyPressMsg) string {
	if msg.Text != "" {
		return msg.Text
	}
	if msg.Mod == 0 && unicode.IsPrint(msg.Code) {
		return string(msg.Code)
	}

	return ""
}

func trimLastRune(text string) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}

	return string(runes[:len(runes)-1])
}

func truncate(text string, width int) string {
	if width <= 0 {
		return text
	}

	runes := []rune(text)
	if len(runes) <= width {
		return text
	}

	return string(runes[:width])
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
