package docker

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// maxStartProbes bounds the inspects used to fill StartedAt, which the list API does not return.
const maxStartProbes = 16

// Containers lists every container sorted by name.
func (s *Service) Containers(ctx context.Context) ([]domain.ContainerSummary, error) {
	ctx, cancel := s.withTimeout(ctx, listTimeout)
	defer cancel()

	res, err := s.api.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, translateError("Containers", err)
	}

	summaries := make([]domain.ContainerSummary, 0, len(res.Items))
	probes := 0
	for _, item := range res.Items {
		summary := containerSummary(item)
		if summary.State == domain.ContainerRunning && probes < maxStartProbes {
			probes++
			summary.StartedAt = s.startedAt(ctx, item.ID)
		}
		summaries = append(summaries, summary)
	}

	slices.SortStableFunc(summaries, func(a, b domain.ContainerSummary) int {
		return strings.Compare(a.Name, b.Name)
	})

	return summaries, nil
}

// startedAt reads StartedAt from an inspect; a failure leaves the optional field empty.
func (s *Service) startedAt(ctx context.Context, id string) time.Time {
	res, err := s.api.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil || res.Container.State == nil {
		return time.Time{}
	}

	started, err := time.Parse(time.RFC3339Nano, res.Container.State.StartedAt)
	if err != nil {
		return time.Time{}
	}

	return started.UTC()
}

// containerSummary maps one row of the daemon list onto the domain summary.
func containerSummary(item container.Summary) domain.ContainerSummary {
	return domain.ContainerSummary{
		ID:        item.ID,
		Name:      containerName(item.Names),
		Image:     item.Image,
		State:     containerState(item.State),
		Health:    healthState(item.Health),
		Ports:     portMappings(item.Ports),
		CreatedAt: time.Unix(item.Created, 0).UTC(),
	}
}

// containerName takes the first name without the leading slash the daemon adds.
func containerName(names []string) string {
	if len(names) == 0 {
		return ""
	}

	return strings.TrimPrefix(names[0], "/")
}

// containerState maps the daemon state; "removing" has no domain state.
func containerState(state container.ContainerState) domain.ContainerState {
	switch state {
	case container.StateCreated:
		return domain.ContainerCreated
	case container.StateRunning:
		return domain.ContainerRunning
	case container.StatePaused:
		return domain.ContainerPaused
	case container.StateRestarting:
		return domain.ContainerRestarting
	case container.StateExited:
		return domain.ContainerExited
	case container.StateDead:
		return domain.ContainerDead
	}

	return domain.ContainerUnknown
}

// healthState maps the summary health, absent when the container has no check.
func healthState(health *container.HealthSummary) domain.HealthState {
	if health == nil {
		return domain.HealthNone
	}

	switch health.Status {
	case container.Starting:
		return domain.HealthStarting
	case container.Healthy:
		return domain.HealthHealthy
	case container.Unhealthy:
		return domain.HealthUnhealthy
	}

	return domain.HealthNone
}

// portMappings maps every exposed port, published to the host or not.
func portMappings(ports []container.PortSummary) []domain.PortMapping {
	if len(ports) == 0 {
		return nil
	}

	mappings := make([]domain.PortMapping, 0, len(ports))
	for _, port := range ports {
		mapping := domain.PortMapping{
			HostPort:      int(port.PublicPort),
			ContainerPort: int(port.PrivatePort),
			Protocol:      protocol(port.Type),
		}
		if port.IP.IsValid() {
			mapping.HostIP = port.IP.String()
		}
		mappings = append(mappings, mapping)
	}

	return mappings
}

// protocol maps the daemon port type; sctp has no domain protocol and falls back to TCP.
func protocol(kind string) domain.Protocol {
	if strings.EqualFold(kind, "udp") {
		return domain.ProtocolUDP
	}

	return domain.ProtocolTCP
}
