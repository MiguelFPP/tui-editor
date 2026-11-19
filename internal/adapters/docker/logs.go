package docker

import (
	"bytes"
	"context"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

// maxLogBytes bounds one page read; a bigger page is truncated.
const maxLogBytes = 1 << 20

// Logs reads one bounded page of container logs in time order.
func (s *Service) Logs(ctx context.Context, id string, opts domain.LogOptions) (domain.LogPage, error) {
	if err := validateID(id); err != nil {
		return domain.LogPage{}, err
	}

	opts = opts.Normalized()

	ctx, cancel := s.withTimeout(ctx, logTimeout)
	defer cancel()

	// The daemon multiplexes stdout and stderr only when the container has no TTY.
	info, err := s.api.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return domain.LogPage{}, translateError("Logs", err)
	}
	tty := info.Container.Config != nil && info.Container.Config.Tty

	logOpts := client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Follow:     false,
		Tail:       strconv.Itoa(opts.Tail),
	}
	if !opts.Before.IsZero() {
		// Backward pagination: "until" is the lower bound of the previous page.
		logOpts.Until = opts.Before.UTC().Format(time.RFC3339Nano)
	}

	stream, err := s.api.ContainerLogs(ctx, id, logOpts)
	if err != nil {
		return domain.LogPage{}, translateError("Logs", err)
	}
	defer stream.Close()

	payload, cut, err := readBounded(stream, maxLogBytes)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return domain.LogPage{}, translateError("Logs", ctxErr)
		}

		return domain.LogPage{}, translateError("Logs", err)
	}

	lines := decodeLines(payload, tty)
	// A request the daemon filled means there is still older history behind the page.
	filled := len(lines) >= opts.Tail
	if !opts.Before.IsZero() {
		// The daemon reads "until" inclusively, so the boundary lines of the previous page go away.
		lines = discardFrom(lines, opts.Before)
	}

	truncated := cut
	if len(lines) > opts.Tail {
		lines = lines[len(lines)-opts.Tail:]
		truncated = true
	}

	page := domain.LogPage{Lines: lines, Truncated: truncated}
	if filled && len(lines) > 0 && !lines[0].Timestamp.IsZero() {
		page.NextBefore = lines[0].Timestamp
	}

	return page, nil
}

// readBounded reads at most limit bytes and reports whether the source had more.
func readBounded(r io.Reader, limit int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > limit {
		return data[:limit], true, nil
	}

	return data, false, nil
}

// decodeLines demultiplexes the payload and returns the lines in timestamp order.
func decodeLines(payload []byte, tty bool) []domain.LogLine {
	if len(payload) == 0 {
		return nil
	}

	stdout, stderr := payload, []byte(nil)
	if !tty {
		var outBuf, errBuf bytes.Buffer
		if _, err := stdcopy.StdCopy(&outBuf, &errBuf, bytes.NewReader(payload)); err != nil {
			// A payload without framing (a TTY container, for instance) stays raw stdout.
			stdout, stderr = payload, nil
		} else {
			stdout, stderr = outBuf.Bytes(), errBuf.Bytes()
		}
	}

	lines := parseLines(stdout, domain.Stdout)
	lines = append(lines, parseLines(stderr, domain.Stderr)...)

	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].Timestamp.Before(lines[j].Timestamp)
	})

	return lines
}

// parseLines splits one stream into lines with the RFC3339Nano prefix of the daemon.
func parseLines(data []byte, stream domain.LogStream) []domain.LogLine {
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil
	}

	raw := strings.Split(text, "\n")
	lines := make([]domain.LogLine, 0, len(raw))
	for _, line := range raw {
		stamp, body := splitTimestamp(strings.TrimSuffix(line, "\r"))
		lines = append(lines, domain.LogLine{Timestamp: stamp, Stream: stream, Text: body})
	}

	return lines
}

// splitTimestamp separates the timestamp prefix; a line without one keeps its text.
func splitTimestamp(line string) (time.Time, string) {
	stamp, body, found := strings.Cut(line, " ")
	if !found {
		return time.Time{}, line
	}

	parsed, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return time.Time{}, line
	}

	return parsed.UTC(), body
}

// discardFrom drops the lines at or after the cursor of the previous page.
func discardFrom(lines []domain.LogLine, before time.Time) []domain.LogLine {
	kept := lines[:0]
	for _, line := range lines {
		if line.Timestamp.Before(before) {
			kept = append(kept, line)
		}
	}

	return kept
}
