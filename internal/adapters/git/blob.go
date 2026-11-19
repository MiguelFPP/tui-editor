package git

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// missingMarkers are the LC_ALL=C messages git prints for a path absent from a revision.
var missingMarkers = []string{
	"does not exist in",
	"exists on disk, but not in",
	"does not exist (neither on disk nor in the index",
}

// blobSpec renders the `git show` spec that names one path inside one revision.
func blobSpec(rev domain.FileRev, path domain.RelPath) (string, error) {
	if err := validateRev(rev); err != nil {
		return "", err
	}
	if path.IsZero() {
		return "", fmt.Errorf("%w: falta la ruta del blob", domain.ErrInvalidArgument)
	}
	if err := validatePath(path); err != nil {
		return "", err
	}

	if rev == domain.RevIndex {
		return ":" + string(path), nil
	}

	return "HEAD:" + string(path), nil
}

// validateRev rejects the revisions that hold no blob.
func validateRev(rev domain.FileRev) error {
	if !rev.Valid() {
		return fmt.Errorf("%w: %q no es una revisión de contenido", domain.ErrInvalidArgument, rev)
	}

	return nil
}

// parseBlob turns one blob into a bounded text preview.
func parseBlob(path domain.RelPath, rev domain.FileRev, raw []byte, truncated bool) (domain.RevisionContent, error) {
	if bytes.IndexByte(raw, 0) >= 0 {
		return domain.RevisionContent{}, fmt.Errorf("%w: %q", domain.ErrBinary, path)
	}

	text := string(raw)
	if len(raw) > domain.MaxPreviewBytes {
		text = trimPartialRune(text[:domain.MaxPreviewBytes])
		truncated = true
	}
	if clipped, cut := truncateLines(text, domain.MaxPreviewLines); cut {
		text = clipped
		truncated = true
	}

	return domain.RevisionContent{Path: path, Rev: rev, Text: text, Truncated: truncated}, nil
}

// fileAtError maps the missing-path messages of git onto the not-found sentinel.
func fileAtError(err error) error {
	var runErr *RunError
	if errors.As(err, &runErr) {
		stderr := strings.ToLower(runErr.Stderr)
		for _, marker := range missingMarkers {
			if strings.Contains(stderr, marker) {
				return fmt.Errorf("%w: FileAt", domain.ErrNotFound)
			}
		}
	}

	return translateError("FileAt", err)
}

// truncateLines keeps at most maxLines complete lines.
func truncateLines(text string, maxLines int) (string, bool) {
	if maxLines <= 0 {
		return "", text != ""
	}

	lines := 0
	for i, r := range text {
		if r != '\n' {
			continue
		}

		lines++
		if lines == maxLines && i+1 < len(text) {
			return text[:i+1], true
		}
	}

	return text, false
}

// trimPartialRune drops a trailing multi-byte rune cut in half by the byte limit.
func trimPartialRune(text string) string {
	for len(text) > 0 {
		r, size := utf8.DecodeLastRuneInString(text)
		if r != utf8.RuneError || size > 1 {
			return text
		}

		text = text[:len(text)-1]
	}

	return text
}
