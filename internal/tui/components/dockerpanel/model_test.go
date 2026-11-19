package dockerpanel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
)

// testModel builds a panel of a fixed size; the ticker is shortened so a batch never waits.
func testModel(docker ports.DockerService) Model {
	m := New(docker)
	m.interval = 0
	m.SetSize(100, 24)

	return m
}

func sampleContainers() []domain.ContainerSummary {
	return []domain.ContainerSummary{
		{
			ID:     "aaaaaaaaaaaaaaaa",
			Name:   "web",
			Image:  "nginx:latest",
			State:  domain.ContainerRunning,
			Health: domain.HealthHealthy,
			Ports:  []domain.PortMapping{{HostPort: 8080, ContainerPort: 80, Protocol: domain.ProtocolTCP}},
		},
		{ID: "bbbbbbbbbbbbbbbb", Name: "db", Image: "postgres:16", State: domain.ContainerExited},
	}
}

func manyContainers(count int) []domain.ContainerSummary {
	out := make([]domain.ContainerSummary, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, domain.ContainerSummary{
			ID:    fmt.Sprintf("%032d", i),
			Name:  fmt.Sprintf("svc-%02d", i),
			Image: "busybox",
			State: domain.ContainerRunning,
		})
	}

	return out
}

func loaded(t *testing.T, docker ports.DockerService) Model {
	t.Helper()

	m := testModel(docker)
	m.SetActive(true) // marks the first load in flight, like entering the tab does
	m = feed(t, m, containersMsg{seq: m.containersSeq, containers: sampleContainers()})

	return m
}

// feed applies one message; it unwraps batches and drops the timer of the conditional ticker so a
// unit test never waits for the wall clock.
func feed(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()

	if msg == nil {
		return m
	}
	if _, ok := msg.(tickMsg); ok {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, nested := range batch {
			if nested == nil {
				continue
			}

			m = feed(t, m, nested())
		}

		return m
	}

	next, cmd := m.Update(msg)

	return drain(t, next, cmd)
}

func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		return m
	}

	return feed(t, m, cmd())
}

func typed(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func openLogs(t *testing.T, m Model) Model {
	t.Helper()

	next, cmd := m.Update(typed('l'))
	if cmd == nil {
		t.Fatal("l did not load the logs")
	}

	return drain(t, next, cmd)
}

func withLogs(docker *fakes.DockerService, page domain.LogPage) *fakes.DockerService {
	docker.OnLogs = func(context.Context, string, domain.LogOptions) (domain.LogPage, error) {
		return page, nil
	}

	return docker
}

func TestInactiveNoPoll(t *testing.T) {
	docker := fakes.NewDockerService()
	m := testModel(docker)

	if cmd := m.SetActive(true); cmd == nil {
		t.Fatal("SetActive(true) = nil, want the load and tick commands")
	}
	m.SetActive(false)

	next, cmd := m.Update(tickMsg{gen: m.tickGen})
	if cmd != nil {
		t.Fatalf("an inactive tick scheduled a command, want none")
	}
	if next.active {
		t.Error("active = true after SetActive(false)")
	}
	if got := docker.CallCount("Containers"); got != 0 {
		t.Errorf("Containers calls = %d, want 0 while the tab is hidden", got)
	}
	if got := docker.CallCount("Stats"); got != 0 {
		t.Errorf("Stats calls = %d, want 0 while the tab is hidden", got)
	}
}

func TestInactiveNoPollStaleGeneration(t *testing.T) {
	docker := fakes.NewDockerService()
	m := testModel(docker)

	m.SetActive(true)
	stale := m.tickGen
	m.SetActive(false)
	m.SetActive(true) // a new session bumps the generation

	next, cmd := m.Update(tickMsg{gen: stale})
	if cmd != nil {
		t.Fatal("a stale tick scheduled a command, want none")
	}
	if got := docker.CallCount("Containers"); got != 0 {
		t.Errorf("Containers calls = %d, want 0 for a stale generation", got)
	}
	if !next.active {
		t.Error("active = false, want the fresh session kept")
	}
}

func TestInactiveNoTickIsScheduled(t *testing.T) {
	m := testModel(fakes.NewDockerService())
	if m.active {
		t.Fatal("a fresh panel must not poll")
	}
	if cmd := m.scheduleTick(); cmd != nil {
		t.Error("scheduleTick() armed a ticker while inactive")
	}
}

func TestDebounceSkipsWhileLoading(t *testing.T) {
	docker := fakes.NewDockerService()
	m := testModel(docker)

	m.SetActive(true) // the entry load is still in flight
	if !m.loadingContainers {
		t.Fatal("SetActive(true) must start a load")
	}

	_, cmd := m.Update(tickMsg{gen: m.tickGen})
	if cmd == nil {
		t.Fatal("an active tick must re-arm the ticker")
	}

	next := drain(t, m, cmd)
	if got := docker.CallCount("Containers"); got != 0 {
		t.Errorf("Containers calls = %d, want 0 while a load is in flight", got)
	}
	if !next.loadingContainers {
		t.Error("loadingContainers = false, want the in-flight load kept")
	}
}

func TestDebounceOnePollPerTick(t *testing.T) {
	if defaultTickInterval != time.Second {
		t.Fatalf("defaultTickInterval = %v, want 1s", defaultTickInterval)
	}

	fresh := New(fakes.NewDockerService())
	if fresh.interval != time.Second {
		t.Errorf("interval = %v, want 1s by default", fresh.interval)
	}

	docker := fakes.NewDockerService()
	m := loaded(t, docker)
	docker.Reset()

	_, cmd := m.Update(tickMsg{gen: m.tickGen})
	if cmd == nil {
		t.Fatal("an active tick must re-arm the ticker")
	}
	m = drain(t, m, cmd)

	if got := docker.CallCount("Containers"); got != 1 {
		t.Errorf("Containers calls = %d, want exactly one per tick", got)
	}
	if got := docker.CallCount("Stats"); got != 1 {
		t.Errorf("Stats calls = %d, want exactly one per tick", got)
	}
}

func TestLogsOpenLoadsTailWithoutFollow(t *testing.T) {
	docker := fakes.NewDockerService()
	stamp := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	var gotID string

	var gotOpts domain.LogOptions

	docker.OnLogs = func(_ context.Context, id string, opts domain.LogOptions) (domain.LogPage, error) {
		gotID, gotOpts = id, opts

		return domain.LogPage{
			Lines: []domain.LogLine{
				{Timestamp: stamp, Stream: domain.Stdout, Text: "arrancando"},
				{Timestamp: stamp.Add(time.Second), Stream: domain.Stderr, Text: "aviso"},
			},
			NextBefore: stamp,
		}, nil
	}

	m := openLogs(t, loaded(t, docker))

	if !m.CapturesInput() {
		t.Fatal("CapturesInput() = false, want the logs to own the keyboard")
	}
	if gotID != sampleContainers()[0].ID {
		t.Errorf("Logs id = %q, want the selected container", gotID)
	}
	if gotOpts.Tail != domain.DefaultLogTail {
		t.Errorf("Tail = %d, want %d", gotOpts.Tail, domain.DefaultLogTail)
	}
	if !gotOpts.Before.IsZero() {
		t.Errorf("Before = %v, want the newest page first", gotOpts.Before)
	}

	view := m.View()
	for _, want := range []string{"logs", "arrancando", "aviso"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
}

func TestLogsEscapeCloses(t *testing.T) {
	m := openLogs(t, loaded(t, withLogs(fakes.NewDockerService(), domain.LogPage{})))

	next, cmd := m.Update(key(tea.KeyEscape))
	if cmd != nil {
		t.Errorf("esc returned a command, want none")
	}
	if next.CapturesInput() {
		t.Error("CapturesInput() = true, want the logs closed")
	}
}

func TestLogsReloadResetsToNewest(t *testing.T) {
	docker := fakes.NewDockerService()
	var opts []domain.LogOptions

	docker.OnLogs = func(_ context.Context, _ string, o domain.LogOptions) (domain.LogPage, error) {
		opts = append(opts, o)

		return domain.LogPage{Lines: []domain.LogLine{{Text: "línea"}}}, nil
	}

	m := openLogs(t, loaded(t, docker))

	next, cmd := m.Update(typed('r'))
	if cmd == nil {
		t.Fatal("r must reload the logs")
	}
	next = drain(t, next, cmd)

	if got := docker.CallCount("Logs"); got != 2 {
		t.Fatalf("Logs calls = %d, want 2", got)
	}
	if !opts[len(opts)-1].Before.IsZero() {
		t.Errorf("reload Before = %v, want the newest page", opts[len(opts)-1].Before)
	}
	if !next.CapturesInput() {
		t.Error("CapturesInput() = false, want the logs kept open")
	}
}

func TestLogsPaginationLoadsOlderBefore(t *testing.T) {
	docker := fakes.NewDockerService()
	older := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	var opts []domain.LogOptions

	docker.OnLogs = func(_ context.Context, _ string, o domain.LogOptions) (domain.LogPage, error) {
		opts = append(opts, o)
		if o.Before.IsZero() {
			return domain.LogPage{
				Lines:      []domain.LogLine{{Timestamp: older.Add(time.Minute), Text: "nuevo"}},
				NextBefore: older,
			}, nil
		}

		return domain.LogPage{Lines: []domain.LogLine{{Timestamp: older, Text: "antiguo"}}}, nil
	}

	m := openLogs(t, loaded(t, docker))

	next, cmd := m.Update(typed('k'))
	if cmd == nil {
		t.Fatal("scrolling up at the top must load the older page")
	}
	next = drain(t, next, cmd)

	if got := opts[len(opts)-1].Before; !got.Equal(older) {
		t.Errorf("older page Before = %v, want %v", got, older)
	}
	view := next.View()
	if !strings.Contains(view, "antiguo") || !strings.Contains(view, "nuevo") {
		t.Errorf("View() = %q, want the older page prepended", view)
	}
}

func TestLogsScrollsWithKeyboardAndWheel(t *testing.T) {
	lines := make([]domain.LogLine, 0, 40)
	for i := 0; i < 40; i++ {
		lines = append(lines, domain.LogLine{Text: fmt.Sprintf("línea-%02d", i)})
	}

	m := openLogs(t, loaded(t, withLogs(fakes.NewDockerService(), domain.LogPage{Lines: lines})))

	scrolled, _ := m.Update(typed('j'))
	if scrolled.viewport.YOffset() == 0 {
		t.Error("j did not scroll the logs")
	}

	wheeled, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if wheeled.viewport.YOffset() <= scrolled.viewport.YOffset() {
		t.Errorf("wheel offset = %d, want more than %d", wheeled.viewport.YOffset(), scrolled.viewport.YOffset())
	}
}

func TestLogsCapturesTheKeyboard(t *testing.T) {
	m := openLogs(t, loaded(t, withLogs(fakes.NewDockerService(), domain.LogPage{})))

	if !m.CapturesInput() {
		t.Fatal("CapturesInput() = false, want the logs to capture the input")
	}
}

func TestActionsRequireConfirmation(t *testing.T) {
	docker := fakes.NewDockerService()
	m := loaded(t, docker)

	next, cmd := m.Update(typed('s'))
	if cmd != nil {
		t.Errorf("s returned a command, want only a confirmation")
	}
	if !next.CapturesInput() {
		t.Fatal("s did not ask for confirmation")
	}
	if got := docker.CallCount("Start"); got != 0 {
		t.Fatalf("Start calls = %d, want 0 before the confirmation", got)
	}

	cancelled, _ := next.Update(typed('n'))
	if cancelled.CapturesInput() {
		t.Error("n did not cancel the confirmation")
	}
	if got := docker.CallCount("Start"); got != 0 {
		t.Errorf("Start calls = %d, want 0 after cancel", got)
	}

	escaped, _ := next.Update(key(tea.KeyEscape))
	if escaped.CapturesInput() {
		t.Error("esc did not cancel the confirmation")
	}
}

func TestActionsStartStopRestart(t *testing.T) {
	cases := []struct {
		name   string
		key    rune
		method string
	}{
		{name: "start", key: 's', method: "Start"},
		{name: "stop", key: 'x', method: "Stop"},
		{name: "restart", key: 'R', method: "Restart"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			docker := fakes.NewDockerService()
			m := loaded(t, docker)
			docker.Reset()

			confirming, cmd := m.Update(typed(tc.key))
			if cmd != nil {
				t.Fatalf("%q returned a command before the confirmation", tc.key)
			}
			if got := docker.CallCount(tc.method); got != 0 {
				t.Fatalf("%s calls = %d, want 0 before the confirmation", tc.method, got)
			}

			next, cmd := confirming.Update(typed('y'))
			if cmd == nil {
				t.Fatal("y did not start the action")
			}
			next = drain(t, next, cmd)

			if got := docker.CallCount(tc.method); got != 1 {
				t.Errorf("%s calls = %d, want 1", tc.method, got)
			}
			if next.CapturesInput() {
				t.Error("CapturesInput() = true, want the confirmation closed")
			}
			if next.writing {
				t.Error("writing = true after the action finished")
			}
			if got := docker.CallCount("Containers"); got != 1 {
				t.Errorf("Containers calls = %d, want a refresh after the action", got)
			}
		})
	}
}

func TestActionsErrorIsShown(t *testing.T) {
	docker := fakes.NewDockerService()
	docker.OnStart = func(context.Context, string) error { return domain.ErrPermissionDenied }

	m := loaded(t, docker)

	confirming, _ := m.Update(typed('s'))
	next, cmd := confirming.Update(typed('y'))
	if cmd == nil {
		t.Fatal("y did not start the action")
	}
	next = drain(t, next, cmd)

	if !errors.Is(next.Err(), domain.ErrPermissionDenied) {
		t.Fatalf("Err() = %v, want %v", next.Err(), domain.ErrPermissionDenied)
	}
	if next.CapturesInput() {
		t.Error("CapturesInput() = true, want the confirmation closed")
	}
	if !strings.Contains(next.View(), "error al actuar") {
		t.Errorf("View() = %q, want the action error", next.View())
	}
	if got := docker.CallCount("Containers"); got != 0 {
		t.Errorf("Containers calls = %d, want no refresh after a failed action", got)
	}
}

func TestActionsWithoutService(t *testing.T) {
	m := testModel(nil)
	m = feed(t, m, containersMsg{seq: m.containersSeq, containers: sampleContainers()})

	confirming, _ := m.Update(typed('s'))
	if !confirming.CapturesInput() {
		t.Fatal("s did not ask for confirmation")
	}

	next, cmd := confirming.Update(typed('y'))
	if cmd == nil {
		t.Fatal("y did not start the action")
	}
	next = drain(t, next, cmd)

	if !errors.Is(next.Err(), errNoService) {
		t.Fatalf("Err() = %v, want %v", next.Err(), errNoService)
	}
	if next.View() == "" {
		t.Fatal("View() is empty after a failed action")
	}
}

func TestUnavailableShowsActionableState(t *testing.T) {
	docker := fakes.NewDockerService()
	docker.OnContainers = func(context.Context) ([]domain.ContainerSummary, error) {
		return nil, domain.ErrDockerUnavailable
	}

	m := testModel(docker)
	m = feed(t, m, containersMsg{seq: m.containersSeq, err: domain.ErrDockerUnavailable})

	view := m.View()
	if !strings.Contains(view, "no disponible") {
		t.Errorf("View() = %q, want the unavailable state", view)
	}
	if !strings.Contains(view, "reintentar") {
		t.Errorf("View() = %q, want an actionable hint", view)
	}

	next, cmd := m.Update(typed('r'))
	if cmd == nil {
		t.Fatal("r did not retry")
	}
	next = drain(t, next, cmd)
	if !errors.Is(next.Err(), domain.ErrDockerUnavailable) {
		t.Errorf("Err() = %v, want the retry failure", next.Err())
	}
}

func TestUnavailableWithoutService(t *testing.T) {
	m := testModel(nil)

	if !strings.Contains(m.View(), "no disponible") {
		t.Errorf("View() = %q, want the unavailable state without a service", m.View())
	}

	next, cmd := m.Update(typed('r'))
	next = drain(t, next, cmd)
	if !errors.Is(next.Err(), errNoService) {
		t.Errorf("Err() = %v, want %v", next.Err(), errNoService)
	}
}

func TestTableRendersContainers(t *testing.T) {
	view := loaded(t, fakes.NewDockerService()).View()

	for _, want := range []string{"nombre", "imagen", "estado", "health", "puertos",
		"web", "nginx:latest", "running", "healthy", "8080->80/tcp", "db", "postgres:16", "exited"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
}

func TestStatsShownAfterEnter(t *testing.T) {
	docker := fakes.NewDockerService()
	docker.OnStats = func(_ context.Context, id string) (domain.ContainerStats, error) {
		return domain.ContainerStats{
			ContainerID: id,
			CPUPercent:  12.5,
			MemoryUsed:  10 * 1024 * 1024,
			MemoryLimit: 512 * 1024 * 1024,
		}, nil
	}

	m := loaded(t, docker)

	next, cmd := m.Update(key(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("enter did not read the stats")
	}
	next = drain(t, next, cmd)

	if got := docker.CallCount("Stats"); got != 1 {
		t.Fatalf("Stats calls = %d, want 1", got)
	}
	view := next.View()
	for _, want := range []string{"cpu 12.5%", "mem 10.0 MiB"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() = %q, want %q", view, want)
		}
	}
}

func TestNavigationMovesCursor(t *testing.T) {
	m := loaded(t, fakes.NewDockerService())

	m, _ = m.Update(typed('j'))
	if m.cursor != 1 {
		t.Fatalf("j: cursor = %d, want 1", m.cursor)
	}

	m, _ = m.Update(typed('k'))
	if m.cursor != 0 {
		t.Fatalf("k: cursor = %d, want 0", m.cursor)
	}

	m, _ = m.Update(key(tea.KeyEnd))
	if m.cursor != len(sampleContainers())-1 {
		t.Errorf("end: cursor = %d, want the last row", m.cursor)
	}

	m, _ = m.Update(key(tea.KeyHome))
	if m.cursor != 0 {
		t.Errorf("home: cursor = %d, want the first row", m.cursor)
	}
}

func TestClickSelectsRow(t *testing.T) {
	m := loaded(t, fakes.NewDockerService())
	m.SetBounds(0, 0, 100, 24)

	// The body starts below the title and the column header, so the second row sits at y = 3.
	next, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	if next.cursor != 1 {
		t.Errorf("cursor = %d, want the clicked row", next.cursor)
	}
}

func TestMouseWheelScrollsList(t *testing.T) {
	m := testModel(fakes.NewDockerService())
	m.SetActive(true)
	m = feed(t, m, containersMsg{seq: m.containersSeq, containers: manyContainers(30)})

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if next.offset == 0 {
		t.Error("the wheel did not scroll the list")
	}
	if next.offset != wheelStep {
		t.Errorf("offset = %d, want %d", next.offset, wheelStep)
	}
}

func TestCapturesInputWithLogsAndConfirm(t *testing.T) {
	m := loaded(t, withLogs(fakes.NewDockerService(), domain.LogPage{}))

	if m.CapturesInput() {
		t.Error("the container list must not capture the input")
	}

	confirming, _ := m.Update(typed('s'))
	if !confirming.CapturesInput() {
		t.Error("the confirmation must capture the input")
	}

	logs := openLogs(t, m)
	if !logs.CapturesInput() {
		t.Error("the logs must capture the input")
	}
}
