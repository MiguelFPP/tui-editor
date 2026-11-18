package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestModelWindowSize(t *testing.T) {
	m := New()

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd != nil {
		t.Fatalf("WindowSizeMsg produced a command, want nil")
	}

	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want ui.Model", next)
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
			next, cmd := New().Update(tc.msg)
			if cmd == nil {
				t.Fatalf("%q produced no command, want quit command", tc.name)
			}
			if _, ok := next.(Model); !ok {
				t.Fatalf("Update returned %T, want ui.Model", next)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("command returned %T, want tea.QuitMsg", cmd())
			}
		})
	}
}
