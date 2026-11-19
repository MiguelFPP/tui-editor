package git

import (
	"errors"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// hunkDiff is one modification with a context line, a removal and an addition.
func hunkDiff() domain.Diff {
	return domain.Diff{
		Path: "code.go",
		Hunks: []domain.DiffHunk{{
			Header:   `@@ -4,2 +4,2 @@ import "fmt"`,
			OldStart: 4, OldLines: 2, NewStart: 4, NewLines: 2,
			Section: `import "fmt"`,
			Lines: []domain.DiffLine{
				{Kind: domain.Context, Text: "", OldLine: 4, NewLine: 4},
				{Kind: domain.Removed, Text: "\tprintln(\"viejo\")", OldLine: 5},
				{Kind: domain.Added, Text: "\tprintln(\"nuevo\")", NewLine: 5},
			},
		}},
	}
}

func TestBuildPatchRendersAModifiedFile(t *testing.T) {
	patch, err := buildPatch(hunkDiff(), []int{0})
	if err != nil {
		t.Fatalf("buildPatch() error = %v, want nil", err)
	}

	want := "diff --git a/code.go b/code.go\n" +
		"--- a/code.go\n" +
		"+++ b/code.go\n" +
		"@@ -4,2 +4,2 @@ import \"fmt\"\n" +
		" \n" +
		"-\tprintln(\"viejo\")\n" +
		"+\tprintln(\"nuevo\")\n"
	if string(patch) != want {
		t.Errorf("buildPatch() =\n%q\nwant\n%q", patch, want)
	}
}

func TestBuildPatchRendersNewAndDeletedFiles(t *testing.T) {
	t.Run("new", func(t *testing.T) {
		diff := domain.Diff{
			Path: "nuevo.txt",
			New:  true,
			Hunks: []domain.DiffHunk{{
				OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 2,
				Lines: []domain.DiffLine{
					{Kind: domain.Added, Text: "uno", NewLine: 1},
					{Kind: domain.Added, Text: "dos", NewLine: 2},
				},
			}},
		}

		patch, err := buildPatch(diff, []int{0})
		if err != nil {
			t.Fatalf("buildPatch() error = %v, want nil", err)
		}

		want := "diff --git a/nuevo.txt b/nuevo.txt\n" +
			"new file mode 100644\n" +
			"--- /dev/null\n" +
			"+++ b/nuevo.txt\n" +
			"@@ -0,0 +1,2 @@\n" +
			"+uno\n" +
			"+dos\n"
		if string(patch) != want {
			t.Errorf("buildPatch() =\n%q\nwant\n%q", patch, want)
		}
	})

	t.Run("deleted", func(t *testing.T) {
		diff := domain.Diff{
			Path:    "ido.txt",
			Deleted: true,
			Hunks: []domain.DiffHunk{{
				OldStart: 1, OldLines: 2, NewStart: 0, NewLines: 0,
				Lines: []domain.DiffLine{
					{Kind: domain.Removed, Text: "uno", OldLine: 1},
					{Kind: domain.Removed, Text: "dos", OldLine: 2},
				},
			}},
		}

		patch, err := buildPatch(diff, []int{0})
		if err != nil {
			t.Fatalf("buildPatch() error = %v, want nil", err)
		}

		want := "diff --git a/ido.txt b/ido.txt\n" +
			"deleted file mode 100644\n" +
			"--- a/ido.txt\n" +
			"+++ /dev/null\n" +
			"@@ -1,2 +0,0 @@\n" +
			"-uno\n" +
			"-dos\n"
		if string(patch) != want {
			t.Errorf("buildPatch() =\n%q\nwant\n%q", patch, want)
		}
	})
}

func TestBuildPatchKeepsCRLFAndTheNoNewlineMarker(t *testing.T) {
	diff := domain.Diff{
		Path: "crlf.txt",
		Hunks: []domain.DiffHunk{{
			OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
			Lines: []domain.DiffLine{
				{Kind: domain.Removed, Text: "y\r", OldLine: 1},
				{Kind: domain.Added, Text: "z\r", NewLine: 1},
				{Kind: domain.Context, Text: "final\r", OldLine: 2, NewLine: 2},
				{Kind: domain.NoNewline},
			},
		}},
	}

	patch, err := buildPatch(diff, []int{0})
	if err != nil {
		t.Fatalf("buildPatch() error = %v, want nil", err)
	}

	want := "diff --git a/crlf.txt b/crlf.txt\n" +
		"--- a/crlf.txt\n" +
		"+++ b/crlf.txt\n" +
		"@@ -1,2 +1,2 @@\n" +
		"-y\r\n" +
		"+z\r\n" +
		" final\r\n" +
		"\\ No newline at end of file\n"
	if string(patch) != want {
		t.Errorf("buildPatch() =\n%q\nwant\n%q", patch, want)
	}
}

func TestBuildPatchRecountsRangesFromTheLines(t *testing.T) {
	diff := hunkDiff()
	diff.Hunks[0].OldLines = 99
	diff.Hunks[0].NewLines = 99
	diff.Hunks[0].Section = ""

	patch, err := buildPatch(diff, []int{0})
	if err != nil {
		t.Fatalf("buildPatch() error = %v, want nil", err)
	}
	if !strings.Contains(string(patch), "@@ -4,2 +4,2 @@\n") {
		t.Errorf("buildPatch() =\n%q\nwant the recounted hunk header", patch)
	}
	if strings.Contains(string(patch), "@@ -4,99") {
		t.Errorf("buildPatch() =\n%q, want the declared counts ignored", patch)
	}
}

func TestBuildPatchOrdersAndDeduplicatesHunks(t *testing.T) {
	diff := domain.Diff{Path: "code.go", Hunks: []domain.DiffHunk{
		{OldStart: 1, NewStart: 1, Lines: []domain.DiffLine{{Kind: domain.Added, Text: "uno", NewLine: 1}}},
		{OldStart: 20, NewStart: 20, Lines: []domain.DiffLine{{Kind: domain.Added, Text: "dos", NewLine: 20}}},
		{OldStart: 40, NewStart: 40, Lines: []domain.DiffLine{{Kind: domain.Added, Text: "tres", NewLine: 40}}},
	}}

	patch, err := buildPatch(diff, []int{2, 0, 2})
	if err != nil {
		t.Fatalf("buildPatch() error = %v, want nil", err)
	}

	body := string(patch)
	if got := strings.Count(body, "@@ -1,0 +1,1 @@"); got != 1 {
		t.Errorf("the first hunk appears %d times, want 1: %q", got, body)
	}
	if got := strings.Count(body, "@@ -40,0 +40,1 @@"); got != 1 {
		t.Errorf("the third hunk appears %d times, want 1: %q", got, body)
	}
	if strings.Contains(body, "dos") {
		t.Errorf("buildPatch() =\n%q, want the unselected hunk left out", body)
	}
	if strings.Index(body, "uno") > strings.Index(body, "tres") {
		t.Errorf("buildPatch() =\n%q, want the hunks in diff order", body)
	}
}

func TestBuildPatchRejectsUnsupportedRequests(t *testing.T) {
	binary := hunkDiff()
	binary.Binary = true
	truncated := hunkDiff()
	truncated.Truncated = true
	both := hunkDiff()
	both.New, both.Deleted = true, true
	renamed := hunkDiff()
	renamed.OrigPath = "viejo.go"
	noPath := hunkDiff()
	noPath.Path = ""

	cases := []struct {
		name  string
		diff  domain.Diff
		hunks []int
	}{
		{name: "zero path", diff: noPath, hunks: []int{0}},
		{name: "binary", diff: binary, hunks: []int{0}},
		{name: "truncated", diff: truncated, hunks: []int{0}},
		{name: "new and deleted", diff: both, hunks: []int{0}},
		{name: "rename", diff: renamed, hunks: []int{0}},
		{name: "no hunks selected", diff: hunkDiff(), hunks: nil},
		{name: "empty diff", diff: domain.Diff{Path: "code.go"}, hunks: []int{0}},
		{name: "index out of range", diff: hunkDiff(), hunks: []int{1}},
		{name: "negative index", diff: hunkDiff(), hunks: []int{-1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patch, err := buildPatch(tc.diff, tc.hunks)
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("buildPatch() error = %v, want %v", err, domain.ErrInvalidArgument)
			}
			if patch != nil {
				t.Errorf("buildPatch() = %q, want no patch", patch)
			}
		})
	}
}

func TestBuildPatchRejectsOversizedPatches(t *testing.T) {
	diff := domain.Diff{Path: "grande.txt", Hunks: []domain.DiffHunk{{
		OldStart: 1, NewStart: 1,
		Lines: []domain.DiffLine{{Kind: domain.Added, Text: strings.Repeat("x", domain.MaxPatchBytes)}},
	}}}

	patch, err := buildPatch(diff, []int{0})
	if !errors.Is(err, domain.ErrTooLarge) {
		t.Fatalf("buildPatch() error = %v, want %v", err, domain.ErrTooLarge)
	}
	if patch != nil {
		t.Errorf("buildPatch() = %d bytes, want no patch", len(patch))
	}
}

func TestQuotePathMatchesGit(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "plain", path: "dir/file.txt", want: "dir/file.txt"},
		{name: "empty", path: "", want: `""`},
		{name: "inner space", path: "a b/c d.txt", want: "a b/c d.txt"},
		{name: "leading space", path: " a.txt", want: `" a.txt"`},
		{name: "trailing space", path: "a.txt ", want: `"a.txt "`},
		{name: "utf8", path: "uni-é.txt", want: "uni-é.txt"},
		{name: "tab", path: "ta\tb.txt", want: `"ta\tb.txt"`},
		{name: "newline", path: "a\nb.txt", want: `"a\nb.txt"`},
		{name: "quote", path: `a"b.txt`, want: `"a\"b.txt"`},
		{name: "backslash", path: `a\b.txt`, want: `"a\\b.txt"`},
		{name: "control", path: "a\x01b.txt", want: `"a\001b.txt"`},
		{name: "delete byte", path: "a\x7fb.txt", want: `"a\177b.txt"`},
		{name: "carriage return", path: "a\rb.txt", want: `"a\rb.txt"`},
		{name: "vision bell", path: "a\ab.txt", want: `"a\ab.txt"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := quotePath(tc.path); got != tc.want {
				t.Errorf("quotePath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestQuotePathRoundTripsThroughUnquotePath(t *testing.T) {
	paths := []string{
		"dir/file.txt",
		"with space.txt",
		" espaciado.txt",
		"espaciado.txt ",
		"uni-é.txt",
		"ta\tb.txt",
		`a"b\c.txt`,
		"a\x01b.txt",
		"a\nb.txt",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			if got := unquotePath(quotePath(path)); got != path {
				t.Errorf("unquotePath(quotePath(%q)) = %q, want the original path", path, got)
			}
		})
	}
}

func TestQuotePathLeavesInnerSpacesLiteral(t *testing.T) {
	path := "a/with space.txt"

	if got := quotePath(path); got != path {
		t.Errorf("quotePath(%q) = %q, want inner spaces literal", path, got)
	}
}
