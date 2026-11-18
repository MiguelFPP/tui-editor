package main

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/adapters/editor"
	"github.com/MiguelFPP/tui-editor/internal/adapters/fs"
	"github.com/MiguelFPP/tui-editor/internal/adapters/git"
	"github.com/MiguelFPP/tui-editor/internal/config"
	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/tui/app"
)

const version = "0.2.0"

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
		fmt.Fprintf(os.Stdout, "tui-editor %s\n", version)

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

	program := tea.NewProgram(app.New(app.Deps{Config: cfg, Files: files, Git: gitSvc, Editor: editorSvc}))
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("ejecutar la TUI: %w", err)
	}

	return nil
}
