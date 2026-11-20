package gitpanel

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

// BenchmarkInactiveTickNoPoll asserts that a tick of a hidden tab costs nothing: no read and no
// new timer, so a panel left in the background never touches the repository.
func BenchmarkInactiveTickNoPoll(b *testing.B) {
	git := fakes.NewGitService()
	m := New(git, keymap.Default())
	m.interval = 0
	m.SetSize(100, 24)

	m.SetActive(true)
	m.SetActive(false)

	msg := tea.Msg(tickMsg{gen: m.tickGen})

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		next, cmd := m.Update(msg)
		if cmd != nil {
			b.Fatal("an inactive tick scheduled a command")
		}

		m = next
	}

	b.StopTimer()

	if got := git.CallCount("Status"); got != 0 {
		b.Fatalf("Status calls = %d, want none while the tab is hidden", got)
	}
}
