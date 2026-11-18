package domain

import (
	"fmt"
	"time"
)

// GitStatusCode is one status character of `git status --porcelain=v2`.
type GitStatusCode byte

const (
	GitUnmodified GitStatusCode = '.'
	GitModified   GitStatusCode = 'M'
	GitTypeChange GitStatusCode = 'T'
	GitAdded      GitStatusCode = 'A'
	GitDeleted    GitStatusCode = 'D'
	GitRenamed    GitStatusCode = 'R'
	GitCopied     GitStatusCode = 'C'
	GitUnmerged   GitStatusCode = 'U'
	GitUntracked  GitStatusCode = '?'
)

func (c GitStatusCode) String() string {
	return string(rune(c))
}

// GitFileStatus is the index and worktree state of one path.
type GitFileStatus struct {
	Path       RelPath
	OrigPath   RelPath
	Index      GitStatusCode
	Worktree   GitStatusCode
	Conflicted bool
}

// Staged reports an index change; conflicted entries are neither staged nor unstaged.
func (s GitFileStatus) Staged() bool {
	if s.Conflicted {
		return false
	}

	switch s.Index {
	case GitModified, GitTypeChange, GitAdded, GitDeleted, GitRenamed, GitCopied:
		return true
	}

	return false
}

// Unstaged reports worktree changes, including untracked files.
func (s GitFileStatus) Unstaged() bool {
	if s.Conflicted {
		return false
	}
	if s.Untracked() {
		return true
	}

	switch s.Worktree {
	case GitModified, GitTypeChange, GitDeleted:
		return true
	}

	return false
}

func (s GitFileStatus) Untracked() bool {
	if s.Conflicted {
		return false
	}

	return s.Index == GitUntracked || s.Worktree == GitUntracked
}

func (s GitFileStatus) Renamed() bool {
	return s.Index == GitRenamed || s.Worktree == GitRenamed || !s.OrigPath.IsZero()
}

// GitStatus is the bounded snapshot of the working tree.
type GitStatus struct {
	Branch    string
	Upstream  string
	Ahead     int
	Behind    int
	Detached  bool
	Unborn    bool
	Files     []GitFileStatus
	Truncated bool
}

func (s GitStatus) HasChanges() bool {
	return len(s.Files) > 0
}

func (s GitStatus) Staged() []GitFileStatus {
	return filterFiles(s.Files, GitFileStatus.Staged)
}

func (s GitStatus) Unstaged() []GitFileStatus {
	return filterFiles(s.Files, GitFileStatus.Unstaged)
}

func (s GitStatus) Conflicted() []GitFileStatus {
	return filterFiles(s.Files, func(f GitFileStatus) bool { return f.Conflicted })
}

func filterFiles(files []GitFileStatus, keep func(GitFileStatus) bool) []GitFileStatus {
	var kept []GitFileStatus

	for _, file := range files {
		if keep(file) {
			kept = append(kept, file)
		}
	}

	return kept
}

type Branch struct {
	Name     string
	Upstream string
	Current  bool
	Gone     bool
	Ahead    int
	Behind   int
}

type StashEntry struct {
	Index   int
	Message string
	Branch  string
	Created time.Time
}

func (e StashEntry) Ref() string {
	return fmt.Sprintf("stash@{%d}", e.Index)
}

// ConflictKind is the unmerged state pair reported by porcelain v2.
type ConflictKind uint8

const (
	BothModified ConflictKind = iota
	BothAdded
	BothDeleted
	DeletedByUs
	DeletedByThem
	AddedByUs
	AddedByThem
)

type Conflict struct {
	Path RelPath
	Kind ConflictKind
}

type Resolution uint8

const (
	Ours Resolution = iota
	Theirs
)

type MergeStatus uint8

const (
	Clean MergeStatus = iota
	FastForward
	UpToDate
	Conflicted
)

// MergeResult is the outcome of a merge or pull; conflicts are never resolved automatically.
type MergeResult struct {
	Status    MergeStatus
	Conflicts []Conflict
}

func (r MergeResult) HasConflicts() bool {
	return len(r.Conflicts) > 0
}

// Commit is one entry of a bounded history page.
type Commit struct {
	Hash      string
	ShortHash string
	Author    string
	Email     string
	Subject   string
	Date      time.Time
}

// CommitQuery selects one page of history, optionally limited to one path.
type CommitQuery struct {
	Ref   string
	Path  RelPath
	Limit int
	Skip  int
}

// Normalized clamps the page size to the domain limits.
func (q CommitQuery) Normalized() CommitQuery {
	switch {
	case q.Limit <= 0:
		q.Limit = DefaultCommits
	case q.Limit > MaxCommits:
		q.Limit = MaxCommits
	}

	if q.Skip < 0 {
		q.Skip = 0
	}

	return q
}

// CommitPage is one page of history; Truncated reports commits left out.
type CommitPage struct {
	Commits   []Commit
	Truncated bool
}

// CommitDetail is one commit with its message body and its per-file diffs.
type CommitDetail struct {
	Commit    Commit
	Body      string
	Files     []Diff
	Truncated bool
}

// Totals sums the line changes of every file in the commit.
func (d CommitDetail) Totals() (additions, deletions int) {
	for _, file := range d.Files {
		additions += file.Additions
		deletions += file.Deletions
	}

	return additions, deletions
}

// FileStat is the numstat entry of one path.
type FileStat struct {
	Path      RelPath
	OrigPath  RelPath
	Binary    bool
	Additions int
	Deletions int
}

// DiffStat is the bounded numstat summary of the worktree or of the index.
type DiffStat struct {
	Files     []FileStat
	Additions int
	Deletions int
	Truncated bool
}

func (s DiffStat) IsEmpty() bool {
	return len(s.Files) == 0
}
