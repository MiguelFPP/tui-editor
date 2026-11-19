package editor

import (
	"path/filepath"
	"strings"
	"sync"

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

	highlighted, ok := colourise(path, source)
	if !ok {
		return source
	}

	return highlighted
}

// HighlightLines colours source and returns its lines; it falls back to the plain lines.
func HighlightLines(path domain.RelPath, source string) []string {
	trimmed := trimFinalNewline(source)
	plain := strings.Split(trimmed, "\n")
	if trimmed == "" {
		return plain // one empty row, so the gutter always numbers a line
	}

	highlighted, ok := colourise(path, trimmed)
	if !ok {
		return plain
	}

	lines := strings.Split(trimFinalNewline(highlighted), "\n")
	if len(lines) != len(plain) {
		return plain // a lexer that rewrites the line count is not usable here
	}

	return lines
}

// colourise tokenises and formats source; ok is false when no lexer or style applies.
func colourise(path domain.RelPath, source string) (string, bool) {
	lexer := lexerFor(path, source)
	if lexer == nil {
		return "", false
	}

	style := chromastyles.Get(highlightStyle)
	if style == nil {
		return "", false
	}

	formatter := formatters.Get(highlightFormat)
	if formatter == nil {
		return "", false
	}

	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		return "", false
	}

	var out strings.Builder
	if err := formatter.Format(&out, style, iterator); err != nil {
		return "", false
	}

	return out.String(), true
}

// nameCache memoises the lexers matched by file name; the content analysis is never cached.
type nameCache struct {
	mu     sync.RWMutex
	byName map[string]chroma.Lexer
}

func (c *nameCache) get(name string) (chroma.Lexer, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	lexer, ok := c.byName[name]

	return lexer, ok
}

// put stores one lexer, keeping the map bounded on repos with many distinct names.
func (c *nameCache) put(name string, lexer chroma.Lexer) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.byName) >= nameCacheLimit {
		return
	}

	c.byName[name] = lexer
}

const nameCacheLimit = 512

var nameLexers = &nameCache{byName: make(map[string]chroma.Lexer)}

// lexerFor matches the file name first and falls back to a content analysis.
func lexerFor(path domain.RelPath, source string) chroma.Lexer {
	name := filepath.Base(path.String())
	if name != "" && name != "." {
		if lexer, ok := nameLexers.get(name); ok {
			return lexer
		}

		if lexer := lexers.Match(name); lexer != nil {
			coalesced := chroma.Coalesce(lexer)
			nameLexers.put(name, coalesced)

			return coalesced
		}
	}

	if lexer := lexers.Analyse(source); lexer != nil {
		return chroma.Coalesce(lexer)
	}

	return nil
}
