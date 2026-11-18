package domain

import (
	"fmt"
	"strings"
)

// EditorCommand is a binary plus its arguments, never a shell command line.
type EditorCommand struct {
	Binary string
	Args   []string
}

func (c EditorCommand) IsZero() bool {
	return c.Binary == ""
}

func (c EditorCommand) String() string {
	if c.IsZero() {
		return ""
	}

	return strings.Join(append([]string{c.Binary}, c.Args...), " ")
}

// ParseEditorCommand splits raw on whitespace; quotes are not interpreted.
func ParseEditorCommand(raw string) (EditorCommand, error) {
	if strings.ContainsRune(raw, 0) {
		return EditorCommand{}, fmt.Errorf("%w: el comando contiene un byte NUL", ErrInvalidArgument)
	}

	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return EditorCommand{}, fmt.Errorf("%w: el comando del editor está vacío", ErrInvalidArgument)
	}

	return EditorCommand{Binary: fields[0], Args: fields[1:]}, nil
}

// EditorTarget is the file and line the editor must open.
type EditorTarget struct {
	Path RelPath
	Line int
}
