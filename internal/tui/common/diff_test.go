package common

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

var (
	ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	updateGold  = flag.Bool("update", false, "reescribe los golden files de testdata")
)

func plain(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
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

// sampleDiff is a two-hunk unified diff with context, added, removed and no-newline lines.
func sampleDiff() domain.Diff {
	return domain.Diff{
		Path:      "internal/tui/app/model.go",
		Additions: 4,
		Deletions: 2,
		Hunks: []domain.DiffHunk{
			{
				Header:   "@@ -12,5 +12,6 @@",
				OldStart: 12, OldLines: 5, NewStart: 12, NewLines: 6,
				Section: "func (m Model) Update(",
				Lines: []domain.DiffLine{
					{Kind: domain.Context, Text: "\tswitch msg := msg.(type) {", OldLine: 12, NewLine: 12},
					{Kind: domain.Removed, Text: "\tcase tea.KeyMsg:", OldLine: 13},
					{Kind: domain.Added, Text: "\tcase tea.KeyPressMsg:", NewLine: 13},
					{Kind: domain.Added, Text: "\tcase tea.MouseWheelMsg:", NewLine: 14},
					{Kind: domain.Context, Text: "\t}", OldLine: 14, NewLine: 15},
					{Kind: domain.NoNewline},
				},
			},
			{
				Header:   "@@ -40,2 +42,3 @@",
				OldStart: 40, OldLines: 2, NewStart: 42, NewLines: 3,
				Lines: []domain.DiffLine{
					{Kind: domain.Context, Text: "\treturn m, nil", OldLine: 40, NewLine: 42},
					{Kind: domain.Added, Text: "// fin", NewLine: 43},
				},
			},
		},
	}
}

func TestRenderDiffMatchesGolden(t *testing.T) {
	checkGolden(t, "diff.golden", RenderDiff(sampleDiff(), 0, false))
}

func TestRenderDiffBinaryMatchesGolden(t *testing.T) {
	diff := domain.Diff{Path: "assets/logo.png", Binary: true}

	checkGolden(t, "binary.golden", RenderDiff(diff, 0, false))

	view := plain(RenderDiff(diff, 0, true))
	if !strings.Contains(view, "diferencia binaria") {
		t.Errorf("RenderDiff() = %q, want the binary message", view)
	}
}

func TestRenderDiffEmptyAndTruncated(t *testing.T) {
	if got := RenderDiff(domain.Diff{}, 0, false); got != "sin cambios" {
		t.Errorf("RenderDiff() = %q, want %q", got, "sin cambios")
	}

	diff := sampleDiff()
	diff.Truncated = true

	view := plain(RenderDiff(diff, 0, true))
	if !strings.HasSuffix(view, "diff truncado") {
		t.Errorf("RenderDiff() = %q, want the truncation notice at the end", view)
	}
}

func TestRenderDiffColourKeepsPlainText(t *testing.T) {
	const width = 60

	coloured := RenderDiff(sampleDiff(), width, true)
	if !ansiPattern.MatchString(coloured) {
		t.Error("RenderDiff(color=true) has no ANSI escapes, want colour")
	}
	if plainAgain := plain(coloured); plainAgain != RenderDiff(sampleDiff(), width, false) {
		t.Errorf("plain(coloured) = %q, want the plain render", plainAgain)
	}
}

func TestRenderDiffIsDeterministic(t *testing.T) {
	first := RenderDiff(sampleDiff(), 72, false)
	for range 5 {
		if again := RenderDiff(sampleDiff(), 72, false); again != first {
			t.Fatalf("RenderDiff() = %q, want the stable %q", again, first)
		}
	}
}

func TestRenderDiffRespectsWidth(t *testing.T) {
	for _, width := range []int{12, 24, 40, 72} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			for i, line := range strings.Split(RenderDiff(sampleDiff(), width, false), "\n") {
				if got := lipgloss.Width(line); got > width {
					t.Errorf("line %d = %q is %d cells wide, want at most %d", i, line, got, width)
				}
			}
		})
	}
}

func TestRenderDiffKeepsDomainNumbersAndMarkers(t *testing.T) {
	diff := domain.Diff{Hunks: []domain.DiffHunk{{
		OldStart: 7, OldLines: 2, NewStart: 8, NewLines: 2,
		Lines: []domain.DiffLine{
			{Kind: domain.Context, Text: "contexto", OldLine: 7, NewLine: 8},
			{Kind: domain.Removed, Text: "vieja", OldLine: 8},
			{Kind: domain.Added, Text: "nueva", NewLine: 9},
		},
	}}}

	lines := strings.Split(RenderDiff(diff, 0, false), "\n")
	if len(lines) != 4 {
		t.Fatalf("len(lines) = %d, want 4 lines (header plus three)", len(lines))
	}
	if !strings.Contains(lines[0], "@@ -7,2 +8,2 @@") {
		t.Errorf("header = %q, want the rebuilt @@ header", lines[0])
	}

	for i, want := range []string{"7 8 │  contexto", "8   │ -vieja", "  9 │ +nueva"} {
		if !strings.Contains(lines[i+1], want) {
			t.Errorf("line %d = %q, want it to contain %q", i+1, lines[i+1], want)
		}
	}
}

func TestRenderDiffNoNewlineMarker(t *testing.T) {
	diff := domain.Diff{Hunks: []domain.DiffHunk{{
		OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
		Lines: []domain.DiffLine{
			{Kind: domain.Context, Text: "única", OldLine: 1, NewLine: 1},
			{Kind: domain.NoNewline},
		},
	}}}

	view := RenderDiff(diff, 0, false)
	if !strings.Contains(view, `\ No newline at end of file`) {
		t.Errorf("RenderDiff() = %q, want the no-newline marker", view)
	}
	if got := strings.Count(view, "\n"); got != 2 {
		t.Errorf("RenderDiff() has %d newlines, want 2", got)
	}
}
