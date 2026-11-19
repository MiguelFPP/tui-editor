//go:build integration

package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// Opt-in integration tests: they need DOCKER_INTEGRATION=1 and a reachable socket.
const (
	integrationEnv    = "DOCKER_INTEGRATION"
	integrationPrefix = "tui-editor-test-"
	integrationWait   = 10 * time.Second
)

// Candidate images already present in the host cache; the test skips when none is usable.
var integrationImages = []string{"alpine:3.24.4", "alpine:latest", "busybox:latest", "golang:1.27.4-alpine"}

func TestIntegrationDockerList(t *testing.T) {
	service := integrationService(t)
	api := integrationClient(t)
	image := integrationImage(t, api)

	name := integrationName(t, "list")
	id := createFixture(t, api, name, image, []string{"sh", "-c", "sleep 300"})

	summaries := integrationList(t, service)
	found, ok := findSummary(summaries, id)
	if !ok {
		t.Fatalf("Containers() no incluye %q", id)
	}
	if found.Name != name {
		t.Errorf("Name = %q, want %q", found.Name, name)
	}
	if found.Image != image {
		t.Errorf("Image = %q, want %q", found.Image, image)
	}
	if found.State != domain.ContainerCreated {
		t.Errorf("State = %v, want %v", found.State, domain.ContainerCreated)
	}

	if err := service.Start(context.Background(), id); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	found, ok = integrationWaitForSummary(t, service, id)
	if !ok {
		t.Fatalf("Containers() no incluye %q tras arrancarlo", id)
	}
	if found.State != domain.ContainerRunning {
		t.Errorf("State = %v, want %v", found.State, domain.ContainerRunning)
	}
	if found.StartedAt.IsZero() {
		t.Error("StartedAt = cero, want la fecha de arranque")
	}
	if found.CreatedAt.IsZero() {
		t.Error("CreatedAt = cero, want la fecha de creación")
	}
}

func TestIntegrationDockerStats(t *testing.T) {
	service := integrationService(t)
	api := integrationClient(t)
	image := integrationImage(t, api)

	name := integrationName(t, "stats")
	id := createFixture(t, api, name, image, []string{"sh", "-c", "sleep 300"})

	if err := service.Start(context.Background(), id); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	stats, err := service.Stats(context.Background(), id)
	if err != nil {
		t.Fatalf("Stats() error = %v, want nil", err)
	}
	if stats.ContainerID == "" {
		t.Error("ContainerID = \"\", want el id del contenedor")
	}
	if stats.CollectedAt.IsZero() {
		t.Error("CollectedAt = cero, want la fecha de la muestra")
	}
	if stats.CPUPercent < 0 {
		t.Errorf("CPUPercent = %v, want no negativo", stats.CPUPercent)
	}
	if percent := stats.MemoryPercent(); percent < 0 || percent > 100 {
		t.Errorf("MemoryPercent() = %v, want entre 0 y 100", percent)
	}
}

func TestIntegrationDockerLogs(t *testing.T) {
	service := integrationService(t)
	api := integrationClient(t)
	image := integrationImage(t, api)

	name := integrationName(t, "logs")
	id := createFixture(t, api, name, image, []string{"sh", "-c", "echo hola; echo adios 1>&2; sleep 300"})

	if err := service.Start(context.Background(), id); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	var page domain.LogPage
	integrationWaitUntil(t, func() bool {
		var err error
		page, err = service.Logs(context.Background(), id, domain.LogOptions{Tail: 100})
		if err != nil {
			t.Fatalf("Logs() error = %v, want nil", err)
		}

		return len(page.Lines) >= 2
	})

	streams := make(map[string]domain.LogStream)
	for _, line := range page.Lines {
		if line.Timestamp.IsZero() {
			t.Errorf("Timestamp de %q = cero, want la fecha del daemon", line.Text)
		}
		if strings.Contains(line.Text, "\n") {
			t.Errorf("Text = %q, want sin salto de línea", line.Text)
		}
		streams[line.Text] = line.Stream
	}

	if streams["hola"] != domain.Stdout {
		t.Errorf("stream de %q = %v, want %v", "hola", streams["hola"], domain.Stdout)
	}
	if streams["adios"] != domain.Stderr {
		t.Errorf("stream de %q = %v, want %v", "adios", streams["adios"], domain.Stderr)
	}
}

func TestIntegrationDockerActions(t *testing.T) {
	service := integrationService(t)
	api := integrationClient(t)
	image := integrationImage(t, api)

	name := integrationName(t, "actions")
	id := createFixture(t, api, name, image, []string{"sh", "-c", "sleep 300"})

	ctx := context.Background()

	if err := service.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if err := service.Restart(ctx, id); err != nil {
		t.Fatalf("Restart() error = %v, want nil", err)
	}
	if err := service.Stop(ctx, id); err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}

	if _, ok := integrationWaitForSummary(t, service, id); !ok {
		t.Fatalf("Containers() no incluye %q tras las acciones", id)
	}
}

func TestIntegrationDockerEvents(t *testing.T) {
	service := integrationService(t)
	api := integrationClient(t)
	image := integrationImage(t, api)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := service.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error = %v, want nil", err)
	}

	name := integrationName(t, "events")
	id := createFixture(t, api, name, image, []string{"sh", "-c", "sleep 300"})

	if err := service.Start(context.Background(), id); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	deadline := time.After(integrationWait)
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				t.Fatal("Events() cerró el canal antes de recibir el arranque")
			}
			if event.ContainerID == id && event.Kind == domain.DockerEventStarted {
				if event.At.IsZero() {
					t.Error("At = cero, want la fecha del evento")
				}

				return
			}
		case <-deadline:
			t.Fatalf("no llegó el evento de arranque de %q", id)
		}
	}
}

// integrationService builds the adapter under test and skips without the opt-in setup.
func integrationService(t *testing.T) *Service {
	t.Helper()

	if os.Getenv(integrationEnv) != "1" {
		t.Skipf("%s=1 no está activado", integrationEnv)
	}

	service, err := New("")
	if err != nil {
		t.Skipf("cliente de docker no disponible: %v", err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), integrationWait)
	defer cancel()

	if err := service.Ping(ctx); err != nil {
		if errors.Is(err, domain.ErrDockerUnavailable) || errors.Is(err, domain.ErrPermissionDenied) {
			t.Skipf("daemon de docker no disponible: %v", err)
		}

		t.Fatalf("Ping() error = %v, want nil", err)
	}

	return service
}

// integrationClient builds the raw client used only to create and clean the fixtures.
func integrationClient(t *testing.T) *client.Client {
	t.Helper()

	api, err := client.New(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("cliente de docker no disponible: %v", err)
	}
	t.Cleanup(func() {
		if err := api.Close(); err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}
	})

	return api
}

// integrationImage returns a cached image able to run a shell, or skips the test.
func integrationImage(t *testing.T, api *client.Client) string {
	t.Helper()

	cached := make(map[string]bool)
	for _, item := range listImages(t, api).Items {
		for _, tag := range item.RepoTags {
			cached[tag] = true
		}
	}

	for _, candidate := range integrationImages {
		if cached[candidate] {
			return candidate
		}
	}

	var seen []string
	for tag := range cached {
		seen = append(seen, tag)
	}

	t.Skipf("ninguna imagen local utilizable entre %v (caché: %v)", integrationImages, seen)

	return ""
}

// integrationName returns a fresh container name in the reserved test namespace.
func integrationName(t *testing.T, kind string) string {
	t.Helper()

	return fmt.Sprintf("%s%s-%d", integrationPrefix, kind, time.Now().UnixNano())
}

// createFixture creates a container and removes it in the cleanup.
func createFixture(t *testing.T, api *client.Client, name, image string, cmd []string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), integrationWait)
	defer cancel()

	// A leftover of an aborted run would collide with the fresh name.
	_, _ = api.ContainerRemove(ctx, name, client.ContainerRemoveOptions{Force: true})

	created, err := api.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:   name,
		Config: &container.Config{Image: image, Cmd: cmd},
	})
	if err != nil {
		t.Fatalf("ContainerCreate(%q) error = %v, want nil", name, err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), integrationWait)
		defer cleanupCancel()

		if _, err := api.ContainerRemove(cleanupCtx, created.ID, client.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		}); err != nil {
			t.Errorf("ContainerRemove(%q) error = %v, want nil", name, err)
		}
	})

	if !strings.HasPrefix(name, integrationPrefix) {
		t.Fatalf("nombre %q fuera del prefijo %q", name, integrationPrefix)
	}

	return created.ID
}

func integrationList(t *testing.T, service *Service) []domain.ContainerSummary {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), integrationWait)
	defer cancel()

	summaries, err := service.Containers(ctx, domain.ContainerScope{})
	if err != nil {
		t.Fatalf("Containers() error = %v, want nil", err)
	}

	return summaries
}

// integrationWaitForSummary polls the list until the container shows up.
func integrationWaitForSummary(t *testing.T, service *Service, id string) (domain.ContainerSummary, bool) {
	t.Helper()

	var (
		found domain.ContainerSummary
		ok    bool
	)

	integrationWaitUntil(t, func() bool {
		found, ok = findSummary(integrationList(t, service), id)

		return ok
	})

	return found, ok
}

// integrationWaitUntil retries a condition until the integration deadline.
func integrationWaitUntil(t *testing.T, condition func() bool) {
	t.Helper()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.After(integrationWait)
	for {
		if condition() {
			return
		}

		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatal("la condición no se cumplió dentro del margen de integración")
		}
	}
}

func findSummary(summaries []domain.ContainerSummary, id string) (domain.ContainerSummary, bool) {
	for _, summary := range summaries {
		if summary.ID == id {
			return summary, true
		}
	}

	return domain.ContainerSummary{}, false
}

func listImages(t *testing.T, api *client.Client) client.ImageListResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), integrationWait)
	defer cancel()

	images, err := api.ImageList(ctx, client.ImageListOptions{})
	if err != nil {
		t.Skipf("ImageList() error = %v", err)
	}

	return images
}
