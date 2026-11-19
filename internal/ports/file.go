package ports

import (
	"context"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// FileService reads and writes files inside the repository root.
type FileService interface {
	ReadDir(ctx context.Context, dir domain.RelPath) (domain.DirListing, error)
	ReadFile(ctx context.Context, path domain.RelPath) (domain.FileContent, error)
	// WriteFile only overwrites when the current stamp still matches expected.
	WriteFile(ctx context.Context, path domain.RelPath, content string, expected domain.FileStamp) (domain.FileStamp, error)
}
