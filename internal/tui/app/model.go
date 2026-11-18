// Package app holds the root Bubble Tea model of the TUI.
package app

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/config"
)

const (
	defaultWidth  = 80
	defaultHeight = 24

	minWidth  = 40
	minHeight = 10

	editorChrome = 4
	minEditor    = 3
)

// Model is the root model: it owns the session state and the layout.
type Model struct {
	cfg    config.Config
	width  int
	height int
}

var styles = struct {
	title  lipgloss.Style
	editor lipgloss.Style
	status lipgloss.Style
}{
	title:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7")),
	editor: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#565f89")),
	status: lipgloss.NewStyle().Foreground(lipgloss.Color("#9aa5ce")),
}

// New builds the root model for the given configuration.
func New(cfg config.Config) Model {
	return Model{cfg: cfg, width: defaultWidth, height: defaultHeight}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) View() tea.View {
	width := max(m.width, minWidth)
	height := max(m.height, minHeight)
	editorHeight := max(height-editorChrome, minEditor)

	titleText := styles.title.Render(" tui-editor ")
	editorPane := styles.editor.Width(width - 4).Height(editorHeight).Render("área de edición")
	statusBar := styles.status.Render("repo: " + m.cfg.Repo + "  •  q: salir  •  ctrl+c: salir")

	content := lipgloss.JoinVertical(lipgloss.Left, titleText, editorPane, statusBar)

	v := tea.NewView(content)
	v.AltScreen = true

	return v
}
