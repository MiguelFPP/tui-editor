package fakes

import (
	"context"
	"fmt"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// FileService is a test double for ports.FileService.
type FileService struct {
	recorder

	OnReadDir   func(ctx context.Context, dir domain.RelPath) (domain.DirListing, error)
	OnReadFile  func(ctx context.Context, path domain.RelPath) (domain.FileContent, error)
	OnWriteFile func(ctx context.Context, path domain.RelPath, content string, expected domain.FileStamp) (domain.FileStamp, error)
}

func NewFileService() *FileService {
	return &FileService{}
}

func (f *FileService) ReadDir(ctx context.Context, dir domain.RelPath) (domain.DirListing, error) {
	f.record("ReadDir", dir)
	if err := ctx.Err(); err != nil {
		return domain.DirListing{}, fmt.Errorf("ReadDir: %w", err)
	}
	if fn := hook(&f.recorder, &f.OnReadDir); fn != nil {
		return fn(ctx, dir)
	}

	return domain.DirListing{}, nil
}

func (f *FileService) ReadFile(ctx context.Context, path domain.RelPath) (domain.FileContent, error) {
	f.record("ReadFile", path)
	if err := ctx.Err(); err != nil {
		return domain.FileContent{}, fmt.Errorf("ReadFile: %w", err)
	}
	if fn := hook(&f.recorder, &f.OnReadFile); fn != nil {
		return fn(ctx, path)
	}

	return domain.FileContent{}, nil
}

func (f *FileService) WriteFile(ctx context.Context, path domain.RelPath, content string, expected domain.FileStamp) (domain.FileStamp, error) {
	f.record("WriteFile", path, content, expected)
	if err := ctx.Err(); err != nil {
		return domain.FileStamp{}, fmt.Errorf("WriteFile: %w", err)
	}
	if fn := hook(&f.recorder, &f.OnWriteFile); fn != nil {
		return fn(ctx, path, content, expected)
	}

	return domain.FileStamp{}, nil
}
