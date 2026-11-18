package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

const (
	initialOID   = "(initial)"
	detachedHead = "(detached)"
)

// parseStatus reads `status --porcelain=v2 -z --branch`, record by record.
func parseStatus(ctx context.Context, raw []byte) (domain.GitStatus, error) {
	status := domain.GitStatus{}
	tokens := splitNUL(raw)

	for i := 0; i < len(tokens); i++ {
		if err := ctx.Err(); err != nil {
			return domain.GitStatus{}, fmt.Errorf("git: Status: %w", err)
		}

		token := tokens[i]
		if token == "" {
			continue
		}
		if token[0] == '#' {
			for _, header := range strings.Split(token, "\n") {
				header = strings.TrimRight(header, "\r")
				if strings.HasPrefix(header, "#") {
					applyStatusHeader(&status, header)
				}
			}

			continue
		}
		if len(status.Files) >= domain.MaxStatusEntries {
			status.Truncated = true

			break
		}
		if token[0] == '2' && i+1 >= len(tokens) {
			status.Truncated = true

			break
		}

		file, consumed, ok := parseStatusRecord(token, tokens[i+1:])
		if !ok {
			continue
		}

		i += consumed
		status.Files = append(status.Files, file)
	}

	return status, nil
}

// applyStatusHeader reads one `# branch.*` line.
func applyStatusHeader(status *domain.GitStatus, header string) {
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return
	}

	switch fields[1] {
	case "branch.oid":
		status.Unborn = fields[2] == initialOID
	case "branch.head":
		if fields[2] == detachedHead {
			status.Detached = true
			status.Branch = ""

			return
		}

		status.Branch = fields[2]
	case "branch.upstream":
		status.Upstream = fields[2]
	case "branch.ab":
		if len(fields) < 4 {
			return
		}

		status.Ahead = parseSigned(fields[2])
		status.Behind = parseSigned(fields[3])
	}
}

// parseStatusRecord decodes one entry and how many extra tokens it consumed.
func parseStatusRecord(record string, rest []string) (domain.GitFileStatus, int, bool) {
	switch record[0] {
	case '1':
		return parseOrdinary(record)
	case '2':
		if len(rest) == 0 {
			return domain.GitFileStatus{}, 0, false
		}

		return parseRenamed(record, rest[0])
	case 'u':
		return parseUnmerged(record)
	case '?':
		return parseUntracked(record)
	default:
		return domain.GitFileStatus{}, 0, false // `!` and unknown record types are skipped
	}
}

// 1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
func parseOrdinary(record string) (domain.GitFileStatus, int, bool) {
	fields := strings.SplitN(record, " ", 9)
	if len(fields) < 9 {
		return domain.GitFileStatus{}, 0, false
	}

	path, ok := statusPath(fields[8])
	if !ok {
		return domain.GitFileStatus{}, 0, false
	}

	index, worktree := statusCodes(fields[1])

	return domain.GitFileStatus{Path: path, Index: index, Worktree: worktree}, 0, true
}

// 2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <Xscore> <path>, with the pre-image path as its own token.
func parseRenamed(record, orig string) (domain.GitFileStatus, int, bool) {
	fields := strings.SplitN(record, " ", 10)
	if len(fields) < 10 {
		return domain.GitFileStatus{}, 0, false
	}

	path, ok := statusPath(fields[9])
	if !ok {
		return domain.GitFileStatus{}, 0, false
	}

	origPath, ok := statusPath(orig)
	if !ok {
		return domain.GitFileStatus{}, 0, false
	}

	index, worktree := statusCodes(fields[1])

	return domain.GitFileStatus{Path: path, OrigPath: origPath, Index: index, Worktree: worktree}, 1, true
}

// u <XY> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>
func parseUnmerged(record string) (domain.GitFileStatus, int, bool) {
	fields := strings.SplitN(record, " ", 11)
	if len(fields) < 11 {
		return domain.GitFileStatus{}, 0, false
	}

	path, ok := statusPath(fields[10])
	if !ok {
		return domain.GitFileStatus{}, 0, false
	}

	index, worktree := statusCodes(fields[1])

	return domain.GitFileStatus{Path: path, Index: index, Worktree: worktree, Conflicted: true}, 0, true
}

// ? <path>
func parseUntracked(record string) (domain.GitFileStatus, int, bool) {
	fields := strings.SplitN(record, " ", 2)
	if len(fields) < 2 {
		return domain.GitFileStatus{}, 0, false
	}

	path, ok := statusPath(fields[1])
	if !ok {
		return domain.GitFileStatus{}, 0, false
	}

	return domain.GitFileStatus{Path: path, Index: domain.GitUntracked, Worktree: domain.GitUntracked}, 0, true
}

func statusCodes(field string) (domain.GitStatusCode, domain.GitStatusCode) {
	if len(field) < 2 {
		return domain.GitUnmodified, domain.GitUnmodified
	}

	return domain.GitStatusCode(field[0]), domain.GitStatusCode(field[1])
}

// statusPath decodes and validates one path; unrepresentable paths are dropped.
func statusPath(raw string) (domain.RelPath, bool) {
	path, err := domain.NewRelPath(unquotePath(raw))
	if err != nil || path.IsZero() {
		return "", false
	}

	return path, true
}

// parseSigned reads the `+N`/`-N` counters of `# branch.ab` as magnitudes.
func parseSigned(field string) int {
	value, err := strconv.Atoi(strings.TrimLeft(field, "+-"))
	if err != nil {
		return 0
	}

	return value
}

// splitNUL splits NUL-terminated git output, dropping the final empty token.
func splitNUL(raw []byte) []string {
	text := string(raw)
	if text == "" {
		return nil
	}

	return strings.Split(strings.TrimSuffix(text, "\x00"), "\x00")
}
