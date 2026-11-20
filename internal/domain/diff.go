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

// Equal reports whether both diffs describe the same change; a reload that answers the same diff
// keeps the hunk selection of the user.
func (d Diff) Equal(other Diff) bool {
	if d.Path != other.Path || d.OrigPath != other.OrigPath || d.Staged != other.Staged {
		return false
	}
	if d.Binary != other.Binary || d.Truncated != other.Truncated {
		return false
	}
	if d.New != other.New || d.Deleted != other.Deleted {
		return false
	}
	if d.Additions != other.Additions || d.Deletions != other.Deletions {
		return false
	}
	if len(d.Hunks) != len(other.Hunks) {
		return false
	}

	for i, hunk := range d.Hunks {
		if !hunk.equal(other.Hunks[i]) {
			return false
		}
	}

	return true
}

func (h DiffHunk) equal(other DiffHunk) bool {
	if h.Header != other.Header || h.Section != other.Section {
		return false
	}
	if h.OldStart != other.OldStart || h.OldLines != other.OldLines {
		return false
	}
	if h.NewStart != other.NewStart || h.NewLines != other.NewLines {
		return false
	}
	if len(h.Lines) != len(other.Lines) {
		return false
	}

	for i, line := range h.Lines {
		if line != other.Lines[i] {
			return false
		}
	}

	return true
}
