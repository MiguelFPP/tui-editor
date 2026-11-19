package git

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

func TestFileAtRunsTheExactShowCommand(t *testing.T) {
	cases := []struct {
		name string
		rev  domain.FileRev
		want []string
	}{
		{
			name: "head",
			rev:  domain.RevHead,
			want: []string{"show", "--no-color", "--no-ext-diff", "--no-textconv", "--no-patch", "--format=", "HEAD:dir/file.txt"},
		},
		{
			name: "index",
			rev:  domain.RevIndex,
			want: []string{"show", "--no-color", "--no-ext-diff", "--no-textconv", "--no-patch", "--format=", ":dir/file.txt"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{}

			if _, err := newTestService(t, runner).FileAt(context.Background(), tc.rev, "dir/file.txt"); err != nil {
				t.Fatalf("FileAt() error = %v, want nil", err)
			}
			if got := runner.last(); !slices.Equal(got, tc.want) {
				t.Errorf("args = %q, want %q", got, tc.want)
			}
			if got := runner.count(); got != 1 {
				t.Errorf("runner calls = %d, want 1", got)
			}
		})
	}
}

func TestFileAtReadsTheBlobOfTheRevision(t *testing.T) {
	runner := &fakeRunner{result: Result{Stdout: []byte("uno\ndos\n")}}

	content, err := newTestService(t, runner).FileAt(context.Background(), domain.RevHead, "dir/file.txt")
	if err != nil {
		t.Fatalf("FileAt() error = %v, want nil", err)
	}
	if content.Path != "dir/file.txt" || content.Rev != domain.RevHead || content.Text != "uno\ndos\n" || content.Truncated {
		t.Errorf("FileAt() = %+v, want the blob of the revision", content)
	}
}

func TestFileAtPropagatesRunnerTruncation(t *testing.T) {
	runner := &fakeRunner{result: Result{Stdout: []byte("uno\n"), Truncated: true}}

	content, err := newTestService(t, runner).FileAt(context.Background(), domain.RevIndex, "file.txt")
	if err != nil {
		t.Fatalf("FileAt() error = %v, want nil", err)
	}
	if !content.Truncated {
		t.Errorf("FileAt() = %+v, want the truncation flag of the runner", content)
	}
}

func TestFileAtRejectsABinaryBlob(t *testing.T) {
	runner := &fakeRunner{result: Result{Stdout: []byte("\x00\x01binario\x00")}}

	if _, err := newTestService(t, runner).FileAt(context.Background(), domain.RevHead, "bin.dat"); !errors.Is(err, domain.ErrBinary) {
		t.Errorf("FileAt() error = %v, want %v", err, domain.ErrBinary)
	}
}

func TestFileAtTruncatesLongBlobsByLines(t *testing.T) {
	var raw strings.Builder
	for range domain.MaxPreviewLines + 5 {
		raw.WriteString("linea\n")
	}

	runner := &fakeRunner{result: Result{Stdout: []byte(raw.String())}}

	content, err := newTestService(t, runner).FileAt(context.Background(), domain.RevHead, "big.txt")
	if err != nil {
		t.Fatalf("FileAt() error = %v, want nil", err)
	}
	if !content.Truncated {
		t.Error("Truncated = false, want true")
	}
	if got := strings.Count(content.Text, "\n"); got != domain.MaxPreviewLines {
		t.Errorf("lines = %d, want %d", got, domain.MaxPreviewLines)
	}
}

func TestFileAtTruncatesLongBlobsByBytes(t *testing.T) {
	runner := &fakeRunner{result: Result{Stdout: []byte(strings.Repeat("a", domain.MaxPreviewBytes+16))}}

	content, err := newTestService(t, runner).FileAt(context.Background(), domain.RevHead, "big.txt")
	if err != nil {
		t.Fatalf("FileAt() error = %v, want nil", err)
	}
	if !content.Truncated || len(content.Text) != domain.MaxPreviewBytes {
		t.Errorf("FileAt() = %d bytes, truncated %v, want %d bytes truncated",
			len(content.Text), content.Truncated, domain.MaxPreviewBytes)
	}
}

func TestFileAtMapsAMissingPathOntoNotFound(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
	}{
		{name: "exists on disk but not in HEAD", stderr: "fatal: path 'x' exists on disk, but not in 'HEAD'"},
		{name: "neither on disk nor in the index", stderr: "fatal: path 'x' does not exist (neither on disk nor in the index)"},
		{name: "not in HEAD", stderr: "fatal: path 'x' does not exist in 'HEAD'"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{err: &RunError{Args: []string{"show"}, Code: 128, Stderr: tc.stderr}}

			if _, err := newTestService(t, runner).FileAt(context.Background(), domain.RevHead, "x"); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("FileAt() error = %v, want %v", err, domain.ErrNotFound)
			}
		})
	}
}

func TestFileAtKeepsAnUnknownFailure(t *testing.T) {
	runner := &fakeRunner{err: &RunError{Args: []string{"show"}, Code: 128, Stderr: "boom"}}

	_, err := newTestService(t, runner).FileAt(context.Background(), domain.RevHead, "file.txt")
	if err == nil {
		t.Fatal("FileAt() error = nil, want the failure")
	}
	if errors.Is(err, domain.ErrNotFound) {
		t.Errorf("FileAt() error = %v, want it not to be %v", err, domain.ErrNotFound)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("FileAt() error = %q, want the technical stderr preserved", err)
	}
}

func TestFileAtKeepsACancelledContext(t *testing.T) {
	runner := &fakeRunner{err: &RunError{Args: []string{"show"}, Err: context.Canceled}}

	if _, err := newTestService(t, runner).FileAt(context.Background(), domain.RevHead, "file.txt"); !errors.Is(err, context.Canceled) {
		t.Errorf("FileAt() error = %v, want %v", err, context.Canceled)
	}
}

func TestFileAtRejectsUnusableRequests(t *testing.T) {
	cases := []struct {
		name string
		rev  domain.FileRev
		path domain.RelPath
		want error
	}{
		{name: "zero revision", rev: domain.FileRev(0), path: "file.txt", want: domain.ErrInvalidArgument},
		{name: "unknown revision", rev: domain.FileRev(99), path: "file.txt", want: domain.ErrInvalidArgument},
		{name: "empty path", rev: domain.RevHead, path: "", want: domain.ErrInvalidArgument},
		{name: "non-normalised path", rev: domain.RevIndex, path: "a/../b", want: domain.ErrInvalidPath},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{}

			_, err := newTestService(t, runner).FileAt(context.Background(), tc.rev, tc.path)
			if !errors.Is(err, tc.want) {
				t.Errorf("FileAt(%v, %q) error = %v, want %v", tc.rev, tc.path, err, tc.want)
			}
			if got := runner.count(); got != 0 {
				t.Errorf("runner calls = %d, want 0: an unusable request never reaches git", got)
			}
		})
	}
}
