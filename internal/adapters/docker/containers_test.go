package docker

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestContainersMaps(t *testing.T) {
	const started = "2026-01-02T03:04:05.123456789Z"

	api := &fakeAPI{}
	api.list = func(_ context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
		if !opts.All {
			t.Error("ContainerList() sin All")
		}

		return client.ContainerListResult{Items: []container.Summary{
			{
				ID:      "cccc",
				Names:   []string{"/zeta"},
				Image:   "zeta:1",
				State:   container.StateExited,
				Created: 1700000200,
				Health:  &container.HealthSummary{Status: container.Unhealthy},
				Ports:   []container.PortSummary{{PrivatePort: 5432, Type: "tcp"}},
			},
			{
				ID:      "aaaa",
				Names:   []string{"/alfa", "/alias"},
				Image:   "alfa:1",
				State:   container.StateRunning,
				Created: 1700000000,
				Health:  &container.HealthSummary{Status: container.Healthy},
				Ports: []container.PortSummary{
					{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 8080, PublicPort: 18080, Type: "tcp"},
					{IP: netip.MustParseAddr("127.0.0.1"), PrivatePort: 53, PublicPort: 53, Type: "udp"},
				},
			},
			{
				ID:      "bbbb",
				Names:   []string{"/kappa"},
				Image:   "kappa:1",
				State:   container.StateRemoving,
				Created: 1700000100,
			},
		}}, nil
	}
	api.inspect = func(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		if id != "aaaa" {
			t.Errorf("ContainerInspect(%q), want %q", id, "aaaa")
		}

		return client.ContainerInspectResult{
			Container: container.InspectResponse{State: &container.State{StartedAt: started}},
		}, nil
	}

	service := newService(api, newFakeClock())

	got, err := service.Containers(context.Background(), domain.ContainerScope{})
	if err != nil {
		t.Fatalf("Containers() error = %v, want nil", err)
	}
	if len(got) != 3 {
		t.Fatalf("Containers() = %d filas, want 3", len(got))
	}

	want := []domain.ContainerSummary{
		{
			ID:        "aaaa",
			Name:      "alfa",
			Image:     "alfa:1",
			State:     domain.ContainerRunning,
			Health:    domain.HealthHealthy,
			CreatedAt: time.Unix(1700000000, 0).UTC(),
			StartedAt: mustParseTime(t, started),
			Ports: []domain.PortMapping{
				{HostIP: "0.0.0.0", HostPort: 18080, ContainerPort: 8080, Protocol: domain.ProtocolTCP},
				{HostIP: "127.0.0.1", HostPort: 53, ContainerPort: 53, Protocol: domain.ProtocolUDP},
			},
		},
		{
			ID:        "bbbb",
			Name:      "kappa",
			Image:     "kappa:1",
			State:     domain.ContainerUnknown,
			Health:    domain.HealthNone,
			CreatedAt: time.Unix(1700000100, 0).UTC(),
		},
		{
			ID:        "cccc",
			Name:      "zeta",
			Image:     "zeta:1",
			State:     domain.ContainerExited,
			Health:    domain.HealthUnhealthy,
			CreatedAt: time.Unix(1700000200, 0).UTC(),
			Ports:     []domain.PortMapping{{ContainerPort: 5432, Protocol: domain.ProtocolTCP}},
		},
	}

	for i, expected := range want {
		if !sameSummary(got[i], expected) {
			t.Errorf("fila %d = %+v, want %+v", i, got[i], expected)
		}
	}
}

func TestContainersLimitsStartProbes(t *testing.T) {
	const total = maxStartProbes + 4

	items := make([]container.Summary, 0, total)
	for i := range total {
		items = append(items, container.Summary{
			ID:      fmt.Sprintf("id-%02d", i),
			Names:   []string{fmt.Sprintf("/c-%02d", i)},
			State:   container.StateRunning,
			Created: 1,
		})
	}

	probes := 0
	api := &fakeAPI{}
	api.list = func(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
		return client.ContainerListResult{Items: items}, nil
	}
	api.inspect = func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		probes++

		return client.ContainerInspectResult{
			Container: container.InspectResponse{State: &container.State{StartedAt: "2026-01-02T03:04:05Z"}},
		}, nil
	}

	service := newService(api, newFakeClock())

	got, err := service.Containers(context.Background(), domain.ContainerScope{})
	if err != nil {
		t.Fatalf("Containers() error = %v, want nil", err)
	}
	if len(got) != total {
		t.Errorf("Containers() = %d filas, want %d", len(got), total)
	}
	if probes != maxStartProbes {
		t.Errorf("ContainerInspect() = %d veces, want %d", probes, maxStartProbes)
	}
}

func TestContainersIgnoresInspectFailures(t *testing.T) {
	api := &fakeAPI{}
	api.list = func(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
		return client.ContainerListResult{Items: []container.Summary{{
			ID:    "aaaa",
			Names: []string{"/alfa"},
			State: container.StateRunning,
		}}}, nil
	}
	api.inspect = func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		return client.ContainerInspectResult{}, errors.New("inspect roto")
	}

	service := newService(api, newFakeClock())

	got, err := service.Containers(context.Background(), domain.ContainerScope{})
	if err != nil {
		t.Fatalf("Containers() error = %v, want nil", err)
	}
	if !got[0].StartedAt.IsZero() {
		t.Errorf("StartedAt = %v, want el valor cero", got[0].StartedAt)
	}
}

func TestContainersEmpty(t *testing.T) {
	api := &fakeAPI{}
	api.list = func(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
		return client.ContainerListResult{}, nil
	}

	service := newService(api, newFakeClock())

	got, err := service.Containers(context.Background(), domain.ContainerScope{})
	if err != nil {
		t.Fatalf("Containers() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("Containers() = %d filas, want 0", len(got))
	}
}

func TestContainersTranslateErrors(t *testing.T) {
	api := &fakeAPI{}
	api.list = func(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
		return client.ContainerListResult{}, errors.New("daemon roto")
	}

	service := newService(api, newFakeClock())

	if _, err := service.Containers(context.Background(), domain.ContainerScope{}); err == nil {
		t.Error("Containers() error = nil, want un error")
	}
}

func sameSummary(got, want domain.ContainerSummary) bool {
	if got.ID != want.ID || got.Name != want.Name || got.Image != want.Image ||
		got.State != want.State || got.Health != want.Health {
		return false
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.StartedAt.Equal(want.StartedAt) {
		return false
	}
	if len(got.Ports) != len(want.Ports) {
		return false
	}
	for i := range got.Ports {
		if got.Ports[i] != want.Ports[i] {
			return false
		}
	}

	return true
}

func TestContainerSummaryCarriesComposeLabels(t *testing.T) {
	got := containerSummary(container.Summary{
		ID: "aaaa",
		Labels: map[string]string{
			domain.LabelComposeProject:    "proj",
			domain.LabelComposeWorkingDir: "/repo/proj",
		},
	})

	if got.Project != "proj" {
		t.Errorf("Project = %q, want %q", got.Project, "proj")
	}
	if got.WorkingDir != "/repo/proj" {
		t.Errorf("WorkingDir = %q, want %q", got.WorkingDir, "/repo/proj")
	}
}

func TestContainersFiltersByScope(t *testing.T) {
	api := &fakeAPI{}
	api.list = func(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
		return client.ContainerListResult{Items: []container.Summary{
			{
				ID: "aaaa", Names: []string{"/web"}, Image: "nginx", State: container.StateExited,
				Labels: map[string]string{domain.LabelComposeWorkingDir: "/repo/proj"},
			},
			{
				ID: "bbbb", Names: []string{"/other"}, Image: "redis", State: container.StateExited,
				Labels: map[string]string{domain.LabelComposeWorkingDir: "/repo/other"},
			},
			{ID: "cccc", Names: []string{"/bare"}, Image: "busybox", State: container.StateExited},
		}}, nil
	}

	service := newService(api, newFakeClock())

	got, err := service.Containers(context.Background(), domain.ContainerScope{WorkingDir: "/repo/proj"})
	if err != nil {
		t.Fatalf("Containers() error = %v, want nil", err)
	}
	if len(got) != 1 || got[0].ID != "aaaa" {
		t.Fatalf("Containers() = %+v, want only aaaa", got)
	}

	all, err := service.Containers(context.Background(), domain.ContainerScope{})
	if err != nil {
		t.Fatalf("Containers() error = %v, want nil", err)
	}
	if len(all) != 3 {
		t.Errorf("Containers() with the zero scope = %d rows, want 3", len(all))
	}
}

func TestContainersSkipsProbesOutsideScope(t *testing.T) {
	items := make([]container.Summary, 0, 20)
	for i := 0; i < 18; i++ {
		items = append(items, container.Summary{
			ID:     fmt.Sprintf("out%02d", i),
			Names:  []string{fmt.Sprintf("/out-%02d", i)},
			State:  container.StateRunning,
			Labels: map[string]string{domain.LabelComposeWorkingDir: "/repo/other"},
		})
	}
	for i := 0; i < 2; i++ {
		items = append(items, container.Summary{
			ID:     fmt.Sprintf("in%02d", i),
			Names:  []string{fmt.Sprintf("/in-%02d", i)},
			State:  container.StateRunning,
			Labels: map[string]string{domain.LabelComposeWorkingDir: "/repo/proj"},
		})
	}

	inspected := 0
	api := &fakeAPI{}
	api.list = func(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
		return client.ContainerListResult{Items: items}, nil
	}
	api.inspect = func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		inspected++

		return client.ContainerInspectResult{}, nil
	}

	service := newService(api, newFakeClock())

	got, err := service.Containers(context.Background(), domain.ContainerScope{WorkingDir: "/repo/proj"})
	if err != nil {
		t.Fatalf("Containers() error = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("Containers() = %d rows, want 2", len(got))
	}
	if inspected != 2 {
		t.Errorf("inspected %d containers, want only the 2 in scope", inspected)
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", value, err)
	}

	return parsed.UTC()
}
