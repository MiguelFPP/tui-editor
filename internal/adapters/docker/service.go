package docker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
	"github.com/moby/moby/client"
)

// Adapter budgets; a caller deadline shorter than these always wins.
const (
	listTimeout   = 3 * time.Second
	statsTimeout  = 2 * time.Second
	actionTimeout = 5 * time.Second
	logTimeout    = 5 * time.Second
	pingTimeout   = 3 * time.Second
)

// stopGraceSeconds is the daemon-side grace before SIGKILL; the adapter never forces.
const stopGraceSeconds = 3

// mobyAPI is the Moby surface the adapter uses; tests inject a fake.
type mobyAPI interface {
	Close() error
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerStats(ctx context.Context, containerID string, options client.ContainerStatsOptions) (client.ContainerStatsResult, error)
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	ContainerStart(ctx context.Context, containerID string, options client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerStop(ctx context.Context, containerID string, options client.ContainerStopOptions) (client.ContainerStopResult, error)
	ContainerRestart(ctx context.Context, containerID string, options client.ContainerRestartOptions) (client.ContainerRestartResult, error)
	Events(ctx context.Context, options client.EventsListOptions) client.EventsResult
}

// Service implements the Docker port on top of the Moby client.
type Service struct {
	api   mobyAPI
	clock clock
}

var (
	_ mobyAPI             = (*client.Client)(nil)
	_ ports.DockerService = (*Service)(nil)
)

// New builds the client without contacting the daemon, so an absent daemon is not a construction error.
func New(host string) (*Service, error) {
	opts := []client.Opt{client.WithAPIVersionNegotiation()}
	if trimmed := strings.TrimSpace(host); trimmed == "" {
		opts = append(opts, client.FromEnv)
	} else {
		opts = append(opts, client.WithHost(trimmed))
	}

	api, err := client.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: host de docker %q inválido", domain.ErrInvalidArgument, host)
	}

	return newService(api, realClock{}), nil
}

// newService wires an API implementation and a clock; tests inject both.
func newService(api mobyAPI, clk clock) *Service {
	return &Service{api: api, clock: clk}
}

// Close releases the idle connections held by the client.
func (s *Service) Close() error {
	if s.api == nil {
		return nil
	}

	return s.api.Close()
}

// Ping reports whether the daemon answers.
func (s *Service) Ping(ctx context.Context) error {
	ctx, cancel := s.withTimeout(ctx, pingTimeout)
	defer cancel()

	if _, err := s.api.Ping(ctx, client.PingOptions{}); err != nil {
		return translateError("Ping", err)
	}

	return nil
}

// Start starts one container without extra options.
func (s *Service) Start(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}

	ctx, cancel := s.withTimeout(ctx, actionTimeout)
	defer cancel()

	if _, err := s.api.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
		return translateError("Start", err)
	}

	return nil
}

// Stop stops one container with a graceful timeout and no force.
func (s *Service) Stop(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}

	ctx, cancel := s.withTimeout(ctx, actionTimeout)
	defer cancel()

	grace := stopGraceSeconds
	if _, err := s.api.ContainerStop(ctx, id, client.ContainerStopOptions{Timeout: &grace}); err != nil {
		return translateError("Stop", err)
	}

	return nil
}

// Restart restarts one container with a graceful timeout and no force.
func (s *Service) Restart(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}

	ctx, cancel := s.withTimeout(ctx, actionTimeout)
	defer cancel()

	grace := stopGraceSeconds
	if _, err := s.api.ContainerRestart(ctx, id, client.ContainerRestartOptions{Timeout: &grace}); err != nil {
		return translateError("Restart", err)
	}

	return nil
}

// withTimeout applies the adapter budget unless the caller brought a shorter deadline.
func (s *Service) withTimeout(ctx context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= budget {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, budget)
}

// validateID rejects an empty container id before any request.
func validateID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: falta el id del contenedor", domain.ErrInvalidArgument)
	}

	return nil
}

// translateError maps the failures of the client and the daemon onto the domain sentinels.
//
// The daemon answers 404 for an unknown container with a "not found" class that would
// require the containerd/errdefs module to detect; that class stays as a wrapped error.
func translateError(op string, err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("docker: %s: %w", op, err)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%w: %s", domain.ErrPermissionDenied, op)
	case client.IsErrConnectionFailed(err):
		return fmt.Errorf("%w: %s", domain.ErrDockerUnavailable, op)
	}

	return fmt.Errorf("docker: %s: %w", op, err)
}
