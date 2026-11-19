package main

import (
	"bytes"
	"strings"
	"testing"
)

// outputLines splits text into its non-empty lines, dropping the trailing newline.
func outputLines(text string) []string {
	trimmed := strings.TrimSuffix(text, "\n")
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

func TestPrintThemes(t *testing.T) {
	var out bytes.Buffer

	printThemes(&out)

	lines := outputLines(out.String())
	if len(lines) != 4 {
		t.Fatalf("printThemes() escribió %d líneas, want 4:\n%s", len(lines), out.String())
	}

	for _, want := range []string{"tokyonight-night", "catppuccin-mocha", "dracula", "gruvbox-dark"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("printThemes() no incluye %q:\n%s", want, out.String())
		}
	}

	flagged := 0
	for _, line := range lines {
		if !strings.Contains(line, "(default)") {
			continue
		}

		flagged++
		if !strings.HasPrefix(line, "tokyonight-night ") {
			t.Errorf("línea con (default) = %q, want tokyonight-night", line)
		}
	}
	if flagged != 1 {
		t.Errorf("printThemes() marcó %d temas por defecto, want 1:\n%s", flagged, out.String())
	}
}

func TestPrintKeys(t *testing.T) {
	var out bytes.Buffer

	printKeys(&out)

	text := out.String()
	lines := outputLines(text)
	if len(lines) == 0 {
		t.Fatal("printKeys() no escribió nada")
	}
	if lines[0] != "accion\tscope\tteclas\ttipo" {
		t.Errorf("printKeys() cabecera = %q, want %q", lines[0], "accion\tscope\tteclas\ttipo")
	}

	for _, want := range []string{
		"quit\troot\tq / ctrl+c\tconfigurable",
		"git_commit_save\tgit_commit\tctrl+s\tfija",
		"docker_logs\tdocker\tl\tconfigurable",
	} {
		if !strings.Contains("\n"+text, "\n"+want+"\n") {
			t.Errorf("printKeys() no incluye %q:\n%s", want, text)
		}
	}

	for index, line := range lines[1:] {
		if columns := strings.Split(line, "\t"); len(columns) != 4 {
			t.Errorf("línea %d = %q tiene %d columnas, want 4", index+2, line, len(columns))
		}
	}
}
