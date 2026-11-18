package domain

import (
	"fmt"
	"strings"
)

// NormalizeCommitMessage validates a commit message and returns it with LF line endings
// and exactly one trailing newline; blank lines at the end collapse into that newline.
func NormalizeCommitMessage(raw string) (string, error) {
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("%w: el mensaje contiene un byte NUL", ErrInvalidArgument)
	}
	if len(raw) > MaxCommitMessageBytes {
		return "", fmt.Errorf("%w: el mensaje supera %d bytes", ErrTooLarge, MaxCommitMessageBytes)
	}

	message := strings.ReplaceAll(raw, "\r\n", "\n")
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("%w: el mensaje del commit está vacío", ErrInvalidArgument)
	}

	return strings.TrimRight(message, "\n") + "\n", nil
}
