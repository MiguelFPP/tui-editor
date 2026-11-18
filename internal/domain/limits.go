package domain

// Hard limits applied by adapters before returning data to the TUI.
const (
	MaxPreviewBytes  = 1 << 20
	MaxPreviewLines  = 2000
	MaxDirEntries    = 2000
	MaxTreeDepth     = 32
	MaxStatusEntries = 5000
	MaxDiffLines     = 20000
	MaxLogLines      = 1000
	DefaultLogTail   = 100
	MaxCommits       = 200
	DefaultCommits   = 50
	MaxBranches      = 500
	MaxStashes       = 100
	MaxDiffFiles     = 500
)
