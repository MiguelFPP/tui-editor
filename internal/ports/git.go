package ports

import (
	"context"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// GitReader exposes the read-only Git operations of one repository.
type GitReader interface {
	Status(ctx context.Context) (domain.GitStatus, error)
	Diff(ctx context.Context, req domain.DiffRequest) (domain.Diff, error)
	DiffStat(ctx context.Context, staged bool) (domain.DiffStat, error)
	Branches(ctx context.Context) ([]domain.Branch, error)
	Stashes(ctx context.Context) ([]domain.StashEntry, error)
	Commits(ctx context.Context, query domain.CommitQuery) (domain.CommitPage, error)
	Show(ctx context.Context, hash string) (domain.CommitDetail, error)
}

// GitStager moves files and hunks between the worktree and the index.
type GitStager interface {
	Stage(ctx context.Context, paths []domain.RelPath) error
	Unstage(ctx context.Context, paths []domain.RelPath) error
	// StageHunks and UnstageHunks take the diff the hunks belong to plus their indices.
	StageHunks(ctx context.Context, diff domain.Diff, hunks []int) error
	UnstageHunks(ctx context.Context, diff domain.Diff, hunks []int) error
}

// GitCommitter records the staged changes.
type GitCommitter interface {
	Commit(ctx context.Context, message string) error
}

// GitBranchWriter creates and switches branches.
type GitBranchWriter interface {
	Checkout(ctx context.Context, branch string) error
	CreateBranch(ctx context.Context, name, startPoint string) error
	DeleteBranch(ctx context.Context, name string, force bool) error
}

// GitStashWriter manages the stash; entries are addressed by position, not by ref.
type GitStashWriter interface {
	StashPush(ctx context.Context, message string) error
	StashApply(ctx context.Context, index int) error
	StashPop(ctx context.Context, index int) error
	StashDrop(ctx context.Context, index int) error
}

// GitMergeWriter merges and resolves conflicts; conflicts are never resolved automatically.
type GitMergeWriter interface {
	Merge(ctx context.Context, branch string) (domain.MergeResult, error)
	AbortMerge(ctx context.Context) error
	ResolveConflict(ctx context.Context, path domain.RelPath, resolution domain.Resolution) error
}

// GitSyncWriter exchanges commits with the configured remotes.
type GitSyncWriter interface {
	Fetch(ctx context.Context) error
	Pull(ctx context.Context) (domain.MergeResult, error)
	Push(ctx context.Context) error
}

// GitWriter mutates the repository; the adapter serialises these calls.
type GitWriter interface {
	GitStager
	GitCommitter
	GitBranchWriter
	GitStashWriter
	GitMergeWriter
	GitSyncWriter
}

// GitChanges is the port the commit flow needs: read the working tree and stage and commit it.
type GitChanges interface {
	GitReader
	GitStager
	GitCommitter
}

// GitService is the full Git port consumed by the Git panel.
type GitService interface {
	GitReader
	GitWriter
}
