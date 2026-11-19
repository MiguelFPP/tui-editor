package main

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/adapters/docker"
	"github.com/MiguelFPP/tui-editor/internal/adapters/editor"
	"github.com/MiguelFPP/tui-editor/internal/adapters/fs"
	"github.com/MiguelFPP/tui-editor/internal/adapters/git"
	"github.com/MiguelFPP/tui-editor/internal/config"
	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/MiguelFPP/tui-editor/internal/tui/app"
)

// version and commit are overridden at build time with
// -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "0.3.1"
	commit  = "dev"
)

func main() {
	if err := run(); err != nil {
		if errors.Is(err, config.ErrHelp) {
			fmt.Fprintln(os.Stdout, config.Usage)
			return
		}

		fmt.Fprintf(os.Stderr, "tui-editor: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Parse(os.Args[1:], os.Getenv)
	if err != nil {
		return err
	}

	if cfg.ShowVersion {
		fmt.Fprintln(os.Stdout, versionLine())

		return nil
	}

	root, err := domain.NewRepoPath(cfg.Repo)
	if err != nil {
		return fmt.Errorf("resolver la raíz del repositorio: %w", err)
	}

	files, err := fs.New(root)
	if err != nil {
		return fmt.Errorf("construir el servicio de archivos: %w", err)
	}

	gitSvc, err := git.New(root, git.NewExecRunner(root))
	if err != nil {
		return fmt.Errorf("construir el servicio de git: %w", err)
	}

	editorSvc, err := editor.New(root, cfg.Editor)
	if err != nil {
		return fmt.Errorf("construir el servicio de editor: %w", err)
	}

	// An unavailable daemon is not fatal: the Docker panel shows an actionable state instead.
	var dockerSvc ports.DockerService
	if service, err := docker.New(cfg.DockerHost); err == nil {
		dockerSvc = service
	}

	deps := app.Deps{Config: cfg, Files: files, Git: gitSvc, Editor: editorSvc, Docker: dockerSvc}

	program := tea.NewProgram(app.New(deps))
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("ejecutar la TUI: %w", err)
	}

	return nil
}

// versionLine prints tui-editor <version> and appends the commit when one was
// injected by the release build; local builds keep the default "dev" marker.
func versionLine() string {
	if commit == "" || commit == "dev" {
		return "tui-editor " + version
	}

	return fmt.Sprintf("tui-editor %s (%s)", version, commit)
}
