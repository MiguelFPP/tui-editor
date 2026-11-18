package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/config"
)

func newTestConfig(t *testing.T) config.Config {
	t.Helper()

	return config.Config{Repo: t.TempDir(), Editor: "vim"}
}

func TestNewKeepsConfig(t *testing.T) {
	cfg := newTestConfig(t)

	m := New(cfg)
	if m.cfg != cfg {
		t.Errorf("cfg = %+v, want %+v", m.cfg, cfg)
	}
}

func TestModelWindowSize(t *testing.T) {
	m := New(newTestConfig(t))

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd != nil {
		t.Fatalf("WindowSizeMsg produced a command, want nil")
	}

	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want app.Model", next)
	}
	if got.width != 120 {
		t.Errorf("width = %d, want 120", got.width)
	}
	if got.height != 40 {
		t.Errorf("height = %d, want 40", got.height)
	}
}

func TestModelQuitKeys(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{name: "q", msg: tea.KeyPressMsg{Code: 'q', Text: "q"}},
		{name: "ctrl+c", msg: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, cmd := New(newTestConfig(t)).Update(tc.msg)
			if cmd == nil {
				t.Fatalf("%q produced no command, want quit command", tc.name)
			}
			if _, ok := next.(Model); !ok {
				t.Fatalf("Update returned %T, want app.Model", next)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("command returned %T, want tea.QuitMsg", cmd())
			}
		})
	}
}

func TestModelIgnoresOtherKeys(t *testing.T) {
	m := New(newTestConfig(t))

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil {
		t.Fatalf("unhandled key produced a command, want nil")
	}
	if _, ok := next.(Model); !ok {
		t.Fatalf("Update returned %T, want app.Model", next)
	}
}

func TestViewShowsTitleRepoAndAltScreen(t *testing.T) {
	cfg := newTestConfig(t)

	view := New(cfg).View()
	if !view.AltScreen {
		t.Error("AltScreen = false, want true")
	}
	if !strings.Contains(view.Content, "tui-editor") {
		t.Errorf("content %q does not contain the program name", view.Content)
	}
	if !strings.Contains(view.Content, cfg.Repo) {
		t.Errorf("content %q does not contain the repo %q", view.Content, cfg.Repo)
	}
}
