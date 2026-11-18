package gitpanel

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	case stashesLoadedMsg:
		m.applyStashes(msg)
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
		// The modals get everything the panel does not own, like their cursor blink.
		if m.commit.open {
			return m, m.updateCommit(msg)
		}
		if m.prompt.open {
			return m, m.updatePrompt(msg)
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

// handleKey routes the keyboard of the open modal first, then the sections.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case m.confirm.open:
		return m.handleConfirmKey(msg)
	case m.prompt.open:
		return m.handlePromptKey(msg)
	case m.commit.open:
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
	case "n":
		return m.openNewBranch()
	case "x":
		return m.destructiveKey()
	case "m":
		return m.mergeKey()
	case "z":
		return m.stashKey()
	case "o":
		return m.resolveKey(domain.Ours)
	case "t":
		return m.resolveKey(domain.Theirs)
	case "A":
		return m.openAbortMerge()
	case "f":
		return m.fetchKey()
	case "p":
		return m.pullKey()
	case "P":
		return m.pushKey()
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

// goSection moves the focus and loads the sections that are read on demand.
func (m *Model) goSection(delta int) tea.Cmd {
	m.section = Section((int(m.section) + delta + sectionCount) % sectionCount)
	m.clamp(m.section)

	switch m.section {
	case Stash:
		if !m.stashesLoaded {
			m.stashesLoaded = true

			return m.reloadStashes()
		}
	case History:
		if !m.historyLoaded {
			m.historyLoaded = true

			return m.reloadHistory()
		}
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

// activate opens the diff of the selected file, checks out the selected branch, or applies the
// selected stash.
func (m *Model) activate() tea.Cmd {
	switch m.section {
	case Changes:
		file, ok := m.selectedFile()
		if !ok {
			return nil
		}

		return m.loadDiff(file.Path)
	case Branches:
		return m.checkoutKey()
	case Stash:
		return m.stashApplyKey()
	default:
		return nil
	}
}

// destructiveKey asks for confirmation before removing the selected branch or stash.
func (m *Model) destructiveKey() tea.Cmd {
	switch m.section {
	case Branches:
		return m.openDeleteBranch()
	case Stash:
		return m.openStashDrop()
	default:
		return nil
	}
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
	commands := make([]tea.Cmd, 0, 5)

	m.seqs.status++
	m.loading.status = true
	commands = append(commands, loadStatus(m.git, m.seqs.status))

	m.seqs.branches++
	m.loading.branches = true
	commands = append(commands, loadBranches(m.git, m.seqs.branches))

	if m.historyLoaded {
		commands = append(commands, m.reloadHistory())
	}
	if m.stashesLoaded {
		commands = append(commands, m.reloadStashes())
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

	commands := make([]tea.Cmd, 0, 5)

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
	if m.stashesLoaded {
		commands = append(commands, m.reloadStashes())
	}

	return tea.Batch(commands...)
}

// beginWrite marks the panel as writing and returns the command that performs it.
func (m *Model) beginWrite(op string, run func(context.Context, ports.GitService) error) tea.Cmd {
	return m.startWrite(op, func(ctx context.Context, git ports.GitService) (string, error) {
		return "", run(ctx, git)
	})
}

// beginWriteNotice runs a write that also reports what to show once it succeeds.
func (m *Model) beginWriteNotice(op string, run func(context.Context, ports.GitService) (string, error)) tea.Cmd {
	return m.startWrite(op, run)
}

// startWrite serialises the writes of the panel: one at a time, the rest are ignored.
func (m *Model) startWrite(op string, run func(context.Context, ports.GitService) (string, error)) tea.Cmd {
	if m.writing {
		return nil
	}

	m.seqs.write++
	m.writing = true
	m.writeErr = nil
	m.notice = ""

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
		m.showWriteFailure(msg.op, msg.err)

		return nil // a failed write changed nothing, so there is nothing to reload
	}

	m.notice = msg.notice
	m.closeModals()
	m.resetHunks()

	return m.refreshAfterWrite(forcesHistory(msg.op))
}

// showWriteFailure mirrors the failure in the modal that started the write.
func (m *Model) showWriteFailure(op string, err error) {
	switch {
	case op == opCommit && m.commit.open:
		m.commit.err = err
	case m.prompt.open:
		m.prompt.err = err
	case m.confirm.open:
		m.confirm.err = err
	}
}

// closeModals closes the modals a successful write finishes.
func (m *Model) closeModals() {
	if m.commit.open {
		m.commit.open = false
		m.commit.err = nil
		m.commit.area.SetValue("")
	}

	m.cancelPrompt()
	m.confirm = confirmModal{}
}

// stageFile stages the selected entry, including the pre-image of a rename.
func (m *Model) stageFile() tea.Cmd {
	paths, ok := m.selectedPaths()
	if !ok {
		return nil
	}

	return m.beginWrite(opStage, func(ctx context.Context, git ports.GitService) error {
		return git.Stage(ctx, paths)
	})
}

// unstageFile takes the selected entry out of the index.
func (m *Model) unstageFile() tea.Cmd {
	paths, ok := m.selectedPaths()
	if !ok {
		return nil
	}

	return m.beginWrite(opUnstage, func(ctx context.Context, git ports.GitService) error {
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

	return m.beginWrite(op, func(ctx context.Context, git ports.GitService) error {
		if stage {
			return git.StageHunks(ctx, diff, hunks)
		}

		return git.UnstageHunks(ctx, diff, hunks)
	})
}

// checkoutKey switches to the selected branch.
func (m *Model) checkoutKey() tea.Cmd {
	branch, ok := m.selectedBranch()
	if !ok || branch.Current {
		return nil
	}

	name := branch.Name

	return m.beginWrite(opCheckout, func(ctx context.Context, git ports.GitService) error {
		return git.Checkout(ctx, name)
	})
}

// openNewBranch asks for the name and the optional start point of a new branch.
func (m *Model) openNewBranch() tea.Cmd {
	if m.section != Branches || m.git == nil {
		return nil
	}

	return m.openPrompt(promptNewBranch)
}

// openPrompt shows the one-line prompt of one kind.
func (m *Model) openPrompt(kind promptKind) tea.Cmd {
	m.prompt = newPromptModal(kind, m.width)
	m.prompt.open = true

	return m.prompt.input.Focus()
}

// openDeleteBranch asks for confirmation before deleting the selected branch, never by force.
func (m *Model) openDeleteBranch() tea.Cmd {
	branch, ok := m.selectedBranch()
	if !ok || branch.Current {
		return nil
	}

	m.confirm = confirmModal{open: true, kind: confirmDeleteBranch, branch: branch.Name}

	return nil
}

// mergeKey merges the selected branch into the current one.
func (m *Model) mergeKey() tea.Cmd {
	if m.section != Branches {
		return nil
	}

	branch, ok := m.selectedBranch()
	if !ok || branch.Current {
		return nil
	}

	name := branch.Name

	return m.beginWriteNotice(opMerge, func(ctx context.Context, git ports.GitService) (string, error) {
		result, err := git.Merge(ctx, name)

		return mergeNotice(result), err
	})
}

// stashKey asks for the message of a new stash, or pops the selected entry.
func (m *Model) stashKey() tea.Cmd {
	switch m.section {
	case Changes:
		if m.git == nil {
			return nil
		}

		return m.openPrompt(promptStashMessage)
	case Stash:
		return m.stashPopKey()
	default:
		return nil
	}
}

// stashApplyKey restores the selected entry and keeps it in the list.
func (m *Model) stashApplyKey() tea.Cmd {
	index, ok := m.selectedStashIndex()
	if !ok {
		return nil
	}

	return m.beginWrite(opStashApply, func(ctx context.Context, git ports.GitService) error {
		return git.StashApply(ctx, index)
	})
}

// stashPopKey restores the selected entry and drops it when nothing conflicts.
func (m *Model) stashPopKey() tea.Cmd {
	index, ok := m.selectedStashIndex()
	if !ok {
		return nil
	}

	return m.beginWrite(opStashPop, func(ctx context.Context, git ports.GitService) error {
		return git.StashPop(ctx, index)
	})
}

// openStashDrop asks for confirmation before dropping the selected entry.
func (m *Model) openStashDrop() tea.Cmd {
	index, ok := m.selectedStashIndex()
	if !ok {
		return nil
	}

	m.confirm = confirmModal{open: true, kind: confirmStashDrop, index: index}

	return nil
}

// resolveKey stages one side of the selected conflicted file; nothing is resolved automatically.
func (m *Model) resolveKey(resolution domain.Resolution) tea.Cmd {
	if m.section != Changes {
		return nil
	}

	file, ok := m.selectedFile()
	if !ok || !file.Conflicted || file.Path.IsZero() {
		return nil
	}

	path := file.Path

	return m.beginWrite(opResolve, func(ctx context.Context, git ports.GitService) error {
		return git.ResolveConflict(ctx, path, resolution)
	})
}

// openAbortMerge asks for confirmation before undoing the merge in progress.
func (m *Model) openAbortMerge() tea.Cmd {
	if len(m.status.Conflicts()) == 0 {
		return nil
	}

	m.confirm = confirmModal{open: true, kind: confirmAbortMerge}

	return nil
}

// fetchKey downloads the remote refs.
func (m *Model) fetchKey() tea.Cmd {
	return m.beginWriteNotice(opFetch, func(ctx context.Context, git ports.GitService) (string, error) {
		return noticeFetch, git.Fetch(ctx)
	})
}

// pullKey fuses the upstream into the current branch.
func (m *Model) pullKey() tea.Cmd {
	return m.beginWriteNotice(opPull, func(ctx context.Context, git ports.GitService) (string, error) {
		result, err := git.Pull(ctx)

		return mergeNotice(result), err
	})
}

// pushKey publishes the current branch to its upstream.
func (m *Model) pushKey() tea.Cmd {
	return m.beginWriteNotice(opPush, func(ctx context.Context, git ports.GitService) (string, error) {
		return noticePush, git.Push(ctx)
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

	return m.beginWrite(opCommit, func(ctx context.Context, git ports.GitService) error {
		return git.Commit(ctx, message)
	})
}

// handlePromptKey drives the one-line prompt: enter confirms and esc cancels.
func (m *Model) handlePromptKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		return m.submitPrompt()
	case "esc":
		m.cancelPrompt()

		return nil
	}

	return m.updatePrompt(msg)
}

// updatePrompt forwards one message to the text input.
func (m *Model) updatePrompt(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd

	m.prompt.input, cmd = m.prompt.input.Update(msg)

	return cmd
}

// cancelPrompt closes the prompt without an answer.
func (m *Model) cancelPrompt() {
	m.prompt.open = false
	m.prompt.err = nil
	m.prompt.input.SetValue("")
}

// submitPrompt sends the value of the prompt; an unusable value never reaches git.
func (m *Model) submitPrompt() tea.Cmd {
	if m.writing {
		return nil
	}

	value := strings.TrimSpace(m.prompt.input.Value())

	switch m.prompt.kind {
	case promptNewBranch:
		return m.submitNewBranch(value)
	case promptStashMessage:
		return m.submitStashPush(value)
	default:
		return nil
	}
}

// submitNewBranch creates the branch of the prompt at its optional start point.
func (m *Model) submitNewBranch(value string) tea.Cmd {
	name, startPoint := splitBranch(value)
	if name == "" {
		m.prompt.err = fmt.Errorf("%w: el nombre de la rama está vacío", domain.ErrInvalidArgument)

		return nil
	}

	m.prompt.err = nil

	return m.beginWrite(opCreateBranch, func(ctx context.Context, git ports.GitService) error {
		return git.CreateBranch(ctx, name, startPoint)
	})
}

// splitBranch reads `nombre [punto de partida]` out of one prompt value.
func splitBranch(value string) (string, string) {
	fields := strings.Fields(value)
	switch len(fields) {
	case 0:
		return "", ""
	case 1:
		return fields[0], ""
	default:
		return fields[0], fields[1]
	}
}

// submitStashPush saves the tracked changes with the optional message of the prompt.
func (m *Model) submitStashPush(message string) tea.Cmd {
	m.prompt.err = nil

	return m.beginWrite(opStashPush, func(ctx context.Context, git ports.GitService) error {
		return git.StashPush(ctx, message)
	})
}

// handleConfirmKey resolves the confirmation: y or enter accepts, n or esc cancels.
func (m *Model) handleConfirmKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "y", "enter":
		return m.submitConfirm()
	case "n", "esc":
		m.confirm = confirmModal{}

		return nil
	}

	return nil
}

// submitConfirm starts the confirmed operation.
func (m *Model) submitConfirm() tea.Cmd {
	if m.writing {
		return nil
	}

	confirm := m.confirm

	switch confirm.kind {
	case confirmDeleteBranch:
		return m.beginWrite(opDeleteBranch, func(ctx context.Context, git ports.GitService) error {
			return git.DeleteBranch(ctx, confirm.branch, false)
		})
	case confirmStashDrop:
		return m.beginWrite(opStashDrop, func(ctx context.Context, git ports.GitService) error {
			return git.StashDrop(ctx, confirm.index)
		})
	case confirmAbortMerge:
		return m.beginWrite(opAbortMerge, func(ctx context.Context, git ports.GitService) error {
			return git.AbortMerge(ctx)
		})
	default:
		return nil
	}
}

// reloadHistory asks for the first page of history again.
func (m *Model) reloadHistory() tea.Cmd {
	m.seqs.history++
	m.loading.history = true
	m.errs.history = nil

	return loadHistory(m.git, m.seqs.history, domain.DefaultCommits)
}

// reloadStashes asks for the stash list again.
func (m *Model) reloadStashes() tea.Cmd {
	m.seqs.stashes++
	m.loading.stashes = true
	m.errs.stashes = nil

	return loadStashes(m.git, m.seqs.stashes)
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

func (m *Model) applyStashes(msg stashesLoadedMsg) {
	if msg.seq != m.seqs.stashes {
		return
	}

	m.loading.stashes = false
	m.errs.stashes = msg.err
	if msg.err == nil {
		m.stashes = msg.stashes
	}
	m.clamp(Stash)
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

// writeTimeoutFor bounds one write; the operations that talk to a remote get a longer budget.
func writeTimeoutFor(op string) time.Duration {
	switch op {
	case opFetch, opPull, opPush:
		return networkTimeout
	default:
		return writeTimeout
	}
}

// writeCmd runs one write with its own timeout and reports the outcome to the panel.
func writeCmd(git ports.GitService, seq int, op string, run func(context.Context, ports.GitService) (string, error)) tea.Cmd {
	if git == nil {
		return func() tea.Msg { return writeDoneMsg{seq: seq, op: op, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeoutFor(op))
		defer cancel()

		notice, err := run(ctx, git)

		return writeDoneMsg{seq: seq, op: op, notice: notice, err: err}
	}
}

func loadStatus(git ports.GitService, seq int) tea.Cmd {
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

func loadBranches(git ports.GitService, seq int) tea.Cmd {
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

func loadStashes(git ports.GitService, seq int) tea.Cmd {
	if git == nil {
		return func() tea.Msg { return stashesLoadedMsg{seq: seq, err: errNoService} }
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()

		stashes, err := git.Stashes(ctx)

		return stashesLoadedMsg{seq: seq, stashes: stashes, err: err}
	}
}

func loadHistory(git ports.GitService, seq, limit int) tea.Cmd {
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

func loadDiff(git ports.GitService, seq int, request domain.DiffRequest) tea.Cmd {
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
