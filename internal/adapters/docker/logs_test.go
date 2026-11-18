package docker

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

var (
	firstLogTime  = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	secondLogTime = time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC)
	thirdLogTime  = time.Date(2026, 1, 2, 3, 4, 7, 0, time.UTC)
)

func TestLogsStdCopy(t *testing.T) {
	payload := multiplexed(t,
		logFrame{stream: stdcopy.Stdout, text: stamped(firstLogTime) + "hola\n"},
		logFrame{stream: stdcopy.Stderr, text: stamped(secondLogTime) + "adios\n"},
	)

	var gotOptions client.ContainerLogsOptions
	api := &fakeAPI{}
	api.inspect = func(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		if id != "abc" {
			t.Errorf("ContainerInspect(%q), want %q", id, "abc")
		}

		return client.ContainerInspectResult{
			Container: container.InspectResponse{Config: &container.Config{Tty: false}},
		}, nil
	}
	api.logs = func(_ context.Context, _ string, opts client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		gotOptions = opts

		return io.NopCloser(bytes.NewReader(payload)), nil
	}

	service := newService(api, newFakeClock())

	page, err := service.Logs(context.Background(), "abc", domain.LogOptions{})
	if err != nil {
		t.Fatalf("Logs() error = %v, want nil", err)
	}

	if gotOptions.Tail != "100" {
		t.Errorf("Tail = %q, want %q", gotOptions.Tail, "100")
	}
	if gotOptions.Follow {
		t.Error("Follow = true, want false")
	}
	if !gotOptions.Timestamps {
		t.Error("Timestamps = false, want true")
	}
	if !gotOptions.ShowStdout || !gotOptions.ShowStderr {
		t.Error("ShowStdout/ShowStderr = false, want ambos true")
	}
	if gotOptions.Until != "" {
		t.Errorf("Until = %q, want vacío sin Before", gotOptions.Until)
	}

	want := []domain.LogLine{
		{Timestamp: firstLogTime, Stream: domain.Stdout, Text: "hola"},
		{Timestamp: secondLogTime, Stream: domain.Stderr, Text: "adios"},
	}
	if !sameLines(page.Lines, want) {
		t.Errorf("Lines = %+v, want %+v", page.Lines, want)
	}
	if page.Truncated {
		t.Error("Truncated = true, want false")
	}
	if !page.NextBefore.IsZero() {
		t.Errorf("NextBefore = %v, want el valor cero en una página incompleta", page.NextBefore)
	}
}

func TestLogsTTY(t *testing.T) {
	payload := []byte(stamped(firstLogTime) + "sin marco\n" + stamped(secondLogTime) + "directo\n")

	api := &fakeAPI{}
	api.inspect = func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		return client.ContainerInspectResult{
			Container: container.InspectResponse{Config: &container.Config{Tty: true}},
		}, nil
	}
	api.logs = func(_ context.Context, _ string, _ client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}

	service := newService(api, newFakeClock())

	page, err := service.Logs(context.Background(), "abc", domain.LogOptions{})
	if err != nil {
		t.Fatalf("Logs() error = %v, want nil", err)
	}

	want := []domain.LogLine{
		{Timestamp: firstLogTime, Stream: domain.Stdout, Text: "sin marco"},
		{Timestamp: secondLogTime, Stream: domain.Stdout, Text: "directo"},
	}
	if !sameLines(page.Lines, want) {
		t.Errorf("Lines = %+v, want %+v", page.Lines, want)
	}
}

func TestLogsBeforePaginates(t *testing.T) {
	cursor := secondLogTime
	payload := multiplexed(t,
		logFrame{stream: stdcopy.Stdout, text: stamped(firstLogTime) + "viejo\n"},
		logFrame{stream: stdcopy.Stdout, text: stamped(secondLogTime) + "borde\n"},
		logFrame{stream: stdcopy.Stdout, text: stamped(thirdLogTime) + "nuevo\n"},
	)

	var gotOptions client.ContainerLogsOptions
	api := &fakeAPI{}
	api.inspect = func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		return client.ContainerInspectResult{}, nil
	}
	api.logs = func(_ context.Context, _ string, opts client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		gotOptions = opts

		return io.NopCloser(bytes.NewReader(payload)), nil
	}

	service := newService(api, newFakeClock())

	page, err := service.Logs(context.Background(), "abc", domain.LogOptions{Tail: 3, Before: cursor})
	if err != nil {
		t.Fatalf("Logs() error = %v, want nil", err)
	}

	if want := cursor.UTC().Format(time.RFC3339Nano); gotOptions.Until != want {
		t.Errorf("Until = %q, want %q", gotOptions.Until, want)
	}

	want := []domain.LogLine{{Timestamp: firstLogTime, Stream: domain.Stdout, Text: "viejo"}}
	if !sameLines(page.Lines, want) {
		t.Errorf("Lines = %+v, want %+v", page.Lines, want)
	}
	if !page.NextBefore.Equal(firstLogTime) {
		t.Errorf("NextBefore = %v, want %v", page.NextBefore, firstLogTime)
	}
}

func TestLogsPaginationContinuesAfterFullPage(t *testing.T) {
	// The daemon answers "until" inclusively, so the second page repeats the boundary line.
	pages := [][]logFrame{
		{
			{stream: stdcopy.Stdout, text: stamped(secondLogTime) + "dos\n"},
			{stream: stdcopy.Stdout, text: stamped(thirdLogTime) + "tres\n"},
		},
		{
			{stream: stdcopy.Stdout, text: stamped(secondLogTime) + "dos\n"},
			{stream: stdcopy.Stdout, text: stamped(firstLogTime) + "uno\n"},
		},
	}

	calls := 0
	api := &fakeAPI{}
	api.inspect = func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		return client.ContainerInspectResult{}, nil
	}
	api.logs = func(_ context.Context, _ string, _ client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		frames := pages[calls]
		calls++

		return io.NopCloser(bytes.NewReader(multiplexed(t, frames...))), nil
	}

	service := newService(api, newFakeClock())
	ctx := context.Background()

	first, err := service.Logs(ctx, "abc", domain.LogOptions{Tail: 2})
	if err != nil {
		t.Fatalf("Logs() error = %v, want nil", err)
	}

	firstWant := []domain.LogLine{
		{Timestamp: secondLogTime, Stream: domain.Stdout, Text: "dos"},
		{Timestamp: thirdLogTime, Stream: domain.Stdout, Text: "tres"},
	}
	if !sameLines(first.Lines, firstWant) {
		t.Fatalf("Lines = %+v, want %+v", first.Lines, firstWant)
	}
	if !first.NextBefore.Equal(secondLogTime) {
		t.Fatalf("NextBefore = %v, want %v", first.NextBefore, secondLogTime)
	}

	second, err := service.Logs(ctx, "abc", domain.LogOptions{Tail: 2, Before: first.NextBefore})
	if err != nil {
		t.Fatalf("Logs() error = %v, want nil", err)
	}

	secondWant := []domain.LogLine{{Timestamp: firstLogTime, Stream: domain.Stdout, Text: "uno"}}
	if !sameLines(second.Lines, secondWant) {
		t.Errorf("Lines = %+v, want %+v", second.Lines, secondWant)
	}
	if !second.NextBefore.Equal(firstLogTime) {
		t.Errorf("NextBefore = %v, want %v", second.NextBefore, firstLogTime)
	}
}

func TestLogsTailTruncates(t *testing.T) {
	payload := multiplexed(t,
		logFrame{stream: stdcopy.Stdout, text: stamped(firstLogTime) + "uno\n"},
		logFrame{stream: stdcopy.Stdout, text: stamped(secondLogTime) + "dos\n"},
		logFrame{stream: stdcopy.Stdout, text: stamped(thirdLogTime) + "tres\n"},
	)

	var gotOptions client.ContainerLogsOptions
	api := &fakeAPI{}
	api.inspect = func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		return client.ContainerInspectResult{}, nil
	}
	api.logs = func(_ context.Context, _ string, opts client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		gotOptions = opts

		return io.NopCloser(bytes.NewReader(payload)), nil
	}

	service := newService(api, newFakeClock())

	page, err := service.Logs(context.Background(), "abc", domain.LogOptions{Tail: 2})
	if err != nil {
		t.Fatalf("Logs() error = %v, want nil", err)
	}

	if gotOptions.Tail != "2" {
		t.Errorf("Tail = %q, want %q", gotOptions.Tail, "2")
	}

	want := []domain.LogLine{
		{Timestamp: secondLogTime, Stream: domain.Stdout, Text: "dos"},
		{Timestamp: thirdLogTime, Stream: domain.Stdout, Text: "tres"},
	}
	if !sameLines(page.Lines, want) {
		t.Errorf("Lines = %+v, want %+v", page.Lines, want)
	}
	if !page.Truncated {
		t.Error("Truncated = false, want true")
	}
	if !page.NextBefore.Equal(secondLogTime) {
		t.Errorf("NextBefore = %v, want %v", page.NextBefore, secondLogTime)
	}
}

func TestLogsTruncatesBigPayload(t *testing.T) {
	huge := strings.Repeat("x", maxLogBytes+1024)

	api := &fakeAPI{}
	api.inspect = func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		return client.ContainerInspectResult{
			Container: container.InspectResponse{Config: &container.Config{Tty: true}},
		}, nil
	}
	api.logs = func(_ context.Context, _ string, _ client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		return io.NopCloser(strings.NewReader(huge)), nil
	}

	service := newService(api, newFakeClock())

	page, err := service.Logs(context.Background(), "abc", domain.LogOptions{})
	if err != nil {
		t.Fatalf("Logs() error = %v, want nil", err)
	}
	if !page.Truncated {
		t.Error("Truncated = false, want true")
	}
	if len(page.Lines) != 1 {
		t.Fatalf("Lines = %d, want 1", len(page.Lines))
	}
	if len(page.Lines[0].Text) > maxLogBytes {
		t.Errorf("línea de %d bytes, want <= %d", len(page.Lines[0].Text), maxLogBytes)
	}
}

func TestLogsEmpty(t *testing.T) {
	api := &fakeAPI{}
	api.inspect = func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
		return client.ContainerInspectResult{}, nil
	}
	api.logs = func(_ context.Context, _ string, _ client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}

	service := newService(api, newFakeClock())

	page, err := service.Logs(context.Background(), "abc", domain.LogOptions{Tail: 0})
	if err != nil {
		t.Fatalf("Logs() error = %v, want nil", err)
	}
	if len(page.Lines) != 0 {
		t.Errorf("Lines = %+v, want ninguna", page.Lines)
	}
	if page.Truncated || !page.NextBefore.IsZero() {
		t.Errorf("página = %+v, want vacía y sin truncar", page)
	}
}

func TestLogsRejectsEmptyID(t *testing.T) {
	api := &fakeAPI{}
	service := newService(api, newFakeClock())

	if _, err := service.Logs(context.Background(), "", domain.LogOptions{}); err == nil {
		t.Error("Logs(\"\") error = nil, want un error")
	}
}

// stamped prefixes a timestamp the way the daemon does when Timestamps is on.
func stamped(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano) + " "
}

func sameLines(got, want []domain.LogLine) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i].Stream != want[i].Stream || got[i].Text != want[i].Text {
			return false
		}
		if !got[i].Timestamp.Equal(want[i].Timestamp) {
			return false
		}
	}

	return true
}
