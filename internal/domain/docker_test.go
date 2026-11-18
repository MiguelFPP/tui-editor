package domain

import (
	"math"
	"strings"
	"testing"
)

func TestContainerStateString(t *testing.T) {
	cases := []struct {
		state ContainerState
		want  string
	}{
		{state: ContainerUnknown, want: "unknown"},
		{state: ContainerCreated, want: "created"},
		{state: ContainerRunning, want: "running"},
		{state: ContainerPaused, want: "paused"},
		{state: ContainerRestarting, want: "restarting"},
		{state: ContainerExited, want: "exited"},
		{state: ContainerDead, want: "dead"},
		{state: ContainerState(200), want: "unknown"},
	}

	for _, tc := range cases {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("ContainerState(%d).String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestHealthStateAndProtocolString(t *testing.T) {
	healths := map[HealthState]string{
		HealthNone:      "none",
		HealthStarting:  "starting",
		HealthHealthy:   "healthy",
		HealthUnhealthy: "unhealthy",
	}
	for state, want := range healths {
		if got := state.String(); got != want {
			t.Errorf("HealthState(%d).String() = %q, want %q", state, got, want)
		}
	}

	if got, want := ProtocolTCP.String(), "tcp"; got != want {
		t.Errorf("ProtocolTCP.String() = %q, want %q", got, want)
	}
	if got, want := ProtocolUDP.String(), "udp"; got != want {
		t.Errorf("ProtocolUDP.String() = %q, want %q", got, want)
	}
	if got, want := Protocol(9).String(), "tcp"; got != want {
		t.Errorf("Protocol(9).String() = %q, want %q", got, want)
	}
}

func TestPortMappingPublishedAndString(t *testing.T) {
	cases := []struct {
		name      string
		port      PortMapping
		published bool
		want      string
	}{
		{
			name:      "host ip",
			port:      PortMapping{HostIP: "127.0.0.1", HostPort: 8080, ContainerPort: 80, Protocol: ProtocolTCP},
			published: true,
			want:      "127.0.0.1:8080->80/tcp",
		},
		{
			name:      "wildcard",
			port:      PortMapping{HostIP: "0.0.0.0", HostPort: 5432, ContainerPort: 5432, Protocol: ProtocolTCP},
			published: true,
			want:      "0.0.0.0:5432->5432/tcp",
		},
		{
			name:      "no host ip",
			port:      PortMapping{HostPort: 53, ContainerPort: 53, Protocol: ProtocolUDP},
			published: true,
			want:      "53->53/udp",
		},
		{
			name:      "unpublished",
			port:      PortMapping{HostIP: "127.0.0.1", ContainerPort: 80, Protocol: ProtocolTCP},
			published: false,
			want:      "80/tcp",
		},
		{
			name:      "zero",
			port:      PortMapping{},
			published: false,
			want:      "0/tcp",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.port.Published(); got != tc.published {
				t.Errorf("Published() = %v, want %v", got, tc.published)
			}
			if got := tc.port.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestContainerSummaryShortIDAndRunning(t *testing.T) {
	full := strings.Repeat("ab", 32)

	cases := []struct {
		name string
		id   string
		want string
	}{
		{name: "full", id: full, want: full[:12]},
		{name: "exact", id: strings.Repeat("c", 12), want: strings.Repeat("c", 12)},
		{name: "short", id: "abc", want: "abc"},
		{name: "empty", id: "", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (ContainerSummary{ID: tc.id}).ShortID(); got != tc.want {
				t.Errorf("ShortID() = %q, want %q", got, tc.want)
			}
		})
	}

	if !(ContainerSummary{State: ContainerRunning}).Running() {
		t.Error("running container: Running() = false, want true")
	}
	for _, state := range []ContainerState{ContainerUnknown, ContainerCreated, ContainerPaused, ContainerRestarting, ContainerExited, ContainerDead} {
		if (ContainerSummary{State: state}).Running() {
			t.Errorf("ContainerState(%s).Running() = true, want false", state)
		}
	}
}

func TestContainerStatsMemoryPercent(t *testing.T) {
	cases := []struct {
		name  string
		used  uint64
		limit uint64
		want  float64
	}{
		{name: "empty", used: 0, limit: 0, want: 0},
		{name: "no limit", used: 4096, limit: 0, want: 0},
		{name: "half", used: 512, limit: 1024, want: 50},
		{name: "full", used: 1024, limit: 1024, want: 100},
		{name: "over limit", used: 2048, limit: 1024, want: 200},
		{name: "small", used: 1, limit: 3, want: 100.0 / 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stats := ContainerStats{MemoryUsed: tc.used, MemoryLimit: tc.limit}
			if got := stats.MemoryPercent(); math.Abs(got-tc.want) > 0.000001 {
				t.Errorf("MemoryPercent() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDockerEventKindsAreDistinct(t *testing.T) {
	kinds := []DockerEventKind{
		DockerEventUnknown,
		DockerEventCreated,
		DockerEventStarted,
		DockerEventRestarted,
		DockerEventStopped,
		DockerEventDied,
		DockerEventRemoved,
		DockerEventPaused,
		DockerEventUnpaused,
		DockerEventHealthChanged,
	}

	seen := make(map[DockerEventKind]bool, len(kinds))
	for _, kind := range kinds {
		if seen[kind] {
			t.Fatalf("DockerEventKind(%d) is duplicated", kind)
		}
		seen[kind] = true
	}

	if DockerEventUnknown != 0 {
		t.Errorf("DockerEventUnknown = %d, want the zero value", DockerEventUnknown)
	}
}
