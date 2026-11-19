package ports

import (
	"context"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// EditorService resolves the external editor command for a file and line.
type EditorService interface {
	Resolve(ctx context.Context, target domain.EditorTarget) (domain.EditorCommand, error)
}
