package editor

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
)

const (
	goldenWidth  = 48
	goldenHeight = 8

	longLines = 30
)

const (
	goSource       = "package main\n\nimport \"fmt\"\n\n// Saluda imprime un saludo.\nfunc main() {\n\tfmt.Println(\"hola\")\n}\n"
	markdownSource = "# Guía\n\n- paso uno\n- paso dos\n\n> nota al margen\n"
	unknownSource  = "notas sin formato reconocido\n\ntexto suelto sobre el editor\n"
)

var (
	ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	updateGold  = flag.Bool("update", false, "reescribe los golden files de testdata")
)

// fixture is one file served by the fake port.
type fixture struct {
	text      string
	truncated bool
	err       error
}

// unrelatedMsg is a message the component must ignore.
type unrelatedMsg struct{}

var fixtures = map[domain.RelPath]fixture{
	"main.go":       {text: goSource},
	"docs/guide.md": {text: markdownSource},
	"notes.xyz":     {text: unknownSource},
	"empty.txt":     {},
	"long.txt":      {text: longSource()},
	"huge.log":      {text: longSource(), truncated: true},
	"logo.png":      {err: fmt.Errorf("%w: %q", domain.ErrBinary, "logo.png")},
	"secreto.txt":   {err: fmt.Errorf("%w: %q", domain.ErrPermissionDenied, "secreto.txt")},
	"ausente.txt":   {err: fmt.Errorf("%w: %q", domain.ErrNotFound, "ausente.txt")},
	"enorme.log":    {err: fmt.Errorf("%w: %q", domain.ErrTooLarge, "enorme.log")},
}

func longSource() string {
	var out strings.Builder
	for i := 1; i <= longLines; i++ {
		fmt.Fprintf(&out, "line-%d\n", i)
	}

	return out.String()
}

func filesFake(t *testing.T) *fakes.FileService {
	t.Helper()

	files := fakes.NewFileService()
	files.OnReadFile = func(_ context.Context, path domain.RelPath) (domain.FileContent, error) {
		item, ok := fixtures[path]
		if !ok {
			return domain.FileContent{}, fmt.Errorf("%w: %q", domain.ErrNotFound, path)
		}
		if item.err != nil {
			return domain.FileContent{}, item.err
		}

		return domain.FileContent{Path: path, Text: item.text, Truncated: item.truncated}, nil
	}

	return files
}

// open runs Init, feeds its message back and returns the loaded model.
func open(t *testing.T, path domain.RelPath, width, height int) Model {
	t.Helper()

	m := New(filesFake(t), path)
	m.SetSize(width, height)

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init produced no command, want the read command")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("the read command returned no message, want previewLoadedMsg")
	}

	next, nested := m.Update(msg)
	if nested != nil {
		t.Fatal("the loaded preview produced another command, want nil")
	}

	return next
}

func scroll(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()

	next, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatal("scrolling produced a command, want nil")
	}

	return next
}

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func plain(view string) string {
	return ansiPattern.ReplaceAllString(view, "")
}

// visible strips the ANSI escapes and the padding the viewport adds, so goldens
// keep only the text the user sees.
func visible(view string) string {
	lines := strings.Split(plain(view), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}

	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name)
	if *updateGold {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("creando testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
			t.Fatalf("escribiendo %s: %v", path, err)
		}

		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leyendo %s: %v", path, err)
	}

	if want := strings.TrimSuffix(string(data), "\n"); got != want {
		t.Errorf("%s:\n got %q\nwant %q", name, got, want)
	}
}

func TestPreviewLoadsFileThroughThePort(t *testing.T) {
	files := filesFake(t)
	m := New(files, "main.go")
	m.SetSize(goldenWidth, 10)

	if !m.loading {
		t.Error("loading = false before Init, want true")
	}
	if view := plain(m.View()); !strings.Contains(view, "cargando") {
		t.Errorf("View() = %q, want the loading state", view)
	}
	if got := files.CallCount("ReadFile"); got != 0 {
		t.Fatalf("ReadFile calls = %d before Init, want 0", got)
	}

	msg := m.Init()()
	loaded, ok := msg.(previewLoadedMsg)
	if !ok {
		t.Fatalf("the Init command returned %T, want previewLoadedMsg", msg)
	}
	if loaded.path != "main.go" {
		t.Errorf("previewLoadedMsg.path = %q, want %q", loaded.path, "main.go")
	}
	if loaded.err != nil {
		t.Fatalf("previewLoadedMsg.err = %v, want nil", loaded.err)
	}

	m, cmd := m.Update(loaded)
	if cmd != nil {
		t.Error("the loaded preview produced a command, want nil")
	}
	if m.loading {
		t.Error("loading = true after the load, want false")
	}
	if got := files.CallCount("ReadFile"); got != 1 {
		t.Errorf("ReadFile calls = %d, want 1", got)
	}
	if m.raw != goSource {
		t.Errorf("raw = %q, want the file text", m.raw)
	}

	view := visible(m.View())
	for _, want := range []string{"main.go · archivo · 8 líneas", "1 │ package main", "8 │ }", `7 │     fmt.Println("hola")`} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
}

func TestPreviewHeaderFitsTheWidth(t *testing.T) {
	narrow := open(t, "docs/guide.md", 16, 4)
	header, _, _ := strings.Cut(visible(narrow.View()), "\n")

	if got := lipgloss.Width(header); got > 16 {
		t.Errorf("header = %q is %d cells wide, want at most 16", header, got)
	}
	if !strings.Contains(header, "…") {
		t.Errorf("header = %q, want the truncated marker", header)
	}

	wide := open(t, "docs/guide.md", goldenWidth, goldenHeight)
	wideHeader, _, _ := strings.Cut(visible(wide.View()), "\n")
	if want := "docs/guide.md · archivo · 6 líneas"; wideHeader != want {
		t.Errorf("header = %q, want %q", wideHeader, want)
	}
}

func TestPreviewWindowSizeMsgResizes(t *testing.T) {
	m := open(t, "long.txt", goldenWidth, goldenHeight)

	m, cmd := m.Update(tea.WindowSizeMsg{Width: 32, Height: 4})
	if cmd != nil {
		t.Error("WindowSizeMsg produced a command, want nil")
	}
	if got := m.viewport.Width(); got != 32 {
		t.Errorf("viewport.Width() = %d, want 32", got)
	}
	if got := m.viewport.Height(); got != 3 {
		t.Errorf("viewport.Height() = %d, want 3 (4 rows minus the header)", got)
	}
}

func TestPreviewSetSizeKeepsScrollPosition(t *testing.T) {
	m := open(t, "long.txt", 40, 6)
	m = scroll(t, m, key(tea.KeyPgDown))
	m = scroll(t, m, key(tea.KeyPgDown))

	if got := m.viewport.YOffset(); got != 10 {
		t.Fatalf("YOffset() = %d, want 10", got)
	}

	m.SetSize(40, 8)
	if got := m.viewport.YOffset(); got != 10 {
		t.Errorf("YOffset() = %d after growing, want 10", got)
	}

	m.SetSize(40, 3)
	if got := m.viewport.YOffset(); got != 10 {
		t.Errorf("YOffset() = %d after shrinking, want 10", got)
	}

	m.SetSize(40, 40)
	if got := m.viewport.YOffset(); got != 0 {
		t.Errorf("YOffset() = %d when everything fits, want 0", got)
	}
}

func TestPreviewSmallSizesDoNotPanic(t *testing.T) {
	m := open(t, "long.txt", 40, 6)

	for _, size := range []struct{ width, height int }{{0, 0}, {1, 1}, {2, 0}, {80, 0}} {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			m.SetSize(size.width, size.height)

			if got := m.viewport.Height(); got != max(size.height-headerRows, 0) {
				t.Errorf("viewport.Height() = %d, want %d", got, max(size.height-headerRows, 0))
			}
			_ = m.View()
		})
	}
}

func TestPreviewScrollsWithKeyboardAndWheel(t *testing.T) {
	m := open(t, "long.txt", 40, 6)

	m = scroll(t, m, key(tea.KeyDown))
	if got := m.viewport.YOffset(); got != 1 {
		t.Fatalf("down: YOffset() = %d, want 1", got)
	}
	m = scroll(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if got := m.viewport.YOffset(); got != 2 {
		t.Fatalf("j: YOffset() = %d, want 2", got)
	}
	m = scroll(t, m, key(tea.KeyUp))
	if got := m.viewport.YOffset(); got != 1 {
		t.Fatalf("up: YOffset() = %d, want 1", got)
	}
	m = scroll(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("k: YOffset() = %d, want 0", got)
	}

	m = scroll(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 1, Y: 1})
	if got := m.viewport.YOffset(); got != 3 {
		t.Fatalf("wheel down: YOffset() = %d, want 3", got)
	}
	if view := visible(m.View()); !strings.Contains(view, "4 │ line-4") {
		t.Errorf("View() = %q, want the scrolled body", view)
	}

	m = scroll(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 1, Y: 1})
	if got := m.viewport.YOffset(); got != 0 {
		t.Errorf("wheel up: YOffset() = %d, want 0", got)
	}

	m = scroll(t, m, key(tea.KeyPgDown))
	if got := m.viewport.YOffset(); got != m.viewport.Height() {
		t.Errorf("pgdown: YOffset() = %d, want %d", got, m.viewport.Height())
	}

	m = scroll(t, m, key(tea.KeyPgUp))
	if got := m.viewport.YOffset(); got != 0 {
		t.Errorf("pgup: YOffset() = %d, want 0", got)
	}
}

func TestPreviewNumbersAreRightAligned(t *testing.T) {
	m := open(t, "long.txt", 40, 6)
	view := visible(m.View())

	for _, want := range []string{" 1 │ line-1", " 5 │ line-5"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want the right-aligned %q", view, want)
		}
	}

	m = open(t, "long.txt", 40, 6)
	for range 4 {
		m = scroll(t, m, key(tea.KeyPgDown))
	}
	if view := visible(m.View()); !strings.Contains(view, "21 │ line-21") {
		t.Errorf("View() = %q, want the scrolled page", view)
	}

	m = scroll(t, m, key(tea.KeyPgDown))
	view = visible(m.View())
	if !strings.Contains(view, "30 │ line-30") {
		t.Errorf("View() = %q, want the two-digit gutter at the bottom", view)
	}
}

func TestPreviewErrorStates(t *testing.T) {
	cases := []struct {
		name string
		path domain.RelPath
		want string
	}{
		{name: "binario", path: "logo.png", want: "archivo binario"},
		{name: "permiso", path: "secreto.txt", want: "permiso denegado"},
		{name: "no encontrado", path: "ausente.txt", want: "no encontrado"},
		{name: "demasiado grande", path: "enorme.log", want: "demasiado grande"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := open(t, tc.path, goldenWidth, goldenHeight)
			if m.err == nil {
				t.Fatal("err = nil, want the read failure")
			}
			if m.loading {
				t.Error("loading = true after a failure, want false")
			}

			view := visible(m.View())
			if !strings.Contains(view, "error: "+tc.want) {
				t.Errorf("View() = %q, want the %q state", view, tc.want)
			}
			if !strings.Contains(view, string(tc.path)) {
				t.Errorf("View() = %q, want the path", view)
			}
			if strings.Contains(view, " │ ") {
				t.Errorf("View() = %q, want no numbered body in an error state", view)
			}
		})
	}
}

func TestPreviewMissingServiceReportsAnError(t *testing.T) {
	m := New(nil, "main.go")
	m.SetSize(goldenWidth, goldenHeight)

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init produced no command, want a failed load")
	}

	m, _ = m.Update(cmd())
	if !errors.Is(m.err, errNoService) {
		t.Fatalf("err = %v, want %v", m.err, errNoService)
	}

	view := visible(m.View())
	if !strings.Contains(view, "error") || !strings.Contains(view, "servicio de archivos") {
		t.Errorf("View() = %q, want the missing-service error", view)
	}
}

func TestPreviewEmptyFileShowsEmptyState(t *testing.T) {
	m := open(t, "empty.txt", goldenWidth, goldenHeight)

	if m.raw != "" {
		t.Errorf("raw = %q, want the empty file", m.raw)
	}

	view := visible(m.View())
	if !strings.Contains(view, "vacío") || !strings.Contains(view, "0 líneas") {
		t.Errorf("View() = %q, want the empty state", view)
	}
}

func TestPreviewTruncatedFileIsMarked(t *testing.T) {
	m := open(t, "huge.log", goldenWidth, goldenHeight)

	if !m.truncated {
		t.Fatal("truncated = false, want the FileContent flag")
	}

	view := visible(m.View())
	if !strings.Contains(view, "truncado") {
		t.Errorf("View() = %q, want the truncated marker", view)
	}
	if !strings.Contains(view, "1 │ line-1") {
		t.Errorf("View() = %q, want the truncated content", view)
	}
}

func TestPreviewIgnoresResultsOfAnotherPath(t *testing.T) {
	m := New(filesFake(t), "main.go")
	m.SetSize(goldenWidth, goldenHeight)

	next, cmd := m.Update(previewLoadedMsg{path: "notes.xyz", content: domain.FileContent{Path: "notes.xyz", Text: unknownSource}})
	if cmd != nil {
		t.Error("a stale preview produced a command, want nil")
	}
	if !next.loading {
		t.Error("loading = false, want the pending state kept")
	}
	if next.raw != "" || next.content != "" {
		t.Errorf("raw = %q, content = %q, want both empty", next.raw, next.content)
	}
	if view := visible(next.View()); !strings.Contains(view, "cargando") {
		t.Errorf("View() = %q, want the loading state", view)
	}
}

func TestPreviewIgnoresUnrelatedMessages(t *testing.T) {
	m := open(t, "main.go", goldenWidth, goldenHeight)

	next, cmd := m.Update(unrelatedMsg{})
	if cmd != nil {
		t.Error("an unrelated message produced a command, want nil")
	}
	if next.raw != m.raw || next.viewport.YOffset() != m.viewport.YOffset() {
		t.Error("an unrelated message changed the state")
	}
}

func TestPreviewGoldenFiles(t *testing.T) {
	cases := []struct {
		golden string
		path   domain.RelPath
	}{
		{golden: "go.golden", path: "main.go"},
		{golden: "markdown.golden", path: "docs/guide.md"},
		{golden: "unknown.golden", path: "notes.xyz"},
		{golden: "binary.golden", path: "logo.png"},
		{golden: "truncated.golden", path: "huge.log"},
	}

	for _, tc := range cases {
		t.Run(tc.golden, func(t *testing.T) {
			m := open(t, tc.path, goldenWidth, goldenHeight)

			checkGolden(t, tc.golden, visible(m.View()))
		})
	}
}

func TestNewDiffRendersInMemoryDiff(t *testing.T) {
	diff := domain.Diff{
		Path:      "main.go",
		Additions: 2,
		Deletions: 1,
		Hunks: []domain.DiffHunk{{
			Header:   "@@ -1,3 +1,4 @@",
			OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 4,
			Lines: []domain.DiffLine{
				{Kind: domain.Context, Text: "package main", OldLine: 1, NewLine: 1},
				{Kind: domain.Removed, Text: "vieja", OldLine: 2},
				{Kind: domain.Added, Text: "nueva", NewLine: 2},
				{Kind: domain.Added, Text: "extra", NewLine: 3},
				{Kind: domain.Context, Text: "", OldLine: 3, NewLine: 4},
				{Kind: domain.Context, Text: "func main() {}", OldLine: 4, NewLine: 5},
				{Kind: domain.Context, Text: "", OldLine: 5, NewLine: 6},
				{Kind: domain.Context, Text: "// fin", OldLine: 6, NewLine: 7},
			},
		}},
	}

	m := NewDiff(diff)
	m.SetSize(40, 5)

	if cmd := m.Init(); cmd != nil {
		t.Fatal("the diff preview produced a command, want nil: it must not read files")
	}
	if m.loading {
		t.Error("loading = true in diff mode, want false")
	}

	view := visible(m.View())
	for _, want := range []string{"main.go · diff", "@@ -1,3 +1,4 @@", "-vieja", "+nueva"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}

	m = scroll(t, m, key(tea.KeyDown))
	if got := m.viewport.YOffset(); got != 1 {
		t.Fatalf("down: YOffset() = %d, want 1", got)
	}
	if view := visible(m.View()); strings.Contains(view, "@@") {
		t.Errorf("View() = %q, want the hunk header scrolled off", view)
	}
}

func TestNewDiffStates(t *testing.T) {
	t.Run("vacío", func(t *testing.T) {
		if view := visible(NewDiff(domain.Diff{Path: "main.go"}).View()); !strings.Contains(view, "sin cambios") {
			t.Errorf("View() = %q, want the empty-diff message", view)
		}
	})

	t.Run("binario", func(t *testing.T) {
		view := visible(NewDiff(domain.Diff{Path: "logo.png", Binary: true}).View())
		if !strings.Contains(view, "binario") || !strings.Contains(view, "diferencia binaria") {
			t.Errorf("View() = %q, want the binary diff state", view)
		}
	})

	t.Run("staged y truncado", func(t *testing.T) {
		diff := domain.Diff{
			Path:      "main.go",
			Staged:    true,
			Truncated: true,
			Hunks: []domain.DiffHunk{{
				Header:   "@@ -1,1 +1,2 @@",
				OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 2,
				Lines: []domain.DiffLine{
					{Kind: domain.Removed, Text: "vieja", OldLine: 1},
					{Kind: domain.Added, Text: "nueva", NewLine: 1},
				},
			}},
		}

		view := visible(NewDiff(diff).View())
		if !strings.Contains(view, "main.go · diff · staged") {
			t.Errorf("View() = %q, want the staged marker", view)
		}
		if !strings.Contains(view, "diff truncado") {
			t.Errorf("View() = %q, want the truncation notice", view)
		}
	})
}

func TestPreviewHighlightUsesChroma(t *testing.T) {
	highlighted := Highlight("main.go", goSource)
	if !strings.Contains(highlighted, "\x1b[") {
		t.Fatalf("Highlight() = %q, want ANSI escapes for the Go keywords", highlighted)
	}
	if got := plain(highlighted); got != goSource {
		t.Errorf("plain(Highlight()) = %q, want the source unchanged", got)
	}

	markdown := Highlight("docs/guide.md", markdownSource)
	if !ansiPattern.MatchString(markdown) {
		t.Errorf("Highlight() = %q, want ANSI escapes for Markdown", markdown)
	}
	if got := plain(markdown); got != markdownSource {
		t.Errorf("plain(Highlight()) = %q, want the Markdown source unchanged", got)
	}
}

func TestHighlightFallsBackToPlainText(t *testing.T) {
	if got := Highlight("notes.xyz", unknownSource); got != unknownSource {
		t.Errorf("Highlight() = %q, want the source unchanged", got)
	}
	if lexer := lexerFor("notes.xyz", unknownSource); lexer != nil {
		t.Errorf("lexerFor() = %q, want no lexer for an unknown file", lexer.Config().Name)
	}
	if got := Highlight("", ""); got != "" {
		t.Errorf("Highlight() = %q, want an empty string", got)
	}
}

func TestLexerForMatchesPathThenContent(t *testing.T) {
	cases := []struct {
		name   string
		path   domain.RelPath
		source string
		want   string
	}{
		{name: "extensión go", path: "main.go", source: goSource, want: "Go"},
		{name: "extensión markdown", path: "docs/guide.md", source: markdownSource, want: "markdown"},
		{name: "nombre completo", path: "Makefile", source: "all:\n\tgo build ./...\n", want: "Makefile"},
		{name: "contenido sin extensión", path: "snippet", source: goSource, want: "Go"},
		{name: "sin pistas", path: "notes.xyz", source: unknownSource, want: ""},
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
