package domain

import (
	"fmt"
	"strings"
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
	// Kind is the unmerged state pair, set only when Conflicted is true.
	Kind ConflictKind
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

// Equal reports whether both snapshots describe the same working tree.
func (s GitStatus) Equal(other GitStatus) bool {
	if s.Branch != other.Branch || s.Upstream != other.Upstream {
		return false
	}
	if s.Ahead != other.Ahead || s.Behind != other.Behind {
		return false
	}
	if s.Detached != other.Detached || s.Unborn != other.Unborn || s.Truncated != other.Truncated {
		return false
	}
	if len(s.Files) != len(other.Files) {
		return false
	}

	for i, file := range s.Files {
		if file != other.Files[i] {
			return false
		}
	}

	return true
}

// SameRefs reports whether both snapshots point at the same branch and the same upstream counts;
// the files are ignored, so a worktree edit alone never changes it.
func (s GitStatus) SameRefs(other GitStatus) bool {
	return s.Branch == other.Branch && s.Upstream == other.Upstream &&
		s.Ahead == other.Ahead && s.Behind == other.Behind &&
		s.Detached == other.Detached && s.Unborn == other.Unborn
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

// Conflicts lists the unmerged entries with the kind of the porcelain v2 pair.
func (s GitStatus) Conflicts() []Conflict {
	var conflicts []Conflict

	for _, file := range s.Files {
		if file.Conflicted {
			conflicts = append(conflicts, Conflict{Path: file.Path, Kind: file.Kind})
		}
	}

	return conflicts
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

// ConflictKindFor maps the two status codes of an unmerged record onto its kind; an unknown pair
// is reported as BothModified.
func ConflictKindFor(index, worktree GitStatusCode) ConflictKind {
	switch {
	case index == GitUnmerged && worktree == GitUnmerged:
		return BothModified
	case index == GitAdded && worktree == GitAdded:
		return BothAdded
	case index == GitDeleted && worktree == GitDeleted:
		return BothDeleted
	case index == GitDeleted && worktree == GitUnmerged:
		return DeletedByUs
	case index == GitUnmerged && worktree == GitDeleted:
		return DeletedByThem
	case index == GitAdded && worktree == GitUnmerged:
		return AddedByUs
	case index == GitUnmerged && worktree == GitAdded:
		return AddedByThem
	default:
		return BothModified
	}
}

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

// refForbidden are the characters git refuses in a reference name.
const refForbidden = "~^:?*[\\"

// ValidateBranchName rejects anything git would refuse as a local branch name and anything that
// could be read as an option or as a path instead of a reference.
func ValidateBranchName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return fmt.Errorf("%w: el nombre de la rama está vacío", ErrInvalidArgument)
	case len(name) > MaxBranchNameBytes:
		return fmt.Errorf("%w: el nombre de la rama supera %d bytes", ErrTooLarge, MaxBranchNameBytes)
	case strings.ContainsRune(name, 0):
		return fmt.Errorf("%w: el nombre de la rama contiene un byte NUL", ErrInvalidArgument)
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("%w: %q empieza por guion", ErrInvalidArgument, name)
	case strings.HasPrefix(name, "/"), strings.HasSuffix(name, "/"),
		strings.HasSuffix(name, "."), strings.Contains(name, "//"),
		strings.Contains(name, ".."), strings.Contains(name, "@{"):
		return fmt.Errorf("%w: %q no es un nombre de rama válido", ErrInvalidArgument, name)
	case name == "@":
		return fmt.Errorf("%w: %q es el atajo del HEAD y no una rama", ErrInvalidArgument, name)
	}

	for _, r := range name {
		if r <= ' ' || r == 0x7f {
			return fmt.Errorf("%w: %q contiene espacios o caracteres de control", ErrInvalidArgument, name)
		}
		if strings.ContainsRune(refForbidden, r) {
			return fmt.Errorf("%w: %q contiene %q", ErrInvalidArgument, name, r)
		}
	}

	for _, segment := range strings.Split(name, "/") {
		if strings.HasPrefix(segment, ".") || strings.HasSuffix(segment, ".lock") {
			return fmt.Errorf("%w: %q no es un nombre de rama válido", ErrInvalidArgument, name)
		}
	}

	return nil
}
