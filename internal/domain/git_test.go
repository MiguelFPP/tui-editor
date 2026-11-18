package domain

import (
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
