package common

import (
	"strings"
	"sync"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

const goHighlightSource = "package main\n\nimport \"fmt\"\n\n// Saluda imprime un saludo.\nfunc main() {\n\tfmt.Println(\"hola\")\n}\n"

const unknownHighlightSource = "uno\ndos\ntres\n"

func TestLexerForMatchesPathThenContent(t *testing.T) {
	cases := []struct {
		name   string
		path   domain.RelPath
		source string
		want   string
	}{
		{name: "extensión go", path: "main.go", source: goHighlightSource, want: "Go"},
		{name: "nombre completo", path: "Makefile", source: "all:\n\tgo build ./...\n", want: "Makefile"},
		{name: "contenido sin extensión", path: "snippet", source: goHighlightSource, want: "Go"},
		{name: "sin pistas", path: "notes.xyz", source: unknownHighlightSource, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lexer := lexerFor(tc.path, tc.source)
			if tc.want == "" {
				if lexer != nil {
					t.Fatalf("lexerFor() = %q, want none", lexer.Config().Name)
				}

				return
			}

			if lexer == nil {
				t.Fatalf("lexerFor() = nil, want %q", tc.want)
			}
			if got := lexer.Config().Name; got != tc.want {
				t.Errorf("lexerFor() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLexerForCachesByName(t *testing.T) {
	first := lexerFor("main.go", goHighlightSource)
	if first == nil {
		t.Fatal("lexerFor() = nil, want the Go lexer")
	}

	second := lexerFor("docs/main.go", unknownHighlightSource)
	if second == nil {
		t.Fatal("lexerFor() = nil on the cached name, want the Go lexer")
	}
	if got := second.Config().Name; got != "Go" {
		t.Errorf("lexerFor() = %q, want the cached %q", got, "Go")
	}
}

func TestHighlightLinesKeepsTheLines(t *testing.T) {
	lines := HighlightLines("main.go", goHighlightSource)
	want := strings.Split(strings.TrimSuffix(goHighlightSource, "\n"), "\n")

	if len(lines) != len(want) {
		t.Fatalf("HighlightLines() = %d lines, want %d", len(lines), len(want))
	}
	for i, line := range lines {
		if got := plainCommon(line); got != want[i] {
			t.Errorf("line %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestHighlightLinesFallsBackToPlainText(t *testing.T) {
	lines := HighlightLines("notes.xyz", unknownHighlightSource)
	want := strings.Split(strings.TrimSuffix(unknownHighlightSource, "\n"), "\n")

	if len(lines) != len(want) {
		t.Fatalf("HighlightLines() = %d lines, want %d", len(lines), len(want))
	}
	for i, line := range lines {
		if line != want[i] {
			t.Errorf("line %d = %q, want the plain %q", i, line, want[i])
		}
	}
}

func TestHighlightLinesIsSafeForConcurrentRendering(t *testing.T) {
	paths := []domain.RelPath{"main.go", "Makefile", "snippet", "notes.xyz"}

	var group sync.WaitGroup
	for _, path := range paths {
		group.Add(1)

		go func() {
			defer group.Done()

			for range 4 {
				if lines := HighlightLines(path, goHighlightSource); len(lines) == 0 {
					t.Error("HighlightLines() = nil, want the lines of the source")
				}
			}
		}()
	}

	group.Wait()
}

// TestHighlightFileKeepsTheLineCount checks the whole-file pass keeps every line.
func TestHighlightFileKeepsTheLineCount(t *testing.T) {
	lines, ok := HighlightFile("main.go", goHighlightSource)
	if !ok {
		t.Fatal("got = false, want true for a Go source")
	}

	want := strings.Split(strings.TrimSuffix(goHighlightSource, "\n"), "\n")
	if len(lines) != len(want) {
		t.Fatalf("got = %d lines, want %d", len(lines), len(want))
	}
	for index, line := range lines {
		if got := plainCommon(line); got != want[index] {
			t.Errorf("line %d = %q, want %q", index, got, want[index])
		}
	}
}

// TestHighlightFileReportsNoLexer falls back to the plain lines without a lexer.
func TestHighlightFileReportsNoLexer(t *testing.T) {
	lines, ok := HighlightFile("notes.xyz", unknownHighlightSource)
	if ok {
		t.Fatal("got = true, want false without a lexer")
	}

	want := strings.Split(strings.TrimSuffix(unknownHighlightSource, "\n"), "\n")
	if len(lines) != len(want) {
		t.Fatalf("got = %d lines, want %d", len(lines), len(want))
	}
	for index, line := range lines {
		if line != want[index] {
			t.Errorf("line %d = %q, want %q", index, line, want[index])
		}
	}
}

// TestHighlightFileHandlesEmptySource returns one empty row and no lexer.
func TestHighlightFileHandlesEmptySource(t *testing.T) {
	lines, ok := HighlightFile("main.go", "")
	if ok {
		t.Error("got = true, want false for an empty source")
	}
	if len(lines) != 1 || lines[0] != "" {
		t.Errorf("got = %q, want one empty line", lines)
	}
}

// TestHighlightFileColoursAMultiLineString proves the whole file reaches the lexer in one pass.
func TestHighlightFileColoursAMultiLineString(t *testing.T) {
	const source = "package main\n\nvar text = `uno\ndos`\n"

	lines, ok := HighlightFile("main.go", source)
	if !ok {
		t.Fatal("got = false, want true for a Go source")
	}
	if len(lines) != 4 {
		t.Fatalf("got = %d lines, want 4", len(lines))
	}

	// The last two rows live inside one raw string literal, so only the whole-file pass colours both.
	if got, want := plainCommon(lines[2]), "var text = `uno"; got != want {
		t.Errorf("line 2 = %q, want %q", got, want)
	}
	if got, want := plainCommon(lines[3]), "dos`"; got != want {
		t.Errorf("line 3 = %q, want %q", got, want)
	}
	for _, index := range []int{2, 3} {
		if !strings.Contains(lines[index], "\x1b[") {
			t.Errorf("line %d = %q, want ANSI escapes inside the literal", index, lines[index])
		}
	}
}

// plainCommon strips the ANSI escapes of one line.
func plainCommon(line string) string {
	var out strings.Builder

	inEscape := false
	for _, r := range line {
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
