package docker

import (
	"context"
	"strings"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

// Debounce bounds the event updates delivered to the TUI to one per second.
const (
	debounceWindow = time.Second
	eventsBuffer   = 1
)

// Events streams the debounced container events; the adapter closes the channel when ctx is done.
func (s *Service) Events(ctx context.Context) (<-chan domain.DockerEvent, error) {
	// The port has no error channel for the stream, so the daemon is probed before opening it.
	if err := s.Ping(ctx); err != nil {
		return nil, err
	}

	out := make(chan domain.DockerEvent, eventsBuffer)
	go s.pumpEvents(ctx, s.api.Events(ctx, client.EventsListOptions{}), out)

	return out, nil
}

// pumpEvents maps the stream and delivers at most one event per window, the last one winning.
func (s *Service) pumpEvents(ctx context.Context, res client.EventsResult, out chan<- domain.DockerEvent) {
	defer close(out)

	var (
		pending *domain.DockerEvent
		timer   timer
		fired   <-chan time.Time
	)

	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-res.Err:
			return
		case msg, ok := <-res.Messages:
			if !ok {
				return
			}

			event, ok := dockerEvent(msg)
			if !ok {
				continue
			}

			if timer == nil {
				if !deliver(ctx, out, event) {
					return
				}
				timer = s.clock.NewTimer(debounceWindow)
				fired = timer.C()

				continue
			}

			pending = &event
		case <-fired:
			timer, fired = nil, nil
			if pending == nil {
				continue
			}

			event := *pending
			pending = nil

			if !deliver(ctx, out, event) {
				return
			}
			timer = s.clock.NewTimer(debounceWindow)
			fired = timer.C()
		}
	}
}

// deliver hands one event to the caller or reports that the stream must stop.
func deliver(ctx context.Context, out chan<- domain.DockerEvent, event domain.DockerEvent) bool {
	select {
	case out <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// dockerEvent converts one daemon message; other types and unknown actions are dropped.
func dockerEvent(msg events.Message) (domain.DockerEvent, bool) {
	if msg.Type != events.ContainerEventType {
		return domain.DockerEvent{}, false
	}

	kind, ok := eventKind(msg.Action)
	if !ok {
		return domain.DockerEvent{}, false
	}

	return domain.DockerEvent{
		At:          eventTime(msg).UTC(),
		Kind:        kind,
		ContainerID: msg.Actor.ID,
	}, true
}

// eventKind maps the daemon action onto the domain kind, reporting whether it is tracked.
func eventKind(action events.Action) (domain.DockerEventKind, bool) {
	switch action {
	case events.ActionCreate:
		return domain.DockerEventCreated, true
	case events.ActionStart:
		return domain.DockerEventStarted, true
	case events.ActionRestart:
		return domain.DockerEventRestarted, true
	case events.ActionStop:
		return domain.DockerEventStopped, true
	case events.ActionDie:
		return domain.DockerEventDied, true
	case events.ActionDestroy, events.ActionRemove:
		return domain.DockerEventRemoved, true
	case events.ActionPause:
		return domain.DockerEventPaused, true
	case events.ActionUnPause:
		return domain.DockerEventUnpaused, true
	}

	if strings.HasPrefix(string(action), string(events.ActionHealthStatus)) {
		return domain.DockerEventHealthChanged, true
	}

	return domain.DockerEventUnknown, false
}

// eventTime prefers the nanosecond stamp and falls back to the second one.
func eventTime(msg events.Message) time.Time {
	if msg.TimeNano != 0 {
		return time.Unix(0, msg.TimeNano)
	}

	return time.Unix(msg.Time, 0)
}
