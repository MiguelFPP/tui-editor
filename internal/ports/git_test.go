package ports_test

import (
	"context"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
)

// GitService must expose both halves: one value satisfies the reader and the writer.
var (
	_ ports.GitReader = (ports.GitService)(nil)
	_ ports.GitWriter = (ports.GitService)(nil)
)

// The writer half is the sum of its subinterfaces, and the service requires every one of them.
var (
	_ ports.GitStager       = ports.GitWriter(nil)
	_ ports.GitCommitter    = ports.GitWriter(nil)
	_ ports.GitBranchWriter = ports.GitWriter(nil)
	_ ports.GitStashWriter  = ports.GitWriter(nil)
	_ ports.GitMergeWriter  = ports.GitWriter(nil)
	_ ports.GitSyncWriter   = ports.GitWriter(nil)
)

type readerOnly struct{}

func (readerOnly) Status(context.Context) (domain.GitStatus, error) {
	return domain.GitStatus{}, nil
}

func (readerOnly) Diff(context.Context, domain.DiffRequest) (domain.Diff, error) {
	return domain.Diff{}, nil
}

func (readerOnly) DiffStat(context.Context, bool) (domain.DiffStat, error) {
	return domain.DiffStat{}, nil
}

func (readerOnly) Branches(context.Context) ([]domain.Branch, error) {
	return nil, nil
}

func (readerOnly) Stashes(context.Context) ([]domain.StashEntry, error) {
	return nil, nil
}

func (readerOnly) Commits(context.Context, domain.CommitQuery) (domain.CommitPage, error) {
	return domain.CommitPage{}, nil
}

func (readerOnly) Show(context.Context, string) (domain.CommitDetail, error) {
	return domain.CommitDetail{}, nil
}

func TestGitServiceRequiresBothHalves(t *testing.T) {
	var reader any = readerOnly{}

	if _, ok := reader.(ports.GitReader); !ok {
		t.Fatal("readerOnly must satisfy ports.GitReader")
	}
	if _, ok := reader.(ports.GitWriter); ok {
		t.Fatal("readerOnly must not satisfy ports.GitWriter")
	}
	if _, ok := reader.(ports.GitService); ok {
		t.Fatal("ports.GitService must require the writer half as well")
	}
}

// changesOnly is a reader that can stage and commit, but nothing else.
type changesOnly struct{ readerOnly }

func (changesOnly) Stage(context.Context, []domain.RelPath) error          { return nil }
func (changesOnly) Unstage(context.Context, []domain.RelPath) error        { return nil }
func (changesOnly) StageHunks(context.Context, domain.Diff, []int) error   { return nil }
func (changesOnly) UnstageHunks(context.Context, domain.Diff, []int) error { return nil }
func (changesOnly) Commit(context.Context, string) error                   { return nil }

func TestGitServiceNeedsTheBranchStashMergeAndSyncHalves(t *testing.T) {
	var value any = changesOnly{}

	if _, ok := value.(ports.GitReader); !ok {
		t.Fatal("changesOnly must satisfy ports.GitReader")
	}
	if _, ok := value.(ports.GitWriter); ok {
		t.Fatal("ports.GitWriter must also require the branch, stash, merge and sync halves")
	}
	if _, ok := value.(ports.GitService); ok {
		t.Fatal("ports.GitService must require the full writer half")
	}
	if _, ok := value.(ports.GitCommitter); !ok {
		t.Fatal("changesOnly must satisfy ports.GitCommitter")
	}
}

func TestGitWriterNeedsEveryHalf(t *testing.T) {
	var value any = stagerOnly{}

	if _, ok := value.(ports.GitStager); !ok {
		t.Fatal("stagerOnly must satisfy ports.GitStager")
	}
	if _, ok := value.(ports.GitCommitter); ok {
		t.Error("ports.GitCommitter must not be satisfied by the staging methods alone")
	}
	if _, ok := value.(ports.GitService); ok {
		t.Error("ports.GitService must require the full writer half")
	}
	if _, ok := value.(ports.GitWriter); ok {
		t.Error("ports.GitWriter must require the branch, stash, merge and sync halves")
	}
}

// stagerOnly implements the staging half and nothing else.
type stagerOnly struct{}

func (stagerOnly) Stage(context.Context, []domain.RelPath) error          { return nil }
func (stagerOnly) Unstage(context.Context, []domain.RelPath) error        { return nil }
func (stagerOnly) StageHunks(context.Context, domain.Diff, []int) error   { return nil }
func (stagerOnly) UnstageHunks(context.Context, domain.Diff, []int) error { return nil }
