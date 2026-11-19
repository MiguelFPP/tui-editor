package git

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// mergeArgs are the arguments of the merge command the adapter runs.
var mergeArgs = []string{"merge", "--no-edit", "--no-stat", "otra"}

func TestMergeValidatesTheBranchName(t *testing.T) {
	for _, branch := range []string{"", "con espacio", "-x", "--all", "ra..ma", "ra@{1}", "ra~ma", "ra\ny"} {
		t.Run(branch, func(t *testing.T) {
			service, runner := writeService(t, ok)

			if _, err := service.Merge(context.Background(), branch); !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("Merge(%q) error = %v, want %v", branch, err, domain.ErrInvalidArgument)
			}
			if got := runner.count(); got != 0 {
				t.Errorf("runner calls = %d, want 0: an invalid branch never reaches git", got)
			}
		})
	}
}

func TestMergeRefusesAMergeInProgress(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "rev-parse" && args[len(args)-1] == "MERGE_HEAD" {
			return Result{Stdout: []byte("abc\n")}, nil
		}

		return Result{}, nil
	})

	result, err := service.Merge(context.Background(), "otra")
	if !errors.Is(err, domain.ErrMergeInProgress) {
		t.Fatalf("Merge() error = %v, want %v", err, domain.ErrMergeInProgress)
	}
	if result.HasConflicts() || result.Status != domain.Clean {
		t.Errorf("result = %+v, want an empty result", result)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1: only the pre-check runs", got)
	}
}

func TestMergeClassifiesTheOutcome(t *testing.T) {
	cases := []struct {
		name          string
		before, after string
		target        string
		want          domain.MergeStatus
	}{
		{name: "al día", before: "aaa", after: "aaa", target: "bbb", want: domain.UpToDate},
		{name: "avance rápido", before: "aaa", after: "bbb", target: "bbb", want: domain.FastForward},
		{name: "commit de fusión", before: "aaa", after: "ccc", target: "bbb", want: domain.Clean},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			heads := []string{tc.before, tc.after}
			headIndex := 0

			service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
				switch {
				case args[0] == "rev-parse" && args[len(args)-1] == "MERGE_HEAD":
					return Result{Code: 1}, &RunError{Args: args, Code: 1}
				case args[0] == "rev-parse" && args[len(args)-1] == "HEAD":
					id := heads[min(headIndex, len(heads)-1)]
					headIndex++

					return Result{Stdout: []byte(id + "\n")}, nil
				case args[0] == "rev-parse":
					return Result{Stdout: []byte(tc.target + "\n")}, nil
				}

				return Result{}, nil
			})

			result, err := service.Merge(context.Background(), "otra")
			if err != nil {
				t.Fatalf("Merge() error = %v, want nil", err)
			}
			if result.Status != tc.want {
				t.Errorf("Status = %d, want %d", result.Status, tc.want)
			}
			if result.HasConflicts() {
				t.Errorf("Conflicts = %+v, want none", result.Conflicts)
			}
			if got := callWith(runner, "merge"); !slices.Equal(got, mergeArgs) {
				t.Errorf("args = %q, want %q", got, mergeArgs)
			}
		})
	}
}

func TestMergeReportsConflictsWithoutAnError(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		switch {
		case args[0] == "rev-parse" && args[len(args)-1] == "MERGE_HEAD":
			return Result{Code: 1}, &RunError{Args: args, Code: 1}
		case args[0] == "rev-parse":
			return Result{Stdout: []byte("abc\n")}, nil
		case args[0] == "status":
			return Result{Stdout: fixture(t, "status_v2_unmerged.z")}, nil
		}

		return Result{Code: 1}, &RunError{
			Args: args, Code: 1, Stderr: "CONFLICT (content): Merge conflict in conflict.txt",
		}
	})

	result, err := service.Merge(context.Background(), "otra")
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil: a conflict is an outcome, not a failure", err)
	}
	if result.Status != domain.Conflicted {
		t.Errorf("Status = %d, want %d", result.Status, domain.Conflicted)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("Conflicts = %+v, want the conflicted file", result.Conflicts)
	}
	if result.Conflicts[0].Path != "conflict.txt" || result.Conflicts[0].Kind != domain.BothModified {
		t.Errorf("conflict = %+v, want conflict.txt as BothModified", result.Conflicts[0])
	}
	if got := callWith(runner, "status"); !slices.Equal(got, statusArgs) {
		t.Errorf("status args = %q, want %q", got, statusArgs)
	}
}

func TestMergeKeepsAnOrdinaryFailure(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "rev-parse" {
			if args[len(args)-1] == "MERGE_HEAD" {
				return Result{Code: 1}, &RunError{Args: args, Code: 1}
			}

			return Result{Stdout: []byte("abc\n")}, nil
		}
		if args[0] == "status" {
			return Result{}, nil
		}

		return Result{Code: 128}, &RunError{Args: args, Code: 128, Stderr: "fatal: refusing to merge unrelated histories"}
	})

	result, err := service.Merge(context.Background(), "otra")
	if err == nil {
		t.Fatal("Merge() error = nil, want the failure without conflicts")
	}
	if !strings.Contains(err.Error(), "unrelated histories") {
		t.Errorf("Merge() error = %q, want the technical stderr preserved", err)
	}
	if result.HasConflicts() {
		t.Errorf("result = %+v, want no conflicts", result)
	}
	if got := callWith(runner, "merge"); !slices.Equal(got, mergeArgs) {
		t.Errorf("args = %q, want %q", got, mergeArgs)
	}
}

func TestMergeAbortRequiresAnOpenMerge(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		return Result{Code: 1}, &RunError{Args: args, Code: 1}
	})

	if err := service.AbortMerge(context.Background()); !errors.Is(err, domain.ErrNoMergeInProgress) {
		t.Fatalf("AbortMerge() error = %v, want %v", err, domain.ErrNoMergeInProgress)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1", got)
	}
}

func TestMergeAbortRunsTheCommandAndIsNotRetried(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "rev-parse" {
			return Result{Stdout: []byte("abc\n")}, nil
		}

		return Result{Code: 128}, &RunError{
			Args: args, Code: 128, Stderr: "fatal: Unable to create '.git/index.lock': File exists.",
		}
	})

	err := service.AbortMerge(context.Background())
	if !errors.Is(err, domain.ErrGitIndexLocked) {
		t.Fatalf("AbortMerge() error = %v, want %v", err, domain.ErrGitIndexLocked)
	}

	calls := runner.Calls()
	if len(calls) != 2 {
		t.Fatalf("runner calls = %d, want the check and the abort only", len(calls))
	}
	if got, want := calls[1].Args, []string{"merge", "--abort"}; !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestMergeResolveStagesTheChosenSide(t *testing.T) {
	cases := []struct {
		name       string
		resolution domain.Resolution
		flag       string
	}{
		{name: "ours", resolution: domain.Ours, flag: "--ours"},
		{name: "theirs", resolution: domain.Theirs, flag: "--theirs"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, runner := writeService(t, ok)

			if err := service.ResolveConflict(context.Background(), "dir/conflict.txt", tc.resolution); err != nil {
				t.Fatalf("ResolveConflict() error = %v, want nil", err)
			}

			calls := runner.Calls()
			if len(calls) != 2 {
				t.Fatalf("runner calls = %d, want the checkout and the add", len(calls))
			}
			if got, want := calls[0].Args, []string{"checkout", tc.flag, "--", "dir/conflict.txt"}; !slices.Equal(got, want) {
				t.Errorf("checkout args = %q, want %q", got, want)
			}
			if got, want := calls[1].Args, []string{"add", "--all", "--", "dir/conflict.txt"}; !slices.Equal(got, want) {
				t.Errorf("add args = %q, want %q", got, want)
			}
		})
	}
}

func TestMergeResolveRejectsAnUnusableRequest(t *testing.T) {
	paths := []domain.RelPath{"", "../fuera.txt", "/etc/passwd", "dir//file.txt"}

	for _, path := range paths {
		t.Run("ruta "+string(path), func(t *testing.T) {
			service, runner := writeService(t, ok)

			if err := service.ResolveConflict(context.Background(), path, domain.Ours); err == nil {
				t.Fatalf("ResolveConflict(%q) error = nil, want a failure", path)
			}
			if got := runner.count(); got != 0 {
				t.Errorf("runner calls = %d, want 0", got)
			}
		})
	}

	t.Run("resolución desconocida", func(t *testing.T) {
		service, runner := writeService(t, ok)

		if err := service.ResolveConflict(context.Background(), "a.txt", domain.Resolution(9)); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("ResolveConflict(9) error = %v, want %v", err, domain.ErrInvalidArgument)
		}
		if got := runner.count(); got != 0 {
			t.Errorf("runner calls = %d, want 0", got)
		}
	})
}
