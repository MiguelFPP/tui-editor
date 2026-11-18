package git

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

func TestParseStatusChanges(t *testing.T) {
	status, err := parseStatus(context.Background(), fixture(t, "status_v2_changes.z"))
	if err != nil {
		t.Fatalf("parseStatus() error = %v, want nil", err)
	}

	if status.Branch != "main" || status.Upstream != "origin/main" {
		t.Errorf("branch = %q/%q, want main/origin/main", status.Branch, status.Upstream)
	}
	if status.Ahead != 1 || status.Behind != 0 {
		t.Errorf("ahead/behind = %d/%d, want 1/0", status.Ahead, status.Behind)
	}
	if status.Detached || status.Unborn || status.Truncated {
		t.Errorf("flags = detached:%v unborn:%v truncated:%v, want all false", status.Detached, status.Unborn, status.Truncated)
	}

	want := []domain.RelPath{"normal.txt", "renamed space.txt", "staged.txt", "uni-é.txt", "untracked"}
	if got := paths(status.Files); !slices.Equal(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}

	modified := status.Files[0]
	if modified.Index != domain.GitUnmodified || modified.Worktree != domain.GitModified {
		t.Errorf("normal.txt = %s%s, want .M", modified.Index, modified.Worktree)
	}
	if !modified.Unstaged() || modified.Staged() {
		t.Error("normal.txt must be unstaged only")
	}

	renamed := status.Files[1]
	if renamed.OrigPath != "with space.txt" || !renamed.Renamed() {
		t.Errorf("renamed = %+v, want the pre-image path with spaces", renamed)
	}
	if !renamed.Staged() {
		t.Error("renamed space.txt must be staged")
	}

	staged := status.Files[2]
	if !staged.Staged() || staged.Untracked() {
		t.Errorf("staged.txt = %+v, want a staged addition", staged)
	}

	untracked := status.Files[4]
	if !untracked.Untracked() || !untracked.Unstaged() {
		t.Errorf("untracked/ = %+v, want an untracked directory without the trailing slash", untracked)
	}
}

func TestParseStatusUnmergedAndDetachedAndUnborn(t *testing.T) {
	t.Run("unmerged", func(t *testing.T) {
		status, err := parseStatus(context.Background(), fixture(t, "status_v2_unmerged.z"))
		if err != nil {
			t.Fatalf("parseStatus() error = %v, want nil", err)
		}

		if got := paths(status.Conflicted()); !slices.Equal(got, []domain.RelPath{"conflict.txt"}) {
			t.Fatalf("conflicted = %v, want conflict.txt", got)
		}

		conflict := status.Conflicted()[0]
		if conflict.Index != domain.GitUnmerged || conflict.Worktree != domain.GitUnmerged {
			t.Errorf("conflict = %s%s, want UU", conflict.Index, conflict.Worktree)
		}
		if conflict.Staged() || conflict.Unstaged() || conflict.Untracked() {
			t.Error("a conflicted file is neither staged nor unstaged")
		}

		if got := paths(status.Unstaged()); !slices.Equal(got, []domain.RelPath{"untracked.txt"}) {
			t.Errorf("unstaged = %v, want untracked.txt", got)
		}
	})

	t.Run("detached", func(t *testing.T) {
		status, err := parseStatus(context.Background(), fixture(t, "status_v2_detached.z"))
		if err != nil {
			t.Fatalf("parseStatus() error = %v, want nil", err)
		}

		if !status.Detached {
			t.Error("Detached = false, want true")
		}
		if status.Branch != "" {
			t.Errorf("Branch = %q, want empty on a detached HEAD", status.Branch)
		}
		if status.Unborn || len(status.Files) != 0 {
			t.Errorf("status = %+v, want no files and no unborn flag", status)
		}
	})

	t.Run("unborn", func(t *testing.T) {
		status, err := parseStatus(context.Background(), fixture(t, "status_v2_unborn.z"))
		if err != nil {
			t.Fatalf("parseStatus() error = %v, want nil", err)
		}

		if !status.Unborn {
			t.Error("Unborn = false, want true")
		}
		if status.Branch != "main" || status.Detached {
			t.Errorf("branch = %q, detached = %v, want main and false", status.Branch, status.Detached)
		}
	})
}

func TestParseStatusAcceptsLFHeadersAndSkipsUnknownRecords(t *testing.T) {
	raw := []byte("# branch.head main\n\x00# branch.upstream origin/main\n\x00" +
		"# branch.ab +2 -3\x00! ignored.txt\x00X unknown record\x00? nuevo.txt\x00")

	status, err := parseStatus(context.Background(), raw)
	if err != nil {
		t.Fatalf("parseStatus() error = %v, want nil", err)
	}

	if status.Branch != "main" || status.Upstream != "origin/main" {
		t.Errorf("branch = %q/%q, want main/origin/main", status.Branch, status.Upstream)
	}
	if status.Ahead != 2 || status.Behind != 3 {
		t.Errorf("ahead/behind = %d/%d, want 2/3", status.Ahead, status.Behind)
	}
	if got := paths(status.Files); !slices.Equal(got, []domain.RelPath{"nuevo.txt"}) {
		t.Errorf("files = %v, want only the untracked record", got)
	}
}

func TestParseStatusSkipsMalformedRecords(t *testing.T) {
	raw := []byte("1 .M N...\x00" +
		"1 .M N... 100644 100644 100644 a b /etc/passwd\x00" +
		"u UU N... 100644\x00" +
		"? \x00" +
		"1 .M N... 100644 100644 100644 a b bueno.txt\x00" +
		"2 R. N... 100644 100644 100644 a b R100 destino.txt\x00")

	status, err := parseStatus(context.Background(), raw)
	if err != nil {
		t.Fatalf("parseStatus() error = %v, want nil", err)
	}

	if got := paths(status.Files); !slices.Equal(got, []domain.RelPath{"bueno.txt"}) {
		t.Errorf("files = %v, want only the well-formed record", got)
	}
}

func TestParseStatusTruncatesAtMaxEntries(t *testing.T) {
	var raw strings.Builder
	for i := range domain.MaxStatusEntries + 2 {
		fmt.Fprintf(&raw, "1 .M N... 100644 100644 100644 a b file-%05d.txt\x00", i)
	}

	status, err := parseStatus(context.Background(), []byte(raw.String()))
	if err != nil {
		t.Fatalf("parseStatus() error = %v, want nil", err)
	}

	if !status.Truncated {
		t.Error("Truncated = false, want true")
	}
	if len(status.Files) != domain.MaxStatusEntries {
		t.Errorf("len(files) = %d, want %d", len(status.Files), domain.MaxStatusEntries)
	}
}

func TestParseStatusStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := parseStatus(ctx, fixture(t, "status_v2_changes.z")); !errors.Is(err, context.Canceled) {
		t.Fatalf("parseStatus() error = %v, want %v", err, context.Canceled)
	}
}

func TestParseStatusSignedCounters(t *testing.T) {
	for _, tc := range []struct {
		field string
		want  int
	}{
		{field: "+0", want: 0},
		{field: "+12", want: 12},
		{field: "-3", want: 3},
		{field: "5", want: 5},
		{field: "no", want: 0},
	} {
		if got := parseSigned(tc.field); got != tc.want {
			t.Errorf("parseSigned(%q) = %d, want %d", tc.field, got, tc.want)
		}
	}
}

func paths(files []domain.GitFileStatus) []domain.RelPath {
	got := make([]domain.RelPath, 0, len(files))
	for _, file := range files {
		got = append(got, file.Path)
	}

	return got
}

func TestParseStatusHeadersWithLF(t *testing.T) {
	raw := []byte("# branch.oid abc\n# branch.head main\n# branch.upstream origin/main\n# branch.ab +2 -3\n")

	status, err := parseStatus(context.Background(), raw)
	if err != nil {
		t.Fatalf("parseStatus() error = %v, want nil", err)
	}
	if status.Branch != "main" || status.Upstream != "origin/main" {
		t.Errorf("branch = %q/%q, want main/origin/main", status.Branch, status.Upstream)
	}
	if status.Ahead != 2 || status.Behind != 3 {
		t.Errorf("ahead/behind = %d/%d, want 2/3", status.Ahead, status.Behind)
	}
	if status.Unborn || status.Detached || status.Truncated {
		t.Errorf("flags = unborn:%v detached:%v truncated:%v, want all false", status.Unborn, status.Detached, status.Truncated)
	}
}
