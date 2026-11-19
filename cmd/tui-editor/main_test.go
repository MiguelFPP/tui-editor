package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/config"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
	"github.com/MiguelFPP/tui-editor/internal/tui/styles"
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

// configOutput mirrors the JSON document written by printConfig.
type configOutput struct {
	ConfigPath    string              `json:"config_path"`
	Theme         string              `json:"theme"`
	TreeWidth     int                 `json:"tree_width"`
	TreeCollapsed bool                `json:"tree_collapsed"`
	GitLayout     string              `json:"git_layout"`
	Keys          map[string][]string `json:"keys"`
}

// decodeConfigOutput parses the JSON emitted by printConfig.
func decodeConfigOutput(t *testing.T, text string) configOutput {
	t.Helper()

	var out configOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", text, err)
	}

	return out
}

// failingWriter rejects every write, to cover the encoding error path.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("escritura fallida")
}

func TestExampleConfigIsValid(t *testing.T) {
	path := filepath.Join("..", "..", "config.example.json")

	cfg, err := config.Parse([]string{"--config", path, "--repo", t.TempDir()}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if _, err := keymap.Default().Apply(cfg.Keys); err != nil {
		t.Errorf("keymap.Apply() error = %v, want nil", err)
	}
	if _, ok := styles.ThemeByName(cfg.Theme); !ok {
		t.Errorf("ThemeByName(%q) = false, want true", cfg.Theme)
	}
	if cfg.TreeWidth != 42 {
		t.Errorf("TreeWidth = %d, want 42", cfg.TreeWidth)
	}
	if cfg.TreeCollapsed {
		t.Error("TreeCollapsed = true, want false")
	}
}

func TestInitConfigTemplateHasTreeLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")

	if err := config.WriteDefault(path, false); err != nil {
		t.Fatalf("WriteDefault() error = %v, want nil", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	for _, want := range []string{`"tree_width"`, `"tree_collapsed"`, `"git_layout"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("template = %q, want %s", body, want)
		}
	}

	// The template must stay a valid, strictly-parsed config file.
	cfg, err := config.Parse([]string{"--config", path, "--repo", t.TempDir()}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if cfg.TreeWidth != 0 || cfg.TreeCollapsed {
		t.Errorf("Parse() = %+v, want zero tree layout", cfg)
	}
}

func TestPrintConfig(t *testing.T) {
	var out bytes.Buffer

	cfg := config.Config{
		ConfigPath:    "/tmp/config.json",
		Theme:         "dracula",
		TreeWidth:     42,
		TreeCollapsed: true,
		GitLayout:     "single",
		Keys:          map[string][]string{"quit": {"q", "c"}, "docker_logs": {"o"}},
	}

	if err := printConfig(&out, cfg); err != nil {
		t.Fatalf("printConfig() error = %v, want nil", err)
	}

	got := decodeConfigOutput(t, out.String())
	if got.ConfigPath != cfg.ConfigPath {
		t.Errorf("config_path = %q, want %q", got.ConfigPath, cfg.ConfigPath)
	}
	if got.Theme != "dracula" {
		t.Errorf("theme = %q, want %q", got.Theme, "dracula")
	}
	if got.TreeWidth != 42 {
		t.Errorf("tree_width = %d, want 42", got.TreeWidth)
	}
	if !got.TreeCollapsed {
		t.Error("tree_collapsed = false, want true")
	}
	if got.GitLayout != "single" {
		t.Errorf("git_layout = %q, want %q", got.GitLayout, "single")
	}
	if !slices.Equal(got.Keys["quit"], []string{"q", "c"}) {
		t.Errorf("keys[quit] = %v, want [q c]", got.Keys["quit"])
	}
	if !slices.Equal(got.Keys["docker_logs"], []string{"o"}) {
		t.Errorf("keys[docker_logs] = %v, want [o]", got.Keys["docker_logs"])
	}
	if !strings.Contains(out.String(), "\n  \"theme\":") {
		t.Errorf("salida = %q, want JSON indentado con dos espacios", out.String())
	}

	// The field order stays stable: config_path, theme, tree_width,
	// tree_collapsed, git_layout, keys.
	last := -1
	for _, key := range []string{`"config_path"`, `"theme"`, `"tree_width"`, `"tree_collapsed"`, `"git_layout"`, `"keys"`} {
		index := strings.Index(out.String(), key)
		if index < 0 {
			t.Fatalf("salida no incluye %s: %q", key, out.String())
		}
		if index < last {
			t.Errorf("orden de campos incorrecto en %q", out.String())
		}

		last = index
	}
}

func TestPrintConfigDefaultTheme(t *testing.T) {
	var out bytes.Buffer

	if err := printConfig(&out, config.Config{}); err != nil {
		t.Fatalf("printConfig() error = %v, want nil", err)
	}

	got := decodeConfigOutput(t, out.String())
	if got.Theme != styles.DefaultTheme().Name {
		t.Errorf("theme = %q, want %q", got.Theme, styles.DefaultTheme().Name)
	}
	if got.TreeWidth != 0 {
		t.Errorf("tree_width = %d, want 0", got.TreeWidth)
	}
	if got.TreeCollapsed {
		t.Error("tree_collapsed = true, want false")
	}
	if got.GitLayout != "split" {
		t.Errorf("git_layout = %q, want the split default", got.GitLayout)
	}
	if got.Keys == nil {
		t.Error("keys = nil, want an empty object")
	}
	if len(got.Keys) != 0 {
		t.Errorf("keys = %v, want empty", got.Keys)
	}
	if !strings.Contains(out.String(), `"keys": {}`) {
		t.Errorf("salida = %q, want un objeto keys vacío", out.String())
	}
	if !strings.Contains(out.String(), `"tree_width": 0`) {
		t.Errorf("salida = %q, want la clave tree_width", out.String())
	}
	if !strings.Contains(out.String(), `"tree_collapsed": false`) {
		t.Errorf("salida = %q, want la clave tree_collapsed", out.String())
	}
}

func TestPrintConfigInvalidTheme(t *testing.T) {
	var out bytes.Buffer

	if err := printConfig(&out, config.Config{Theme: "nope"}); err == nil {
		t.Fatal("printConfig() error = nil, want an error")
	}
	if out.Len() != 0 {
		t.Errorf("printConfig() escribió %q, want nothing", out.String())
	}
}

func TestPrintConfigNilWriter(t *testing.T) {
	if err := printConfig(nil, config.Config{Theme: "dracula"}); err != nil {
		t.Fatalf("printConfig(nil) error = %v, want nil", err)
	}
}

func TestPrintConfigWriteError(t *testing.T) {
	if err := printConfig(failingWriter{}, config.Config{Theme: "dracula"}); err == nil {
		t.Fatal("printConfig() error = nil, want an error")
	}
}
