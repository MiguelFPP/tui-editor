package ports

import (
	"context"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// GitReader exposes the read-only Git operations of one repository.
type GitReader interface {
	Status(ctx context.Context) (domain.GitStatus, error)
	Diff(ctx context.Context, req domain.DiffRequest) (domain.Diff, error)
	Branches(ctx context.Context) ([]domain.Branch, error)
	Stashes(ctx context.Context) ([]domain.StashEntry, error)
}

// GitWriter mutates the repository; the adapter serialises these calls.
type GitWriter interface {
	Stage(ctx context.Context, paths []domain.RelPath) error
	Unstage(ctx context.Context, paths []domain.RelPath) error
	StageHunks(ctx context.Context, path domain.RelPath, hunks []domain.DiffHunk) error
	UnstageHunks(ctx context.Context, path domain.RelPath, hunks []domain.DiffHunk) error
	Commit(ctx context.Context, message string) error
	Checkout(ctx context.Context, branch string) error
	CreateBranch(ctx context.Context, name, startPoint string) error
	DeleteBranch(ctx context.Context, name string, force bool) error
	StashPush(ctx context.Context, message string) error
	// Stash entries are addressed by position, not by ref: pass domain.StashEntry.Index.
	StashApply(ctx context.Context, index int) error
	StashPop(ctx context.Context, index int) error
	StashDrop(ctx context.Context, index int) error
	Merge(ctx context.Context, branch string) (domain.MergeResult, error)
	AbortMerge(ctx context.Context) error
	ResolveConflict(ctx context.Context, path domain.RelPath, resolution domain.Resolution) error
	Fetch(ctx context.Context) error
	Pull(ctx context.Context) (domain.MergeResult, error)
	Push(ctx context.Context) error
}

// GitService is the full Git port consumed by the Git panel.
type GitService interface {
	GitReader
	GitWriter
}
