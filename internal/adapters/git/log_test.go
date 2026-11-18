package git

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

func TestParseCommitsFixture(t *testing.T) {
	raw := fixture(t, "log.z")

	page := parseCommits(raw, 4)
	if page.Truncated {
		t.Error("Truncated = true for a page that fits the limit, want false")
	}
	if len(page.Commits) != 4 {
		t.Fatalf("len(commits) = %d, want 4", len(page.Commits))
	}

	first := page.Commits[0]
	if !isHash(first.Hash) || first.ShortHash != first.Hash[:7] {
		t.Errorf("hash = %q/%q, want a full hash and its abbreviation", first.Hash, first.ShortHash)
	}
	if first.ShortHash != "114a649" || first.Subject != "merge side into main" {
		t.Errorf("commit = %+v, want the merge commit", first)
	}
	if first.Author != "Test User" || first.Email != "user@example.com" {
		t.Errorf("author = %q <%q>, want the fixture identity", first.Author, first.Email)
	}

	want := time.Date(2026, 1, 2, 3, 4, 8, 0, time.UTC)
	if !first.Date.Equal(want) {
		t.Errorf("Date = %v, want %v", first.Date, want)
	}

	if got := page.Commits[2].Subject; got != "cambia base y añade binario" {
		t.Errorf("subject = %q, want the accented subject", got)
	}
	if last := page.Commits[3]; last.Subject != "initial commit" {
		t.Errorf("last commit = %+v, want the root commit", last)
	}
}

func TestParseCommitsTruncatesByLimit(t *testing.T) {
	raw := fixture(t, "log.z")

	page := parseCommits(raw, 2)
	if !page.Truncated {
		t.Error("Truncated = false, want true: git was asked for one commit more than the limit")
	}
	if len(page.Commits) != 2 {
		t.Fatalf("len(commits) = %d, want 2", len(page.Commits))
	}

	full := parseCommits(raw, 10)
	if full.Truncated || len(full.Commits) != 4 {
		t.Errorf("page = %+v, want every commit without truncation", full)
	}
}

func TestParseCommitsSkipsIncompleteGroups(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{name: "empty", raw: "", want: 0},
		{name: "separator only", raw: "\n", want: 0},
		{name: "incomplete group", raw: "hash\x00short\x00autor\x00", want: 0},
		{name: "bad hash", raw: "no-es-un-hash\x00no\x00autor\x00mail\x002026-01-02T03:04:05Z\x00asunto\x00", want: 0},
		{name: "unparsable date", raw: strings.Repeat("a", 40) + "\x00aaaaaaa\x00autor\x00mail\x00ayer\x00asunto\x00", want: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := parseCommits([]byte(tc.raw), 5)
			if len(page.Commits) != tc.want {
				t.Fatalf("len(commits) = %d, want %d", len(page.Commits), tc.want)
			}
			if tc.want == 1 && !page.Commits[0].Date.IsZero() {
				t.Errorf("Date = %v, want the unparsable date ignored", page.Commits[0].Date)
			}
		})
	}
}

func TestParseShowFixture(t *testing.T) {
	detail := parseShow(fixture(t, "show_commit.txt"))

	if detail.Commit.ShortHash != "4ef2705" || detail.Commit.Subject != "cambia base y añade binario" {
		t.Errorf("commit = %+v, want the fixture commit", detail.Commit)
	}
	if detail.Truncated {
		t.Error("Truncated = true, want false")
	}
	if want := "Cuerpo del commit con una línea extra\ny una segunda línea del cuerpo."; detail.Body != want {
		t.Errorf("Body = %q, want %q", detail.Body, want)
	}
	if got, want := diffPaths(detail.Files), []domain.RelPath{"base.txt", "bin.dat"}; !slices.Equal(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	if additions, deletions := detail.Totals(); additions != 1 || deletions != 1 {
		t.Errorf("Totals() = %d/%d, want 1/1", additions, deletions)
	}
	if !detail.Files[1].Binary {
		t.Error("the binary file must be marked as such")
	}
}

func TestParseShowMergeWithoutPatch(t *testing.T) {
	detail := parseShow(fixture(t, "show_merge.txt"))

	if detail.Commit.Subject != "merge side into main" {
		t.Errorf("subject = %q, want the merge commit", detail.Commit.Subject)
	}
	if detail.Body != "" || len(detail.Files) != 0 || detail.Truncated {
		t.Errorf("detail = %+v, want metadata only", detail)
	}
	if additions, deletions := detail.Totals(); additions != 0 || deletions != 0 {
		t.Errorf("Totals() = %d/%d, want 0/0", additions, deletions)
	}
}

func TestParseShowMalformedInput(t *testing.T) {
	for _, raw := range []string{"", "solo\x00dos\x00tres\x00", "\x00\x00\x00\x00\x00\x00"} {
		if detail := parseShow([]byte(raw)); len(detail.Files) != 0 || detail.Body != "" {
			t.Errorf("parseShow(%q) = %+v, want the zero detail", raw, detail)
		}
	}
}

func TestParseShowTruncatesFilesAndLines(t *testing.T) {
	metadata := strings.Repeat("a", 40) + "\x00aaaaaaa\x00Test User\x00user@example.com\x002026-01-02T03:04:05Z\x00asunto\x00\x00"

	t.Run("files", func(t *testing.T) {
		var raw strings.Builder
		raw.WriteString(metadata)
		for range domain.MaxDiffFiles + 3 {
			raw.WriteString("\ndiff --git f.txt f.txt\n--- f.txt\n+++ f.txt\n@@ -1 +1 @@\n-a\n+b\n")
		}

		detail := parseShow([]byte(raw.String()))
		if !detail.Truncated {
			t.Error("Truncated = false, want true")
		}
		if len(detail.Files) != domain.MaxDiffFiles {
			t.Errorf("len(files) = %d, want %d", len(detail.Files), domain.MaxDiffFiles)
		}
	})

	t.Run("lines", func(t *testing.T) {
		var raw strings.Builder
		raw.WriteString(metadata)
		raw.WriteString("\ndiff --git big.txt big.txt\n--- big.txt\n+++ big.txt\n@@ -1,1 +1,1 @@\n")
		for range domain.MaxDiffLines + 1 {
			raw.WriteString("+linea\n")
		}

		detail := parseShow([]byte(raw.String()))
		if !detail.Truncated {
			t.Error("Truncated = false, want true")
		}
		if len(detail.Files) != 1 || len(detail.Files[0].Hunks[0].Lines) != domain.MaxDiffLines {
			t.Errorf("files = %+v, want one file capped at %d lines", detail.Files, domain.MaxDiffLines)
		}
	})
}
