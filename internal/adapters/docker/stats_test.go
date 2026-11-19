package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestStatsMapsSample(t *testing.T) {
	read := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	raw := container.StatsResponse{
		ID:   "abc123",
		Read: read,
		CPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 200, PercpuUsage: []uint64{1, 2}},
			SystemUsage: 1000,
			OnlineCPUs:  2,
		},
		PreCPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 100},
			SystemUsage: 500,
		},
		MemoryStats: container.MemoryStats{Usage: 512, Limit: 2048},
		Networks: map[string]container.NetworkStats{
			"eth1": {RxBytes: 5, TxBytes: 7},
			"eth0": {RxBytes: 10, TxBytes: 20},
		},
		BlkioStats: container.BlkioStats{IoServiceBytesRecursive: []container.BlkioStatEntry{
			{Op: "Read", Value: 3},
			{Op: "write", Value: 4},
			{Op: "sync", Value: 9},
		}},
		PidsStats: container.PidsStats{Current: 7},
	}

	api := &fakeAPI{}
	api.stats = func(_ context.Context, id string, opts client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
		if id != "abc123" {
			t.Errorf("ContainerStats(%q), want %q", id, "abc123")
		}
		if opts.Stream {
			t.Error("ContainerStats() con Stream, want una sola muestra")
		}
		if !opts.IncludePreviousSample {
			t.Error("ContainerStats() sin IncludePreviousSample, want precpu_stats")
		}

		return client.ContainerStatsResult{Body: bodyOf(t, raw)}, nil
	}

	service := newService(api, newFakeClock())

	got, err := service.Stats(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("Stats() error = %v, want nil", err)
	}

	want := domain.ContainerStats{
		ContainerID: "abc123",
		CPUPercent:  40,
		MemoryUsed:  512,
		MemoryLimit: 2048,
		NetworkRx:   15,
		NetworkTx:   27,
		BlockRead:   3,
		BlockWrite:  4,
		PIDs:        7,
		CollectedAt: read,
	}

	if !sameStats(got, want) {
		t.Errorf("Stats() = %+v, want %+v", got, want)
	}
	if percent := got.MemoryPercent(); percent != 25 {
		t.Errorf("MemoryPercent() = %v, want 25", percent)
	}
}

// sameStats compares the fields of one sample, times by instant.
func sameStats(got, want domain.ContainerStats) bool {
	return got.ContainerID == want.ContainerID &&
		got.CPUPercent == want.CPUPercent &&
		got.MemoryUsed == want.MemoryUsed &&
		got.MemoryLimit == want.MemoryLimit &&
		got.NetworkRx == want.NetworkRx &&
		got.NetworkTx == want.NetworkTx &&
		got.BlockRead == want.BlockRead &&
		got.BlockWrite == want.BlockWrite &&
		got.PIDs == want.PIDs &&
		got.CollectedAt.Equal(want.CollectedAt)
}

func TestStatsWithoutPreviousSample(t *testing.T) {
	raw := container.StatsResponse{
		ID:          "abc",
		Read:        time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		CPUStats:    container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 200}, SystemUsage: 1000},
		MemoryStats: container.MemoryStats{Usage: 100},
	}

	api := &fakeAPI{}
	api.stats = func(_ context.Context, _ string, _ client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
		return client.ContainerStatsResult{Body: bodyOf(t, raw)}, nil
	}

	service := newService(api, newFakeClock())

	got, err := service.Stats(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Stats() error = %v, want nil", err)
	}
	if got.CPUPercent != 0 {
		t.Errorf("CPUPercent = %v, want 0", got.CPUPercent)
	}
	if percent := got.MemoryPercent(); percent != 0 {
		t.Errorf("MemoryPercent() = %v, want 0", percent)
	}
}

func TestStatsUsesRequestedIDAndClock(t *testing.T) {
	raw := container.StatsResponse{
		CPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 10},
			SystemUsage: 100,
			OnlineCPUs:  1,
		},
		PreCPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 5},
			SystemUsage: 50,
		},
	}

	api := &fakeAPI{}
	api.stats = func(_ context.Context, _ string, _ client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
		return client.ContainerStatsResult{Body: bodyOf(t, raw)}, nil
	}

	clk := newFakeClock()
	service := newService(api, clk)

	got, err := service.Stats(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Stats() error = %v, want nil", err)
	}
	if got.ContainerID != "abc" {
		t.Errorf("ContainerID = %q, want %q", got.ContainerID, "abc")
	}
	if !got.CollectedAt.Equal(clk.Now()) {
		t.Errorf("CollectedAt = %v, want %v", got.CollectedAt, clk.Now())
	}
	if got.CPUPercent != 10 {
		t.Errorf("CPUPercent = %v, want 10", got.CPUPercent)
	}
}

func TestStatsRejectsEmptyID(t *testing.T) {
	api := &fakeAPI{}
	service := newService(api, newFakeClock())

	if _, err := service.Stats(context.Background(), " "); err == nil {
		t.Error("Stats(\" \") error = nil, want un error")
	}
}

func TestStatsBrokenBody(t *testing.T) {
	api := &fakeAPI{}
	api.stats = func(_ context.Context, _ string, _ client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
		return client.ContainerStatsResult{Body: io.NopCloser(bytes.NewReader([]byte("no es json")))}, nil
	}

	service := newService(api, newFakeClock())

	if _, err := service.Stats(context.Background(), "abc"); err == nil {
		t.Error("Stats() error = nil, want un error")
	}
}

func bodyOf(t *testing.T, raw container.StatsResponse) io.ReadCloser {
	t.Helper()

	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	return io.NopCloser(bytes.NewReader(encoded))
}
