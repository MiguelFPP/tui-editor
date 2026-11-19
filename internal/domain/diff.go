package domain

// DiffRequest selects the worktree or the staged diff of one path.
type DiffRequest struct {
	Path   RelPath
	Staged bool
}

type DiffLineKind uint8

const (
	Context DiffLineKind = iota
	Added
	Removed
	NoNewline
)

// DiffLine carries the text without diff prefix and without the trailing newline.
type DiffLine struct {
	Kind    DiffLineKind
	Text    string
	OldLine int
	NewLine int
}

type DiffHunk struct {
	Header   string
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Section  string
	Lines    []DiffLine
}

// Diff is the bounded diff of one path.
type Diff struct {
	Path      RelPath
	OrigPath  RelPath
	Staged    bool
	Binary    bool
	Truncated bool
	New       bool
	Deleted   bool
	Additions int
	Deletions int
	Hunks     []DiffHunk
}

func (d Diff) IsEmpty() bool {
	return !d.Binary && len(d.Hunks) == 0
}
