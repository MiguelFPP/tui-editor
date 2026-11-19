package git

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

const (
	stashRefPrefix  = "stash@{"
	stashRefSuffix  = "}"
	wipSubject      = "WIP on "
	customSubjectOn = "On "
)

// parseBranches reads the TAB-separated `for-each-ref` output.
func parseBranches(raw []byte) []domain.Branch {
	var branches []domain.Branch

	for _, line := range splitLines(raw) {
		if len(branches) >= domain.MaxBranches {
			break
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}

		name := strings.TrimSpace(fields[0])
		if name == "" {
			continue
		}

		branch := domain.Branch{
			Name:     name,
			Upstream: strings.TrimSpace(fields[1]),
			Current:  fields[2] == "*",
		}
		applyTrack(&branch, strings.TrimSpace(fields[3]))

		branches = append(branches, branch)
	}

	return branches
}

// applyTrack reads `ahead N`, `behind M`, `gone` or any combination of them.
func applyTrack(branch *domain.Branch, track string) {
	for _, part := range strings.Split(track, ",") {
		fields := strings.Fields(part)
		switch len(fields) {
		case 0:
			continue
		case 1:
			branch.Gone = branch.Gone || fields[0] == "gone"

			continue
		}

		value, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		switch fields[0] {
		case "ahead":
			branch.Ahead = value
		case "behind":
			branch.Behind = value
		}
	}
}

// parseStashes reads the NUL-separated `stash list` output in groups of three.
func parseStashes(raw []byte) []domain.StashEntry {
	tokens := splitNUL(raw)

	var entries []domain.StashEntry
	for i := 0; i+2 < len(tokens); i += 3 {
		if len(entries) >= domain.MaxStashes {
			break
		}

		entry, ok := parseStash(tokens[i], tokens[i+1], tokens[i+2])
		if !ok {
			continue
		}

		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Index < entries[j].Index })

	return entries
}

func parseStash(ref, subject, date string) (domain.StashEntry, bool) {
	index, ok := parseStashRef(ref)
	if !ok {
		return domain.StashEntry{}, false
	}

	branch, message := splitStashSubject(subject)

	entry := domain.StashEntry{Index: index, Message: message, Branch: branch}
	if created, err := time.Parse(time.RFC3339, strings.TrimSpace(date)); err == nil {
		entry.Created = created
	}

	return entry, true
}

func parseStashRef(ref string) (int, bool) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, stashRefPrefix) || !strings.HasSuffix(ref, stashRefSuffix) {
		return 0, false
	}

	index, err := strconv.Atoi(ref[len(stashRefPrefix) : len(ref)-len(stashRefSuffix)])
	if err != nil {
		return 0, false
	}

	return index, true
}

// splitStashSubject separates `WIP on <branch>: ...` and `On <branch>: ...`.
func splitStashSubject(subject string) (string, string) {
	subject = strings.TrimSpace(subject)
	rest := subject

	switch {
	case strings.HasPrefix(rest, wipSubject):
		rest = rest[len(wipSubject):]
	case strings.HasPrefix(rest, customSubjectOn):
		rest = rest[len(customSubjectOn):]
	default:
		return "", subject
	}

	branch, message, found := strings.Cut(rest, ": ")
	if !found {
		return "", subject
	}

	return branch, message
}

// splitLines returns the non-empty lines of a text stream.
func splitLines(raw []byte) []string {
	text := strings.TrimRight(strings.ToValidUTF8(string(raw), "\uFFFD"), "\n")
	if text == "" {
		return nil
	}

	return strings.Split(text, "\n")
}
