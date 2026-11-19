// Package common holds the TUI pieces shared by more than one component.
package common

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

const (
	// noNewlineText is the unified-diff marker, not a translated message.
	noNewlineText = `\ No newline at end of file`
	cellSeparator = " │ "
	numberGap     = " "
)

// RenderDiff renders d as a unified diff; color=false returns plain, stable text.
func RenderDiff(d domain.Diff, width int, color bool) string {
	switch {
	case d.Binary:
		return styled(styles.MetaPlain, fit("diferencia binaria: no se muestra", width), color)
	case d.IsEmpty():
		return styled(styles.MetaPlain, fit("sin cambios", width), color)
	}

	lines := make([]string, 0, len(d.Hunks)*4+1)
	for _, hunk := range d.Hunks {
		oldWidth, newWidth := numberWidths(hunk)
		lines = append(lines, styled(styles.DiffHeader, fit(hunkTitle(hunk), width), color))

		for _, line := range hunk.Lines {
			lines = append(lines, renderLine(line, oldWidth, newWidth, width, color))
		}
	}

	if d.Truncated {
		lines = append(lines, styled(styles.MetaPlain, fit("diff truncado", width), color))
	}

	return strings.Join(lines, "\n")
}

func renderLine(line domain.DiffLine, oldWidth, newWidth, width int, color bool) string {
	prefix := numberPrefix(line, oldWidth, newWidth)
	body := lineBody(line)

	if width > 0 {
		body = truncate(body, max(width-lipgloss.Width(prefix), 0))
		prefix = truncate(prefix, width)
	}

	if !color {
		return prefix + body
	}

	return styles.Gutter.Render(prefix) + styled(lineStyle(line.Kind), body, color)
}

// numberPrefix right-aligns the old and new line numbers; zero means "no number".
func numberPrefix(line domain.DiffLine, oldWidth, newWidth int) string {
	oldNumber, newNumber := "", ""
	if line.OldLine > 0 {
		oldNumber = strconv.Itoa(line.OldLine)
	}
	if line.NewLine > 0 {
		newNumber = strconv.Itoa(line.NewLine)
	}

	return fmt.Sprintf("%*s%s%*s%s", oldWidth, oldNumber, numberGap, newWidth, newNumber, cellSeparator)
}

func lineBody(line domain.DiffLine) string {
	switch line.Kind {
	case domain.Added:
		return "+" + line.Text
	case domain.Removed:
		return "-" + line.Text
	case domain.NoNewline:
		return noNewlineText
	default:
		return numberGap + line.Text
	}
}

func lineStyle(kind domain.DiffLineKind) lipgloss.Style {
	switch kind {
	case domain.Added:
		return styles.Added
	case domain.Removed:
		return styles.Removed
	case domain.NoNewline:
		return styles.MetaPlain
	default:
		return styles.PlainCells(lipgloss.NewStyle())
	}
}

// numberWidths sizes both columns from the hunk range and the lines it carries.
func numberWidths(hunk domain.DiffHunk) (int, int) {
	oldMax, newMax := hunk.OldStart, hunk.NewStart
	for _, line := range hunk.Lines {
		oldMax = max(oldMax, line.OldLine)
		newMax = max(newMax, line.NewLine)
	}

	return len(strconv.Itoa(max(oldMax, 1))), len(strconv.Itoa(max(newMax, 1)))
}

// hunkTitle prefers the header parsed by the adapter and rebuilds it when missing.
func hunkTitle(hunk domain.DiffHunk) string {
	title := hunk.Header
	if title == "" {
		title = fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
	}
	if hunk.Section != "" && !strings.HasSuffix(title, hunk.Section) {
		title += numberGap + hunk.Section
	}

	return title
}

func styled(style lipgloss.Style, text string, color bool) string {
	if !color || text == "" {
		return text
	}

	return style.Render(text)
}

// fit cuts text to width cells; width <= 0 means "no limit".
func fit(text string, width int) string {
	if width <= 0 {
		return text
	}

	return truncate(text, width)
}

func truncate(text string, width int) string {
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}

	return string(runes[:max(width, 0)])
}
