package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// maxStatsBytes bounds one stats body to avoid unbounded reads.
const maxStatsBytes = 1 << 20

// Stats reads one sample of the resource usage of a container, never a stream.
func (s *Service) Stats(ctx context.Context, id string) (domain.ContainerStats, error) {
	if err := validateID(id); err != nil {
		return domain.ContainerStats{}, err
	}

	ctx, cancel := s.withTimeout(ctx, statsTimeout)
	defer cancel()

	// The previous sample keeps precpu_stats populated, so CPU% is a real delta.
	res, err := s.api.ContainerStats(ctx, id, client.ContainerStatsOptions{IncludePreviousSample: true})
	if err != nil {
		return domain.ContainerStats{}, translateError("Stats", err)
	}
	defer res.Body.Close()

	var raw container.StatsResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, maxStatsBytes)).Decode(&raw); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return domain.ContainerStats{}, translateError("Stats", ctxErr)
		}

		return domain.ContainerStats{}, fmt.Errorf("docker: Stats: %w", err)
	}

	return s.mapStats(id, raw), nil
}

// mapStats converts the daemon sample; a missing timestamp falls back to the adapter clock.
func (s *Service) mapStats(id string, raw container.StatsResponse) domain.ContainerStats {
	collected := raw.Read
	if collected.IsZero() {
		collected = s.clock.Now()
	}

	containerID := raw.ID
	if containerID == "" {
		containerID = id
	}

	rx, tx := networkTotals(raw.Networks)
	read, write := blockTotals(raw.BlkioStats.IoServiceBytesRecursive)

	return domain.ContainerStats{
		ContainerID: containerID,
		CPUPercent:  cpuPercent(raw),
		MemoryUsed:  raw.MemoryStats.Usage,
		MemoryLimit: raw.MemoryStats.Limit,
		NetworkRx:   rx,
		NetworkTx:   tx,
		BlockRead:   read,
		BlockWrite:  write,
		PIDs:        uint(raw.PidsStats.Current),
		CollectedAt: collected.UTC(),
	}
}

// cpuPercent is the delta between the two samples; an empty or flat delta is zero.
func cpuPercent(raw container.StatsResponse) float64 {
	usage := float64(raw.CPUStats.CPUUsage.TotalUsage) - float64(raw.PreCPUStats.CPUUsage.TotalUsage)
	system := float64(raw.CPUStats.SystemUsage) - float64(raw.PreCPUStats.SystemUsage)
	if usage <= 0 || system <= 0 {
		return 0
	}

	cpus := raw.CPUStats.OnlineCPUs
	if cpus == 0 {
		cpus = uint32(len(raw.CPUStats.CPUUsage.PercpuUsage))
	}
	if cpus == 0 {
		return 0
	}

	return usage / system * float64(cpus) * 100
}

// networkTotals aggregates the traffic of every interface.
func networkTotals(networks map[string]container.NetworkStats) (rx, tx uint64) {
	for _, network := range networks {
		rx += network.RxBytes
		tx += network.TxBytes
	}

	return rx, tx
}

// blockTotals aggregates the read and write bytes of the cgroup entries.
func blockTotals(entries []container.BlkioStatEntry) (read, write uint64) {
	for _, entry := range entries {
		switch strings.ToLower(entry.Op) {
		case "read":
			read += entry.Value
		case "write":
			write += entry.Value
		}
	}

	return read, write
}
