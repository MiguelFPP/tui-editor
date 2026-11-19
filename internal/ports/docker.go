package ports

import (
	"context"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// DockerService exposes the daemon operations of the Docker panel.
type DockerService interface {
	Ping(ctx context.Context) error
	Containers(ctx context.Context, scope domain.ContainerScope) ([]domain.ContainerSummary, error)
	Stats(ctx context.Context, id string) (domain.ContainerStats, error)
	Logs(ctx context.Context, id string, opts domain.LogOptions) (domain.LogPage, error)
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string) error
	Restart(ctx context.Context, id string) error
	// Events starts a stream; the adapter closes the returned channel when ctx is done.
	Events(ctx context.Context) (<-chan domain.DockerEvent, error)
}
