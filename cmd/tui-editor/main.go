package main

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/config"
	"github.com/MiguelFPP/tui-editor/internal/tui/app"
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

	if _, err := tea.NewProgram(app.New(cfg)).Run(); err != nil {
		return fmt.Errorf("ejecutar la TUI: %w", err)
	}

	return nil
}
