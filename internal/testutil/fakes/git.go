package fakes

import (
	"context"
	"fmt"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// GitService is a test double for ports.GitService.
type GitService struct {
	recorder

	OnStatus          func(ctx context.Context) (domain.GitStatus, error)
	OnDiff            func(ctx context.Context, req domain.DiffRequest) (domain.Diff, error)
	OnDiffStat        func(ctx context.Context, staged bool) (domain.DiffStat, error)
	OnBranches        func(ctx context.Context) ([]domain.Branch, error)
	OnStashes         func(ctx context.Context) ([]domain.StashEntry, error)
	OnCommits         func(ctx context.Context, query domain.CommitQuery) (domain.CommitPage, error)
	OnShow            func(ctx context.Context, hash string) (domain.CommitDetail, error)
	OnStage           func(ctx context.Context, paths []domain.RelPath) error
	OnUnstage         func(ctx context.Context, paths []domain.RelPath) error
	OnStageHunks      func(ctx context.Context, path domain.RelPath, hunks []domain.DiffHunk) error
	OnUnstageHunks    func(ctx context.Context, path domain.RelPath, hunks []domain.DiffHunk) error
	OnCommit          func(ctx context.Context, message string) error
	OnCheckout        func(ctx context.Context, branch string) error
	OnCreateBranch    func(ctx context.Context, name, startPoint string) error
	OnDeleteBranch    func(ctx context.Context, name string, force bool) error
	OnStashPush       func(ctx context.Context, message string) error
	OnStashApply      func(ctx context.Context, index int) error
	OnStashPop        func(ctx context.Context, index int) error
	OnStashDrop       func(ctx context.Context, index int) error
	OnMerge           func(ctx context.Context, branch string) (domain.MergeResult, error)
	OnAbortMerge      func(ctx context.Context) error
	OnResolveConflict func(ctx context.Context, path domain.RelPath, resolution domain.Resolution) error
	OnFetch           func(ctx context.Context) error
	OnPull            func(ctx context.Context) (domain.MergeResult, error)
	OnPush            func(ctx context.Context) error
}

func NewGitService() *GitService {
	return &GitService{}
}

func (g *GitService) Status(ctx context.Context) (domain.GitStatus, error) {
	g.record("Status")
	if err := ctx.Err(); err != nil {
		return domain.GitStatus{}, fmt.Errorf("Status: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnStatus); fn != nil {
		return fn(ctx)
	}

	return domain.GitStatus{}, nil
}

func (g *GitService) Diff(ctx context.Context, req domain.DiffRequest) (domain.Diff, error) {
	g.record("Diff", req)
	if err := ctx.Err(); err != nil {
		return domain.Diff{}, fmt.Errorf("Diff: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnDiff); fn != nil {
		return fn(ctx, req)
	}

	return domain.Diff{}, nil
}

func (g *GitService) DiffStat(ctx context.Context, staged bool) (domain.DiffStat, error) {
	g.record("DiffStat", staged)
	if err := ctx.Err(); err != nil {
		return domain.DiffStat{}, fmt.Errorf("DiffStat: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnDiffStat); fn != nil {
		return fn(ctx, staged)
	}

	return domain.DiffStat{}, nil
}

func (g *GitService) Branches(ctx context.Context) ([]domain.Branch, error) {
	g.record("Branches")
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("Branches: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnBranches); fn != nil {
		return fn(ctx)
	}

	return nil, nil
}

func (g *GitService) Stashes(ctx context.Context) ([]domain.StashEntry, error) {
	g.record("Stashes")
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("Stashes: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnStashes); fn != nil {
		return fn(ctx)
	}

	return nil, nil
}

func (g *GitService) Commits(ctx context.Context, query domain.CommitQuery) (domain.CommitPage, error) {
	g.record("Commits", query)
	if err := ctx.Err(); err != nil {
		return domain.CommitPage{}, fmt.Errorf("Commits: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnCommits); fn != nil {
		return fn(ctx, query)
	}

	return domain.CommitPage{}, nil
}

func (g *GitService) Show(ctx context.Context, hash string) (domain.CommitDetail, error) {
	g.record("Show", hash)
	if err := ctx.Err(); err != nil {
		return domain.CommitDetail{}, fmt.Errorf("Show: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnShow); fn != nil {
		return fn(ctx, hash)
	}

	return domain.CommitDetail{}, nil
}

func (g *GitService) Stage(ctx context.Context, paths []domain.RelPath) error {
	g.record("Stage", paths)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Stage: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnStage); fn != nil {
		return fn(ctx, paths)
	}

	return nil
}

func (g *GitService) Unstage(ctx context.Context, paths []domain.RelPath) error {
	g.record("Unstage", paths)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Unstage: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnUnstage); fn != nil {
		return fn(ctx, paths)
	}

	return nil
}

func (g *GitService) StageHunks(ctx context.Context, path domain.RelPath, hunks []domain.DiffHunk) error {
	g.record("StageHunks", path, hunks)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("StageHunks: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnStageHunks); fn != nil {
		return fn(ctx, path, hunks)
	}

	return nil
}

func (g *GitService) UnstageHunks(ctx context.Context, path domain.RelPath, hunks []domain.DiffHunk) error {
	g.record("UnstageHunks", path, hunks)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("UnstageHunks: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnUnstageHunks); fn != nil {
		return fn(ctx, path, hunks)
	}

	return nil
}

func (g *GitService) Commit(ctx context.Context, message string) error {
	g.record("Commit", message)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Commit: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnCommit); fn != nil {
		return fn(ctx, message)
	}

	return nil
}

func (g *GitService) Checkout(ctx context.Context, branch string) error {
	g.record("Checkout", branch)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Checkout: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnCheckout); fn != nil {
		return fn(ctx, branch)
	}

	return nil
}

func (g *GitService) CreateBranch(ctx context.Context, name, startPoint string) error {
	g.record("CreateBranch", name, startPoint)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("CreateBranch: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnCreateBranch); fn != nil {
		return fn(ctx, name, startPoint)
	}

	return nil
}

func (g *GitService) DeleteBranch(ctx context.Context, name string, force bool) error {
	g.record("DeleteBranch", name, force)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("DeleteBranch: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnDeleteBranch); fn != nil {
		return fn(ctx, name, force)
	}

	return nil
}

func (g *GitService) StashPush(ctx context.Context, message string) error {
	g.record("StashPush", message)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("StashPush: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnStashPush); fn != nil {
		return fn(ctx, message)
	}

	return nil
}

func (g *GitService) StashApply(ctx context.Context, index int) error {
	g.record("StashApply", index)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("StashApply: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnStashApply); fn != nil {
		return fn(ctx, index)
	}

	return nil
}

func (g *GitService) StashPop(ctx context.Context, index int) error {
	g.record("StashPop", index)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("StashPop: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnStashPop); fn != nil {
		return fn(ctx, index)
	}

	return nil
}

func (g *GitService) StashDrop(ctx context.Context, index int) error {
	g.record("StashDrop", index)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("StashDrop: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnStashDrop); fn != nil {
		return fn(ctx, index)
	}

	return nil
}

func (g *GitService) Merge(ctx context.Context, branch string) (domain.MergeResult, error) {
	g.record("Merge", branch)
	if err := ctx.Err(); err != nil {
		return domain.MergeResult{}, fmt.Errorf("Merge: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnMerge); fn != nil {
		return fn(ctx, branch)
	}

	return domain.MergeResult{}, nil
}

func (g *GitService) AbortMerge(ctx context.Context) error {
	g.record("AbortMerge")
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("AbortMerge: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnAbortMerge); fn != nil {
		return fn(ctx)
	}

	return nil
}

func (g *GitService) ResolveConflict(ctx context.Context, path domain.RelPath, resolution domain.Resolution) error {
	g.record("ResolveConflict", path, resolution)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("ResolveConflict: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnResolveConflict); fn != nil {
		return fn(ctx, path, resolution)
	}

	return nil
}

func (g *GitService) Fetch(ctx context.Context) error {
	g.record("Fetch")
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Fetch: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnFetch); fn != nil {
		return fn(ctx)
	}

	return nil
}

func (g *GitService) Pull(ctx context.Context) (domain.MergeResult, error) {
	g.record("Pull")
	if err := ctx.Err(); err != nil {
		return domain.MergeResult{}, fmt.Errorf("Pull: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnPull); fn != nil {
		return fn(ctx)
	}

	return domain.MergeResult{}, nil
}

func (g *GitService) Push(ctx context.Context) error {
	g.record("Push")
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Push: %w", err)
	}
	if fn := hook(&g.recorder, &g.OnPush); fn != nil {
		return fn(ctx)
	}

	return nil
}
