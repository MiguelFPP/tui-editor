package fakes_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
)

const eventsBuffer = 16

func TestDockerServiceEventsDeliversAndCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	svc := fakes.NewDockerService()

	events, err := svc.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error = %v, want nil", err)
	}
	if events == nil {
		t.Fatal("Events() returned a nil channel")
	}
	if got, want := cap(events), eventsBuffer; got != want {
		t.Errorf("cap(events) = %d, want %d", got, want)
	}

	first := domain.DockerEvent{Kind: domain.DockerEventStarted, ContainerID: "abc"}
	second := domain.DockerEvent{Kind: domain.DockerEventStopped, ContainerID: "abc"}
	svc.Emit(first)
	svc.Emit(second)

	if got := <-events; got != first {
		t.Errorf("event = %+v, want %+v", got, first)
	}
	if got := <-events; got != second {
		t.Errorf("event = %+v, want %+v", got, second)
	}

	cancel()
	waitClosed(t, events)

	svc.Emit(domain.DockerEvent{Kind: domain.DockerEventRemoved, ContainerID: "abc"})

	if got, want := svc.CallCount("Events"), 1; got != want {
		t.Errorf("CallCount(Events) = %d, want %d", got, want)
	}
}

func TestDockerServiceEmitDropsWhenBufferIsFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := fakes.NewDockerService()
	events, err := svc.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error = %v, want nil", err)
	}

	for range eventsBuffer * 4 {
		svc.Emit(domain.DockerEvent{Kind: domain.DockerEventCreated, ContainerID: "abc"})
	}

	if got, want := len(events), eventsBuffer; got != want {
		t.Errorf("len(events) = %d, want %d: Emit must drop events when the buffer is full", got, want)
	}
}

func TestDockerServiceEventsReusesOneChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	svc := fakes.NewDockerService()

	first, err := svc.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error = %v, want nil", err)
	}
	second, err := svc.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error = %v, want nil", err)
	}

	if first != second {
		t.Error("Events() must return the same channel for one fake")
	}
	if got, want := svc.CallCount("Events"), 2; got != want {
		t.Errorf("CallCount(Events) = %d, want %d", got, want)
	}

	cancel()
	waitClosed(t, first)
}

func TestDockerServiceHooksAndZeroValues(t *testing.T) {
	svc := fakes.NewDockerService()

	if err := svc.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() with a nil hook error = %v, want nil", err)
	}
	containers, err := svc.Containers(context.Background())
	if err != nil || len(containers) != 0 {
		t.Errorf("Containers() = %v, %v, want none and nil", containers, err)
	}
	stats, err := svc.Stats(context.Background(), "abc")
	if err != nil || stats != (domain.ContainerStats{}) {
		t.Errorf("Stats() = %+v, %v, want the zero stats and nil", stats, err)
	}
	page, err := svc.Logs(context.Background(), "abc", domain.LogOptions{})
	if err != nil || len(page.Lines) != 0 || page.Truncated {
		t.Errorf("Logs() = %+v, %v, want an empty page and nil", page, err)
	}
	if err := svc.Start(context.Background(), "abc"); err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}
	if err := svc.Stop(context.Background(), "abc"); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
	if err := svc.Restart(context.Background(), "abc"); err != nil {
		t.Errorf("Restart() error = %v, want nil", err)
	}

	svc.OnPing = func(ctx context.Context) error {
		return domain.ErrDockerUnavailable
	}
	if err := svc.Ping(context.Background()); !errors.Is(err, domain.ErrDockerUnavailable) {
		t.Errorf("Ping() error = %v, want %v", err, domain.ErrDockerUnavailable)
	}

	svc.OnContainers = func(ctx context.Context) ([]domain.ContainerSummary, error) {
		return []domain.ContainerSummary{{ID: "abc", Name: "web", State: domain.ContainerRunning}}, nil
	}
	containers, err = svc.Containers(context.Background())
	if err != nil || len(containers) != 1 || !containers[0].Running() {
		t.Errorf("Containers() = %+v, %v, want one running container and nil", containers, err)
	}

	svc.OnLogs = func(ctx context.Context, id string, opts domain.LogOptions) (domain.LogPage, error) {
		return domain.LogPage{
			Lines:     []domain.LogLine{{Stream: domain.Stdout, Text: "hola"}},
			Truncated: true,
		}, nil
	}
	page, err = svc.Logs(context.Background(), "abc", domain.LogOptions{Tail: -1})
	if err != nil || len(page.Lines) != 1 || !page.Truncated {
		t.Errorf("Logs() = %+v, %v, want the hook page and nil", page, err)
	}

	svc.OnEvents = func(ctx context.Context) (<-chan domain.DockerEvent, error) {
		return nil, domain.ErrDockerUnavailable
	}
	if _, err := svc.Events(context.Background()); !errors.Is(err, domain.ErrDockerUnavailable) {
		t.Errorf("Events() error = %v, want %v", err, domain.ErrDockerUnavailable)
	}

	if got, want := svc.CallCount("Ping"), 2; got != want {
		t.Errorf("CallCount(Ping) = %d, want %d", got, want)
	}

	logged := make([]domain.LogOptions, 0, 2)
	for _, call := range svc.Calls() {
		if call.Method != "Logs" {
			continue
		}
		if got, want := call.Args[0], "abc"; got != want {
			t.Errorf("Logs id = %#v, want %#v", got, want)
		}
		opts, ok := call.Args[1].(domain.LogOptions)
		if !ok {
			t.Fatalf("Logs opts type = %T, want domain.LogOptions", call.Args[1])
		}
		logged = append(logged, opts)
	}

	if len(logged) != 2 {
		t.Fatalf("recorded %d Logs calls, want 2", len(logged))
	}
	if logged[0].Tail != 0 || logged[1].Tail != -1 {
		t.Errorf("Logs tails = %d and %d, want 0 and -1: the fake must not normalise the options", logged[0].Tail, logged[1].Tail)
	}
}

func waitClosed(t *testing.T, events <-chan domain.DockerEvent) {
	t.Helper()

	select {
	case event, ok := <-events:
		if ok {
			t.Fatalf("received %+v after cancellation, want a closed channel", event)
		}
	case <-time.After(time.Second):
		t.Fatal("Events channel was not closed after cancellation")
	}
}
