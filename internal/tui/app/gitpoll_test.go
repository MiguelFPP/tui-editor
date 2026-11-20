package app

import (
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
)

// gitTab opens the Git tab and drains the command it returns: entering it starts the poller.
func gitTab(t *testing.T, m Model) Model {
	t.Helper()

	m, cmd := press(t, m, typed('2'))
	if m.Tab() != GitTab {
		t.Fatalf("Tab() = %v, want Git", m.Tab())
	}

	return drain(t, m, cmd)
}

func TestGitTabStartsThePoll(t *testing.T) {
	deps := testDeps(t)
	git := deps.Git.(*fakes.GitService)
	m := newSized(t, deps, 120, 40)
	drain(t, m, m.Init())
	git.Reset()

	m = gitTab(t, m)

	if got := git.CallCount("Status"); got == 0 {
		t.Error("Status calls = 0, want the immediate read of entering the Git tab")
	}
}

func TestGitTabSwitchKeepsThePanelUsable(t *testing.T) {
	deps := testDeps(t)
	m := newSized(t, deps, 120, 40)
	drain(t, m, m.Init())

	m = gitTab(t, m)

	if m.Focus() != FocusGit {
		t.Errorf("Focus() = %v, want the Git panel focused", m.Focus())
	}
	if view := m.View().Content; strings.TrimSpace(view) == "" {
		t.Error("View() is empty after entering the Git tab")
	}
}

func TestLeavingTheGitTabStopsThePoll(t *testing.T) {
	deps := testDeps(t)
	m := newSized(t, deps, 120, 40)
	drain(t, m, m.Init())
	m = gitTab(t, m)

	m, _ = press(t, m, typed('1')) // back to the Files tab
	if m.Tab() != FilesTab {
		t.Fatalf("Tab() = %v, want Files", m.Tab())
	}
	if m.Focus() != FocusTree {
		t.Errorf("Focus() = %v, want the Git panel to hand the keyboard back", m.Focus())
	}
}
