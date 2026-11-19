package dockerpanel

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
)

// BenchmarkInactiveTickNoPoll shows that a tick of a hidden or stale tab never reaches the port and
// does not allocate: the guard returns before any Docker call or new timer is built.
func BenchmarkInactiveTickNoPoll(b *testing.B) {
	docker := fakes.NewDockerService()
	m := New(docker, "")
	m.interval = 0
	m.SetSize(100, 24)

	m.SetActive(true)
	m.SetActive(false) // hides the tab and invalidates the generation

	msg := tea.Msg(tickMsg{gen: m.tickGen})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		next, cmd := m.Update(msg)
		if cmd != nil {
			b.Fatal("an inactive tick scheduled a command")
		}

		m = next
	}

	b.StopTimer()

	if got := docker.CallCount("Containers"); got != 0 {
		b.Fatalf("Containers calls = %d, want 0 while the tab is hidden", got)
	}
	if got := docker.CallCount("Stats"); got != 0 {
		b.Fatalf("Stats calls = %d, want 0 while the tab is hidden", got)
	}
}
