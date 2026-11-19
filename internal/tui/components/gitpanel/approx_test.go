package gitpanel

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// TestApproxNoteIsEmptyOnTheExactPath keeps the notice out of the exact render.
func TestApproxNoteIsEmptyOnTheExactPath(t *testing.T) {
	git, files := panelWithSides(t, hunkDiff("uno", "dos", "DOS"), "uno\ndos\n", "uno\nDOS\n")
	m := openExact(t, git, files)

	if got, want := m.approxNote(), ""; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if body := plain(strings.Join(m.diffRows(), "\n")); strings.Contains(body, "aproximado") {
		t.Errorf("got = %v, want %v", body, "no approximate notice")
	}
}

// TestApproxNoteAppearsWhenASideIsMissing names the missing revision in the preview.
func TestApproxNoteAppearsWhenASideIsMissing(t *testing.T) {
	git, files := panelWithSides(t, hunkDiff("uno", "dos", "DOS"), "uno\ndos\n", "uno\nDOS\n")
	git.OnFileAt = func(context.Context, domain.FileRev, domain.RelPath) (domain.RevisionContent, error) {
		return domain.RevisionContent{}, fmt.Errorf("git: FileAt: %w", domain.ErrNotFound)
	}
	m := openExact(t, git, files)

	note := m.approxNote()
	for _, want := range []string{"aproximado", "sin contenido"} {
		if !strings.Contains(note, want) {
			t.Errorf("got = %v, want %v", note, want)
		}
	}

	body := plain(strings.Join(m.diffRows(), "\n"))
	if !strings.Contains(body, "aproximado") {
		t.Errorf("got = %v, want %v", body, "the notice in the preview")
	}
}

// TestApproxNoteExplainsATruncatedSide names the truncation in the preview.
func TestApproxNoteExplainsATruncatedSide(t *testing.T) {
	git, files := panelWithSides(t, hunkDiff("uno", "dos", "DOS"), "uno\ndos\n", "uno\nDOS\n")
	git.OnFileAt = func(_ context.Context, rev domain.FileRev, path domain.RelPath) (domain.RevisionContent, error) {
		return domain.RevisionContent{Path: path, Rev: rev, Text: "uno\ndos\n", Truncated: true}, nil
	}
	m := openExact(t, git, files)

	if got := m.approxNote(); !strings.Contains(got, "truncado") {
		t.Errorf("got = %v, want %v", got, "truncado")
	}
}

// TestApproxNoteExplainsABinarySide names the binary blob in the preview.
func TestApproxNoteExplainsABinarySide(t *testing.T) {
	git, files := panelWithSides(t, hunkDiff("uno", "dos", "DOS"), "uno\ndos\n", "uno\nDOS\n")
	git.OnFileAt = func(context.Context, domain.FileRev, domain.RelPath) (domain.RevisionContent, error) {
		return domain.RevisionContent{}, fmt.Errorf("git: %w", domain.ErrBinary)
	}
	m := openExact(t, git, files)

	if got := m.approxNote(); !strings.Contains(got, "binario") {
		t.Errorf("got = %v, want %v", got, "binario")
	}
}

// TestApproxNoteNeedsTheMatchingDiff drops the notice when the sides belong to another diff.
func TestApproxNoteNeedsTheMatchingDiff(t *testing.T) {
	git, files := panelWithSides(t, hunkDiff("uno", "dos", "DOS"), "uno\ndos\n", "uno\nDOS\n")
	git.OnFileAt = func(context.Context, domain.FileRev, domain.RelPath) (domain.RevisionContent, error) {
		return domain.RevisionContent{}, fmt.Errorf("git: FileAt: %w", domain.ErrNotFound)
	}
	m := openExact(t, git, files)

	if m.approxNote() == "" {
		t.Fatal("got = \"\", want the notice while the sides match the diff")
	}

	m.sides.path = "otro.go"
	m.sync()

	if got, want := m.approxNote(), ""; got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
}

// TestApproxNoticeAddsNoKeys keeps the footer and the help free of new keys.
func TestApproxNoticeAddsNoKeys(t *testing.T) {
	exactGit, exactFiles := panelWithSides(t, hunkDiff("uno", "dos", "DOS"), "uno\ndos\n", "uno\nDOS\n")
	exact := openExact(t, exactGit, exactFiles)

	approxGit, approxFiles := panelWithSides(t, hunkDiff("uno", "dos", "DOS"), "uno\ndos\n", "uno\nDOS\n")
	approxGit.OnFileAt = func(context.Context, domain.FileRev, domain.RelPath) (domain.RevisionContent, error) {
		return domain.RevisionContent{}, fmt.Errorf("git: FileAt: %w", domain.ErrNotFound)
	}
	approx := openExact(t, approxGit, approxFiles)

	if approx.approxNote() == "" {
		t.Fatal("got = \"\", want the approximate notice")
	}

	if got, want := approx.footerText(), exact.footerText(); got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
	if got, want := strings.Join(approx.helpLines(), "\n"), strings.Join(exact.helpLines(), "\n"); got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
}

// TestSidesReason covers the six reasons of the exact-content fallback.
func TestSidesReason(t *testing.T) {
	cases := []struct {
		name   string
		oldErr error
		newErr error
		old    domain.RevisionContent
		new    domain.RevisionContent
		want   string
	}{
		{name: "both sides exact", want: ""},
		{name: "binary side", oldErr: fmt.Errorf("git: %w", domain.ErrBinary), want: "binario"},
		{name: "missing side", newErr: fmt.Errorf("git: FileAt: %w", domain.ErrNotFound), want: "sin contenido"},
		{name: "other failure", oldErr: domain.ErrPermissionDenied, want: "sin contenido"},
		{name: "truncated side", old: domain.RevisionContent{Text: "uno", Truncated: true}, want: "truncado"},
		{name: "both sides empty", old: domain.RevisionContent{}, new: domain.RevisionContent{}, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sidesReason(tc.oldErr, tc.newErr, tc.old, tc.new); got != tc.want {
				t.Errorf("got = %v, want %v", got, tc.want)
			}
		})
	}
}
