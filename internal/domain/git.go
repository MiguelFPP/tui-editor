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
