package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// eventsBuffer is the size of the fake event channel required by the contract tests.
const eventsBuffer = 16

// DockerService is a test double for ports.DockerService.
type DockerService struct {
	recorder

	OnPing       func(ctx context.Context) error
	OnContainers func(ctx context.Context, scope domain.ContainerScope) ([]domain.ContainerSummary, error)
	OnStats      func(ctx context.Context, id string) (domain.ContainerStats, error)
	OnLogs       func(ctx context.Context, id string, opts domain.LogOptions) (domain.LogPage, error)
	OnStart      func(ctx context.Context, id string) error
	OnStop       func(ctx context.Context, id string) error
	OnRestart    func(ctx context.Context, id string) error
	OnEvents     func(ctx context.Context) (<-chan domain.DockerEvent, error)

	mu     sync.Mutex
	events chan domain.DockerEvent
	closed bool
}

func NewDockerService() *DockerService {
	return &DockerService{}
}

// Emit queues an event without blocking; it drops the event when the buffer is full.
func (d *DockerService) Emit(ev domain.DockerEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return
	}

	select {
	case d.ensureEvents() <- ev:
	default:
	}
}

func (d *DockerService) Ping(ctx context.Context) error {
	d.record("Ping")
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Ping: %w", err)
	}
	if fn := hook(&d.recorder, &d.OnPing); fn != nil {
		return fn(ctx)
	}

	return nil
}

func (d *DockerService) Containers(ctx context.Context, scope domain.ContainerScope) ([]domain.ContainerSummary, error) {
	d.record("Containers", scope)
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("Containers: %w", err)
	}
	if fn := hook(&d.recorder, &d.OnContainers); fn != nil {
		return fn(ctx, scope)
	}

	return nil, nil
}

func (d *DockerService) Stats(ctx context.Context, id string) (domain.ContainerStats, error) {
	d.record("Stats", id)
	if err := ctx.Err(); err != nil {
		return domain.ContainerStats{}, fmt.Errorf("Stats: %w", err)
	}
	if fn := hook(&d.recorder, &d.OnStats); fn != nil {
		return fn(ctx, id)
	}

	return domain.ContainerStats{}, nil
}

func (d *DockerService) Logs(ctx context.Context, id string, opts domain.LogOptions) (domain.LogPage, error) {
	d.record("Logs", id, opts)
	if err := ctx.Err(); err != nil {
		return domain.LogPage{}, fmt.Errorf("Logs: %w", err)
	}
	if fn := hook(&d.recorder, &d.OnLogs); fn != nil {
		return fn(ctx, id, opts)
	}

	return domain.LogPage{}, nil
}

func (d *DockerService) Start(ctx context.Context, id string) error {
	d.record("Start", id)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Start: %w", err)
	}
	if fn := hook(&d.recorder, &d.OnStart); fn != nil {
		return fn(ctx, id)
	}

	return nil
}

func (d *DockerService) Stop(ctx context.Context, id string) error {
	d.record("Stop", id)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Stop: %w", err)
	}
	if fn := hook(&d.recorder, &d.OnStop); fn != nil {
		return fn(ctx, id)
	}

	return nil
}

func (d *DockerService) Restart(ctx context.Context, id string) error {
	d.record("Restart", id)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Restart: %w", err)
	}
	if fn := hook(&d.recorder, &d.OnRestart); fn != nil {
		return fn(ctx, id)
	}

	return nil
}

// Events returns the fake channel and closes it when ctx is done, like the adapter does.
func (d *DockerService) Events(ctx context.Context) (<-chan domain.DockerEvent, error) {
	d.record("Events")
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("Events: %w", err)
	}
	if fn := hook(&d.recorder, &d.OnEvents); fn != nil {
		return fn(ctx)
	}

	d.mu.Lock()
	events := d.ensureEvents()
	d.mu.Unlock()

	go func() {
		<-ctx.Done()

		d.mu.Lock()
		defer d.mu.Unlock()

		if !d.closed {
			d.closed = true
			close(events)
		}
	}()

	return events, nil
}

func (d *DockerService) ensureEvents() chan domain.DockerEvent {
	if d.events == nil {
		d.events = make(chan domain.DockerEvent, eventsBuffer)
	}

	return d.events
}
