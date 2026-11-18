package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"tui-fer-editor/internal/ui"
)

func main() {
	p := tea.NewProgram(ui.New())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui-fer-editor: %v\n", err)
		os.Exit(1)
	}
}
