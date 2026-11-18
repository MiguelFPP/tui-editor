package fakes_test

import (
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
)

// The fakes must keep implementing the ports they replace.
var (
	_ ports.FileService   = (*fakes.FileService)(nil)
	_ ports.GitReader     = (*fakes.GitService)(nil)
	_ ports.GitWriter     = (*fakes.GitService)(nil)
	_ ports.GitChanges    = (*fakes.GitService)(nil)
	_ ports.GitService    = (*fakes.GitService)(nil)
	_ ports.DockerService = (*fakes.DockerService)(nil)
	_ ports.EditorService = (*fakes.EditorService)(nil)
)
