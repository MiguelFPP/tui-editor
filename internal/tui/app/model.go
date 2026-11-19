// Package app holds the root Bubble Tea model of the TUI.
package app

import (
	"image"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/config"
	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/MiguelFPP/tui-editor/internal/tui/components/dockerpanel"
	"github.com/MiguelFPP/tui-editor/internal/tui/components/editor"
	"github.com/MiguelFPP/tui-editor/internal/tui/components/filetree"
	"github.com/MiguelFPP/tui-editor/internal/tui/components/gitpanel"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

const (
	defaultWidth  = 80
	defaultHeight = 24

	minWidth  = 40
	minHeight = 10

	headerRows = 2
	footerRows = 1

	minTree      = 24
	treePercent  = 35
	minEditorBox = 20

	toastTTL = 3 * time.Second

	// dirtyDraft is the notice shown when an action would drop an unsaved draft.
	dirtyDraft = "cambios sin guardar en el editor"
)

// Tab is one top-level section of the TUI.
type Tab uint8

const (
	FilesTab Tab = iota
	GitTab
	DockerTab

	tabCount = 3
)

// tabLabels are the rendered labels, shared by the tab row and its hit-testing.
var tabLabels = [tabCount]string{FilesTab: " Files ", GitTab: " Git ", DockerTab: " Docker "}

func (t Tab) String() string {
	if t >= tabCount {
		return strings.TrimSpace(tabLabels[FilesTab])
	}

	return strings.TrimSpace(tabLabels[t])
}

// Focus is the panel that receives the keyboard of the active tab.
type Focus uint8

const (
	FocusTree Focus = iota
	FocusEditor
	FocusGit
	FocusDocker
)

// Deps are the collaborators injected by the composition root. Keys carries the
// user keymap; an empty registry falls back to the built-in defaults.
type Deps struct {
	Config config.Config
	Files  ports.FileService
	Git    ports.GitService
	Editor ports.EditorService
	Docker ports.DockerService
	Keys   keymap.Registry
}

// Model is the root model: it owns the tabs, the layout, the focus and the toasts.
type Model struct {
	cfg     config.Config
	files   ports.FileService
	editors ports.EditorService
	keys    keymap.Registry

	tree      filetree.Model
	editor    editor.Model
	hasEditor bool
	git       gitpanel.Model
	docker    dockerpanel.Model

	tab   Tab
	focus Focus

	width  int
	height int

	tabRects   [tabCount]image.Rectangle
	treeRect   image.Rectangle
	editorRect image.Rectangle
	gitRect    image.Rectangle
	dockerRect image.Rectangle

	help       bool
	configOpen bool

	spinner  spinner.Model
	spinning bool

	toast    string
	toastSeq int
	lastErr  error
}

// toastExpiredMsg clears the toast unless a newer one has taken its place.
type toastExpiredMsg struct {
	seq int
}

// New builds the root model for the given dependencies.
func New(deps Deps) Model {
	keys := deps.Keys
	if len(keys.All()) == 0 {
		keys = keymap.Default()
	}

	m := Model{
		cfg:      deps.Config,
		files:    deps.Files,
		editors:  deps.Editor,
		keys:     keys,
		tree:     filetree.New(deps.Files, "", keys),
		git:      gitpanel.New(deps.Git, keys),
		docker:   dockerpanel.New(deps.Docker, deps.Config.Repo, keys),
		tab:      FilesTab,
		focus:    FocusTree,
		width:    defaultWidth,
		height:   defaultHeight,
		spinner:  spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(styles.Spinner)),
		spinning: true,
	}
	m.resize()

	return m
}

// Init starts the tree, the Git panel and the spinner.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.tree.Init(), m.git.Init(), m.spinner.Tick)
}

// Update routes one message: keys and mouse to the focused panel, the rest by fan-out.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, 4)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case tea.KeyPressMsg:
		cmd, stop := m.handleKey(msg)
		if stop {
			return m, cmd
		}

		cmds = append(cmds, cmd)
	case tea.MouseClickMsg:
		cmds = append(cmds, m.handleClick(msg))
	case tea.MouseWheelMsg:
		cmds = append(cmds, m.handleWheel(msg))
	case spinner.TickMsg:
		var tick tea.Cmd
		m.spinner, tick = m.spinner.Update(msg)
		m.spinning = false
		if m.loading() {
			m.spinning = true
			cmds = append(cmds, tick)
		}
	case toastExpiredMsg:
		if msg.seq == m.toastSeq {
			m.toast = ""
		}
	case filetree.FileChosenMsg:
		if m.editorDirty() {
			cmds = append(cmds, m.notify(dirtyDraft))
		} else {
			cmds = append(cmds, m.openFile(msg.Path))
		}
	case editor.FileChangedMsg:
		// The editor already reloaded the file: the panel only has to catch up with Git.
		cmds = append(cmds, m.git.Refresh())
	default:
		cmds = append(cmds, m.fanOut(msg))
	}

	cmds = append(cmds, m.afterUpdate())

	return m, tea.Batch(cmds...)
}

// View renders the layout; the alternate screen and the mouse are set here.
func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion

	return view
}

// Tab returns the active tab.
func (m Model) Tab() Tab {
	return m.tab
}

// Focus returns the panel that owns the keyboard.
func (m Model) Focus() Focus {
	return m.focus
}

// HelpVisible reports whether the help overlay is open.
func (m Model) HelpVisible() bool {
	return m.help
}

// ConfigVisible reports whether the read-only configuration overlay is open.
func (m Model) ConfigVisible() bool {
	return m.configOpen
}

// fanOut delivers an async message to every child; each one ignores what it does not own.
func (m *Model) fanOut(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, 0, 4)

	var cmd tea.Cmd
	m.tree, cmd = m.tree.Update(msg)
	cmds = append(cmds, cmd)

	if m.hasEditor {
		m.editor, cmd = m.editor.Update(msg)
		cmds = append(cmds, cmd)
	}

	m.git, cmd = m.git.Update(msg)
	cmds = append(cmds, cmd)

	m.docker, cmd = m.docker.Update(msg)
	cmds = append(cmds, cmd)

	return tea.Batch(cmds...)
}

// openFile replaces the preview with path, focuses it and loads it.
func (m *Model) openFile(path domain.RelPath) tea.Cmd {
	m.editor = editor.New(m.files, m.editors, path, m.keys)
	m.hasEditor = true
	m.tab = FilesTab
	m.focus = FocusEditor
	m.resize()

	return m.editor.Init()
}

// handleKey applies the root keymap; stop means the returned command quits the program.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	key := msg.String()

	// The help overlay only answers to quit, to its own toggle and to esc.
	if m.help {
		action, ok := m.keys.Match(keymap.ScopeRoot, key)
		switch {
		case ok && action == keymap.ActionQuit:
			return m.quit()
		case ok && action == keymap.ActionHelp, key == "esc":
			m.help = false
		}

		return nil, false
	}

	// The configuration overlay only answers to quit, to its own toggle and to esc.
	if m.configOpen {
		action, ok := m.keys.Match(keymap.ScopeRoot, key)
		switch {
		case ok && action == keymap.ActionQuit:
			return m.quit()
		case ok && action == keymap.ActionConfig, key == "esc":
			m.configOpen = false
		}

		return nil, false
	}

	// The draft of the Files tab owns every key but ctrl+c, which never drops unsaved changes.
	if m.tab == FilesTab && m.hasEditor && m.editor.CapturesInput() {
		if key != "ctrl+c" {
			return m.forwardKey(msg), false
		}

		return m.quit()
	}

	// A modal inside the Git panel owns every key but the quit shortcut.
	if m.tab == GitTab && m.git.CapturesInput() {
		if key == "ctrl+c" {
			return m.quit()
		}

		return m.forwardKey(msg), false
	}

	// A confirmation or the logs of the Docker panel own every key but the quit shortcut.
	if m.tab == DockerTab && m.docker.CapturesInput() {
		if key == "ctrl+c" {
			return m.quit()
		}

		return m.forwardKey(msg), false
	}

	action, ok := m.keys.Match(keymap.ScopeRoot, key)
	if !ok {
		return m.forwardKey(msg), false
	}

	switch action {
	case keymap.ActionQuit:
		return m.quit()
	case keymap.ActionHelp:
		m.help = true

		return nil, false
	case keymap.ActionConfig:
		m.configOpen = true

		return nil, false
	case keymap.ActionTabFiles:
		return m.setTab(FilesTab), false
	case keymap.ActionTabGit:
		return m.setTab(GitTab), false
	case keymap.ActionTabDocker:
		return m.setTab(DockerTab), false
	case keymap.ActionFocusNext, keymap.ActionFocusPrev:
		// Outside the Files tab the key belongs to the panel, which uses it for its own sections.
		if m.tab == FilesTab {
			m.toggleFocus()

			return nil, false
		}
	}

	return m.forwardKey(msg), false
}

// forwardKey sends a key to the panel that owns the focus.
func (m *Model) forwardKey(msg tea.KeyPressMsg) tea.Cmd {
	switch m.focus {
	case FocusTree:
		next, cmd := m.tree.Update(msg)
		m.tree = next

		return cmd
	case FocusEditor:
		if !m.hasEditor {
			return nil
		}

		next, cmd := m.editor.Update(msg)
		m.editor = next

		return cmd
	case FocusGit:
		next, cmd := m.git.Update(msg)
		m.git = next

		return cmd
	default:
		next, cmd := m.docker.Update(msg)
		m.docker = next

		return cmd
	}
}

// handleClick routes a click to the top tabs first and then to the panel under the cursor.
func (m *Model) handleClick(msg tea.MouseClickMsg) tea.Cmd {
	if m.help || m.configOpen {
		return nil
	}

	if tab, ok := m.tabAt(msg.X, msg.Y); ok {
		return m.setTab(tab) // an active tab is a no-op: it never steals the focus
	}

	panel, ok := m.panelAt(msg.X, msg.Y)
	if !ok {
		return nil
	}
	m.focus = panel

	return m.dispatchMouse(msg, panel)
}

// handleWheel sends the wheel to the panel under the cursor, not to the focused one.
func (m *Model) handleWheel(msg tea.MouseWheelMsg) tea.Cmd {
	if m.help || m.configOpen {
		return nil
	}

	panel, ok := m.panelAt(msg.X, msg.Y)
	if !ok {
		return nil
	}
	m.focus = panel

	return m.dispatchMouse(msg, panel)
}

// tabAt maps terminal coordinates to the top tab that owns them.
func (m Model) tabAt(x, y int) (Tab, bool) {
	point := image.Pt(x, y)

	for tab := Tab(0); tab < tabCount; tab++ {
		if m.tabRects[tab].Dx() > 0 && point.In(m.tabRects[tab]) {
			return tab, true
		}
	}

	return m.tab, false
}

// panelAt maps terminal coordinates to the panel that owns them.
func (m Model) panelAt(x, y int) (Focus, bool) {
	point := image.Pt(x, y)

	switch m.tab {
	case GitTab:
		return FocusGit, point.In(m.gitRect)
	case DockerTab:
		return FocusDocker, point.In(m.dockerRect)
	}

	switch {
	case point.In(m.treeRect):
		return FocusTree, true
	case point.In(m.editorRect):
		return FocusEditor, m.hasEditor
	default:
		return m.focus, false
	}
}

// editorMouse rebases a terminal mouse event onto the editor rect; the wheel carries no coordinates.
func (m Model) editorMouse(msg tea.Msg) tea.Msg {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		msg.X -= m.editorRect.Min.X
		msg.Y -= m.editorRect.Min.Y

		return msg
	default:
		return msg
	}
}

func (m *Model) dispatchMouse(msg tea.Msg, panel Focus) tea.Cmd {
	switch panel {
	case FocusTree:
		next, cmd := m.tree.Update(msg)
		m.tree = next

		return cmd
	case FocusEditor:
		if !m.hasEditor {
			return nil
		}

		next, cmd := m.editor.Update(m.editorMouse(msg))
		m.editor = next

		return cmd
	case FocusGit:
		next, cmd := m.git.Update(msg)
		m.git = next

		return cmd
	default:
		next, cmd := m.docker.Update(msg)
		m.docker = next

		return cmd
	}
}

// setTab switches the active tab and hands the focus to its main panel; leaving the Docker tab
// stops its ticker.
func (m *Model) setTab(tab Tab) tea.Cmd {
	if tab == m.tab {
		return nil
	}

	var cmd tea.Cmd
	if m.tab == DockerTab {
		m.docker.SetActive(false)
	}

	m.tab = tab
	switch tab {
	case GitTab:
		m.focus = FocusGit
	case DockerTab:
		m.focus = FocusDocker
		cmd = m.docker.SetActive(true)
	default:
		m.focus = FocusTree
	}

	return cmd
}

// toggleFocus alternates between the tree and the editor inside the Files tab.
func (m *Model) toggleFocus() {
	if !m.hasEditor || m.focus == FocusEditor {
		m.focus = FocusTree

		return
	}

	m.focus = FocusEditor
}

// editorDirty reports whether the open file holds a draft that is not on disk yet.
func (m Model) editorDirty() bool {
	return m.hasEditor && m.editor.Dirty()
}

// quit leaves the program, unless a draft would be lost.
func (m *Model) quit() (tea.Cmd, bool) {
	if m.editorDirty() {
		return m.notify(dirtyDraft), false
	}

	return tea.Quit, true
}

// notify shows a toast that does not come from a panel error.
func (m *Model) notify(text string) tea.Cmd {
	m.toast = toastPrefix + text
	m.toastSeq++
	seq := m.toastSeq

	return tea.Tick(toastTTL, func(time.Time) tea.Msg { return toastExpiredMsg{seq: seq} })
}

// afterUpdate reconciles the toast and keeps exactly one spinner tick in flight.
func (m *Model) afterUpdate() tea.Cmd {
	cmds := make([]tea.Cmd, 0, 2)

	if cmd := m.syncToast(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if m.loading() && !m.spinning {
		m.spinning = true
		cmds = append(cmds, m.spinner.Tick)
	}

	return tea.Batch(cmds...)
}

// syncToast shows a toast when the first error changes, and schedules its expiry.
func (m *Model) syncToast() tea.Cmd {
	err := m.currentError()
	if sameError(err, m.lastErr) {
		return nil
	}

	m.lastErr = err
	if err == nil {
		return nil
	}

	m.toast = toastPrefix + err.Error()
	m.toastSeq++
	seq := m.toastSeq

	return tea.Tick(toastTTL, func(time.Time) tea.Msg { return toastExpiredMsg{seq: seq} })
}

// currentError returns the first active error of the panels.
func (m Model) currentError() error {
	if err := m.tree.Err(); err != nil {
		return err
	}
	if m.hasEditor {
		if err := m.editor.Err(); err != nil {
			return err
		}
	}
	if err := m.git.Err(); err != nil {
		return err
	}

	return m.docker.Err()
}

// loading reports whether any panel is still reading.
func (m Model) loading() bool {
	if m.tree.Loading() || m.git.Loading() || m.docker.Loading() {
		return true
	}

	return m.hasEditor && m.editor.Loading()
}

func sameError(a, b error) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Error() == b.Error()
	}
}

func (m Model) repoLabel() string {
	if strings.TrimSpace(m.cfg.Repo) == "" {
		return "."
	}

	return m.cfg.Repo
}

func (m Model) focusLabel() string {
	switch m.focus {
	case FocusEditor:
		return "editor"
	case FocusGit:
		return "git"
	case FocusDocker:
		return "docker"
	default:
		return "árbol"
	}
}
