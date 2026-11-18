package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

// fakeAPI is a scriptable stand-in for the Moby client; an unused method fails loudly.
type fakeAPI struct {
	ping    func(ctx context.Context, options client.PingOptions) (client.PingResult, error)
	list    func(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	inspect func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	stats   func(ctx context.Context, containerID string, options client.ContainerStatsOptions) (client.ContainerStatsResult, error)
	logs    func(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	start   func(ctx context.Context, containerID string, options client.ContainerStartOptions) (client.ContainerStartResult, error)
	stop    func(ctx context.Context, containerID string, options client.ContainerStopOptions) (client.ContainerStopResult, error)
	restart func(ctx context.Context, containerID string, options client.ContainerRestartOptions) (client.ContainerRestartResult, error)
	events  func(ctx context.Context, options client.EventsListOptions) client.EventsResult

	closed int
}

func (f *fakeAPI) Close() error {
	f.closed++

	return nil
}

func (f *fakeAPI) Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error) {
	if f.ping == nil {
		panic("fakeAPI: Ping sin configurar")
	}

	return f.ping(ctx, options)
}

func (f *fakeAPI) ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
	if f.list == nil {
		panic("fakeAPI: ContainerList sin configurar")
	}

	return f.list(ctx, options)
}

func (f *fakeAPI) ContainerInspect(ctx context.Context, id string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if f.inspect == nil {
		panic("fakeAPI: ContainerInspect sin configurar")
	}

	return f.inspect(ctx, id, options)
}

func (f *fakeAPI) ContainerStats(ctx context.Context, id string, options client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
	if f.stats == nil {
		panic("fakeAPI: ContainerStats sin configurar")
	}

	return f.stats(ctx, id, options)
}

func (f *fakeAPI) ContainerLogs(ctx context.Context, id string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	if f.logs == nil {
		panic("fakeAPI: ContainerLogs sin configurar")
	}

	return f.logs(ctx, id, options)
}

func (f *fakeAPI) ContainerStart(ctx context.Context, id string, options client.ContainerStartOptions) (client.ContainerStartResult, error) {
	if f.start == nil {
		panic("fakeAPI: ContainerStart sin configurar")
	}

	return f.start(ctx, id, options)
}

func (f *fakeAPI) ContainerStop(ctx context.Context, id string, options client.ContainerStopOptions) (client.ContainerStopResult, error) {
	if f.stop == nil {
		panic("fakeAPI: ContainerStop sin configurar")
	}

	return f.stop(ctx, id, options)
}

func (f *fakeAPI) ContainerRestart(ctx context.Context, id string, options client.ContainerRestartOptions) (client.ContainerRestartResult, error) {
	if f.restart == nil {
		panic("fakeAPI: ContainerRestart sin configurar")
	}

	return f.restart(ctx, id, options)
}

func (f *fakeAPI) Events(ctx context.Context, options client.EventsListOptions) client.EventsResult {
	if f.events == nil {
		panic("fakeAPI: Events sin configurar")
	}

	return f.events(ctx, options)
}

// fakeClock drives the debounce timers of the tests without sleeping.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	timers  []*fakeTimer
	created int
	armed   chan struct{}
}

func newFakeClock() *fakeClock {
	return &fakeClock{
		now:   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		armed: make(chan struct{}, 32),
	}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *fakeClock) NewTimer(d time.Duration) timer {
	c.mu.Lock()
	defer c.mu.Unlock()

	armed := &fakeTimer{due: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, armed)
	c.created++

	select {
	case c.armed <- struct{}{}:
	default:
	}

	return armed
}

// timersCreated counts every timer the adapter armed; it never decreases.
func (c *fakeClock) timersCreated() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.created
}

// waitTimer blocks until the adapter armed a timer, so Advance never races with the pump.
func (c *fakeClock) waitTimer(t *testing.T) {
	t.Helper()

	select {
	case <-c.armed:
	case <-time.After(2 * time.Second):
		t.Fatal("el adaptador no armó el temporizador de debounce")
	}
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)

	var due []*fakeTimer
	remaining := make([]*fakeTimer, 0, len(c.timers))
	for _, armed := range c.timers {
		if armed.isStopped() {
			continue
		}
		if armed.due.After(c.now) {
			remaining = append(remaining, armed)
			continue
		}
		due = append(due, armed)
	}
	c.timers = remaining
	now := c.now
	c.mu.Unlock()

	for _, armed := range due {
		armed.fire(now)
	}
}

type fakeTimer struct {
	mu      sync.Mutex
	due     time.Time
	ch      chan time.Time
	stopped bool
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stopped {
		return false
	}
	t.stopped = true

	return true
}

func (t *fakeTimer) isStopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.stopped
}

func (t *fakeTimer) fire(at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stopped {
		return
	}

	select {
	case t.ch <- at:
	default:
	}
}

// logFrame is one frame of a multiplexed log stream.
type logFrame struct {
	stream stdcopy.StdType
	text   string
}

// multiplexed builds the header framing of a container without TTY.
func multiplexed(t *testing.T, frames ...logFrame) []byte {
	t.Helper()

	var buf bytes.Buffer
	for _, frame := range frames {
		var header [8]byte
		header[0] = byte(frame.stream)
		binary.BigEndian.PutUint32(header[4:], uint32(len(frame.text)))
		buf.Write(header[:])
		buf.WriteString(frame.text)
	}

	return buf.Bytes()
}
