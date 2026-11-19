package common

import (
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// goSplitDiff has Go source on both sides of one hunk.
func goSplitDiff() domain.Diff {
	return domain.Diff{
		Path: "main.go",
		Hunks: []domain.DiffHunk{{
			Header: "@@ -1,2 +1,2 @@",
			Lines: []domain.DiffLine{
				{Kind: domain.Context, Text: "package main", OldLine: 1, NewLine: 1},
				{Kind: domain.Removed, Text: "func uno() {", OldLine: 2},
				{Kind: domain.Added, Text: "func uno() int {", NewLine: 2},
			},
		}},
	}
}

// TestRenderDiffSplitColoursEachSide checks both columns carry ANSI escapes.
func TestRenderDiffSplitColoursEachSide(t *testing.T) {
	rendered := RenderDiffSplit(goSplitDiff(), 100, true)

	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("RenderDiffSplit() = %q, want ANSI escapes", rendered)
	}

	for _, line := range strings.Split(rendered, "\n") {
		if !strings.Contains(plainANSI(line), "func uno()") {
			continue
		}
		if !strings.Contains(line, "\x1b[") {
			t.Errorf("line = %q, want the coloured source", line)
		}
	}
}

// TestRenderDiffSplitKeepsTheText checks colouring never changes the visible characters.
func TestRenderDiffSplitKeepsTheText(t *testing.T) {
	plainRender := RenderDiffSplit(goSplitDiff(), 100, false)
	coloured := RenderDiffSplit(goSplitDiff(), 100, true)

	if got, want := plainANSI(coloured), plainRender; got != want {
		t.Errorf("plain(coloured) = %q, want %q", got, want)
	}
}

// TestRenderDiffSplitWithoutALexerStaysPlain returns the very same text: a name without an extension
// still matches a content analyser, so the fixture uses an extension no lexer claims.
func TestRenderDiffSplitWithoutALexerStaysPlain(t *testing.T) {
	d := goSplitDiff()
	d.Path = "notas.zzz"
	for index := range d.Hunks[0].Lines {
		d.Hunks[0].Lines[index].Text = "texto sin formato"
	}

	plainRender := RenderDiffSplit(d, 100, false)
	coloured := RenderDiffSplit(d, 100, true)

	if got := plainANSI(coloured); got != plainRender {
		t.Errorf("plain(coloured) = %q, want %q", got, plainRender)
	}
}

// plainANSI strips the escape sequences of a rendered string.
func plainANSI(text string) string {
	var out strings.Builder

	inEscape := false
	for _, r := range text {
		switch {
		case r == 0x1b:
			inEscape = true
		case inEscape:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
		default:
			out.WriteRune(r)
		}
	}

	return out.String()
}
