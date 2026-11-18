package git

import (
	"slices"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

func TestParsePatchBasic(t *testing.T) {
	files := parsePatch(string(fixture(t, "diff_basic.patch")))
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}

	file := files[0]
	if file.Path != "code.go" || file.OrigPath != "" {
		t.Errorf("paths = %q/%q, want code.go and no pre-image", file.Path, file.OrigPath)
	}
	if file.Binary || file.Truncated || file.Additions != 1 || file.Deletions != 1 {
		t.Errorf("flags = %+v, want one addition and one deletion", file)
	}
	if len(file.Hunks) != 1 {
		t.Fatalf("len(hunks) = %d, want 1", len(file.Hunks))
	}

	hunk := file.Hunks[0]
	if hunk.Header != `@@ -4,6 +4,6 @@ import "fmt"` || hunk.Section != `import "fmt"` {
		t.Errorf("header = %q section = %q, want the parsed section", hunk.Header, hunk.Section)
	}
	if hunk.OldStart != 4 || hunk.OldLines != 6 || hunk.NewStart != 4 || hunk.NewLines != 6 {
		t.Errorf("range = %d,%d/%d,%d, want 4,6/4,6", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
	}

	removed, added := hunk.Lines[3], hunk.Lines[4]
	if removed.Kind != domain.Removed || removed.OldLine != 7 || removed.NewLine != 0 {
		t.Errorf("removed = %+v, want old line 7", removed)
	}
	if removed.Text != "\tfmt.Println(\"mundo\")" {
		t.Errorf("removed text = %q, want the line without the diff prefix", removed.Text)
	}
	if added.Kind != domain.Added || added.NewLine != 7 || added.OldLine != 0 {
		t.Errorf("added = %+v, want new line 7", added)
	}

	first := hunk.Lines[0]
	if first.Kind != domain.Context || first.OldLine != 4 || first.NewLine != 4 || first.Text != "" {
		t.Errorf("first line = %+v, want the empty context line 4", first)
	}
}

func TestParsePatchNoNewline(t *testing.T) {
	file := parsePatch(string(fixture(t, "diff_no_newline.patch")))[0]

	lines := file.Hunks[0].Lines
	last := lines[len(lines)-1]
	if last.Kind != domain.NoNewline {
		t.Errorf("last line = %+v, want the no-newline marker", last)
	}
	if last.Text != "" || last.OldLine != 0 || last.NewLine != 0 {
		t.Errorf("marker = %+v, want no text and no line numbers", last)
	}
	if file.Additions != 1 || file.Deletions != 1 {
		t.Errorf("counts = %d/%d, want 1/1", file.Additions, file.Deletions)
	}
}

func TestParsePatchKeepsCRLF(t *testing.T) {
	file := parsePatch(string(fixture(t, "diff_crlf.patch")))[0]

	got := texts(file)
	if want := []string{"x\r", "y\r", "z\r"}; !slices.Equal(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
	if strings.Contains(strings.Join(got, ""), "\n") {
		t.Error("a parsed line kept a newline")
	}
}

func TestParsePatchPathsWithSpacesUnicodeAndQuoting(t *testing.T) {
	cases := []struct {
		fixture string
		path    domain.RelPath
	}{
		{fixture: "diff_spaces.patch", path: "with space.txt"},
		{fixture: "diff_utf8.patch", path: "uni-é.txt"},
		{fixture: "diff_quoted_path.patch", path: "ta\tb.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			files := parsePatch(string(fixture(t, tc.fixture)))
			if len(files) != 1 {
				t.Fatalf("len(files) = %d, want 1", len(files))
			}
			if files[0].Path != tc.path {
				t.Errorf("Path = %q, want %q", files[0].Path, tc.path)
			}
			if files[0].OrigPath != "" {
				t.Errorf("OrigPath = %q, want empty", files[0].OrigPath)
			}
			if files[0].Additions != 1 || files[0].Deletions != 1 {
				t.Errorf("counts = %d/%d, want 1/1", files[0].Additions, files[0].Deletions)
			}
		})
	}
}

func TestParsePatchBinaryAndRename(t *testing.T) {
	t.Run("binary", func(t *testing.T) {
		file := parsePatch(string(fixture(t, "diff_binary.patch")))[0]

		if !file.Binary {
			t.Error("Binary = false, want true")
		}
		if file.Path != "bin.dat" {
			t.Errorf("Path = %q, want bin.dat", file.Path)
		}
		if file.IsEmpty() {
			t.Error("IsEmpty() = true for a binary diff, want false")
		}
	})

	t.Run("rename", func(t *testing.T) {
		file := parsePatch(string(fixture(t, "diff_rename.patch")))[0]

		if file.Path != "moved.txt" || file.OrigPath != "renamed.txt" {
			t.Errorf("paths = %q/%q, want moved.txt from renamed.txt", file.Path, file.OrigPath)
		}
		if file.Binary || len(file.Hunks) != 0 {
			t.Errorf("file = %+v, want a pure rename without hunks", file)
		}
	})
}

func TestParsePatchSeveralFiles(t *testing.T) {
	parts := strings.SplitN(string(fixture(t, "show_commit.txt")), "\x00", showFields+1)
	if len(parts) <= showFields {
		t.Fatalf("the fixture has no patch part: %d tokens", len(parts))
	}

	files := parsePatch(parts[showFields])
	if got, want := diffPaths(files), []domain.RelPath{"base.txt", "bin.dat"}; !slices.Equal(got, want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	if !files[1].Binary {
		t.Error("the second file must be binary")
	}
	if files[0].Additions != 1 || files[0].Deletions != 1 {
		t.Errorf("base.txt counts = %d/%d, want 1/1", files[0].Additions, files[0].Deletions)
	}
}

func TestParsePatchDeletedFileKeepsTheOldPath(t *testing.T) {
	raw := "diff --git gone.txt gone.txt\n" +
		"deleted file mode 100644\n" +
		"index 15bf608..0000000\n" +
		"--- gone.txt\n" +
		"+++ /dev/null\n" +
		"@@ -1,2 +0,0 @@\n" +
		"-uno\n" +
		"-dos\n"

	files := parsePatch(raw)
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if files[0].Path != "gone.txt" || files[0].OrigPath != "" {
		t.Errorf("paths = %q/%q, want the pre-image as the only path", files[0].Path, files[0].OrigPath)
	}
	if files[0].Additions != 0 || files[0].Deletions != 2 {
		t.Errorf("counts = %d/%d, want 0/2", files[0].Additions, files[0].Deletions)
	}
}

func TestParsePatchTruncatesAtMaxDiffLines(t *testing.T) {
	var raw strings.Builder
	raw.WriteString("diff --git big.txt big.txt\n--- big.txt\n+++ big.txt\n@@ -1,1 +1,1 @@\n")
	for range domain.MaxDiffLines + 5 {
		raw.WriteString("+linea\n")
	}

	file := parsePatch(raw.String())[0]
	if !file.Truncated {
		t.Error("Truncated = false, want true")
	}
	if got := len(file.Hunks[0].Lines); got != domain.MaxDiffLines {
		t.Errorf("len(lines) = %d, want %d", got, domain.MaxDiffLines)
	}
}

func TestParsePatchIgnoresGarbage(t *testing.T) {
	if files := parsePatch(""); files != nil {
		t.Errorf("parsePatch(\"\") = %v, want nil", files)
	}
	if files := parsePatch("index 15bf608..0000000\nrandom text\n@@ malformed @@\n"); files != nil {
		t.Errorf("parsePatch(garbage) = %v, want nil", files)
	}

	files := parsePatch("diff --git a.txt a.txt\nserial: 1\n@@ -1 +1 @@\n-a\n+b\n")
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want the file with a hunk", len(files))
	}
	if len(files[0].Hunks) != 1 || files[0].Hunks[0].OldLines != 1 || files[0].Hunks[0].NewLines != 1 {
		t.Errorf("hunk = %+v, want the single-line range without an explicit count", files[0].Hunks)
	}
}

func TestParsePatchHunkHeaders(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		want    domain.DiffHunk
		section string
		ok      bool
	}{
		{
			name:    "with section",
			line:    "@@ -12,5 +12,6 @@ func main() {",
			want:    domain.DiffHunk{OldStart: 12, OldLines: 5, NewStart: 12, NewLines: 6},
			section: "func main() {",
			ok:      true,
		},
		{
			name: "without counts",
			line: "@@ -1 +1 @@",
			want: domain.DiffHunk{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1},
			ok:   true,
		},
		{
			name: "empty range",
			line: "@@ -0,0 +1,4 @@",
			want: domain.DiffHunk{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 4},
			ok:   true,
		},
		{name: "not a hunk", line: "@@ malformed @@"},
		{name: "no plus", line: "@@ -1,1 @@"},
		{name: "text count", line: "@@ -a,b +c,d @@"},
		{name: "combined diff", line: "@@@ -1,1 -1,1 +1,1 @@@"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseHunkHeader(tc.line)
			if ok != tc.ok {
				t.Fatalf("parseHunkHeader(%q) ok = %v, want %v", tc.line, ok, tc.ok)
			}
			if !ok {
				return
			}

			want := tc.want
			want.Header = tc.line
			want.Section = tc.section
			if got.Header != want.Header || got.Section != want.Section || got.OldStart != want.OldStart ||
				got.OldLines != want.OldLines || got.NewStart != want.NewStart || got.NewLines != want.NewLines {
				t.Errorf("parseHunkHeader(%q) = %+v, want %+v", tc.line, got, want)
			}
		})
	}
}

func TestParsePatchUnquotesGitPaths(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain", raw: "dir/file.txt", want: "dir/file.txt"},
		{name: "empty", raw: ""},
		{name: "tab", raw: `"ta\tb.txt"`, want: "ta\tb.txt"},
		{name: "quote and backslash", raw: `"a\"b\\c"`, want: `a"b\c`},
		{name: "newline", raw: `"a\nb"`, want: "a\nb"},
		{name: "bell", raw: `"a\ab"`, want: "a\ab"},
		{name: "octal utf8", raw: `"uni-\303\251.txt"`, want: "uni-é.txt"},
		{name: "two digit octal", raw: `"a\61b"`, want: "a1b"},
		{name: "trailing backslash", raw: `"a\`, want: `"a\`},
		{name: "unterminated quote", raw: `"a`, want: `"a`},
		{name: "invalid utf8", raw: "a\xffb", want: "a\uFFFDb"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unquotePath(tc.raw); got != tc.want {
				t.Errorf("unquotePath(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestSplitDiffPaths(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		wantOld string
		wantNew string
	}{
		{name: "identical", line: "file.txt file.txt", wantOld: "file.txt", wantNew: "file.txt"},
		{name: "renamed", line: "old.txt new.txt", wantOld: "old.txt", wantNew: "new.txt"},
		{name: "spaces", line: "with space.txt with space.txt", wantOld: "with space.txt", wantNew: "with space.txt"},
		{name: "quoted", line: `"ta\tb.txt" "ta\tb.txt"`, wantOld: "ta\tb.txt", wantNew: "ta\tb.txt"},
		{name: "utf8 quoted", line: `"uni-\303\251.txt" "uni-\303\251.txt"`, wantOld: "uni-é.txt", wantNew: "uni-é.txt"},
		{name: "single path", line: "only.txt", wantOld: "only.txt", wantNew: "only.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old, new := splitDiffPaths(tc.line)
			if old != tc.wantOld || new != tc.wantNew {
				t.Errorf("splitDiffPaths(%q) = %q/%q, want %q/%q", tc.line, old, new, tc.wantOld, tc.wantNew)
			}
		})
	}
}
