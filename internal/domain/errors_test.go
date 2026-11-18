package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSentinelsAreUsableAndWellFormed(t *testing.T) {
	sentinels := map[string]error{
		"ErrInvalidPath":       ErrInvalidPath,
		"ErrInvalidArgument":   ErrInvalidArgument,
		"ErrNotFound":          ErrNotFound,
		"ErrConflict":          ErrConflict,
		"ErrBinary":            ErrBinary,
		"ErrTooLarge":          ErrTooLarge,
		"ErrPermissionDenied":  ErrPermissionDenied,
		"ErrNotRepository":     ErrNotRepository,
		"ErrGitIndexLocked":    ErrGitIndexLocked,
		"ErrGitUnavailable":    ErrGitUnavailable,
		"ErrNoRemote":          ErrNoRemote,
		"ErrDockerUnavailable": ErrDockerUnavailable,
	}

	seen := make(map[error]string, len(sentinels))

	for name, err := range sentinels {
		if err == nil {
			t.Errorf("%s is nil", name)
			continue
		}
		if other, ok := seen[err]; ok {
			t.Errorf("%s and %s are the same error", name, other)
		}
		seen[err] = name

		message := err.Error()
		if message == "" {
			t.Errorf("%s has an empty message", name)
			continue
		}
		if message != strings.ToLower(message) {
			t.Errorf("%s message %q must be lowercase", name, message)
		}
		if strings.HasSuffix(message, ".") {
			t.Errorf("%s message %q must not end with a period", name, message)
		}
	}

	if len(seen) != len(sentinels) {
		t.Errorf("got %d distinct sentinels, want %d", len(seen), len(sentinels))
	}
}

func TestSentinelsWrapWithErrorsIs(t *testing.T) {
	wrapped := fmt.Errorf("leer %q: %w", "dir/file.txt", ErrNotFound)

	if !errors.Is(wrapped, ErrNotFound) {
		t.Errorf("errors.Is(%v, ErrNotFound) = false, want true", wrapped)
	}
	if errors.Is(wrapped, ErrConflict) {
		t.Errorf("errors.Is(%v, ErrConflict) = true, want false", wrapped)
	}
	if errors.Is(wrapped, ErrInvalidPath) {
		t.Errorf("errors.Is(%v, ErrInvalidPath) = true, want false", wrapped)
	}
}
