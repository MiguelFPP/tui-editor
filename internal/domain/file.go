package domain

import "time"

// FileKind classifies a tree entry without following symlinks.
type FileKind uint8

const (
	File FileKind = iota
	Dir
	Symlink
	Other
)

// FileNode is one entry of the tree; Children is only filled for directories.
type FileNode struct {
	Name     string
	Path     RelPath
	Kind     FileKind
	Size     int64
	ModTime  time.Time
	Children []FileNode
}

func (n FileNode) Expandable() bool {
	return n.Kind == Dir
}

// DirListing is the bounded content of one directory.
type DirListing struct {
	Dir       RelPath
	Entries   []FileNode
	Truncated bool
}

// FileStamp is the cheap fingerprint used to detect external modifications.
type FileStamp struct {
	Size    int64
	ModTime time.Time
}

func (s FileStamp) IsZero() bool {
	return s.Size == 0 && s.ModTime.IsZero()
}

// FileContent is the preview of a text file, truncated at the domain limits.
type FileContent struct {
	Path      RelPath
	Text      string
	Stamp     FileStamp
	Truncated bool
}
