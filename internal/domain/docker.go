package domain

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type ContainerState uint8

const (
	ContainerUnknown ContainerState = iota
	ContainerCreated
	ContainerRunning
	ContainerPaused
	ContainerRestarting
	ContainerExited
	ContainerDead
)

// String returns the technical Docker state name, not a UI label.
func (s ContainerState) String() string {
	switch s {
	case ContainerCreated:
		return "created"
	case ContainerRunning:
		return "running"
	case ContainerPaused:
		return "paused"
	case ContainerRestarting:
		return "restarting"
	case ContainerExited:
		return "exited"
	case ContainerDead:
		return "dead"
	}

	return "unknown"
}

type HealthState uint8

const (
	HealthNone HealthState = iota
	HealthStarting
	HealthHealthy
	HealthUnhealthy
)

// String returns the technical health name, not a UI label.
func (h HealthState) String() string {
	switch h {
	case HealthStarting:
		return "starting"
	case HealthHealthy:
		return "healthy"
	case HealthUnhealthy:
		return "unhealthy"
	}

	return "none"
}

type Protocol uint8

const (
	ProtocolTCP Protocol = iota
	ProtocolUDP
)

func (p Protocol) String() string {
	if p == ProtocolUDP {
		return "udp"
	}

	return "tcp"
}

// PortMapping is one port of a container, published to the host or not.
type PortMapping struct {
	HostIP        string
	HostPort      int
	ContainerPort int
	Protocol      Protocol
}

func (p PortMapping) Published() bool {
	return p.HostPort > 0
}

func (p PortMapping) String() string {
	container := fmt.Sprintf("%d/%s", p.ContainerPort, p.Protocol)
	if !p.Published() {
		return container
	}

	if p.HostIP == "" {
		return fmt.Sprintf("%d->%s", p.HostPort, container)
	}

	return fmt.Sprintf("%s:%d->%s", p.HostIP, p.HostPort, container)
}

// Compose labels that tie a container to a project directory.
const (
	LabelComposeProject    = "com.docker.compose.project"
	LabelComposeWorkingDir = "com.docker.compose.project.working_dir"
)

// ContainerSummary is one row of the container list.
type ContainerSummary struct {
	ID         string
	Name       string
	Image      string
	State      ContainerState
	Health     HealthState
	Ports      []PortMapping
	CreatedAt  time.Time
	StartedAt  time.Time
	Project    string // com.docker.compose.project
	WorkingDir string // com.docker.compose.project.working_dir
}

const shortIDLength = 12

func (c ContainerSummary) ShortID() string {
	if len(c.ID) <= shortIDLength {
		return c.ID
	}

	return c.ID[:shortIDLength]
}

func (c ContainerSummary) Running() bool {
	return c.State == ContainerRunning
}

// ContainerScope narrows the list to one compose project; the zero value lists everything.
type ContainerScope struct {
	WorkingDir string
	Project    string
}

// Filtered reports whether the scope narrows the list; the zero value does not.
func (s ContainerScope) Filtered() bool {
	return s.WorkingDir != "" || s.Project != ""
}

// InScope reports whether the container belongs to scope.
func (c ContainerSummary) InScope(scope ContainerScope) bool {
	if !scope.Filtered() {
		return true
	}
	if c.WorkingDir != "" && scope.WorkingDir != "" {
		return samePath(c.WorkingDir, scope.WorkingDir)
	}
	if c.Project != "" && scope.Project != "" {
		return c.Project == scope.Project
	}

	return false
}

// samePath compares two container paths after cleaning them and normalising the separators.
func samePath(a, b string) bool {
	return cleanPath(a) == cleanPath(b)
}

// cleanPath makes a Windows-style label comparable on any platform.
func cleanPath(path string) string {
	return filepath.Clean(strings.ReplaceAll(path, `\`, "/"))
}

// ContainerStats is one one-shot stats sample, never a live stream.
type ContainerStats struct {
	ContainerID string
	CPUPercent  float64
	MemoryUsed  uint64
	MemoryLimit uint64
	NetworkRx   uint64
	NetworkTx   uint64
	BlockRead   uint64
	BlockWrite  uint64
	PIDs        uint
	CollectedAt time.Time
}

func (s ContainerStats) MemoryPercent() float64 {
	if s.MemoryLimit == 0 {
		return 0
	}

	return float64(s.MemoryUsed) / float64(s.MemoryLimit) * 100
}

type DockerEventKind uint8

const (
	DockerEventUnknown DockerEventKind = iota
	DockerEventCreated
	DockerEventStarted
	DockerEventRestarted
	DockerEventStopped
	DockerEventDied
	DockerEventRemoved
	DockerEventPaused
	DockerEventUnpaused
	DockerEventHealthChanged
)

// DockerEvent is one debounced event of the daemon stream.
type DockerEvent struct {
	At          time.Time
	Kind        DockerEventKind
	ContainerID string
}
