package fakes

import (
	"context"
	"fmt"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// EditorService is a test double for ports.EditorService.
type EditorService struct {
	recorder

	OnResolve func(ctx context.Context, target domain.EditorTarget) (domain.EditorCommand, error)
}

func NewEditorService() *EditorService {
	return &EditorService{}
}

func (e *EditorService) Resolve(ctx context.Context, target domain.EditorTarget) (domain.EditorCommand, error) {
	e.record("Resolve", target)
	if err := ctx.Err(); err != nil {
		return domain.EditorCommand{}, fmt.Errorf("Resolve: %w", err)
	}
	if fn := hook(&e.recorder, &e.OnResolve); fn != nil {
		return fn(ctx, target)
	}

	return domain.EditorCommand{}, nil
}
