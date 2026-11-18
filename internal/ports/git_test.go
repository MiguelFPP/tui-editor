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

type readerOnly struct{}

func (readerOnly) Status(context.Context) (domain.GitStatus, error) {
	return domain.GitStatus{}, nil
}

func (readerOnly) Diff(context.Context, domain.DiffRequest) (domain.Diff, error) {
	return domain.Diff{}, nil
}

func (readerOnly) Branches(context.Context) ([]domain.Branch, error) {
	return nil, nil
}

func (readerOnly) Stashes(context.Context) ([]domain.StashEntry, error) {
	return nil, nil
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
