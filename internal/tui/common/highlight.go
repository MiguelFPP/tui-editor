package common

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

// highlightFormat is fixed: terminal16m derives every escape from the style itself, so the output
// never depends on the colour profile of the terminal.
const highlightFormat = "terminal16m"

// highlightCacheLimit bounds the lexers memoised by file name.
const highlightCacheLimit = 512

// nameLexers memoises the lexers matched by file name; the content analysis is never cached.
type nameLexersCache struct {
	mu     sync.RWMutex
	byName map[string]chroma.Lexer
}

func (c *nameLexersCache) get(name string) (chroma.Lexer, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	lexer, ok := c.byName[name]

	return lexer, ok
}

func (c *nameLexersCache) put(name string, lexer chroma.Lexer) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.byName) >= highlightCacheLimit {
		return
	}

	c.byName[name] = lexer
}

var nameLexers = &nameLexersCache{byName: make(map[string]chroma.Lexer)}

// Highlight colours source by the lexer of path and returns it unchanged when no lexer applies.
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

// HighlightLines colours source by the lexer of path and returns its lines; it falls back to the
// plain lines when no lexer applies or when the lexer changes the line count.
func HighlightLines(path domain.RelPath, source string) []string {
	trimmed := strings.TrimSuffix(source, "\n")
	plain := strings.Split(trimmed, "\n")
	if trimmed == "" {
		return plain // one empty row, so the gutter always numbers a line
	}

	highlighted, ok := colourise(path, trimmed)
	if !ok {
		return plain
	}

	lines := strings.Split(strings.TrimSuffix(highlighted, "\n"), "\n")
	if len(lines) != len(plain) {
		return plain // a lexer that rewrites the line count is not usable here
	}

	return lines
}

// HighlightDiffLines colours the body of a unified diff line by line, keeping the gutter and the
// +/- markers intact. The path selects the lexer; a path without one keeps every line plain.
func HighlightDiffLines(path domain.RelPath, lines []string) []string {
	if len(lines) == 0 {
		return lines
	}

	bodies := make([]diffBody, 0, len(lines))
	source := make([]string, 0, len(lines))

	for _, line := range lines {
		body := splitDiffLine(line)
		bodies = append(bodies, body)
		source = append(source, body.text)
	}

	coloured := HighlightLines(path, strings.Join(source, "\n"))
	if len(coloured) != len(lines) {
		return lines
	}

	out := make([]string, 0, len(lines))
	for index, line := range lines {
		if bodies[index].text == "" {
			out = append(out, line) // headers and hunk titles keep their own style

			continue
		}

		out = append(out, bodies[index].prefix+coloured[index])
	}

	return out
}

// diffBody splits one rendered diff row into the gutter plus the marker and the source text.
type diffBody struct {
	prefix string
	text   string
}

// splitDiffLine separates the line-number gutter and the +/- marker from the source text.
func splitDiffLine(line string) diffBody {
	index := strings.Index(line, cellSeparator)
	if index < 0 {
		return diffBody{text: ""}
	}

	rest := line[min(index+len(cellSeparator), len(line)):]
	prefix := line[:len(line)-len(rest)]

	switch {
	case strings.HasPrefix(rest, "+"), strings.HasPrefix(rest, "-"):
		prefix += rest[:1]

		return diffBody{prefix: prefix, text: rest[1:]}
	case strings.HasPrefix(rest, numberGap):
		prefix += numberGap

		return diffBody{prefix: prefix, text: rest[1:]}
	default:
		return diffBody{text: ""}
	}
}

// colourise tokenises and formats source; ok is false when no lexer or style applies.
func colourise(path domain.RelPath, source string) (string, bool) {
	lexer := lexerFor(path, source)
	if lexer == nil {
		return "", false
	}

	style := chromastyles.Get(styles.CurrentTheme().ChromaStyle)
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
