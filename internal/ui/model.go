package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type Model struct {
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

func New() Model {
	return Model{width: 80, height: 24}
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
	width := max(m.width, 40)
	height := max(m.height, 10)
	editorHeight := max(height-4, 3)

	titleText := styles.title.Render(" tui-fer-editor ")
	editorPane := styles.editor.Width(width - 4).Height(editorHeight).Render("editor area")
	statusBar := styles.status.Render("q: quit  •  ctrl+c: quit")

	content := lipgloss.JoinVertical(lipgloss.Left, titleText, editorPane, statusBar)

	v := tea.NewView(content)
	v.AltScreen = true

	return v
}
