package git

import (
	"strconv"
	"strings"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// parseNumStat reads `diff --numstat -z` into a bounded summary.
func parseNumStat(raw []byte) domain.DiffStat {
	stat := domain.DiffStat{}
	tokens := splitNUL(raw)

	for i := 0; i < len(tokens); i++ {
		if len(stat.Files) >= domain.MaxDiffFiles {
			stat.Truncated = true

			break
		}

		fields := strings.SplitN(tokens[i], "\t", 3)
		if len(fields) < 3 {
			continue
		}

		added, deleted, binary, ok := parseCounts(fields[0], fields[1])
		if !ok {
			continue
		}

		// A renamed or copied entry prints an empty path and then the pre-image and post-image paths.
		orig, path := "", fields[2]
		if path == "" {
			if i+2 >= len(tokens) {
				continue
			}

			orig, path = tokens[i+1], tokens[i+2]
			i += 2
		}

		current, ok := statusPath(path)
		if !ok {
			continue
		}

		file := domain.FileStat{Path: current, Binary: binary, Additions: added, Deletions: deleted}
		if origPath, ok := statusPath(orig); ok && origPath != current {
			file.OrigPath = origPath
		}

		stat.Files = append(stat.Files, file)
		stat.Additions += added
		stat.Deletions += deleted
	}

	return stat
}

// parseCounts reads the added and deleted columns; `-` marks a binary file.
func parseCounts(added, deleted string) (int, int, bool, bool) {
	if added == "-" && deleted == "-" {
		return 0, 0, true, true
	}

	additions, err := strconv.Atoi(added)
	if err != nil {
		return 0, 0, false, false
	}

	deletions, err := strconv.Atoi(deleted)
	if err != nil {
		return 0, 0, false, false
	}

	return additions, deletions, false, true
}
