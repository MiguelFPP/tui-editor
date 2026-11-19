package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/moby/moby/client"
)

func TestNewAcceptsHosts(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")

	for _, host := range []string{"", "  ", "unix:///var/run/docker.sock", "tcp://127.0.0.1:2375"} {
		service, err := New(host)
		if err != nil {
			t.Errorf("New(%q) error = %v, want nil", host, err)
			continue
		}
		if err := service.Close(); err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}
	}
}

func TestNewRejectsMalformedHosts(t *testing.T) {
	for _, host := range []string{"nope", "unix", "://", "tcp://"} {
		service, err := New(host)
		if !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("New(%q) error = %v, want %v", host, err, domain.ErrInvalidArgument)
		}
		if service != nil {
			t.Errorf("New(%q) service = %v, want nil", host, service)
		}
	}
}

func TestPing(t *testing.T) {
	calls := 0
	api := &fakeAPI{}
	api.ping = func(ctx context.Context, _ client.PingOptions) (client.PingResult, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Error("Ping() sin deadline")
		}

		return client.PingResult{APIVersion: "1.45"}, nil
	}

	service := newService(api, newFakeClock())
	if err := service.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("llamadas al daemon = %d, want 1", calls)
	}
}

func TestActions(t *testing.T) {
	const id = "abc123"

	var (
		started   string
		stopped   string
		restarted string
		grace     int
	)

	api := &fakeAPI{}
	api.start = func(_ context.Context, containerID string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
		started = containerID

		return client.ContainerStartResult{}, nil
	}
	api.stop = func(_ context.Context, containerID string, opts client.ContainerStopOptions) (client.ContainerStopResult, error) {
		stopped = containerID
		grace = timeoutOf(t, "Stop", opts.Timeout)

		return client.ContainerStopResult{}, nil
	}
	api.restart = func(_ context.Context, containerID string, opts client.ContainerRestartOptions) (client.ContainerRestartResult, error) {
		restarted = containerID
		grace = timeoutOf(t, "Restart", opts.Timeout)

		return client.ContainerRestartResult{}, nil
	}

	service := newService(api, newFakeClock())
	ctx := context.Background()

	if err := service.Start(ctx, id); err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}
	if err := service.Stop(ctx, id); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
	if err := service.Restart(ctx, id); err != nil {
		t.Errorf("Restart() error = %v, want nil", err)
	}

	if started != id || stopped != id || restarted != id {
		t.Errorf("ids = %q, %q, %q, want %q", started, stopped, restarted, id)
	}
	if grace <= 0 {
		t.Errorf("gracia = %d, want un timeout positivo", grace)
	}
	if time.Duration(grace)*time.Second >= actionTimeout {
		t.Errorf("gracia = %ds, want menos que el presupuesto de la acción", grace)
	}
}

func TestActionsRejectEmptyID(t *testing.T) {
	calls := 0
	api := &fakeAPI{}
	api.start = func(_ context.Context, _ string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
		calls++

		return client.ContainerStartResult{}, nil
	}
	api.stop = func(_ context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
		calls++

		return client.ContainerStopResult{}, nil
	}
	api.restart = func(_ context.Context, _ string, _ client.ContainerRestartOptions) (client.ContainerRestartResult, error) {
		calls++

		return client.ContainerRestartResult{}, nil
	}

	service := newService(api, newFakeClock())
	ctx := context.Background()

	for _, call := range []func() error{
		func() error { return service.Start(ctx, "  ") },
		func() error { return service.Stop(ctx, "") },
		func() error { return service.Restart(ctx, "") },
	} {
		if err := call(); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("llamada con id vacío = %v, want %v", err, domain.ErrInvalidArgument)
		}
	}

	if calls != 0 {
		t.Errorf("llamadas al daemon = %d, want 0", calls)
	}
}

func TestActionsTranslateErrors(t *testing.T) {
	permission := &fs.PathError{Op: "connect", Path: "/var/run/docker.sock", Err: fs.ErrPermission}
	api := &fakeAPI{}
	api.start = func(_ context.Context, _ string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
		return client.ContainerStartResult{}, fmt.Errorf("error during connect: %w", permission)
	}
	api.stop = func(_ context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
		return client.ContainerStopResult{}, errors.New("daemon roto")
	}

	service := newService(api, newFakeClock())
	ctx := context.Background()

	if err := service.Start(ctx, "abc"); !errors.Is(err, domain.ErrPermissionDenied) {
		t.Errorf("Start() error = %v, want %v", err, domain.ErrPermissionDenied)
	}

	err := service.Stop(ctx, "abc")
	if err == nil || errors.Is(err, domain.ErrPermissionDenied) || errors.Is(err, domain.ErrDockerUnavailable) {
		t.Errorf("Stop() error = %v, want un error técnico envuelto", err)
	}
}

func TestPermissionDenied(t *testing.T) {
	api := &fakeAPI{}
	api.ping = func(_ context.Context, _ client.PingOptions) (client.PingResult, error) {
		return client.PingResult{}, &fs.PathError{Op: "dial", Path: "/var/run/docker.sock", Err: fs.ErrPermission}
	}

	service := newService(api, newFakeClock())
	if err := service.Ping(context.Background()); !errors.Is(err, domain.ErrPermissionDenied) {
		t.Errorf("Ping() error = %v, want %v", err, domain.ErrPermissionDenied)
	}
}

func TestUnavailableOperations(t *testing.T) {
	service, err := New("unix:///nonexistent/tui-editor-test.sock")
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}
	})

	ctx := context.Background()
	calls := map[string]func() error{
		"Ping":       func() error { return service.Ping(ctx) },
		"Containers": func() error { _, err := service.Containers(ctx, domain.ContainerScope{}); return err },
		"Stats":      func() error { _, err := service.Stats(ctx, "abc"); return err },
		"Logs":       func() error { _, err := service.Logs(ctx, "abc", domain.LogOptions{}); return err },
		"Start":      func() error { return service.Start(ctx, "abc") },
		"Stop":       func() error { return service.Stop(ctx, "abc") },
		"Restart":    func() error { return service.Restart(ctx, "abc") },
		"Events":     func() error { _, err := service.Events(ctx); return err },
	}

	for name, call := range calls {
		if err := call(); !errors.Is(err, domain.ErrDockerUnavailable) {
			t.Errorf("%s error = %v, want %v", name, err, domain.ErrDockerUnavailable)
		}
	}
}

func TestTimeoutBudgets(t *testing.T) {
	observed := make(map[string]time.Duration)
	record := func(op string, ctx context.Context) {
		t.Helper()

		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatalf("%s: el contexto no trae deadline", op)
		}
		observed[op] = time.Until(deadline)
	}

	api := &fakeAPI{}
	api.ping = func(ctx context.Context, _ client.PingOptions) (client.PingResult, error) {
		record("Ping", ctx)

		return client.PingResult{}, nil
	}
	api.list = func(ctx context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
		record("Containers", ctx)

		return client.ContainerListResult{}, nil
	}
	api.stats = func(ctx context.Context, _ string, _ client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
		record("Stats", ctx)

		return client.ContainerStatsResult{Body: io.NopCloser(bytes.NewReader([]byte(`{"id":"abc"}`)))}, nil
	}
	api.inspect = func(ctx context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		record("Logs", ctx)

		return client.ContainerInspectResult{}, nil
	}
	api.logs = func(_ context.Context, _ string, _ client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	api.start = func(ctx context.Context, _ string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
		record("Start", ctx)

		return client.ContainerStartResult{}, nil
	}
	api.stop = func(ctx context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
		record("Stop", ctx)

		return client.ContainerStopResult{}, nil
	}
	api.restart = func(ctx context.Context, _ string, _ client.ContainerRestartOptions) (client.ContainerRestartResult, error) {
		record("Restart", ctx)

		return client.ContainerRestartResult{}, nil
	}

	service := newService(api, newFakeClock())
	ctx := context.Background()

	run := map[string]func(){
		"Ping":       func() { _ = service.Ping(ctx) },
		"Containers": func() { _, _ = service.Containers(ctx, domain.ContainerScope{}) },
		"Stats":      func() { _, _ = service.Stats(ctx, "abc") },
		"Logs":       func() { _, _ = service.Logs(ctx, "abc", domain.LogOptions{}) },
		"Start":      func() { _ = service.Start(ctx, "abc") },
		"Stop":       func() { _ = service.Stop(ctx, "abc") },
		"Restart":    func() { _ = service.Restart(ctx, "abc") },
	}

	budgets := map[string]time.Duration{
		"Ping":       pingTimeout,
		"Containers": listTimeout,
		"Stats":      statsTimeout,
		"Logs":       logTimeout,
		"Start":      actionTimeout,
		"Stop":       actionTimeout,
		"Restart":    actionTimeout,
	}

	for _, call := range run {
		call()
	}

	for name, budget := range budgets {
		got, ok := observed[name]
		if !ok {
			t.Errorf("%s: no se observó ningún deadline", name)
			continue
		}
		if got > budget || got <= budget-time.Second {
			t.Errorf("%s: presupuesto observado = %v, want <= %v", name, got, budget)
		}
	}
}

func TestTimeoutCallerDeadlineWins(t *testing.T) {
	const callerBudget = 50 * time.Millisecond

	var seen time.Duration
	api := &fakeAPI{}
	api.list = func(ctx context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("Containers() sin deadline")
		} else {
			seen = time.Until(deadline)
		}

		<-ctx.Done()

		return client.ContainerListResult{}, ctx.Err()
	}

	service := newService(api, newFakeClock())
	ctx, cancel := context.WithTimeout(context.Background(), callerBudget)
	defer cancel()

	_, err := service.Containers(ctx, domain.ContainerScope{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Containers() error = %v, want %v", err, context.DeadlineExceeded)
	}
	if seen > callerBudget {
		t.Errorf("presupuesto observado = %v, want <= %v", seen, callerBudget)
	}
}

func TestTimeoutStatsBlocks(t *testing.T) {
	api := &fakeAPI{}
	api.stats = func(ctx context.Context, _ string, _ client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
		<-ctx.Done()

		return client.ContainerStatsResult{}, ctx.Err()
	}

	service := newService(api, newFakeClock())
	started := time.Now()

	_, err := service.Stats(context.Background(), "abc")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stats() error = %v, want %v", err, context.DeadlineExceeded)
	}
	if elapsed := time.Since(started); elapsed < statsTimeout || elapsed > 2*statsTimeout {
		t.Errorf("Stats() tardó %v, want entre %v y %v", elapsed, statsTimeout, 2*statsTimeout)
	}
}

func timeoutOf(t *testing.T, op string, timeout *int) int {
	t.Helper()

	if timeout == nil {
		t.Fatalf("%s sin timeout de gracia", op)
	}

	return *timeout
}
