package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGitStatusCodeString(t *testing.T) {
	cases := []struct {
		code GitStatusCode
		want string
	}{
		{code: GitUnmodified, want: "."},
		{code: GitModified, want: "M"},
		{code: GitTypeChange, want: "T"},
		{code: GitAdded, want: "A"},
		{code: GitDeleted, want: "D"},
		{code: GitRenamed, want: "R"},
		{code: GitCopied, want: "C"},
		{code: GitUnmerged, want: "U"},
		{code: GitUntracked, want: "?"},
	}

	for _, tc := range cases {
		if got := tc.code.String(); got != tc.want {
			t.Errorf("GitStatusCode(%q).String() = %q, want %q", byte(tc.code), got, tc.want)
		}
	}
}

func TestGitFileStatusIndexWorktree(t *testing.T) {
	cases := []struct {
		name       string
		index      GitStatusCode
		worktree   GitStatusCode
		conflicted bool
		staged     bool
		unstaged   bool
		untracked  bool
	}{
		{name: "M.", index: GitModified, worktree: GitUnmodified, staged: true},
		{name: ".M", index: GitUnmodified, worktree: GitModified, unstaged: true},
		{name: "MM", index: GitModified, worktree: GitModified, staged: true, unstaged: true},
		{name: "A.", index: GitAdded, worktree: GitUnmodified, staged: true},
		{name: "D.", index: GitDeleted, worktree: GitUnmodified, staged: true},
		{name: ".D", index: GitUnmodified, worktree: GitDeleted, unstaged: true},
		{name: "T.", index: GitTypeChange, worktree: GitUnmodified, staged: true},
		{name: ".T", index: GitUnmodified, worktree: GitTypeChange, unstaged: true},
		{name: "R.", index: GitRenamed, worktree: GitUnmodified, staged: true},
		{name: "C.", index: GitCopied, worktree: GitUnmodified, staged: true},
		{name: "..", index: GitUnmodified, worktree: GitUnmodified},
		{name: "??", index: GitUntracked, worktree: GitUntracked, unstaged: true, untracked: true},
		{name: "UU", index: GitUnmerged, worktree: GitUnmerged, conflicted: true},
		{name: "AA", index: GitAdded, worktree: GitAdded, conflicted: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := GitFileStatus{Index: tc.index, Worktree: tc.worktree, Conflicted: tc.conflicted}

			if got := status.Staged(); got != tc.staged {
				t.Errorf("Staged() = %v, want %v", got, tc.staged)
			}
			if got := status.Unstaged(); got != tc.unstaged {
				t.Errorf("Unstaged() = %v, want %v", got, tc.unstaged)
			}
			if got := status.Untracked(); got != tc.untracked {
				t.Errorf("Untracked() = %v, want %v", got, tc.untracked)
			}
		})
	}
}

func TestGitFileStatusRenamed(t *testing.T) {
	cases := []struct {
		name   string
		status GitFileStatus
		want   bool
	}{
		{name: "index", status: GitFileStatus{Index: GitRenamed}, want: true},
		{name: "worktree", status: GitFileStatus{Worktree: GitRenamed}, want: true},
		{name: "orig path", status: GitFileStatus{OrigPath: "old.txt"}, want: true},
		{name: "modified", status: GitFileStatus{Index: GitModified}},
		{name: "untracked", status: GitFileStatus{Index: GitUntracked, Worktree: GitUntracked}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.status.Renamed(); got != tc.want {
				t.Errorf("Renamed() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGitStatusFilters(t *testing.T) {
	status := GitStatus{
		Branch:   "main",
		Upstream: "origin/main",
		Files: []GitFileStatus{
			{Path: "staged.txt", Index: GitModified, Worktree: GitUnmodified},
			{Path: "mixed.txt", Index: GitModified, Worktree: GitModified},
			{Path: "worktree.txt", Index: GitUnmodified, Worktree: GitModified},
			{Path: "new.txt", Index: GitUntracked, Worktree: GitUntracked},
			{Path: "conflict.txt", Index: GitUnmerged, Worktree: GitUnmerged, Conflicted: true},
		},
	}

	cases := []struct {
		name string
		got  []GitFileStatus
		want []RelPath
	}{
		{name: "staged", got: status.Staged(), want: []RelPath{"staged.txt", "mixed.txt"}},
		{name: "unstaged", got: status.Unstaged(), want: []RelPath{"mixed.txt", "worktree.txt", "new.txt"}},
		{name: "conflicted", got: status.Conflicted(), want: []RelPath{"conflict.txt"}},
	}

	if !status.HasChanges() {
		t.Error("HasChanges() = false, want true")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.got) != len(tc.want) {
				t.Fatalf("got %d files, want %d", len(tc.got), len(tc.want))
			}
			for i, want := range tc.want {
				if tc.got[i].Path != want {
					t.Errorf("file %d = %q, want %q", i, tc.got[i].Path, want)
				}
			}
		})
	}

	empty := GitStatus{}
	if empty.HasChanges() {
		t.Error("empty HasChanges() = true, want false")
	}
	if len(empty.Staged()) != 0 || len(empty.Unstaged()) != 0 || len(empty.Conflicted()) != 0 {
		t.Error("empty status filters returned files, want none")
	}
}

func TestStashEntryRef(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	entry := StashEntry{Index: 2, Message: "WIP editor", Branch: "main", Created: created}

	if got, want := entry.Ref(), "stash@{2}"; got != want {
		t.Errorf("Ref() = %q, want %q", got, want)
	}
	if got, want := (StashEntry{}).Ref(), "stash@{0}"; got != want {
		t.Errorf("zero Ref() = %q, want %q", got, want)
	}
}

func TestMergeResultHasConflicts(t *testing.T) {
	if (MergeResult{Status: Clean}).HasConflicts() {
		t.Error("clean merge reports conflicts")
	}

	result := MergeResult{Status: Conflicted, Conflicts: []Conflict{{Path: "main.go", Kind: BothModified}}}
	if !result.HasConflicts() {
		t.Error("conflicted merge does not report conflicts")
	}
}

func TestCommitQueryNormalized(t *testing.T) {
	cases := []struct {
		name      string
		query     CommitQuery
		limit     int
		skip      int
		unchanged bool
	}{
		{name: "zero limit", query: CommitQuery{}, limit: DefaultCommits},
		{name: "negative limit", query: CommitQuery{Limit: -3}, limit: DefaultCommits},
		{name: "small limit", query: CommitQuery{Limit: 10}, limit: 10, unchanged: true},
		{name: "at max", query: CommitQuery{Limit: MaxCommits}, limit: MaxCommits, unchanged: true},
		{name: "above max", query: CommitQuery{Limit: MaxCommits + 1}, limit: MaxCommits},
		{name: "negative skip", query: CommitQuery{Limit: 5, Skip: -1}, limit: 5, skip: 0},
		{name: "positive skip", query: CommitQuery{Limit: 5, Skip: 40}, limit: 5, skip: 40, unchanged: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.query.Normalized()
			if got.Limit != tc.limit {
				t.Errorf("Limit = %d, want %d", got.Limit, tc.limit)
			}
			if got.Skip != tc.skip {
				t.Errorf("Skip = %d, want %d", got.Skip, tc.skip)
			}
			if same := got == tc.query; same != tc.unchanged {
				t.Errorf("query changed = %v, want %v", same, tc.unchanged)
			}
		})
	}

	// Ref and Path are opaque to the query and survive normalisation.
	query := CommitQuery{Ref: "main", Path: "dir/file.txt", Limit: MaxCommits + 1, Skip: -2}.Normalized()
	if query.Ref != "main" || query.Path != "dir/file.txt" {
		t.Errorf("Normalized() = %+v, want the ref and path preserved", query)
	}
}

func TestCommitDetailTotals(t *testing.T) {
	detail := CommitDetail{Files: []Diff{
		{Path: "a.go", Additions: 3, Deletions: 1},
		{Path: "b.go", Additions: 0, Deletions: 4},
		{Path: "logo.png", Binary: true},
	}}

	additions, deletions := detail.Totals()
	if additions != 3 {
		t.Errorf("additions = %d, want 3", additions)
	}
	if deletions != 5 {
		t.Errorf("deletions = %d, want 5", deletions)
	}

	if additions, deletions := (CommitDetail{}).Totals(); additions != 0 || deletions != 0 {
		t.Errorf("empty Totals() = %d/%d, want 0/0", additions, deletions)
	}
}

func TestDiffStatIsEmpty(t *testing.T) {
	if !(DiffStat{}).IsEmpty() {
		t.Error("DiffStat{}.IsEmpty() = false, want true")
	}
	if (DiffStat{Files: []FileStat{{Path: "a.go", Additions: 1}}}).IsEmpty() {
		t.Error("DiffStat with a file: IsEmpty() = true, want false")
	}

	stat := DiffStat{Files: []FileStat{{Path: "logo.png", Binary: true, OrigPath: "old.png"}}, Additions: 0, Deletions: 0}
	if stat.IsEmpty() {
		t.Error("DiffStat with one binary rename: IsEmpty() = true, want false")
	}
	if !stat.Files[0].Binary || stat.Files[0].OrigPath != "old.png" {
		t.Errorf("FileStat = %+v, want the binary rename preserved", stat.Files[0])
	}
}

func TestConflictKindFor(t *testing.T) {
	cases := []struct {
		name     string
		index    GitStatusCode
		worktree GitStatusCode
		want     ConflictKind
	}{
		{name: "UU", index: GitUnmerged, worktree: GitUnmerged, want: BothModified},
		{name: "AA", index: GitAdded, worktree: GitAdded, want: BothAdded},
		{name: "DD", index: GitDeleted, worktree: GitDeleted, want: BothDeleted},
		{name: "DU", index: GitDeleted, worktree: GitUnmerged, want: DeletedByUs},
		{name: "UD", index: GitUnmerged, worktree: GitDeleted, want: DeletedByThem},
		{name: "AU", index: GitAdded, worktree: GitUnmerged, want: AddedByUs},
		{name: "UA", index: GitUnmerged, worktree: GitAdded, want: AddedByThem},
		{name: "desconocido MM", index: GitModified, worktree: GitModified, want: BothModified},
		{name: "desconocido U.", index: GitUnmerged, worktree: GitUnmodified, want: BothModified},
		{name: "sin códigos", index: GitUnmodified, worktree: GitUnmodified, want: BothModified},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ConflictKindFor(tc.index, tc.worktree); got != tc.want {
				t.Errorf("ConflictKindFor(%s, %s) = %d, want %d", tc.index, tc.worktree, got, tc.want)
			}
		})
	}
}

func TestGitStatusConflicts(t *testing.T) {
	status := GitStatus{Files: []GitFileStatus{
		{Path: "a.txt", Index: GitUnmerged, Worktree: GitUnmerged, Conflicted: true, Kind: BothModified},
		{Path: "b.txt", Index: GitAdded, Worktree: GitAdded, Conflicted: true, Kind: BothAdded},
		{Path: "c.txt", Index: GitModified, Worktree: GitUnmodified},
		{Path: "d.txt", Index: GitUntracked, Worktree: GitUntracked},
	}}

	conflicts := status.Conflicts()
	if len(conflicts) != 2 {
		t.Fatalf("Conflicts() = %+v, want the two unmerged entries", conflicts)
	}
	if conflicts[0].Path != "a.txt" || conflicts[0].Kind != BothModified {
		t.Errorf("conflict 0 = %+v, want a.txt as BothModified", conflicts[0])
	}
	if conflicts[1].Path != "b.txt" || conflicts[1].Kind != BothAdded {
		t.Errorf("conflict 1 = %+v, want b.txt as BothAdded", conflicts[1])
	}

	if got := (GitStatus{}).Conflicts(); len(got) != 0 {
		t.Errorf("empty Conflicts() = %+v, want none", got)
	}
	clean := GitStatus{Files: []GitFileStatus{{Path: "a.txt", Index: GitModified, Worktree: GitModified}}}
	if got := clean.Conflicts(); len(got) != 0 {
		t.Errorf("clean Conflicts() = %+v, want none", got)
	}
}

func TestValidateBranchName(t *testing.T) {
	valid := []string{"main", "feature/f08-git", "release/v1.2.3", "a.b", "fix_1", "ñandú", "v1.2.3", "a-b_c/d.e"}

	for _, name := range valid {
		t.Run("valido "+name, func(t *testing.T) {
			if err := ValidateBranchName(name); err != nil {
				t.Errorf("ValidateBranchName(%q) error = %v, want nil", name, err)
			}
		})
	}

	invalid := []struct {
		name string
		want error
	}{
		{name: "", want: ErrInvalidArgument},
		{name: "   ", want: ErrInvalidArgument},
		{name: "\t", want: ErrInvalidArgument},
		{name: "con espacio", want: ErrInvalidArgument},
		{name: "con\nsalto", want: ErrInvalidArgument},
		{name: "con\x00nul", want: ErrInvalidArgument},
		{name: "-rama", want: ErrInvalidArgument},
		{name: "--todo", want: ErrInvalidArgument},
		{name: "/rama", want: ErrInvalidArgument},
		{name: "rama/", want: ErrInvalidArgument},
		{name: "ra//ma", want: ErrInvalidArgument},
		{name: "rama.", want: ErrInvalidArgument},
		{name: "rama.lock", want: ErrInvalidArgument},
		{name: ".rama", want: ErrInvalidArgument},
		{name: "ra/../ma", want: ErrInvalidArgument},
		{name: "ra..ma", want: ErrInvalidArgument},
		{name: "rama@{1}", want: ErrInvalidArgument},
		{name: "@", want: ErrInvalidArgument},
		{name: "ra~ma", want: ErrInvalidArgument},
		{name: "ra^ma", want: ErrInvalidArgument},
		{name: "ra:ma", want: ErrInvalidArgument},
		{name: "ra?ma", want: ErrInvalidArgument},
		{name: "ra*ma", want: ErrInvalidArgument},
		{name: "ra[ma", want: ErrInvalidArgument},
		{name: `ra\ma`, want: ErrInvalidArgument},
		{name: strings.Repeat("a", MaxBranchNameBytes+1), want: ErrTooLarge},
	}

	for _, tc := range invalid {
		t.Run("invalido "+tc.name, func(t *testing.T) {
			err := ValidateBranchName(tc.name)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ValidateBranchName(%q) error = %v, want %v", tc.name, err, tc.want)
			}
			for _, ban := range []string{"--all", "--mirror", "-D"} {
				if strings.Contains(err.Error(), ban) {
					t.Errorf("ValidateBranchName(%q) = %q, want it to name the problem, not %q", tc.name, err, ban)
				}
			}
		})
	}

	if err := ValidateBranchName(strings.Repeat("a", MaxBranchNameBytes)); err != nil {
		t.Errorf("ValidateBranchName(at the limit) error = %v, want nil", err)
	}
}
