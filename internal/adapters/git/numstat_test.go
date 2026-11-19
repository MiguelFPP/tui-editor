package git

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

func TestParseNumStatFixture(t *testing.T) {
	stat := parseNumStat(fixture(t, "numstat.z"))

	want := []domain.RelPath{"bin.dat", "code.go", "crlf.txt", "moved.txt", "noeol.txt", "ta\tb.txt", "uni-é.txt", "with space.txt"}
	if got := statPaths(stat.Files); !slices.Equal(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}

	if stat.IsEmpty() {
		t.Error("IsEmpty() = true, want false")
	}
	if stat.Truncated {
		t.Error("Truncated = true, want false")
	}
	if stat.Additions != 6 || stat.Deletions != 6 {
		t.Errorf("totals = %d/%d, want 6/6", stat.Additions, stat.Deletions)
	}

	binary := stat.Files[0]
	if !binary.Binary || binary.Additions != 0 || binary.Deletions != 0 {
		t.Errorf("bin.dat = %+v, want a binary entry without counts", binary)
	}

	renamed := stat.Files[3]
	if renamed.Path != "moved.txt" || renamed.OrigPath != "renamed.txt" {
		t.Errorf("rename = %+v, want moved.txt from renamed.txt", renamed)
	}

	spaced := stat.Files[7]
	if spaced.Path != "with space.txt" || spaced.Additions != 1 || spaced.Deletions != 1 {
		t.Errorf("with space.txt = %+v, want the path with spaces and its counts", spaced)
	}

	tabbed := stat.Files[5]
	if tabbed.Path != "ta\tb.txt" {
		t.Errorf("tabbed path = %q, want the raw tab preserved", tabbed.Path)
	}
}

func TestParseNumStatSkipsMalformedTokens(t *testing.T) {
	raw := []byte("1\t1\tok.txt\x00" +
		"sin-tabuladores\x00" +
		"a\tb\tmalo.txt\x00" +
		"3\t4\t/tmp/absoluto.txt\x00" +
		"7\t8\totro.txt\x00" +
		"2\t5\t\x00")

	stat := parseNumStat(raw)

	if got, want := statPaths(stat.Files), []domain.RelPath{"ok.txt", "otro.txt"}; !slices.Equal(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	if stat.Additions != 8 || stat.Deletions != 9 {
		t.Errorf("totals = %d/%d, want 8/9", stat.Additions, stat.Deletions)
	}
}

func TestParseNumStatTruncatesAtMaxDiffFiles(t *testing.T) {
	var raw strings.Builder
	for i := range domain.MaxDiffFiles + 3 {
		fmt.Fprintf(&raw, "1\t0\tfile-%05d.txt\x00", i)
	}

	stat := parseNumStat([]byte(raw.String()))
	if !stat.Truncated {
		t.Error("Truncated = false, want true")
	}
	if len(stat.Files) != domain.MaxDiffFiles {
		t.Errorf("len(files) = %d, want %d", len(stat.Files), domain.MaxDiffFiles)
	}
}

func statPaths(files []domain.FileStat) []domain.RelPath {
	got := make([]domain.RelPath, 0, len(files))
	for _, file := range files {
		got = append(got, file.Path)
	}

	return got
}
