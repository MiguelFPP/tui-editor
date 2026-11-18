package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeCommitMessageKeepsTheBodyAndOneFinalNewline(t *testing.T) {
	raw := "titulo\r\n\r\ncuerpo con\n\nvarias lineas"

	got, err := NormalizeCommitMessage(raw)
	if err != nil {
		t.Fatalf("NormalizeCommitMessage() error = %v, want nil", err)
	}

	want := "titulo\n\ncuerpo con\n\nvarias lineas\n"
	if got != want {
		t.Errorf("NormalizeCommitMessage() = %q, want %q", got, want)
	}
	if strings.Count(got, "\r\n") != 0 {
		t.Errorf("NormalizeCommitMessage() = %q, want no CRLF left", got)
	}
}

func TestNormalizeCommitMessageTrimsTheBlankTail(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "without newline", raw: "titulo", want: "titulo\n"},
		{name: "one newline", raw: "titulo\n", want: "titulo\n"},
		{name: "several newlines", raw: "titulo\n\n\n", want: "titulo\n"},
		{name: "crlf tail", raw: "titulo\r\n\r\n", want: "titulo\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeCommitMessage(tc.raw)
			if err != nil {
				t.Fatalf("NormalizeCommitMessage(%q) error = %v, want nil", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeCommitMessage(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestNormalizeCommitMessageRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want error
	}{
		{name: "empty", raw: "", want: ErrInvalidArgument},
		{name: "spaces", raw: "   ", want: ErrInvalidArgument},
		{name: "newlines", raw: "\n\n", want: ErrInvalidArgument},
		{name: "crlf only", raw: "\r\n", want: ErrInvalidArgument},
		{name: "nul", raw: "titulo\x00cuerpo", want: ErrInvalidArgument},
		{name: "too large", raw: strings.Repeat("a", MaxCommitMessageBytes+1), want: ErrTooLarge},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeCommitMessage(tc.raw)
			if !errors.Is(err, tc.want) {
				t.Fatalf("NormalizeCommitMessage() error = %v, want %v", err, tc.want)
			}
			if got != "" {
				t.Errorf("NormalizeCommitMessage() = %q, want an empty string on failure", got)
			}
		})
	}
}

func TestNormalizeCommitMessageAcceptsTheLimit(t *testing.T) {
	raw := strings.Repeat("a", MaxCommitMessageBytes-1)

	got, err := NormalizeCommitMessage(raw)
	if err != nil {
		t.Fatalf("NormalizeCommitMessage() error = %v, want nil", err)
	}
	if len(got) != MaxCommitMessageBytes {
		t.Errorf("len(NormalizeCommitMessage()) = %d, want %d", len(got), MaxCommitMessageBytes)
	}
}
