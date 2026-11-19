// Package dockerpanel renders one table of Docker containers, the one-shot stats of the selection
// and the paginated logs of the chosen container, and drives start, stop and restart behind a
// confirmation. The ticker only exists while the tab is visible.
package dockerpanel

import (
	"context"
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

const (
	defaultWidth  = 80
	defaultHeight = 20

	headerRows = 2 // the title row and the column headers
	footerRows = 1
	minRows    = 1

	// defaultTickInterval is the polling period; the ticker is disabled while the tab is hidden.
	defaultTickInterval = time.Second

	listTimeout   = 3 * time.Second
	statsTimeout  = 2 * time.Second
	logTimeout    = 5 * time.Second
	actionTimeout = 5 * time.Second

	wheelStep = 3

	logTail = domain.DefaultLogTail

	nameColumn   = 24
	imageColumn  = 28
	stateColumn  = 10
	healthColumn = 9
	columnGap    = "  "

	// columnSeparator joins the fields of the title and the footer.
	columnSeparator = " · "

	// unavailableText is the actionable message shown when the daemon cannot be reached.
	unavailableText = "docker no disponible · revisa el socket y pulsa r para reintentar"

	// scope labels name the active mode of the container list.
	scopeProject = "alcance: proyecto"
	scopeAll     = "alcance: todos"

	// logTimeLayout is the timestamp prefix of a rendered log line.
	logTimeLayout = "15:04:05"

	// The actions and the notice each one leaves behind.
	opStart   = "start"
	opStop    = "stop"
	opRestart = "restart"

	noticeStart   = "contenedor iniciado"
	noticeStop    = "contenedor detenido"
	noticeRestart = "contenedor reiniciado"

	// maxErrorCells bounds the line the panel shows for a failure it does not recognise.
	maxErrorCells = 160
)

// containersMsg carries one refresh of the container list.
type containersMsg struct {
	seq        int
	containers []domain.ContainerSummary
	err        error
}

// statsMsg carries one one-shot sample of the selected container.
type statsMsg struct {
	seq   int
	id    string
	stats domain.ContainerStats
	err   error
}

// logsMsg carries one bounded page of logs.
type logsMsg struct {
	seq  int
	id   string
	page domain.LogPage
	err  error
}

// actionMsg carries the outcome of one start, stop or restart.
type actionMsg struct {
	seq int
	id  string
	op  string
	err error
}

// tickMsg re-arms the conditional ticker; the generation invalidates the ticks of a hidden tab.
type tickMsg struct {
	gen int
}

// confirmKind selects which action a confirmation would run.
type confirmKind uint8

const (
	confirmNone confirmKind = iota
	confirmStart
	confirmStop
	confirmRestart
)

func (k confirmKind) op() string {
	switch k {
	case confirmStart:
		return opStart
	case confirmStop:
		return opStop
	case confirmRestart:
		return opRestart
	default:
		return ""
	}
}

func (k confirmKind) question(name string) string {
	switch k {
	case confirmStart:
		return "¿iniciar " + name + "?"
	case confirmStop:
		return "¿detener " + name + "?"
	case confirmRestart:
		return "¿reiniciar " + name + "?"
	default:
		return ""
	}
}

var errNoService = errors.New("dockerpanel: servicio de docker no configurado")

// Model is the Docker panel; it never performs I/O outside a command.
type Model struct {
	docker   ports.DockerService
	repoRoot string
	keys     keymap.Registry
	showAll  bool
	scope    domain.ContainerScope

	containers []domain.ContainerSummary
	rows       []string
	cursor     int
	offset     int
	anchoredID string

	stats      domain.ContainerStats
	statsID    string
	statsReady bool

	logs       domain.LogPage
	logsOpen   bool
	logsErr    error
	logsID     string
	logsName   string
	logsBefore time.Time
	logLines   []string
	viewport   viewport.Model

	active  bool
	tickGen int
	// interval is the polling period; tests shorten it so a batch never waits for the timer.
	interval time.Duration

	loadingContainers bool
	loadingStats      bool
	loadingLogs       bool
	writing           bool

	err       error
	actionErr error
	actionOp  string
	notice    string

	confirm     confirmKind
	confirmID   string
	confirmName string

	containersSeq int
	statsSeq      int
	logsSeq       int
	actionSeq     int

	width  int
	height int
	bounds image.Rectangle

	rowRects []image.Rectangle
}

// New builds the panel scoped to repoRoot; nothing is read until the tab becomes active. An empty
// keymap falls back to the built-in defaults.
func New(docker ports.DockerService, repoRoot string, keys keymap.Registry) Model {
	if len(keys.All()) == 0 {
		keys = keymap.Default()
	}

	m := Model{
		docker:   docker,
		repoRoot: strings.TrimSpace(repoRoot),
		keys:     keys,
		viewport: viewport.New(),
		interval: defaultTickInterval,
	}
	m.showAll = m.repoRoot == "" // an unknown root must never hide every container
	m.scope = m.buildScope()
	m.viewport.FillHeight = true
	m.viewport.MouseWheelEnabled = true
	m.SetSize(defaultWidth, defaultHeight)

	return m
}

// buildScope derives the scope; an unknown root lists everything.
func (m Model) buildScope() domain.ContainerScope {
	if m.showAll || m.repoRoot == "" {
		return domain.ContainerScope{}
	}

	return domain.ContainerScope{WorkingDir: m.repoRoot, Project: filepath.Base(m.repoRoot)}
}

// Loading reports whether a read or an action is still in flight.
func (m Model) Loading() bool {
	return m.writing || m.loadingContainers || m.loadingStats || m.loadingLogs
}

// Err returns the first active failure: an action, a load or an open logs read.
func (m Model) Err() error {
	switch {
	case m.actionErr != nil:
		return m.actionErr
	case m.err != nil:
		return m.err
	case m.logsOpen && m.logsErr != nil:
		return m.logsErr
	default:
		return nil
	}
}

// CapturesInput reports that a confirmation or the logs own the keyboard of the panel.
func (m Model) CapturesInput() bool {
	return m.confirm != confirmNone || m.logsOpen
}

// SetSize stores the terminal size, keeping the origin of the panel.
func (m *Model) SetSize(width, height int) {
	m.SetBounds(m.bounds.Min.X, m.bounds.Min.Y, width, height)
}

// SetBounds places the panel in the terminal and resizes the logs viewport.
func (m *Model) SetBounds(x, y, width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.bounds = image.Rect(x, y, x+m.width, y+m.height)
	m.viewport.SetWidth(max(m.width, 1))
	m.viewport.SetHeight(m.logViewportHeight())
	m.markRows()
}

// bodyHeight is the number of rows the container list can show.
func (m Model) bodyHeight() int {
	return max(m.height-headerRows-footerRows, minRows)
}

// logViewportHeight leaves one row for the logs title and one for its hints.
func (m Model) logViewportHeight() int {
	return max(m.height-2, minRows)
}

// clamp keeps the cursor and the offset inside the loaded list.
func (m *Model) clamp() {
	rows := len(m.containers)
	m.cursor = min(max(m.cursor, 0), max(rows-1, 0))
	m.offset = min(max(m.offset, 0), max(rows-m.bodyHeight(), 0))
	m.scrollToCursor()
	m.markRows()
}

// scrollToCursor moves the offset so the cursor stays visible.
func (m *Model) scrollToCursor() {
	height := m.bodyHeight()
	if height <= 0 {
		return
	}

	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+height {
		m.offset = m.cursor - height + 1
	}
}

// markRows stores the rectangle of every visible row; the offset maps them at click time.
func (m *Model) markRows() {
	m.rowRects = m.rowRects[:0]

	count := min(m.bodyHeight(), len(m.containers))
	if count <= 0 || m.width <= 0 {
		return
	}

	top := m.bounds.Min.Y + headerRows
	for row := 0; row < count; row++ {
		m.rowRects = append(m.rowRects, image.Rect(m.bounds.Min.X, top+row, m.bounds.Max.X, top+row+1))
	}
}

// rowAt maps terminal coordinates to the container row under them.
func (m Model) rowAt(point image.Point) (int, bool) {
	for index, rect := range m.rowRects {
		if !point.In(rect) {
			continue
		}

		row := m.offset + index
		if row >= len(m.containers) {
			return 0, false
		}

		return row, true
	}

	return 0, false
}

// selected returns the container under the cursor.
func (m Model) selected() (domain.ContainerSummary, bool) {
	if m.cursor < 0 || m.cursor >= len(m.containers) {
		return domain.ContainerSummary{}, false
	}

	return m.containers[m.cursor], true
}

// unavailable reports whether the daemon is missing or unreachable.
func (m Model) unavailable() bool {
	if m.docker == nil {
		return true
	}

	return errors.Is(m.Err(), domain.ErrDockerUnavailable) ||
		errors.Is(m.Err(), domain.ErrPermissionDenied) ||
		errors.Is(m.Err(), errNoService)
}

// titleText is the header: identity, scope, state and stats of the selection.
func (m Model) titleText() string {
	parts := []string{"docker", m.scopeLabel()}
	if state := m.stateLabel(); state != "" {
		parts = append(parts, state)
	}
	parts = append(parts, countLabel(len(m.containers)))

	if container, ok := m.selected(); ok {
		parts = append(parts, containerLabel(container))
		if stats := m.statsLabel(); stats != "" {
			parts = append(parts, stats)
		}
	}

	return strings.Join(parts, columnSeparator)
}

// scopeLabel names the active mode of the container list.
func (m Model) scopeLabel() string {
	if m.showAll {
		return scopeAll
	}

	return scopeProject
}

// scopeHint names the key that switches the scope and what it would show.
func (m Model) scopeHint() string {
	if m.showAll {
		return "a: ver proyecto"
	}

	return "a: ver todos"
}

// statsLabel describes the last sample, but only while it belongs to the current selection.
func (m Model) statsLabel() string {
	container, ok := m.selected()
	if !ok || !m.statsReady || m.statsID != container.ID {
		return ""
	}

	parts := []string{fmt.Sprintf("cpu %.1f%%", m.stats.CPUPercent)}
	parts = append(parts, fmt.Sprintf("mem %s/%s", humanBytes(m.stats.MemoryUsed), humanBytes(m.stats.MemoryLimit)))
	if m.stats.PIDs > 0 {
		parts = append(parts, fmt.Sprintf("pids %d", m.stats.PIDs))
	}

	return strings.Join(parts, " ")
}

// stateLabel reports the state of the panel in the header, always next to text.
func (m Model) stateLabel() string {
	switch {
	case m.unavailable():
		return "no disponible"
	case m.writing:
		if m.actionOp == "" {
			return "actuando…"
		}

		return "actuando: " + m.actionOp
	case m.loadingContainers:
		return "cargando…"
	default:
		return ""
	}
}

// containerRow renders one table row with fixed columns, so the rows line up.
func containerRow(container domain.ContainerSummary) string {
	return strings.Join([]string{
		padRight(truncate(containerLabel(container), nameColumn), nameColumn),
		padRight(truncate(container.Image, imageColumn), imageColumn),
		padRight(container.State.String(), stateColumn),
		padRight(healthLabel(container.Health), healthColumn),
		portLabel(container.Ports),
	}, columnGap)
}

// columnHeader names the columns of the container table.
func columnHeader() string {
	return strings.Join([]string{
		padRight("nombre", nameColumn),
		padRight("imagen", imageColumn),
		padRight("estado", stateColumn),
		padRight("health", healthColumn),
		"puertos",
	}, columnGap)
}

func containerLabel(container domain.ContainerSummary) string {
	if container.Name != "" {
		return container.Name
	}

	return container.ShortID()
}

func healthLabel(health domain.HealthState) string {
	if health == domain.HealthNone {
		return "-"
	}

	return health.String()
}

func portLabel(ports []domain.PortMapping) string {
	if len(ports) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, port.String())
	}

	return strings.Join(parts, ",")
}

func countLabel(count int) string {
	switch count {
	case 0:
		return "sin contenedores"
	case 1:
		return "1 contenedor"
	default:
		return strconv.Itoa(count) + " contenedores"
	}
}

func lineLabel(count int) string {
	if count == 1 {
		return "1 línea"
	}

	return strconv.Itoa(count) + " líneas"
}

// humanBytes renders a byte count with the largest unit that fits it.
func humanBytes(size uint64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)

	switch {
	case size >= gib:
		return fmt.Sprintf("%.1f GiB", float64(size)/float64(gib))
	case size >= mib:
		return fmt.Sprintf("%.1f MiB", float64(size)/float64(mib))
	case size >= kib:
		return fmt.Sprintf("%.1f KiB", float64(size)/float64(kib))
	default:
		return strconv.FormatUint(size, 10) + " B"
	}
}

// describeError turns an adapter failure into the message the panel shows.
func describeError(err error) string {
	switch {
	case errors.Is(err, errNoService):
		return "servicio de docker no configurado"
	case errors.Is(err, domain.ErrDockerUnavailable):
		return "docker no está disponible"
	case errors.Is(err, domain.ErrPermissionDenied):
		return "permiso denegado al socket de docker"
	case errors.Is(err, domain.ErrNotFound):
		return "contenedor no encontrado"
	case errors.Is(err, domain.ErrInvalidArgument):
		return "argumento inválido"
	case errors.Is(err, context.DeadlineExceeded):
		return "tiempo agotado"
	case errors.Is(err, context.Canceled):
		return "cancelado"
	default:
		return summarize(err)
	}
}

// summarize reduces an unknown failure to one line without control characters.
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

func padRight(text string, width int) string {
	if width <= 0 {
		return text
	}

	gap := width - len([]rune(text))
	if gap <= 0 {
		return text
	}

	return text + strings.Repeat(" ", gap)
}
