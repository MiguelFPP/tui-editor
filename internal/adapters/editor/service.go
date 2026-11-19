package editor

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
)

// jumpEditors accept a `+N` argument that opens the file on that line.
var jumpEditors = map[string]bool{"nano": true, "vi": true, "vim": true}

// Service implements the editor port over os/exec: it only resolves, never runs.
type Service struct {
	root   domain.RepoPath
	editor domain.EditorCommand

	lookPath func(string) (string, error)
}

var _ ports.EditorService = (*Service)(nil)

// New parses the editor command of the configuration and validates the root.
func New(root domain.RepoPath, editor string) (*Service, error) {
	if root.IsZero() {
		return nil, fmt.Errorf("%w: falta la raíz del repositorio", domain.ErrInvalidPath)
	}

	command, err := domain.ParseEditorCommand(editor)
	if err != nil {
		return nil, err
	}

	return &Service{root: root, editor: command, lookPath: exec.LookPath}, nil
}

// Resolve returns the absolute binary and the absolute path of the file, never a shell line.
func (s *Service) Resolve(ctx context.Context, target domain.EditorTarget) (domain.EditorCommand, error) {
	if err := ctx.Err(); err != nil {
		return domain.EditorCommand{}, fmt.Errorf("editor: Resolve: %w", err)
	}

	clean, err := domain.NewRelPath(string(target.Path))
	if err != nil {
		return domain.EditorCommand{}, err
	}
	if clean.IsZero() || clean != target.Path {
		return domain.EditorCommand{}, fmt.Errorf("%w: %q no es un archivo normalizado", domain.ErrInvalidPath, target.Path)
	}

	path, err := s.root.Resolve(target.Path)
	if err != nil {
		return domain.EditorCommand{}, err
	}

	binary, err := s.lookup(s.editor.Binary)
	if err != nil {
		return domain.EditorCommand{}, fmt.Errorf("%w: %q", domain.ErrEditorUnavailable, s.editor.Binary)
	}

	return domain.EditorCommand{Binary: binary, Args: s.args(binary, path, target.Line)}, nil
}

// lookup resolves the binary, falling back to the real PATH when no lookup was installed.
func (s *Service) lookup(binary string) (string, error) {
	if s.lookPath == nil {
		return exec.LookPath(binary)
	}

	return s.lookPath(binary)
}

// args appends the optional line and the target path; an unknown editor gets the path alone.
func (s *Service) args(binary, path string, line int) []string {
	args := make([]string, 0, len(s.editor.Args)+2)
	args = append(args, s.editor.Args...)

	if line > 0 && jumpsToLine(binary) {
		args = append(args, "+"+strconv.Itoa(line))
	}

	return append(args, path)
}

func jumpsToLine(binary string) bool {
	name := strings.TrimSuffix(filepath.Base(binary), filepath.Ext(binary))

	return jumpEditors[name]
}
