package dockerpanel

import (
	"context"
	"image"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

// Update applies one message; every read and every action goes through a command, never inline.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	case containersMsg:
		m.applyContainers(msg)
	case statsMsg:
		m.applyStats(msg)
	case logsMsg:
		return m, m.applyLogs(msg)
	case actionMsg:
		return m, m.applyAction(msg)
	case tickMsg:
		return m, m.onTick(msg)
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	case tea.MouseClickMsg:
		return m, m.handleClick(msg)
	case tea.MouseWheelMsg:
		return m, m.handleWheel(msg)
	}

	return m, nil
}

// SetActive starts or stops the conditional ticker: it only runs while the Docker tab is visible.
func (m *Model) SetActive(active bool) tea.Cmd {
	if !active {
		m.active = false
		m.tickGen++ // invalidates the ticks already scheduled

		return nil
	}
	if m.active {
		return nil
	}

	m.active = true
	m.tickGen++

	return tea.Batch(m.startLoad(), m.scheduleTick())
}

// startLoad refreshes the container list and the stats of the selection.
func (m *Model) startLoad() tea.Cmd {
	commands := make([]tea.Cmd, 0, 2)
	commands = append(commands, m.loadContainers())

	if cmd := m.loadStats(); cmd != nil {
		commands = append(commands, cmd)
	}

	return tea.Batch(commands...)
}

// scheduleTick arms the next tick of the current generation, or nothing while inactive.
func (m Model) scheduleTick() tea.Cmd {
	if !m.active {
		return nil
	}

	gen := m.tickGen

	return tea.Tick(m.interval, func(time.Time) tea.Msg { return tickMsg{gen: gen} })
}

// onTick refreshes at most once per period, and never starts a second load while one is in flight.
func (m *Model) onTick(msg tickMsg) tea.Cmd {
	if !m.active || msg.gen != m.tickGen {
		return nil // inactive or stale generation: no Docker call and no new tick
	}

	commands := make([]tea.Cmd, 0, 2)
	if !m.loadingContainers {
		commands = append(commands, m.loadContainers())
	}
	if !m.loadingStats {
		if cmd := m.loadStats(); cmd != nil {
			commands = append(commands, cmd)
		}
	}
	if cmd := m.scheduleTick(); cmd != nil {
		commands = append(commands, cmd)
	}

	return tea.Batch(commands...)
}

// handleKey routes the confirmation and the logs first, then resolves the key against the Docker
// scope of the registry.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case m.confirm != confirmNone:
		return m.handleConfirmKey(msg)
	case m.logsOpen:
		return m.handleLogsKey(msg)
	}

	action, ok := m.keys.Match(keymap.ScopeDocker, msg.String())
	if !ok {
		return nil
	}

	switch action {
	case keymap.ActionDockerUp:
		m.moveCursor(1)
	case keymap.ActionDockerDown:
		m.moveCursor(-1)
	case keymap.ActionDockerPageDown:
		m.moveCursor(m.bodyHeight())
	case keymap.ActionDockerPageUp:
		m.moveCursor(-m.bodyHeight())
	case keymap.ActionDockerHome:
		m.moveCursor(-len(m.containers))
	case keymap.ActionDockerEnd:
		m.moveCursor(len(m.containers))
	case keymap.ActionDockerStats:
		return m.loadStats()
	case keymap.ActionDockerLogs:
		return m.openLogs()
	case keymap.ActionDockerStart:
		return m.openConfirm(confirmStart)
	case keymap.ActionDockerStop:
		return m.openConfirm(confirmStop)
	case keymap.ActionDockerRestart:
		return m.openConfirm(confirmRestart)
	case keymap.ActionDockerRefresh:
		return m.refresh()
	case keymap.ActionDockerScopeToggle:
		return m.toggleScope()
	}

	return nil
}

// toggleScope switches between project-only and every container, keeping the selection.
func (m *Model) toggleScope() tea.Cmd {
	if m.repoRoot == "" {
		return nil // without a root there is nothing to narrow to
	}

	if selected, ok := m.selected(); ok {
		m.anchoredID = selected.ID
	}
	m.showAll = !m.showAll
	m.scope = m.buildScope()
	m.cursor, m.offset = 0, 0

	return m.startLoad()
}

// moveCursor moves the selection and keeps it inside the visible window.
func (m *Model) moveCursor(delta int) {
	rows := len(m.containers)
	if rows == 0 {
		return
	}

	m.cursor = min(max(m.cursor+delta, 0), rows-1)
	m.scrollToCursor()
	m.markRows()
}

// refresh reloads everything on demand, also to retry an unavailable daemon.
func (m *Model) refresh() tea.Cmd {
	m.actionErr = nil

	return m.startLoad()
}

// handleConfirmKey resolves a confirmation: y or enter accepts, n or esc cancels.
func (m *Model) handleConfirmKey(msg tea.KeyPressMsg) tea.Cmd {
	action, ok := m.keys.Match(keymap.ScopeDockerConfirm, msg.String())
	if !ok {
		return nil // every other key is ignored: the confirmation keeps the focus
	}

	switch action {
	case keymap.ActionDockerConfirmYes:
		return m.submitConfirm()
	case keymap.ActionDockerConfirmNo:
		if m.writing {
			return nil
		}

		m.cancelConfirm()
	}

	return nil
}

// openConfirm asks for a confirmation before touching the daemon; no action ever runs without it.
func (m *Model) openConfirm(kind confirmKind) tea.Cmd {
	if m.writing {
		return nil
	}

	container, ok := m.selected()
	if !ok {
		return nil
	}

	m.confirm = kind
	m.confirmID = container.ID
	m.confirmName = containerLabel(container)

	return nil
}

// cancelConfirm closes the confirmation without an answer.
func (m *Model) cancelConfirm() {
	m.confirm = confirmNone
	m.confirmID = ""
	m.confirmName = ""
}

// submitConfirm runs the confirmed action through a command with its own timeout.
func (m *Model) submitConfirm() tea.Cmd {
	if m.writing || m.confirm == confirmNone || m.confirmID == "" {
		return nil
	}

	kind := m.confirm
	id := m.confirmID
	m.cancelConfirm()

	m.actionSeq++
	m.writing = true
	m.actionOp = kind.op()
	m.actionErr = nil
	m.notice = ""

	return actionCmd(m.docker, m.actionSeq, id, kind)
}

// handleLogsKey scrolls the loaded logs; scrolling up at the top loads the previous page.
func (m *Model) handleLogsKey(msg tea.KeyPressMsg) tea.Cmd {
	action, ok := m.keys.Match(keymap.ScopeDockerLogs, msg.String())
	if !ok {
		return nil
	}

	switch action {
	case keymap.ActionDockerLogsClose:
		m.closeLogs()
	case keymap.ActionDockerLogsReload:
		m.logsBefore = time.Time{}
		m.logs = domain.LogPage{}
		m.renderLogs()
		m.viewport.GotoTop()

		return m.loadLogs()
	case keymap.ActionDockerLogsUp:
		m.viewport.ScrollDown(1)
	case keymap.ActionDockerLogsDown:
		return m.scrollLogsUp(1)
	case keymap.ActionDockerLogsPageDown:
		m.viewport.PageDown()
	case keymap.ActionDockerLogsPageUp:
		return m.scrollLogsUp(m.viewport.Height())
	case keymap.ActionDockerLogsHome:
		m.viewport.GotoTop()
	case keymap.ActionDockerLogsEnd:
		m.viewport.GotoBottom()
	}

	return nil
}

// scrollLogsUp scrolls the viewport up by lines, or loads the previous page when already at the top.
func (m *Model) scrollLogsUp(lines int) tea.Cmd {
	if m.viewport.AtTop() {
		return m.loadOlder()
	}

	m.viewport.ScrollUp(lines)

	return nil
}

// handleClick selects the row under the cursor; the stats load with enter.
func (m *Model) handleClick(msg tea.MouseClickMsg) tea.Cmd {
	if msg.Button != tea.MouseLeft || m.CapturesInput() {
		return nil
	}

	row, ok := m.rowAt(image.Pt(msg.X, msg.Y))
	if !ok {
		return nil
	}

	m.cursor = row
	m.scrollToCursor()
	m.markRows()

	return nil
}

// handleWheel scrolls the logs viewport, or the container list under the cursor.
func (m *Model) handleWheel(msg tea.MouseWheelMsg) tea.Cmd {
	if m.logsOpen {
		var cmd tea.Cmd

		m.viewport, cmd = m.viewport.Update(msg)

		return cmd
	}

	switch msg.Button {
	case tea.MouseWheelUp:
		m.offset -= wheelStep
	case tea.MouseWheelDown:
		m.offset += wheelStep
	default:
		return nil
	}

	rows := len(m.containers)
	m.offset = min(max(m.offset, 0), max(rows-m.bodyHeight(), 0))
	m.markRows()

	return nil
}

// loadContainers asks the port for the container list of the active scope.
func (m *Model) loadContainers() tea.Cmd {
	m.containersSeq++
	m.loadingContainers = true
	m.err = nil

	return loadContainers(m.docker, m.containersSeq, m.scope)
}

// loadStats asks the port for one sample of the selection.
func (m *Model) loadStats() tea.Cmd {
	container, ok := m.selected()
	if !ok {
		return nil
	}

	m.statsSeq++
	m.loadingStats = true

	return loadStats(m.docker, m.statsSeq, container.ID)
}

// loadLogs reads the page that ends at the current cursor; Follow is never requested.
func (m *Model) loadLogs() tea.Cmd {
	m.logsSeq++
	m.loadingLogs = true
	m.logsErr = nil

	return loadLogs(m.docker, m.logsSeq, m.logsID, domain.LogOptions{Tail: logTail, Before: m.logsBefore})
}

// openLogs opens the logs of the selection with the newest page.
func (m *Model) openLogs() tea.Cmd {
	container, ok := m.selected()
	if !ok {
		return nil
	}

	m.logsOpen = true
	m.logsID = container.ID
	m.logsName = containerLabel(container)
	m.logsBefore = time.Time{}
	m.logs = domain.LogPage{}
	m.logLines = m.logLines[:0]
	m.renderLogs()
	m.viewport.GotoTop()

	return m.loadLogs()
}

// closeLogs closes the logs and forgets the page cursor.
func (m *Model) closeLogs() {
	m.logsOpen = false
	m.logsErr = nil
	m.logsBefore = time.Time{}
}

// loadOlder asks for the page that precedes the loaded one, using the cursor of the adapter.
func (m *Model) loadOlder() tea.Cmd {
	if !m.logsOpen || m.loadingLogs || m.logs.NextBefore.IsZero() {
		return nil
	}

	m.logsBefore = m.logs.NextBefore

	return m.loadLogs()
}

func (m *Model) applyContainers(msg containersMsg) {
	if msg.seq != m.containersSeq {
		return // a late answer of a refresh that is no longer relevant
	}

	m.loadingContainers = false
	m.err = msg.err
	if msg.err == nil {
		m.containers = msg.containers
	}

	m.rebuildRows()
	m.restoreAnchor()
	m.clamp()
}

// restoreAnchor puts the cursor back on the container selected before a scope change.
func (m *Model) restoreAnchor() {
	if m.anchoredID == "" {
		return
	}

	for i, container := range m.containers {
		if container.ID == m.anchoredID {
			m.cursor = i

			break
		}
	}

	m.anchoredID = ""
}

// applyStats stores one sample; a failed sample is best effort and leaves the header unchanged.
func (m *Model) applyStats(msg statsMsg) {
	if msg.seq != m.statsSeq {
		return
	}

	m.loadingStats = false
	if msg.err != nil {
		m.statsReady = false

		return
	}

	m.stats = msg.stats
	m.statsID = msg.id
	m.statsReady = true
}

// applyLogs stores a page; the older pages go on top and keep the visible line in place.
func (m *Model) applyLogs(msg logsMsg) tea.Cmd {
	if msg.seq != m.logsSeq {
		return nil
	}

	m.loadingLogs = false
	m.logsErr = msg.err
	if msg.err != nil {
		return nil
	}

	if m.logsBefore.IsZero() {
		m.logs = msg.page
		m.renderLogs()
		m.viewport.GotoTop()

		return nil
	}

	prepended := len(msg.page.Lines)

	lines := make([]domain.LogLine, 0, len(msg.page.Lines)+len(m.logs.Lines))
	lines = append(lines, msg.page.Lines...)
	lines = append(lines, m.logs.Lines...)

	m.logs.Lines = lines
	m.logs.Truncated = m.logs.Truncated || msg.page.Truncated
	m.logs.NextBefore = msg.page.NextBefore
	dropped := m.boundLogs()
	m.renderLogs()
	m.viewport.SetYOffset(m.viewport.YOffset() + prepended - dropped)

	return nil
}

// boundLogs caps the accumulated buffer and returns how many of the oldest lines it dropped.
func (m *Model) boundLogs() int {
	if len(m.logs.Lines) <= domain.MaxLogLines {
		return 0
	}

	dropped := len(m.logs.Lines) - domain.MaxLogLines
	kept := make([]domain.LogLine, domain.MaxLogLines)
	copy(kept, m.logs.Lines[dropped:])
	m.logs.Lines = kept

	return dropped
}

// rebuildRows renders the table once per load; View only composes the cached strings.
func (m *Model) rebuildRows() {
	m.rows = m.rows[:0]
	for _, container := range m.containers {
		m.rows = append(m.rows, containerRow(container))
	}
}

// renderLogs pushes the accumulated lines into the logs viewport.
func (m *Model) renderLogs() {
	m.logLines = m.logLines[:0]
	for _, line := range m.logs.Lines {
		m.logLines = append(m.logLines, logLine(line))
	}

	m.viewport.SetContent(strings.Join(m.logLines, "\n"))
}

// logLine prefixes one line with its stream and its timestamp.
func logLine(line domain.LogLine) string {
	marker := " "
	if line.Stream == domain.Stderr {
		marker = "!"
	}

	stamp := ""
	if !line.Timestamp.IsZero() {
		stamp = line.Timestamp.Format(logTimeLayout) + " "
	}

	return marker + " " + stamp + strings.ReplaceAll(line.Text, "\t", "    ")
}

// applyAction stores the outcome and refreshes the panel when the action succeeded.
func (m *Model) applyAction(msg actionMsg) tea.Cmd {
	if msg.seq != m.actionSeq {
		return nil
	}

	m.writing = false
	m.actionOp = ""

	if msg.err != nil {
		m.actionErr = msg.err

		return nil // a failed action changed nothing, so there is nothing to reload
	}

	m.actionErr = nil
	m.notice = noticeOf(msg.op)

	return m.startLoad()
}

func noticeOf(op string) string {
	switch op {
	case opStart:
		return noticeStart
	case opStop:
		return noticeStop
	case opRestart:
		return noticeRestart
	default:
		return ""
	}
}

func loadContainers(docker ports.DockerService, seq int, scope domain.ContainerScope) tea.Cmd {
	if docker == nil {
		return func() tea.Msg { return containersMsg{seq: seq, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
		defer cancel()

		containers, err := docker.Containers(ctx, scope)

		return containersMsg{seq: seq, containers: containers, err: err}
	}
}

func loadStats(docker ports.DockerService, seq int, id string) tea.Cmd {
	if docker == nil {
		return func() tea.Msg { return statsMsg{seq: seq, id: id, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), statsTimeout)
		defer cancel()

		stats, err := docker.Stats(ctx, id)

		return statsMsg{seq: seq, id: id, stats: stats, err: err}
	}
}

func loadLogs(docker ports.DockerService, seq int, id string, opts domain.LogOptions) tea.Cmd {
	if docker == nil {
		return func() tea.Msg { return logsMsg{seq: seq, id: id, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logTimeout)
		defer cancel()

		page, err := docker.Logs(ctx, id, opts)

		return logsMsg{seq: seq, id: id, page: page, err: err}
	}
}

func actionCmd(docker ports.DockerService, seq int, id string, kind confirmKind) tea.Cmd {
	op := kind.op()
	if docker == nil {
		return func() tea.Msg { return actionMsg{seq: seq, id: id, op: op, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()

		var err error
		switch kind {
		case confirmStart:
			err = docker.Start(ctx, id)
		case confirmStop:
			err = docker.Stop(ctx, id)
		case confirmRestart:
			err = docker.Restart(ctx, id)
		}

		return actionMsg{seq: seq, id: id, op: op, err: err}
	}
}
