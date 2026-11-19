package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
)

func TestDockerTabSwitchAndFocus(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)

	// Entering the Docker tab returns the commands of the panel; the ticker they carry belongs to
	// the runtime, so the test only presses the key.
	m, _ = press(t, m, typed('3'))
	if m.Tab() != DockerTab {
		t.Fatalf("Tab() = %v, want Docker", m.Tab())
	}
	if m.Focus() != FocusDocker {
		t.Errorf("Focus() = %v, want docker", m.Focus())
	}
	if !strings.Contains(m.View().Content, "Docker") {
		t.Errorf("View() = %q, want the Docker tab label", m.View().Content)
	}
	if got, want := m.dockerRect.Dx()+2*borderSide, 120; got != want {
		t.Errorf("docker frame width = %d, want the full width %d", got, want)
	}

	m, _ = press(t, m, typed('1'))
	if m.Tab() != FilesTab || m.Focus() != FocusTree {
		t.Errorf("tab/focus = %v/%v, want Files/tree", m.Tab(), m.Focus())
	}
}

func TestDockerTabMouseFocusAndWheel(t *testing.T) {
	m := newSized(t, testDeps(t), 120, 40)
	m, _ = press(t, m, typed('3'))

	m.focus = FocusTree
	m = update(t, m, click(10, 5))
	if m.Focus() != FocusDocker {
		t.Errorf("Focus() = %v, want docker after a click in the panel", m.Focus())
	}

	m = update(t, m, wheel(tea.MouseWheelDown, 10, 5))
	if m.Focus() != FocusDocker {
		t.Errorf("Focus() = %v, want docker after the wheel", m.Focus())
	}
}

func TestDockerPanelCapturesInputAtRoot(t *testing.T) {
	deps := testDeps(t)
	docker := deps.Docker.(*fakes.DockerService)

	m := newSized(t, deps, 120, 40)
	m, _ = press(t, m, typed('3'))

	// r loads the containers without involving the ticker, so the batch finishes in the test.
	m, cmd := press(t, m, typed('r'))
	m = drain(t, m, cmd)
	if got := docker.CallCount("Containers"); got != 1 {
		t.Fatalf("Containers calls = %d, want 1 after r", got)
	}

	m, cmd = press(t, m, typed('l'))
	m = drain(t, m, cmd)
	if !m.docker.CapturesInput() {
		t.Fatal("CapturesInput() = false, want the logs open")
	}

	// q, the numbers and the help must reach the panel instead of the root keymap.
	for _, msg := range []tea.KeyPressMsg{typed('q'), typed('1'), typed('2'), typed('?')} {
		m, _ = press(t, m, msg)
	}
	if m.Tab() != DockerTab || m.HelpVisible() {
		t.Errorf("tab = %v, help = %v, want the Docker tab and no help", m.Tab(), m.HelpVisible())
	}
	if !m.docker.CapturesInput() {
		t.Error("CapturesInput() = false, want the logs kept open")
	}

	// ctrl+c still quits while the logs own the keyboard.
	if _, quit := press(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); quit == nil {
		t.Fatal("no command, want quit")
	} else if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatalf("command returned %T, want tea.QuitMsg", quit())
	}

	// esc closes the logs and the root shortcuts work again.
	m, _ = press(t, m, key(tea.KeyEscape))
	if m.docker.CapturesInput() {
		t.Fatal("CapturesInput() = true after esc")
	}
	m, _ = press(t, m, typed('1'))
	if m.Tab() != FilesTab {
		t.Errorf("Tab() = %v, want the root shortcuts back", m.Tab())
	}
}
