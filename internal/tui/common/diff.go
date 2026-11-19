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

// RenderDiffSplit renders d side by side: the old side on the left and the new one on the right,
// with the hunk titles and the change markers of each side. Each column is highlighted on its own,
// so the lexer sees the old lines together and the new ones together.
func RenderDiffSplit(d domain.Diff, width int, color bool) string {
	switch {
	case d.Binary:
		return RenderDiff(d, width, color)
	case d.IsEmpty():
		return RenderDiff(d, width, color)
	}

	aligned := domain.AlignDiff(d)
	lines := make([]string, 0, len(aligned)*8)

	for _, hunk := range aligned {
		title := styled(styles.DiffHeader, fit(hunkTitle(hunk.Hunk), width), color)
		lines = append(lines, title)

		oldWidth, newWidth := splitNumberWidths(hunk.Rows)
		sideWidth := splitSideWidth(width)
		oldText, newText := highlightSides(d.Path, hunk.Rows, splitSideTextWidth(sideWidth, oldWidth, newWidth), color)

		for index, row := range hunk.Rows {
			lines = append(lines,
				splitCell(row.Old, oldText[index], oldWidth, sideWidth, color)+splitSeparator+
					splitCell(row.New, newText[index], newWidth, sideWidth, color))
		}
	}

	if d.Truncated {
		lines = append(lines, styled(styles.MetaPlain, fit("diff truncado", width), color))
	}

	return strings.Join(lines, "\n")
}

// highlightSides colours each column of a hunk in its own pass; an unusable result falls back to the
// plain texts. The text is clipped first, so the escapes are never cut in half.
func highlightSides(path domain.RelPath, rows []domain.SplitRow, width int, color bool) ([]string, []string) {
	oldText := make([]string, len(rows))
	newText := make([]string, len(rows))

	for index, row := range rows {
		oldText[index] = clipSide(row.Old, width)
		newText[index] = clipSide(row.New, width)
	}

	if !color {
		return oldText, newText
	}

	highlightedOld := HighlightLines(path, strings.Join(nonEmpty(oldText), "\n"))
	highlightedNew := HighlightLines(path, strings.Join(nonEmpty(newText), "\n"))

	return mergeHighlighted(oldText, highlightedOld), mergeHighlighted(newText, highlightedNew)
}

// clipSide keeps the visible text of one side inside the cells it has.
func clipSide(cell domain.SplitCell, width int) string {
	if !cell.Filled {
		return ""
	}
	if cell.Kind == domain.NoNewline {
		return noNewlineText
	}
	if width <= 0 {
		return cell.Text
	}

	return truncate(cell.Text, width)
}

// nonEmpty keeps the source lines of one column, in order.
func nonEmpty(texts []string) []string {
	out := make([]string, 0, len(texts))
	for _, text := range texts {
		if text != "" {
			out = append(out, text)
		}
	}

	return out
}

// mergeHighlighted puts the coloured lines back in place of the non-empty ones.
func mergeHighlighted(texts, highlighted []string) []string {
	out := make([]string, len(texts))

	next := 0
	for index, text := range texts {
		if text == "" {
			continue
		}
		if next < len(highlighted) {
			out[index] = highlighted[next]
		} else {
			out[index] = text
		}

		next++
	}

	return out
}

// splitSeparator divides the two sides of the aligned diff.
const splitSeparator = " │ "

// splitSideWidth is the cells each side gets; the separator and the numbers are not counted here.
func splitSideWidth(width int) int {
	if width <= 0 {
		return 0
	}

	return max((width-len([]rune(splitSeparator)))/2, 0)
}

// splitSideTextWidth is the cells left for the source text of one side: its number, the gap and the
// marker take the rest.
func splitSideTextWidth(sideWidth, oldWidth, newWidth int) int {
	number := max(oldWidth, newWidth)

	return max(sideWidth-(number+len([]rune(numberGap))+1), 0)
}

// splitCell renders one side of an aligned row: its number, its marker and its text.
func splitCell(cell domain.SplitCell, text string, numberWidth, width int, color bool) string {
	if !cell.Filled {
		return padLine("", width)
	}

	number := ""
	if cell.Number > 0 {
		number = fmt.Sprintf("%*d", numberWidth, cell.Number)
	}

	prefix := fmt.Sprintf("%s%s%s", number, numberGap, splitMarker(cell.Kind))
	body := text
	if cell.Kind == domain.NoNewline {
		body = noNewlineText
	}
	if !color {
		return padLine(prefix+body, width)
	}

	// The padding goes inside the styled span, so the tint covers the whole cell.
	return styles.Gutter.Render(prefix) +
		styled(splitLineStyle(cell.Kind), padLine(body, max(width-lipgloss.Width(prefix), 0)), color)
}

// splitMarker is the diff prefix of one side.
func splitMarker(kind domain.DiffLineKind) string {
	switch kind {
	case domain.Removed:
		return "-"
	case domain.Added:
		return "+"
	case domain.NoNewline:
		return numberGap
	default:
		return numberGap
	}
}

// splitLineStyle colours one side by the kind of its line.
func splitLineStyle(kind domain.DiffLineKind) lipgloss.Style {
	switch kind {
	case domain.Removed:
		return styles.Removed
	case domain.Added:
		return styles.Added
	case domain.NoNewline:
		return styles.MetaPlain
	default:
		return styles.PlainCells(lipgloss.NewStyle())
	}
}

// splitNumberWidths sizes both numbers from the aligned rows.
func splitNumberWidths(rows []domain.SplitRow) (int, int) {
	oldMax, newMax := 0, 0
	for _, row := range rows {
		oldMax = max(oldMax, row.Old.Number)
		newMax = max(newMax, row.New.Number)
	}

	return len(strconv.Itoa(max(oldMax, 1))), len(strconv.Itoa(max(newMax, 1)))
}

// padLine fills text up to width cells.
func padLine(text string, width int) string {
	if width <= 0 {
		return text
	}
	if gap := width - lipgloss.Width(text); gap > 0 {
		return text + strings.Repeat(" ", gap)
	}

	return text
}

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
