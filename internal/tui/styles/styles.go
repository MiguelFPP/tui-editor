// Package styles holds the Tokyo Night palette and the style tokens shared by the TUI.
package styles

import "charm.land/lipgloss/v2"

// Tokyo Night palette: the only colour literals of the TUI live here.
const (
	ColorAccent = "#7aa2f7" // headings, active tabs and directories
	ColorMuted  = "#9aa5ce" // statusbar and secondary text
	ColorBorder = "#565f89" // frames
	ColorGreen  = "#9ece6a" // added lines and keys
	ColorRed    = "#f7768e" // failures and removed lines
)

// PlainCells keeps tabs untouched, so the rendered rows match the parsed text.
func PlainCells(style lipgloss.Style) lipgloss.Style {
	return style.TabWidth(lipgloss.NoTabConversion)
}

// Border frames an overlay.
var Border = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(ColorBorder))

// Help is the keymap overlay: the neutral frame plus its inner padding.
var Help = Border.Padding(0, 2)

// Tokens of text that never carries tabs: the layout chrome, the tree and the state lines.
var (
	Title        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorAccent))
	Tab          = lipgloss.NewStyle().Faint(true)
	TabActive    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorAccent))
	TabSeparator = lipgloss.NewStyle().Faint(true)
	Status       = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorMuted))
	HelpTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorAccent))
	HelpKey      = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorGreen))
	Placeholder  = lipgloss.NewStyle().Faint(true)
	Spinner      = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent))

	Dir    = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent))
	Cursor = lipgloss.NewStyle().Bold(true).Reverse(true)
	Meta   = lipgloss.NewStyle().Faint(true)
	Fail   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorRed))
)

// The Plain tokens keep the tabs of the rows they render: files, diffs and logs.
var (
	DiffHeader     = PlainCells(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorAccent)))
	TitlePlain     = PlainCells(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorAccent)))
	TabPlain       = PlainCells(lipgloss.NewStyle().Faint(true))
	TabActivePlain = PlainCells(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorAccent)))
	CursorPlain    = PlainCells(lipgloss.NewStyle().Bold(true).Reverse(true))
	MetaPlain      = PlainCells(lipgloss.NewStyle().Faint(true))
	Gutter         = PlainCells(lipgloss.NewStyle().Faint(true))
	Header         = PlainCells(lipgloss.NewStyle().Faint(true))
	Added          = PlainCells(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorGreen)))
	Removed        = PlainCells(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorRed)))
	FailPlain      = PlainCells(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorRed)))
)
