package git

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

const patchFileMode = "100644"

// buildPatch renders the minimal unified patch that stages or unstages the selected hunks of one
// file. Untracked, binary and renamed files are handled only by the whole-file operations, a
// truncated diff cannot be rebuilt, and the hunk starts come from the parsed diff: without
// `--recount`, git apply fails when the index no longer matches the pre-image of the patch.
func buildPatch(diff domain.Diff, hunks []int) ([]byte, error) {
	selected, err := selectedHunks(diff, hunks)
	if err != nil {
		return nil, err
	}

	oldName, newName := patchNames(diff)
	oldRef, newRef := "a/"+oldName, "b/"+newName
	if diff.New {
		oldRef = devNull
	}
	if diff.Deleted {
		newRef = devNull
	}

	var patch bytes.Buffer
	writePatchLine(&patch, "diff --git "+quotePath("a/"+oldName)+" "+quotePath("b/"+newName))
	switch {
	case diff.New:
		writePatchLine(&patch, "new file mode "+patchFileMode)
	case diff.Deleted:
		writePatchLine(&patch, "deleted file mode "+patchFileMode)
	}

	writePatchLine(&patch, "--- "+quotePath(oldRef))
	writePatchLine(&patch, "+++ "+quotePath(newRef))

	for _, index := range selected {
		hunk := diff.Hunks[index]
		writePatchLine(&patch, hunkHeader(hunk))

		for _, line := range hunk.Lines {
			writePatchLine(&patch, patchLine(line))
		}

		if patch.Len() > domain.MaxPatchBytes {
			return nil, fmt.Errorf("%w: el parche supera %d bytes", domain.ErrTooLarge, domain.MaxPatchBytes)
		}
	}

	return patch.Bytes(), nil
}

// selectedHunks validates the request and returns the hunk indices sorted and without duplicates.
func selectedHunks(diff domain.Diff, hunks []int) ([]int, error) {
	switch {
	case diff.Path.IsZero():
		return nil, fmt.Errorf("%w: falta la ruta del archivo", domain.ErrInvalidArgument)
	case diff.Binary:
		return nil, fmt.Errorf("%w: un archivo binario se prepara por archivo", domain.ErrInvalidArgument)
	case diff.Truncated:
		return nil, fmt.Errorf("%w: el diff está truncado y no se puede reconstruir", domain.ErrInvalidArgument)
	case diff.New && diff.Deleted:
		return nil, fmt.Errorf("%w: el diff no puede ser a la vez nuevo y borrado", domain.ErrInvalidArgument)
	case !diff.OrigPath.IsZero():
		return nil, fmt.Errorf("%w: un renombrado se prepara por archivo", domain.ErrInvalidArgument)
	case len(hunks) == 0:
		return nil, fmt.Errorf("%w: no hay hunks seleccionados", domain.ErrInvalidArgument)
	case len(diff.Hunks) == 0:
		return nil, fmt.Errorf("%w: el diff no tiene hunks", domain.ErrInvalidArgument)
	}

	seen := make(map[int]struct{}, len(hunks))
	selected := make([]int, 0, len(hunks))

	for _, index := range hunks {
		if index < 0 || index >= len(diff.Hunks) {
			return nil, fmt.Errorf("%w: el hunk %d no existe", domain.ErrInvalidArgument, index)
		}
		if _, ok := seen[index]; ok {
			continue
		}

		seen[index] = struct{}{}
		selected = append(selected, index)
	}

	slices.Sort(selected)

	return selected, nil
}

// patchNames returns the pre-image and the post-image path of one diff.
func patchNames(diff domain.Diff) (string, string) {
	if diff.OrigPath.IsZero() {
		return string(diff.Path), string(diff.Path)
	}

	return string(diff.OrigPath), string(diff.Path)
}

// hunkHeader keeps the starts of the parsed hunk and recounts both ranges from the emitted lines.
func hunkHeader(hunk domain.DiffHunk) string {
	oldLines, newLines := 0, 0

	for _, line := range hunk.Lines {
		switch line.Kind {
		case domain.Context:
			oldLines++
			newLines++
		case domain.Removed:
			oldLines++
		case domain.Added:
			newLines++
		}
	}

	header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, oldLines, hunk.NewStart, newLines)
	if hunk.Section != "" {
		header += " " + hunk.Section
	}

	return header
}

// patchLine renders one hunk line; the text keeps its CRLF carriage return and never a newline.
func patchLine(line domain.DiffLine) string {
	switch line.Kind {
	case domain.Added:
		return "+" + line.Text
	case domain.Removed:
		return "-" + line.Text
	case domain.NoNewline:
		return noNewlineMarker
	default:
		return " " + line.Text
	}
}

// writePatchLine appends one terminated line; a text that keeps its CR keeps CRLF.
func writePatchLine(patch *bytes.Buffer, line string) {
	patch.WriteString(line)
	patch.WriteByte('\n')
}

// quotePath renders a patch path the way git does: C quoting only for control bytes, quotes,
// backslashes and surrounding spaces; UTF-8 and inner spaces stay literal.
func quotePath(path string) string {
	if !needsQuoting(path) {
		return path
	}

	var quoted strings.Builder
	quoted.Grow(len(path) + 2)
	quoted.WriteByte('"')

	for i := 0; i < len(path); i++ {
		switch c := path[i]; c {
		case '"':
			quoted.WriteString(`\"`)
		case '\\':
			quoted.WriteString(`\\`)
		case '\a':
			quoted.WriteString(`\a`)
		case '\b':
			quoted.WriteString(`\b`)
		case '\f':
			quoted.WriteString(`\f`)
		case '\n':
			quoted.WriteString(`\n`)
		case '\r':
			quoted.WriteString(`\r`)
		case '\t':
			quoted.WriteString(`\t`)
		case '\v':
			quoted.WriteString(`\v`)
		default:
			if c < 0x20 || c == 0x7f {
				fmt.Fprintf(&quoted, `\%03o`, c)

				continue
			}

			quoted.WriteByte(c)
		}
	}

	quoted.WriteByte('"')

	return quoted.String()
}

func needsQuoting(path string) bool {
	if path == "" {
		return true
	}
	if path[0] == ' ' || path[len(path)-1] == ' ' {
		return true
	}

	for i := 0; i < len(path); i++ {
		c := path[i]
		if c < 0x20 || c == 0x7f || c == '"' || c == '\\' {
			return true
		}
	}

	return false
}
