package editor

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// missingBinary is a name no PATH is expected to resolve.
const missingBinary = "tui-editor-missing-f11-binary"

func newService(t *testing.T, root, command string) *Service {
	t.Helper()

	repo, err := domain.NewRepoPath(root)
	if err != nil {
		t.Fatalf("NewRepoPath(%q) error = %v, want nil", root, err)
	}

	service, err := New(repo, command)
	if err != nil {
		t.Fatalf("New(%q, %q) error = %v, want nil", root, command, err)
	}

	return service
}

// stubPath replaces the PATH lookup so the tests never depend on the host.
func stubPath(path string) func(string) (string, error) {
	return func(string) (string, error) { return path, nil }
}

func TestNewRejectsInvalidRootAndCommand(t *testing.T) {
	root, err := domain.NewRepoPath(t.TempDir())
	if err != nil {
		t.Fatalf("NewRepoPath() error = %v, want nil", err)
	}

	if _, err := New(domain.RepoPath{}, "vim"); !errors.Is(err, domain.ErrInvalidPath) {
		t.Errorf("New(zero root) error = %v, want %v", err, domain.ErrInvalidPath)
	}

	cases := []struct {
		name    string
		command string
	}{
		{name: "empty", command: ""},
		{name: "spaces", command: "   "},
		{name: "whitespace", command: "\t\n"},
		{name: "nul", command: "vim\x00-f"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(root, tc.command); !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("New(%q) error = %v, want %v", tc.command, err, domain.ErrInvalidArgument)
			}
		})
	}
}

func TestResolveReturnsTheAbsoluteBinaryAndPath(t *testing.T) {
	dir := t.TempDir()
	service := newService(t, dir, "vim")
	service.lookPath = stubPath("/usr/bin/vim")

	command, err := service.Resolve(context.Background(), domain.EditorTarget{Path: "docs/guide.md", Line: 42})
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}

	if command.Binary != "/usr/bin/vim" {
		t.Errorf("Binary = %q, want %q", command.Binary, "/usr/bin/vim")
	}

	want := []string{"+42", filepath.Join(dir, "docs", "guide.md")}
	if !slices.Equal(command.Args, want) {
		t.Errorf("Args = %q, want %q", command.Args, want)
	}
	if !filepath.IsAbs(command.Args[len(command.Args)-1]) {
		t.Errorf("the file argument %q is not absolute", command.Args[len(command.Args)-1])
	}
}

func TestResolveAddsNoLineFlagToUnknownEditors(t *testing.T) {
	cases := []struct {
		name     string
		command  string
		resolved string
		want     []string
	}{
		{name: "vim", command: "vim", resolved: "/usr/bin/vim", want: []string{"+7"}},
		{name: "vi", command: "vi", resolved: "/usr/bin/vi", want: []string{"+7"}},
		{name: "nano", command: "nano", resolved: "/usr/bin/nano", want: []string{"+7"}},
		{name: "vim with a suffix", command: "vim", resolved: "/usr/bin/vim.basic", want: []string{"+7"}},
		{name: "neovim", command: "nvim", resolved: "/usr/bin/nvim", want: nil},
		{name: "code with flags", command: "code --wait", resolved: "/usr/bin/code", want: []string{"--wait"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			service := newService(t, dir, tc.command)
			service.lookPath = stubPath(tc.resolved)

			command, err := service.Resolve(context.Background(), domain.EditorTarget{Path: "main.go", Line: 7})
			if err != nil {
				t.Fatalf("Resolve() error = %v, want nil", err)
			}

			want := append(slices.Clone(tc.want), filepath.Join(dir, "main.go"))
			if !slices.Equal(command.Args, want) {
				t.Errorf("Args = %q, want %q", command.Args, want)
			}
		})
	}
}

func TestResolveWithoutLineKeepsThePathAlone(t *testing.T) {
	dir := t.TempDir()
	service := newService(t, dir, "vim")
	service.lookPath = stubPath("/usr/bin/vim")

	command, err := service.Resolve(context.Background(), domain.EditorTarget{Path: "main.go"})
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}

	if want := []string{filepath.Join(dir, "main.go")}; !slices.Equal(command.Args, want) {
		t.Errorf("Args = %q, want %q", command.Args, want)
	}
}

func TestResolveRejectsUnusablePaths(t *testing.T) {
	service := newService(t, t.TempDir(), "vim")
	service.lookPath = stubPath("/usr/bin/vim")

	cases := []struct {
		name string
		path domain.RelPath
	}{
		{name: "zero", path: ""},
		{name: "dot", path: "."},
		{name: "parent", path: ".."},
		{name: "escape", path: "../outside"},
		{name: "absolute", path: "/etc/passwd"},
		{name: "unnormalized", path: "dir/../other"},
		{name: "aliased", path: "dir/./file.txt"},
		{name: "nul", path: "a\x00b"},
		{name: "double separator", path: "a//b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Resolve(context.Background(), domain.EditorTarget{Path: tc.path})
			if !errors.Is(err, domain.ErrInvalidPath) {
				t.Fatalf("Resolve(%q) error = %v, want %v", tc.path, err, domain.ErrInvalidPath)
			}
		})
	}
}

func TestResolveReportsAMissingBinary(t *testing.T) {
	t.Run("real lookup", func(t *testing.T) {
		service := newService(t, t.TempDir(), missingBinary)

		if _, err := service.Resolve(context.Background(), domain.EditorTarget{Path: "main.go"}); !errors.Is(err, domain.ErrEditorUnavailable) {
			t.Errorf("Resolve() error = %v, want %v", err, domain.ErrEditorUnavailable)
		}
	})

	t.Run("failed lookup", func(t *testing.T) {
		service := newService(t, t.TempDir(), "vim")
		service.lookPath = func(string) (string, error) { return "", exec.ErrNotFound }

		_, err := service.Resolve(context.Background(), domain.EditorTarget{Path: "main.go"})
		if !errors.Is(err, domain.ErrEditorUnavailable) {
			t.Fatalf("Resolve() error = %v, want %v", err, domain.ErrEditorUnavailable)
		}
		if !strings.Contains(err.Error(), "vim") {
			t.Errorf("error = %v, want the binary name", err)
		}
	})
}

func TestResolveHonoursCancellation(t *testing.T) {
	service := newService(t, t.TempDir(), "vim")
	service.lookPath = stubPath("/usr/bin/vim")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := service.Resolve(ctx, domain.EditorTarget{Path: "main.go"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Resolve() error = %v, want %v", err, context.Canceled)
	}
}
