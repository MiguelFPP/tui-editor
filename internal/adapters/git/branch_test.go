package git

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// callWith returns the arguments of the first recorded call whose subcommand is verb.
func callWith(runner *fakeRunner, verb string) []string {
	for _, call := range runner.Calls() {
		if len(call.Args) > 0 && call.Args[0] == verb {
			return call.Args
		}
	}

	return nil
}

func TestBranchCheckoutUsesNoGuess(t *testing.T) {
	service, runner := writeService(t, ok)

	if err := service.Checkout(context.Background(), "feature/x"); err != nil {
		t.Fatalf("Checkout() error = %v, want nil", err)
	}

	want := []string{"checkout", "--no-guess", "feature/x", "--"}
	if got := runner.last(); !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1", got)
	}
}

func TestBranchOperationsRejectInvalidNames(t *testing.T) {
	cases := []struct {
		name string
		want error
	}{
		{name: "", want: domain.ErrInvalidArgument},
		{name: "con espacio", want: domain.ErrInvalidArgument},
		{name: "-x", want: domain.ErrInvalidArgument},
		{name: "--all", want: domain.ErrInvalidArgument},
		{name: "ra..ma", want: domain.ErrInvalidArgument},
		{name: "ra@{1}", want: domain.ErrInvalidArgument},
		{name: "ra~ma", want: domain.ErrInvalidArgument},
		{name: "ra:ma", want: domain.ErrInvalidArgument},
		{name: "ra\nma", want: domain.ErrInvalidArgument},
		{name: "ra\x00ma", want: domain.ErrInvalidArgument},
		{name: "rama.lock", want: domain.ErrInvalidArgument},
		{name: "rama/", want: domain.ErrInvalidArgument},
		{name: "rama.", want: domain.ErrInvalidArgument},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, runner := writeService(t, ok)
			ctx := context.Background()

			if err := service.Checkout(ctx, tc.name); !errors.Is(err, tc.want) {
				t.Errorf("Checkout(%q) error = %v, want %v", tc.name, err, tc.want)
			}
			if err := service.CreateBranch(ctx, tc.name, ""); !errors.Is(err, tc.want) {
				t.Errorf("CreateBranch(%q) error = %v, want %v", tc.name, err, tc.want)
			}
			if err := service.DeleteBranch(ctx, tc.name, false); !errors.Is(err, tc.want) {
				t.Errorf("DeleteBranch(%q) error = %v, want %v", tc.name, err, tc.want)
			}
			if got := runner.count(); got != 0 {
				t.Errorf("runner calls = %d, want 0: an invalid name never reaches git", got)
			}
		})
	}
}

func TestBranchCreateSwitchesInOneTurn(t *testing.T) {
	cases := []struct {
		name       string
		startPoint string
		want       []string
	}{
		{
			name:       "con punto de partida",
			startPoint: "main",
			want:       []string{"checkout", "--no-guess", "-b", "nueva", "main", "--"},
		},
		{
			name: "sin punto de partida",
			want: []string{"checkout", "--no-guess", "-b", "nueva", "--"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, runner := writeService(t, ok)

			if err := service.CreateBranch(context.Background(), "nueva", tc.startPoint); err != nil {
				t.Fatalf("CreateBranch() error = %v, want nil", err)
			}
			if got := runner.last(); !slices.Equal(got, tc.want) {
				t.Errorf("args = %q, want %q", got, tc.want)
			}
			if got := runner.count(); got != 1 {
				t.Errorf("runner calls = %d, want 1: the branch is created and entered at once", got)
			}
		})
	}
}

func TestBranchCreateRejectsAnInvalidStartPoint(t *testing.T) {
	for _, startPoint := range []string{"-x", "--all", "main..dev", "main@{1}", "con espacio", "main\n"} {
		t.Run(startPoint, func(t *testing.T) {
			service, runner := writeService(t, ok)

			err := service.CreateBranch(context.Background(), "nueva", startPoint)
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("CreateBranch(%q) error = %v, want %v", startPoint, err, domain.ErrInvalidArgument)
			}
			if got := runner.count(); got != 0 {
				t.Errorf("runner calls = %d, want 0", got)
			}
		})
	}
}

func TestBranchDeleteNeverForcesFromTheOutside(t *testing.T) {
	cases := []struct {
		name  string
		force bool
		want  []string
	}{
		{name: "sin forzar", want: []string{"branch", "--delete", "vieja"}},
		{name: "forzando", force: true, want: []string{"branch", "--force", "vieja"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, runner := writeService(t, ok)

			if err := service.DeleteBranch(context.Background(), "vieja", tc.force); err != nil {
				t.Fatalf("DeleteBranch() error = %v, want nil", err)
			}
			if got := runner.last(); !slices.Equal(got, tc.want) {
				t.Errorf("args = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBranchWritesSerialiseAndRetryALockedIndex(t *testing.T) {
	var attempts int

	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		attempts++
		if attempts == 1 {
			return Result{Code: 128}, &RunError{
				Args: args, Code: 128, Stderr: "fatal: Unable to create '.git/index.lock': File exists.",
			}
		}

		return Result{}, nil
	})

	if err := service.Checkout(context.Background(), "main"); err != nil {
		t.Fatalf("Checkout() error = %v, want the retry to succeed", err)
	}
	if got := runner.count(); got != 2 {
		t.Errorf("runner calls = %d, want 2: a branch write retries the locked index once", got)
	}
}
