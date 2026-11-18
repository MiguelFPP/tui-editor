package gitpanel

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
)

// Update applies one message; every read and every write goes through a command, never inline.
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
	case writeDoneMsg:
		return m, m.applyWriteDone(msg)
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	case tea.MouseWheelMsg:
		m.handleWheel(msg.Mouse())
	default:
		// The commit editor gets everything the panel does not own, like its cursor blink.
		if m.commit.open {
			return m, m.updateCommit(msg)
		}
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

// handleKey routes the keyboard of F5 first, plus the staging keys of F7.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.commit.open {
		return m.handleCommitKey(msg)
	}

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
	case "s":
		return m.stageKey()
	case "u":
		return m.unstageKey()
	case "c":
		return m.openCommit()
	case "[":
		m.moveHunk(-1)
	case "]":
		m.moveHunk(1)
	case " ", "space":
		m.toggleHunk()
	case "r":
		return m.refresh()
	case "esc":
		m.back()
	case "?":
		m.help = !m.help
	}

	return nil
}

// stageKey stages the selected file, or the marked hunks of the worktree diff.
func (m *Model) stageKey() tea.Cmd {
	if m.section == Diff {
		return m.stageHunks()
	}

	return m.stageFile()
}

// unstageKey unstages the selected file, or the marked hunks of the staged diff.
func (m *Model) unstageKey() tea.Cmd {
	if m.section == Diff {
		return m.unstageHunks()
	}

	return m.unstageFile()
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

// refreshAfterWrite reloads everything a successful write may have changed.
func (m *Model) refreshAfterWrite(forceHistory bool) tea.Cmd {
	if m.git == nil {
		return nil
	}

	commands := make([]tea.Cmd, 0, 4)

	m.seqs.status++
	m.loading.status = true
	m.errs.status = nil
	commands = append(commands, loadStatus(m.git, m.seqs.status))

	m.seqs.branches++
	m.loading.branches = true
	m.errs.branches = nil
	commands = append(commands, loadBranches(m.git, m.seqs.branches))

	if m.loaded {
		commands = append(commands, m.loadDiff(m.request.Path))
	}
	if forceHistory || m.historyLoaded {
		m.historyLoaded = true
		commands = append(commands, m.reloadHistory())
	}

	return tea.Batch(commands...)
}

// beginWrite marks the panel as writing and returns the command that performs it.
func (m *Model) beginWrite(op string, run func(context.Context, ports.GitChanges) error) tea.Cmd {
	m.seqs.write++
	m.writing = true
	m.writeErr = nil

	return writeCmd(m.git, m.seqs.write, op, run)
}

// applyWriteDone stores the outcome and, when it succeeded, refreshes the panel.
func (m *Model) applyWriteDone(msg writeDoneMsg) tea.Cmd {
	if msg.seq != m.seqs.write {
		return nil
	}

	m.writing = false
	m.writeErr = msg.err
	if msg.err != nil {
		if msg.op == opCommit {
			m.commit.err = msg.err
		}

		return nil // a failed write changed nothing, so there is nothing to reload
	}

	if msg.op == opCommit {
		m.commit.err = nil
		m.commit.open = false
		m.commit.area.SetValue("")
	}

	m.resetHunks()

	return m.refreshAfterWrite(msg.op == opCommit)
}

// stageFile stages the selected entry, including the pre-image of a rename.
func (m *Model) stageFile() tea.Cmd {
	paths, ok := m.selectedPaths()
	if !ok {
		return nil
	}

	return m.beginWrite(opStage, func(ctx context.Context, git ports.GitChanges) error {
		return git.Stage(ctx, paths)
	})
}

// unstageFile takes the selected entry out of the index.
func (m *Model) unstageFile() tea.Cmd {
	paths, ok := m.selectedPaths()
	if !ok {
		return nil
	}

	return m.beginWrite(opUnstage, func(ctx context.Context, git ports.GitChanges) error {
		return git.Unstage(ctx, paths)
	})
}

// selectedPaths lists the paths of the entry under the cursor of the changes section.
func (m Model) selectedPaths() ([]domain.RelPath, bool) {
	if m.git == nil {
		return nil, false
	}

	file, ok := m.selectedFile()
	if !ok || file.Path.IsZero() {
		return nil, false
	}

	paths := []domain.RelPath{file.Path}
	if !file.OrigPath.IsZero() {
		paths = append(paths, file.OrigPath)
	}

	return paths, true
}

// stageHunks stages the marked hunks of the worktree diff.
func (m *Model) stageHunks() tea.Cmd {
	if m.staged {
		return nil
	}

	return m.applyHunks(true)
}

// unstageHunks takes the marked hunks of the staged diff out of the index.
func (m *Model) unstageHunks() tea.Cmd {
	if !m.staged {
		return nil
	}

	return m.applyHunks(false)
}

// applyHunks sends the marked hunks of the loaded diff through the port.
func (m *Model) applyHunks(stage bool) tea.Cmd {
	if m.git == nil || !m.loaded {
		return nil
	}

	hunks := m.selectedHunks()
	if len(hunks) == 0 {
		return nil
	}

	op, diff := opUnstageHunks, m.diff
	if stage {
		op = opStageHunks
	}

	return m.beginWrite(op, func(ctx context.Context, git ports.GitChanges) error {
		if stage {
			return git.StageHunks(ctx, diff, hunks)
		}

		return git.UnstageHunks(ctx, diff, hunks)
	})
}

// openCommit shows the commit modal, keeping the draft of a previous escape.
func (m *Model) openCommit() tea.Cmd {
	if m.git == nil {
		return nil
	}

	m.commit.open = true
	m.commit.err = nil
	m.commit.area.SetWidth(modalWidth(m.width))
	m.commit.area.SetHeight(modalRows(m.height))

	return m.commit.area.Focus()
}

// handleCommitKey drives the modal: the textarea edits, ctrl+s commits and esc cancels.
func (m *Model) handleCommitKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+s":
		return m.submitCommit()
	case "esc":
		m.commit.open = false
		m.commit.err = nil

		return nil
	}

	return m.updateCommit(msg)
}

// updateCommit forwards one message to the commit editor.
func (m *Model) updateCommit(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd

	m.commit.area, cmd = m.commit.area.Update(msg)

	return cmd
}

// submitCommit sends the draft; an empty message never reaches git.
func (m *Model) submitCommit() tea.Cmd {
	if m.writing {
		return nil
	}

	message := m.commit.area.Value()
	if strings.TrimSpace(message) == "" {
		m.commit.err = fmt.Errorf("%w: el mensaje del commit está vacío", domain.ErrInvalidArgument)

		return nil
	}

	m.commit.err = nil

	return m.beginWrite(opCommit, func(ctx context.Context, git ports.GitChanges) error {
		return git.Commit(ctx, message)
	})
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
	m.resetHunks()

	if msg.err == nil {
		m.diff = msg.diff
	}

	m.sync()
	m.clamp(Diff)
}

// writeCmd runs one write with its own timeout and reports the outcome to the panel.
func writeCmd(git ports.GitChanges, seq int, op string, run func(context.Context, ports.GitChanges) error) tea.Cmd {
	if git == nil {
		return func() tea.Msg { return writeDoneMsg{seq: seq, op: op, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		defer cancel()

		return writeDoneMsg{seq: seq, op: op, err: run(ctx, git)}
	}
}

func loadStatus(git ports.GitChanges, seq int) tea.Cmd {
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

func loadBranches(git ports.GitChanges, seq int) tea.Cmd {
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

func loadHistory(git ports.GitChanges, seq, limit int) tea.Cmd {
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

func loadDiff(git ports.GitChanges, seq int, request domain.DiffRequest) tea.Cmd {
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
