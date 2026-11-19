package editor

import (
	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/tui/common"
)

// Highlight colours source by the lexer of path and returns it unchanged when no lexer applies.
func Highlight(path domain.RelPath, source string) string {
	return common.Highlight(path, source)
}

// HighlightLines colours source and returns its lines; it falls back to the plain lines.
func HighlightLines(path domain.RelPath, source string) []string {
	return common.HighlightLines(path, source)
}
