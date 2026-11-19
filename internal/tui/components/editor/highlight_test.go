package editor

import (
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
)

func TestHighlightUsesTheAppliedChromaStyle(t *testing.T) {
	const source = "package main\n"

	outputs := make(map[string]string, len(styles.Themes()))
	for _, theme := range styles.Themes() {
		t.Run(theme.Name, func(t *testing.T) {
			styles.Apply(theme)
			t.Cleanup(func() { styles.Apply(styles.DefaultTheme()) })

			got := Highlight("main.go", source)
			if got == source {
				t.Fatalf("Highlight() = %q, want it colourised, not the plain source", got)
			}
			if !strings.Contains(got, "\x1b[") {
				t.Errorf("Highlight() = %q, want ANSI escapes for the %q theme", got, theme.Name)
			}

			outputs[theme.Name] = got
		})
	}

	// Every theme maps to a different Chroma style, so the escapes have to differ.
	distinct := make(map[string]bool, len(outputs))
	for _, out := range outputs {
		distinct[out] = true
	}
	if len(distinct) != len(styles.Themes()) {
		t.Errorf("the themes produced %d distinct outputs, want %d (the Chroma style must be honoured)", len(distinct), len(styles.Themes()))
	}
}
