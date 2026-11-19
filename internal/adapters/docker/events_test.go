package docker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

func TestEventsMapsActions(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)

	cases := []struct {
		name string
		msg  events.Message
		want domain.DockerEventKind
		ok   bool
	}{
		{"create", containerMessage(events.ActionCreate, at), domain.DockerEventCreated, true},
		{"start", containerMessage(events.ActionStart, at), domain.DockerEventStarted, true},
		{"restart", containerMessage(events.ActionRestart, at), domain.DockerEventRestarted, true},
		{"stop", containerMessage(events.ActionStop, at), domain.DockerEventStopped, true},
		{"die", containerMessage(events.ActionDie, at), domain.DockerEventDied, true},
		{"destroy", containerMessage(events.ActionDestroy, at), domain.DockerEventRemoved, true},
		{"remove", containerMessage(events.ActionRemove, at), domain.DockerEventRemoved, true},
		{"pause", containerMessage(events.ActionPause, at), domain.DockerEventPaused, true},
		{"unpause", containerMessage(events.ActionUnPause, at), domain.DockerEventUnpaused, true},
		{"health", containerMessage(events.ActionHealthStatus, at), domain.DockerEventHealthChanged, true},
		{"health healthy", containerMessage(events.ActionHealthStatusHealthy, at), domain.DockerEventHealthChanged, true},
		{"resize", containerMessage(events.ActionResize, at), domain.DockerEventUnknown, false},
		{"exec", containerMessage(events.ActionExecStart, at), domain.DockerEventUnknown, false},
		{"tipo red", events.Message{Type: events.NetworkEventType, Action: events.ActionCreate}, domain.DockerEventUnknown, false},
		{"sin tipo", events.Message{Action: events.ActionStart}, domain.DockerEventUnknown, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := dockerEvent(tc.msg)
			if ok != tc.ok {
				t.Fatalf("dockerEvent() ok = %t, want %t", ok, tc.ok)
			}
			if !tc.ok {
				return
			}
			if got.Kind != tc.want {
				t.Errorf("Kind = %v, want %v", got.Kind, tc.want)
			}
			if got.ContainerID != "abc123" {
				t.Errorf("ContainerID = %q, want %q", got.ContainerID, "abc123")
			}
			if !got.At.Equal(at) {
				t.Errorf("At = %v, want %v", got.At, at)
			}
		})
	}
}

func TestEventsUsesSecondsWhenNanosAreMissing(t *testing.T) {
	const seconds = 1700000000

	got, ok := dockerEvent(events.Message{
		Type:   events.ContainerEventType,
		Action: events.ActionStart,
		Actor:  events.Actor{ID: "abc123"},
		Time:   seconds,
	})
	if !ok {
		t.Fatal("dockerEvent() ok = false, want true")
	}
	if want := time.Unix(seconds, 0).UTC(); !got.At.Equal(want) {
		t.Errorf("At = %v, want %v", got.At, want)
	}
}

func TestEventsDebounce(t *testing.T) {
	api := &fakeAPI{}
	api.ping = func(context.Context, client.PingOptions) (client.PingResult, error) {
		return client.PingResult{}, nil
	}

	messages := make(chan events.Message)
	errs := make(chan error, 1)
	api.events = func(context.Context, client.EventsListOptions) client.EventsResult {
		return client.EventsResult{Messages: messages, Err: errs}
	}

	clk := newFakeClock()
	service := newService(api, clk)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := service.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error = %v, want nil", err)
	}

	emit := func(action events.Action) {
		t.Helper()

		messages <- events.Message{
			Type:     events.ContainerEventType,
			Action:   action,
			TimeNano: clk.Now().UnixNano(),
			Actor:    events.Actor{ID: "abc123"},
		}
	}

	emit(events.ActionStart)
	clk.waitTimer(t)

	first := receive(t, out)
	if first.Kind != domain.DockerEventStarted {
		t.Errorf("Kind = %v, want %v", first.Kind, domain.DockerEventStarted)
	}
	if first.ContainerID != "abc123" {
		t.Errorf("ContainerID = %q, want %q", first.ContainerID, "abc123")
	}
	if !first.At.Equal(clk.Now()) {
		t.Errorf("At = %v, want %v", first.At, clk.Now())
	}

	// Two events inside the window collapse into the last one.
	emit(events.ActionStop)
	emit(events.ActionRestart)
	clk.Advance(debounceWindow)

	second := receive(t, out)
	if second.Kind != domain.DockerEventRestarted {
		t.Errorf("Kind = %v, want %v", second.Kind, domain.DockerEventRestarted)
	}
	clk.waitTimer(t)
	drain(t, out)

	cancel()
	if _, ok := <-out; ok {
		t.Error("el canal sigue abierto tras cancelar")
	}
}

func TestEventsDropsUntrackedMessages(t *testing.T) {
	api := &fakeAPI{}
	api.ping = func(context.Context, client.PingOptions) (client.PingResult, error) {
		return client.PingResult{}, nil
	}

	messages := make(chan events.Message)
	errs := make(chan error, 1)
	api.events = func(context.Context, client.EventsListOptions) client.EventsResult {
		return client.EventsResult{Messages: messages, Err: errs}
	}

	clk := newFakeClock()
	service := newService(api, clk)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := service.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error = %v, want nil", err)
	}

	// The sends synchronise with the pump, which never arms a timer for these messages.
	messages <- events.Message{Type: events.NetworkEventType, Action: events.ActionCreate}
	messages <- events.Message{Type: events.ContainerEventType, Action: events.ActionResize, Actor: events.Actor{ID: "abc123"}}

	if created := clk.timersCreated(); created != 0 {
		t.Errorf("temporizadores armados = %d, want 0", created)
	}
	drain(t, out)

	// A tracked event still arrives on the leading edge, with no window consumed.
	messages <- events.Message{Type: events.ContainerEventType, Action: events.ActionStart, Actor: events.Actor{ID: "abc123"}}

	if event := receive(t, out); event.Kind != domain.DockerEventStarted {
		t.Errorf("Kind = %v, want %v", event.Kind, domain.DockerEventStarted)
	}
}

func TestEventsClosesOnStreamEnd(t *testing.T) {
	api := &fakeAPI{}
	api.ping = func(context.Context, client.PingOptions) (client.PingResult, error) {
		return client.PingResult{}, nil
	}

	messages := make(chan events.Message)
	errs := make(chan error, 1)
	api.events = func(context.Context, client.EventsListOptions) client.EventsResult {
		return client.EventsResult{Messages: messages, Err: errs}
	}

	service := newService(api, newFakeClock())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := service.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error = %v, want nil", err)
	}

	close(errs)

	if _, ok := <-out; ok {
		t.Error("el canal sigue abierto tras terminar el stream")
	}
}

func TestUnavailableEvents(t *testing.T) {
	api := &fakeAPI{}
	api.ping = func(context.Context, client.PingOptions) (client.PingResult, error) {
		return client.PingResult{}, errors.New("daemon caído")
	}
	api.events = func(context.Context, client.EventsListOptions) client.EventsResult {
		t.Error("Events() abrió el stream con el daemon caído")

		return client.EventsResult{}
	}

	service := newService(api, newFakeClock())
	if _, err := service.Events(context.Background()); err == nil {
		t.Error("Events() error = nil, want un error")
	}
}

func containerMessage(action events.Action, at time.Time) events.Message {
	return events.Message{
		Type:     events.ContainerEventType,
		Action:   action,
		TimeNano: at.UnixNano(),
		Actor:    events.Actor{ID: "abc123"},
	}
}

// receive waits for one event with a guard against a hung adapter.
func receive(t *testing.T, out <-chan domain.DockerEvent) domain.DockerEvent {
	t.Helper()

	select {
	case event, ok := <-out:
		if !ok {
			t.Fatal("el canal se cerró antes de recibir el evento")
		}

		return event
	case <-time.After(2 * time.Second):
		t.Fatal("no llegó ningún evento")

		return domain.DockerEvent{}
	}
}

// drain fails when an event is buffered after the adapter reached a known point.
func drain(t *testing.T, out <-chan domain.DockerEvent) {
	t.Helper()

	select {
	case event := <-out:
		t.Fatalf("evento inesperado: %+v", event)
	default:
	}
}
