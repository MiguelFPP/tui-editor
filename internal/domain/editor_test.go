package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestParseEditorCommand(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want EditorCommand
	}{
		{name: "binary", raw: "vim", want: EditorCommand{Binary: "vim"}},
		{name: "binary and arg", raw: "code --wait", want: EditorCommand{Binary: "code", Args: []string{"--wait"}}},
		{name: "extra whitespace", raw: "  nvim \t -f \n ", want: EditorCommand{Binary: "nvim", Args: []string{"-f"}}},
		{
			name: "quotes are literal",
			raw:  "vim -c 'set ft=go'",
			want: EditorCommand{Binary: "vim", Args: []string{"-c", "'set", "ft=go'"}},
		},
		{name: "windows binary", raw: `C:\tools\code.exe --wait`, want: EditorCommand{Binary: `C:\tools\code.exe`, Args: []string{"--wait"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseEditorCommand(tc.raw)
			if err != nil {
				t.Fatalf("ParseEditorCommand(%q) error = %v, want nil", tc.raw, err)
			}
			if got.Binary != tc.want.Binary {
				t.Errorf("Binary = %q, want %q", got.Binary, tc.want.Binary)
			}
			if !slices.Equal(got.Args, tc.want.Args) {
				t.Errorf("Args = %q, want %q", got.Args, tc.want.Args)
			}
		})
	}
}

func TestParseEditorCommandRejects(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "spaces", raw: "   "},
		{name: "whitespace", raw: "\t\n"},
		{name: "nul", raw: "vim\x00-f"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseEditorCommand(tc.raw)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("ParseEditorCommand(%q) error = %v, want %v", tc.raw, err, ErrInvalidArgument)
			}
			if !got.IsZero() {
				t.Errorf("ParseEditorCommand(%q) = %+v, want the zero command", tc.raw, got)
			}
		})
	}
}

func TestEditorCommandZeroAndString(t *testing.T) {
	if !(EditorCommand{}).IsZero() {
		t.Error("zero IsZero() = false, want true")
	}
	if got := (EditorCommand{}).String(); got != "" {
		t.Errorf("zero String() = %q, want empty", got)
	}

	cmd := EditorCommand{Binary: "code", Args: []string{"--wait", "--reuse-window"}}
	if cmd.IsZero() {
		t.Error("IsZero() = true, want false")
	}
	if got, want := cmd.String(), "code --wait --reuse-window"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if strings.ContainsAny(cmd.String(), "\n\r") {
		t.Errorf("String() = %q keeps a newline", cmd.String())
	}
}

func TestEditorTarget(t *testing.T) {
	target := EditorTarget{Path: "dir/file.txt", Line: 12}

	if target.Path != RelPath("dir/file.txt") {
		t.Errorf("Path = %q, want %q", target.Path, "dir/file.txt")
	}
	if target.Line != 12 {
		t.Errorf("Line = %d, want 12", target.Line)
	}
	if !(EditorTarget{}).Path.IsZero() {
		t.Error("zero target path is not the root")
	}
}
