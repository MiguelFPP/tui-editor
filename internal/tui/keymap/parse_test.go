package keymap

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

func TestCanonicalKey(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "vacía", raw: "", want: ""},
		{name: "solo espacios", raw: "   ", want: ""},
		{name: "espacio", raw: " ", want: "space"},
		{name: "runa minúscula", raw: "e", want: "e"},
		{name: "runa mayúscula", raw: "E", want: "E"},
		{name: "dígito", raw: "1", want: "1"},
		{name: "signo", raw: "?", want: "?"},
		{name: "con espacios alrededor", raw: "  esc  ", want: "esc"},
		{name: "nombrada en mayúsculas", raw: "UP", want: "up"},
		{name: "tab", raw: "TAB", want: "tab"},
		{name: "shift tab", raw: "Shift+Tab", want: "shift+tab"},
		{name: "escape", raw: "Escape", want: "esc"},
		{name: "return", raw: "Return", want: "enter"},
		{name: "page up", raw: "PageUp", want: "pgup"},
		{name: "page down", raw: "PageDown", want: "pgdown"},
		{name: "ctrl mayúsculas", raw: "Ctrl+S", want: "ctrl+s"},
		{name: "alt mayúsculas", raw: "ALT+E", want: "alt+e"},
		{name: "ctrl tab inválida", raw: "Ctrl+Tab", want: "ctrl+tab"},
		{name: "desconocida", raw: "Fol", want: "fol"},
	}

	for _, tc := range cases {
		if got := CanonicalKey(tc.raw); got != tc.want {
			t.Errorf("%s: CanonicalKey(%q) = %q, want %q", tc.name, tc.raw, got, tc.want)
		}
	}
}

func TestCanonicalKeyIsIdempotent(t *testing.T) {
	raws := []string{
		"", " ", "   ", "e", "E", "up", "UP", "esc", "escape", "return", "ctrl+s", "Ctrl+S",
		"alt+E", "shift+tab", "Shift+Tab", "ctrl+tab", "foo", "?",
	}

	for _, raw := range raws {
		once := CanonicalKey(raw)
		if twice := CanonicalKey(once); twice != once {
			t.Errorf("CanonicalKey no es idempotente: raw %q, once %q, twice %q", raw, once, twice)
		}
	}
}

func TestParseKeyAccepts(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "letra minúscula", raw: "a", want: "a"},
		{name: "letra mayúscula", raw: "A", want: "A"},
		{name: "dígito", raw: "1", want: "1"},
		{name: "signo", raw: "?", want: "?"},
		{name: "barra", raw: "/", want: "/"},
		{name: "corchete", raw: "[", want: "["},
		{name: "espacio literal", raw: " ", want: "space"},
		{name: "espacio nombrado", raw: "space", want: "space"},
		{name: "esc", raw: "esc", want: "esc"},
		{name: "escape", raw: "escape", want: "esc"},
		{name: "enter", raw: "enter", want: "enter"},
		{name: "return", raw: "return", want: "enter"},
		{name: "tab", raw: "tab", want: "tab"},
		{name: "shift tab", raw: "shift+tab", want: "shift+tab"},
		{name: "shift tab mayúsculas", raw: "Shift+Tab", want: "shift+tab"},
		{name: "up", raw: "up", want: "up"},
		{name: "down", raw: "down", want: "down"},
		{name: "left", raw: "left", want: "left"},
		{name: "right", raw: "right", want: "right"},
		{name: "home", raw: "home", want: "home"},
		{name: "end", raw: "end", want: "end"},
		{name: "pgup", raw: "pgup", want: "pgup"},
		{name: "pageup", raw: "pageup", want: "pgup"},
		{name: "pgdown", raw: "PgDown", want: "pgdown"},
		{name: "pagedown", raw: "pagedown", want: "pgdown"},
		{name: "backspace", raw: "backspace", want: "backspace"},
		{name: "delete", raw: "delete", want: "delete"},
		{name: "insert", raw: "insert", want: "insert"},
		{name: "f1", raw: "f1", want: "f1"},
		{name: "f12", raw: "f12", want: "f12"},
		{name: "ctrl c", raw: "ctrl+c", want: "ctrl+c"},
		{name: "ctrl s mayúsculas", raw: "Ctrl+S", want: "ctrl+s"},
		{name: "alt e", raw: "alt+e", want: "alt+e"},
		{name: "alt e mayúsculas", raw: "ALT+E", want: "alt+e"},
	}

	for _, tc := range cases {
		got, err := ParseKey(tc.raw)
		if err != nil {
			t.Errorf("%s: ParseKey(%q) = %v, want %q", tc.name, tc.raw, err, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: ParseKey(%q) = %q, want %q", tc.name, tc.raw, got, tc.want)
		}
	}
}

func TestParseKeyRejects(t *testing.T) {
	raws := []string{
		"", "   ", "foo", "ctrl+", "ctrl+ab", "shift+x", "ctrl+tab", "ctrl+shift+a",
		"meta+x", "f13", "returnx",
	}

	for _, raw := range raws {
		if got, err := ParseKey(raw); err == nil {
			t.Errorf("ParseKey(%q) = %q, want an error", raw, got)
		}
	}
}

// messageFor builds the Bubble Tea message a real terminal would deliver for
// the canonical key. It returns the zero message when the key is not one of the
// defaults, so the caller can report it.
func messageFor(key string) tea.KeyPressMsg {
	switch key {
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "delete":
		return tea.KeyPressMsg{Code: tea.KeyDelete}
	case "insert":
		return tea.KeyPressMsg{Code: tea.KeyInsert}
	}

	if rest, ok := strings.CutPrefix(key, "ctrl+"); ok {
		r, _ := utf8.DecodeRuneInString(rest)
		return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
	}
	if rest, ok := strings.CutPrefix(key, "alt+"); ok {
		r, _ := utf8.DecodeRuneInString(rest)
		return tea.KeyPressMsg{Code: r, Mod: tea.ModAlt}
	}
	if utf8.RuneCountInString(key) == 1 {
		r, _ := utf8.DecodeRuneInString(key)
		return tea.KeyPressMsg{Code: r, Text: key}
	}

	return tea.KeyPressMsg{}
}

// TestDefaultKeysRoundTrip goes from the catalog to a Bubble Tea message and
// back: every default key, fixed ones included, must parse and match its action.
func TestDefaultKeysRoundTrip(t *testing.T) {
	registry := Default()

	for _, binding := range registry.All() {
		for _, key := range effectiveKeys(binding) {
			msg := messageFor(key)
			if msg.Code == 0 && msg.Text == "" {
				t.Fatalf("no se pudo construir el mensaje de la tecla %q de %q", key, binding.Action)
			}

			canonical, err := ParseKey(msg.String())
			if err != nil {
				t.Fatalf("ParseKey(%q) para la tecla %q de %q: %v", msg.String(), key, binding.Action, err)
			}
			if canonical != key {
				t.Errorf("ParseKey(%q) = %q, want %q (acción %q)", msg.String(), canonical, key, binding.Action)
			}

			action, ok := registry.Match(binding.Scope, msg.String())
			if !ok || action != binding.Action {
				t.Errorf("Match(%q, %q) = (%q, %v), want (%q, true)", binding.Scope, msg.String(), action, ok, binding.Action)
			}
		}
	}
}
