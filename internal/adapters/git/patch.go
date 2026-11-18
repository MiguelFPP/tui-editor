package git

import (
	"strconv"
	"strings"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

const (
	noNewlineMarker = `\ No newline at end of file`
	devNull         = "/dev/null"
)

// patchFile accumulates the state of one `diff --git` block until it is flushed.
type patchFile struct {
	diff domain.Diff

	pairOld string
	pairNew string

	headOld string
	headNew string
	hasHead bool

	renOld string
	renNew string
	hasRen bool

	isNew     bool
	isDeleted bool
}

// hunkCursor tracks the line numbers of the hunk being read.
type hunkCursor struct {
	hunk    domain.DiffHunk
	oldLine int
	newLine int
}

// parsePatch splits a unified diff into one bounded domain.Diff per file.
func parsePatch(raw string) []domain.Diff {
	if raw == "" {
		return nil
	}

	var (
		files []domain.Diff
		file  patchFile
		cur   *hunkCursor
		open  bool
		lines int
	)

	flush := func() {
		if !open {
			return
		}
		if cur != nil {
			file.diff.Hunks = append(file.diff.Hunks, cur.hunk)
			cur = nil
		}

		file.diff.Path, file.diff.OrigPath = file.names()
		file.diff.New = file.isNew
		file.diff.Deleted = file.isDeleted
		files = append(files, file.diff)
		open = false
	}

	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			file = newPatchFile(strings.TrimPrefix(line, "diff --git "))
			lines, open = 0, true

			continue
		}
		if !open {
			continue
		}

		switch {
		case strings.HasPrefix(line, "@@ "):
			if cur != nil {
				file.diff.Hunks = append(file.diff.Hunks, cur.hunk)
				cur = nil
			}
			if header, ok := parseHunkHeader(line); ok {
				cur = &hunkCursor{hunk: header, oldLine: header.OldStart, newLine: header.NewStart}
			}
		case cur == nil && strings.HasPrefix(line, "--- "):
			file.headOld, file.hasHead = patchPath(strings.TrimPrefix(line, "--- ")), true
		case cur == nil && strings.HasPrefix(line, "+++ "):
			file.headNew, file.hasHead = patchPath(strings.TrimPrefix(line, "+++ ")), true
		case strings.HasPrefix(line, "rename from "), strings.HasPrefix(line, "copy from "):
			file.renOld, file.hasRen = patchPath(afterKeyword(line)), true
		case strings.HasPrefix(line, "rename to "), strings.HasPrefix(line, "copy to "):
			file.renNew, file.hasRen = patchPath(afterKeyword(line)), true
		case strings.HasPrefix(line, "new file mode "):
			file.isNew = true
		case strings.HasPrefix(line, "deleted file mode "):
			file.isDeleted = true
		case strings.HasPrefix(line, "Binary files "), line == "GIT binary patch":
			file.diff.Binary = true
		default:
			if cur == nil || line == "" {
				continue
			}
			if lines >= domain.MaxDiffLines {
				file.diff.Truncated = true

				continue
			}
			if appendDiffLine(&file.diff, cur, line) {
				lines++
			}
		}
	}

	flush()

	return files
}

// names resolves the old and the new path of one file; renames and headers win over the pair line.
func (p patchFile) names() (domain.RelPath, domain.RelPath) {
	old, new := p.pairOld, p.pairNew

	switch {
	case p.hasRen:
		old, new = p.renOld, p.renNew
	case p.hasHead:
		switch {
		case p.headNew == devNull:
			old, new = p.headOld, p.headOld
		case p.headOld == devNull:
			old, new = "", p.headNew
		default:
			old, new = p.headOld, p.headNew
		}
	}

	path, _ := domain.NewRelPath(new)
	if old == new {
		return path, ""
	}

	orig, _ := domain.NewRelPath(old)

	return path, orig
}

// appendDiffLine records one hunk line and advances its line numbers.
func appendDiffLine(diff *domain.Diff, cur *hunkCursor, line string) bool {
	text := sanitize(line[1:])

	switch line[0] {
	case '+':
		cur.hunk.Lines = append(cur.hunk.Lines, domain.DiffLine{Kind: domain.Added, Text: text, NewLine: cur.newLine})
		cur.newLine++
		diff.Additions++
	case '-':
		cur.hunk.Lines = append(cur.hunk.Lines, domain.DiffLine{Kind: domain.Removed, Text: text, OldLine: cur.oldLine})
		cur.oldLine++
		diff.Deletions++
	case ' ':
		cur.hunk.Lines = append(cur.hunk.Lines, domain.DiffLine{
			Kind: domain.Context, Text: text, OldLine: cur.oldLine, NewLine: cur.newLine,
		})
		cur.oldLine++
		cur.newLine++
	case '\\':
		if !strings.HasPrefix(line, noNewlineMarker) {
			return false
		}
		cur.hunk.Lines = append(cur.hunk.Lines, domain.DiffLine{Kind: domain.NoNewline})
	default:
		return false
	}

	return true
}

// parseHunkHeader reads `@@ -old[,n] +new[,m] @@ section` without a regular expression.
func parseHunkHeader(line string) (domain.DiffHunk, bool) {
	const prefix = "@@ -"

	rest := strings.TrimPrefix(line, prefix)
	if rest == line {
		return domain.DiffHunk{}, false
	}

	plus := strings.Index(rest, " +")
	if plus < 0 {
		return domain.DiffHunk{}, false
	}
	tail := rest[plus+2:]

	end := strings.Index(tail, " @@")
	if end < 0 {
		return domain.DiffHunk{}, false
	}

	oldStart, oldLines, ok := parseRange(rest[:plus])
	if !ok {
		return domain.DiffHunk{}, false
	}
	newStart, newLines, ok := parseRange(tail[:end])
	if !ok {
		return domain.DiffHunk{}, false
	}

	return domain.DiffHunk{
		Header:   line,
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
		Section:  strings.TrimSpace(tail[end+3:]),
	}, true
}

// parseRange reads one `start[,count]` pair; a missing count means one line.
func parseRange(field string) (start, count int, ok bool) {
	startField, countField, hasCount := strings.Cut(field, ",")

	start, err := strconv.Atoi(startField)
	if err != nil {
		return 0, 0, false
	}
	if !hasCount {
		return start, 1, true
	}

	count, err = strconv.Atoi(countField)
	if err != nil {
		return 0, 0, false
	}

	return start, count, true
}

// newPatchFile reads the two paths of a `diff --git` line as the last-resort source.
func newPatchFile(rest string) patchFile {
	old, new := splitDiffPaths(rest)

	return patchFile{pairOld: old, pairNew: new}
}

// splitDiffPaths best-effort splits the pair of a `diff --git` line, which is ambiguous for
// paths with spaces; the rename and `---`/`+++` lines take precedence when they are present.
func splitDiffPaths(rest string) (string, string) {
	if strings.HasPrefix(rest, `"`) {
		if first, tail, ok := quotedPath(rest); ok {
			if second, _, ok := quotedPath(strings.TrimSpace(tail)); ok {
				return unquotePath(first), unquotePath(second)
			}
		}

		return unquotePath(rest), unquotePath(rest)
	}

	for i := 0; i < len(rest); i++ {
		if rest[i] == ' ' && rest[:i] == rest[i+1:] {
			return rest[:i], rest[i+1:]
		}
	}

	old, new, found := strings.Cut(rest, " ")
	if !found {
		return rest, rest
	}

	return old, new
}

// quotedPath returns the leading C-quoted path and the rest of the line.
func quotedPath(text string) (quoted, rest string, ok bool) {
	if len(text) < 2 || text[0] != '"' {
		return "", "", false
	}

	for i := 1; i < len(text); i++ {
		switch text[i] {
		case '\\':
			i++
		case '"':
			return text[:i+1], text[i+1:], true
		}
	}

	return "", "", false
}

// patchPath reads one path of a patch header, dropping the TAB git adds after names with spaces.
func patchPath(field string) string {
	return unquotePath(strings.TrimSuffix(field, "\t"))
}

// afterKeyword drops the two leading words of `rename from x` and `copy to x` lines.
func afterKeyword(line string) string {
	_, rest, found := strings.Cut(line, " ")
	if !found {
		return ""
	}
	_, rest, found = strings.Cut(rest, " ")
	if !found {
		return ""
	}

	return rest
}

// unquotePath decodes the C quoting git uses for unusual names.
func unquotePath(raw string) string {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return sanitize(raw)
	}

	body := raw[1 : len(raw)-1]
	var out strings.Builder
	out.Grow(len(body))

	for i := 0; i < len(body); i++ {
		if body[i] != '\\' {
			out.WriteByte(body[i])

			continue
		}

		i++
		if i >= len(body) {
			break
		}

		switch c := body[i]; c {
		case 'a':
			out.WriteByte('\a')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'v':
			out.WriteByte('\v')
		case '\\', '"':
			out.WriteByte(c)
		case '0', '1', '2', '3', '4', '5', '6', '7':
			start := i
			for digits := 0; digits < 3 && i < len(body) && isOctal(body[i]); digits++ {
				i++
			}
			i-- // the loop advance must skip the digits already consumed

			out.WriteByte(octalValue(body[start : i+1]))
		default:
			out.WriteByte(c)
		}
	}

	return sanitize(out.String())
}

// octalValue converts up to three octal digits into a byte.
func octalValue(digits string) byte {
	value := 0
	for i := 0; i < len(digits); i++ {
		value = value*8 + int(digits[i]-'0')
	}

	return byte(value)
}

func isOctal(c byte) bool {
	return c >= '0' && c <= '7'
}

// sanitize replaces the bytes git prints verbatim with something a terminal can render.
func sanitize(text string) string {
	if isASCII(text) {
		return text
	}

	return strings.ToValidUTF8(text, "\uFFFD")
}

func isASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] >= 0x80 {
			return false
		}
	}

	return true
}
