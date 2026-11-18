package gitpanel

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
)

// Update applies one message; every read goes through a command, never inline.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	case statusLoadedMsg:
		m.applyStatus(msg)
	case branchesLoadedMsg:
		m.applyBranches(msg)
	case historyLoadedMsg:
		m.applyHistory(msg)
	case diffLoadedMsg:
		m.applyDiff(msg)
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	case tea.MouseWheelMsg:
		m.handleWheel(msg.Mouse())
	}

	return m, nil
}

// handleWheel scrolls the active section without moving its cursor.
func (m *Model) handleWheel(ev tea.Mouse) {
	switch ev.Button {
	case tea.MouseWheelUp:
		m.offsets[m.section] -= wheelStep
	case tea.MouseWheelDown:
		m.offsets[m.section] += wheelStep
	default:
		return
	}

	rows := len(m.rows(m.section))
	m.offsets[m.section] = min(max(m.offsets[m.section], 0), max(rows-m.bodyHeight(), 0))
}

// handleKey routes the keyboard of F5: sections, cursor, diff, refresh and help.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "tab":
		return m.goSection(1)
	case "shift+tab":
		return m.goSection(-1)
	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "enter":
		return m.activate()
	case "d":
		return m.toggleStaged()
	case "r":
		return m.refresh()
	case "esc":
		m.back()
	case "?":
		m.help = !m.help
	}

	return nil
}

// goSection moves the focus and loads the history the first time it is shown.
func (m *Model) goSection(delta int) tea.Cmd {
	m.section = Section((int(m.section) + delta + sectionCount) % sectionCount)
	m.clamp(m.section)

	if m.section == History && !m.historyLoaded {
		m.historyLoaded = true

		return m.reloadHistory()
	}

	return nil
}

func (m *Model) moveCursor(delta int) {
	rows := len(m.rows(m.section))
	if rows == 0 {
		return
	}

	m.cursors[m.section] = min(max(m.cursors[m.section]+delta, 0), rows-1)
	m.scrollToCursor(m.section)
}

// activate loads the diff of the selected file; F5 does not open a commit yet.
func (m *Model) activate() tea.Cmd {
	if m.section != Changes {
		return nil
	}

	file, ok := m.selectedFile()
	if !ok {
		return nil
	}

	return m.loadDiff(file.Path)
}

// toggleStaged switches between the worktree and the index, reloading the diff in place.
func (m *Model) toggleStaged() tea.Cmd {
	m.staged = !m.staged
	if !m.loaded {
		return nil
	}

	return m.loadDiff(m.request.Path)
}

// refresh reads the status and the branches again, plus whatever is already loaded.
func (m *Model) refresh() tea.Cmd {
	commands := make([]tea.Cmd, 0, 4)

	m.seqs.status++
	m.loading.status = true
	commands = append(commands, loadStatus(m.git, m.seqs.status))

	m.seqs.branches++
	m.loading.branches = true
	commands = append(commands, loadBranches(m.git, m.seqs.branches))

	if m.historyLoaded {
		commands = append(commands, m.reloadHistory())
	}
	if m.loaded {
		commands = append(commands, m.loadDiff(m.request.Path))
	}

	return tea.Batch(commands...)
}

// reloadHistory asks for the first page of history again.
func (m *Model) reloadHistory() tea.Cmd {
	m.seqs.history++
	m.loading.history = true
	m.errs.history = nil

	return loadHistory(m.git, m.seqs.history, domain.DefaultCommits)
}

// back closes the help or returns to the first section.
func (m *Model) back() {
	switch {
	case m.help:
		m.help = false
	case m.section != Changes:
		m.section = Changes
		m.clamp(m.section)
	}
}

// loadDiff asks the port for one diff and remembers the request it belongs to.
func (m *Model) loadDiff(path domain.RelPath) tea.Cmd {
	request := domain.DiffRequest{Path: path, Staged: m.staged}

	m.request = request
	m.loaded = true
	m.loading.diff = true
	m.errs.diff = nil
	m.seqs.diff++

	return loadDiff(m.git, m.seqs.diff, request)
}

func (m *Model) applyStatus(msg statusLoadedMsg) {
	if msg.seq != m.seqs.status {
		return // a late answer of a refresh that is no longer relevant
	}

	m.loading.status = false
	m.errs.status = msg.err
	if msg.err == nil {
		m.status = msg.status
	}
	m.clamp(Changes)
}

func (m *Model) applyBranches(msg branchesLoadedMsg) {
	if msg.seq != m.seqs.branches {
		return
	}

	m.loading.branches = false
	m.errs.branches = msg.err
	if msg.err == nil {
		m.branches = msg.branches
	}
	m.clamp(Branches)
}

func (m *Model) applyHistory(msg historyLoadedMsg) {
	if msg.seq != m.seqs.history {
		return
	}

	m.loading.history = false
	m.errs.history = msg.err
	if msg.err == nil {
		m.history = msg.page
	}
	m.clamp(History)
}

func (m *Model) applyDiff(msg diffLoadedMsg) {
	if msg.seq != m.seqs.diff {
		return
	}

	m.loading.diff = false
	m.errs.diff = msg.err
	m.request = msg.req
	m.diff = domain.Diff{}

	if msg.err == nil {
		m.diff = msg.diff
	}

	m.sync()
	m.clamp(Diff)
}

func loadStatus(git ports.GitReader, seq int) tea.Cmd {
	if git == nil {
		return func() tea.Msg { return statusLoadedMsg{seq: seq, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()

		status, err := git.Status(ctx)

		return statusLoadedMsg{seq: seq, status: status, err: err}
	}
}

func loadBranches(git ports.GitReader, seq int) tea.Cmd {
	if git == nil {
		return func() tea.Msg { return branchesLoadedMsg{seq: seq, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()

		branches, err := git.Branches(ctx)

		return branchesLoadedMsg{seq: seq, branches: branches, err: err}
	}
}

func loadHistory(git ports.GitReader, seq, limit int) tea.Cmd {
	if git == nil {
		return func() tea.Msg { return historyLoadedMsg{seq: seq, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()

		page, err := git.Commits(ctx, domain.CommitQuery{Limit: limit})

		return historyLoadedMsg{seq: seq, page: page, err: err}
	}
}

func loadDiff(git ports.GitReader, seq int, request domain.DiffRequest) tea.Cmd {
	if git == nil {
		return func() tea.Msg { return diffLoadedMsg{seq: seq, req: request, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()

		diff, err := git.Diff(ctx, request)

		return diffLoadedMsg{seq: seq, req: request, diff: diff, err: err}
	}
}
