package editor

import (
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// Fixed style and formatter: the terminal16m formatter derives every escape from
// the style itself, so the output never depends on the terminal colour profile.
const (
	highlightStyle  = "tokyonight-night"
	highlightFormat = "terminal16m"
)

// Highlight colours source by the lexer of path, or by its content, and returns
// source unchanged when no lexer applies.
func Highlight(path domain.RelPath, source string) string {
	if source == "" {
		return ""
	}

	lexer := lexerFor(path, source)
	if lexer == nil {
		return source
	}

	style := chromastyles.Get(highlightStyle)
	if style == nil {
		return source
	}

	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		return source
	}

	var out strings.Builder
	if err := formatters.Get(highlightFormat).Format(&out, style, iterator); err != nil {
		return source
	}

	return out.String()
}

// lexerFor matches the file name first and falls back to a content analysis.
func lexerFor(path domain.RelPath, source string) chroma.Lexer {
	if name := filepath.Base(path.String()); name != "" && name != "." {
		if lexer := lexers.Match(name); lexer != nil {
			return chroma.Coalesce(lexer)
		}
	}

	if lexer := lexers.Analyse(source); lexer != nil {
		return chroma.Coalesce(lexer)
	}

	return nil
}
