package domain

// FileRev is the Git revision a blob is read from; the zero value is not valid.
type FileRev uint8

const (
	// RevIndex reads the staging area.
	RevIndex FileRev = iota + 1
	// RevHead reads the last commit.
	RevHead
)

// String returns "index", "HEAD" or "" when the revision is not valid.
func (r FileRev) String() string {
	switch r {
	case RevIndex:
		return "index"
	case RevHead:
		return "HEAD"
	}

	return ""
}

// Valid reports whether the revision is one of the supported ones.
func (r FileRev) Valid() bool {
	return r == RevIndex || r == RevHead
}

// RevisionContent is the text of one blob read at a given revision.
type RevisionContent struct {
	Path      RelPath
	Rev       FileRev
	Text      string
	Truncated bool
}

// IsEmpty reports whether the revision holds no text.
func (c RevisionContent) IsEmpty() bool {
	return c.Text == ""
}
