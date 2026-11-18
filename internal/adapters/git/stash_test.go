package git

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// statusArgs are the arguments of the status read the stash and merge paths use.
var statusArgs = []string{"status", "--porcelain=v2", "-z", "--untracked-files=no"}

func TestStashPushPreChecksTheWorktreeAndSendsTheMessage(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "status" {
			return Result{Stdout: []byte("1 .M N... 100644 100644 100644 a b cambio.txt\x00")}, nil
		}

		return Result{}, nil
	})

	if err := service.StashPush(context.Background(), "  trabajo a medias  "); err != nil {
		t.Fatalf("StashPush() error = %v, want nil", err)
	}

	calls := runner.Calls()
	if len(calls) != 2 {
		t.Fatalf("runner calls = %d, want the check and the push", len(calls))
	}
	if got := calls[0].Args; !slices.Equal(got, statusArgs) {
		t.Errorf("check args = %q, want %q", got, statusArgs)
	}
	if got, want := calls[1].Args, []string{"stash", "push", "-m", "trabajo a medias"}; !slices.Equal(got, want) {
		t.Errorf("push args = %q, want %q", got, want)
	}
}

func TestStashPushWithoutAMessage(t *testing.T) {
	for _, message := range []string{"", "   ", "\t"} {
		t.Run("mensaje "+message, func(t *testing.T) {
			service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
				if args[0] == "status" {
					return Result{Stdout: []byte("1 .M N... 100644 100644 100644 a b cambio.txt\x00")}, nil
				}

				return Result{}, nil
			})

			if err := service.StashPush(context.Background(), message); err != nil {
				t.Fatalf("StashPush(%q) error = %v, want nil", message, err)
			}

			want := []string{"stash", "push"}
			if got := callWith(runner, "stash"); !slices.Equal(got, want) {
				t.Errorf("args = %q, want %q", got, want)
			}
		})
	}
}

func TestStashPushWithoutChangesDoesNotRun(t *testing.T) {
	service, runner := writeService(t, ok)

	err := service.StashPush(context.Background(), "nada")
	if !errors.Is(err, domain.ErrNoChanges) {
		t.Fatalf("StashPush() error = %v, want %v", err, domain.ErrNoChanges)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1: only the pre-check runs", got)
	}
	if got := runner.last(); !slices.Equal(got, statusArgs) {
		t.Errorf("args = %q, want %q", got, statusArgs)
	}
}

func TestStashPushRejectsAnUnusableMessage(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    error
	}{
		{name: "NUL", message: "titulo\x00cola", want: domain.ErrInvalidArgument},
		{name: "salto de línea", message: "titulo\ncola", want: domain.ErrInvalidArgument},
		{name: "retorno de carro", message: "titulo\rcola", want: domain.ErrInvalidArgument},
		{name: "demasiado largo", message: strings.Repeat("x", domain.MaxCommitMessageBytes+1), want: domain.ErrTooLarge},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, runner := writeService(t, ok)

			if err := service.StashPush(context.Background(), tc.message); !errors.Is(err, tc.want) {
				t.Fatalf("StashPush() error = %v, want %v", err, tc.want)
			}
			if got := runner.count(); got != 0 {
				t.Errorf("runner calls = %d, want 0", got)
			}
		})
	}
}

func TestStashPushNeverTouchesUntrackedFiles(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "status" {
			return Result{Stdout: []byte("1 .M N... 100644 100644 100644 a b cambio.txt\x00")}, nil
		}

		return Result{}, nil
	})

	if err := service.StashPush(context.Background(), "wip"); err != nil {
		t.Fatalf("StashPush() error = %v, want nil", err)
	}

	for _, banned := range []string{"-u", "--include-untracked", "--all", "--keep-index", "--staged"} {
		if slices.Contains(callWith(runner, "stash"), banned) {
			t.Errorf("args = %q must never contain %q", callWith(runner, "stash"), banned)
		}
	}
}

func TestStashApplyPopAndDropUseThePosition(t *testing.T) {
	cases := []struct {
		name string
		run  func(*Service) error
		want []string
	}{
		{
			name: "apply",
			run:  func(s *Service) error { return s.StashApply(context.Background(), 2) },
			want: []string{"stash", "apply", "stash@{2}"},
		},
		{
			name: "pop",
			run:  func(s *Service) error { return s.StashPop(context.Background(), 0) },
			want: []string{"stash", "pop", "stash@{0}"},
		},
		{
			name: "drop",
			run:  func(s *Service) error { return s.StashDrop(context.Background(), 1) },
			want: []string{"stash", "drop", "stash@{1}"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, runner := writeService(t, ok)

			if err := tc.run(service); err != nil {
				t.Fatalf("%s error = %v, want nil", tc.name, err)
			}
			if got := callWith(runner, "stash"); !slices.Equal(got, tc.want) {
				t.Errorf("args = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStashPopIsNotApplyPlusDrop(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "status" {
			return Result{}, nil
		}

		return Result{}, nil
	})

	if err := service.StashPop(context.Background(), 0); err != nil {
		t.Fatalf("StashPop() error = %v, want nil", err)
	}

	stashes := 0
	for _, call := range runner.Calls() {
		if call.Args[0] == "stash" {
			stashes++
			if slices.Contains(call.Args, "apply") || slices.Contains(call.Args, "drop") {
				t.Errorf("args = %q, want the pop subcommand, not apply plus drop", call.Args)
			}
		}
	}
	if stashes != 1 {
		t.Errorf("stash calls = %d, want 1", stashes)
	}
}

func TestStashOperationsRejectAnIndexOutOfRange(t *testing.T) {
	for _, index := range []int{-1, -10, domain.MaxStashes, domain.MaxStashes + 5} {
		t.Run("índice", func(t *testing.T) {
			service, runner := writeService(t, ok)
			ctx := context.Background()

			if err := service.StashApply(ctx, index); !errors.Is(err, domain.ErrInvalidArgument) {
				t.Errorf("StashApply(%d) error = %v, want %v", index, err, domain.ErrInvalidArgument)
			}
			if err := service.StashPop(ctx, index); !errors.Is(err, domain.ErrInvalidArgument) {
				t.Errorf("StashPop(%d) error = %v, want %v", index, err, domain.ErrInvalidArgument)
			}
			if err := service.StashDrop(ctx, index); !errors.Is(err, domain.ErrInvalidArgument) {
				t.Errorf("StashDrop(%d) error = %v, want %v", index, err, domain.ErrInvalidArgument)
			}
			if got := runner.count(); got != 0 {
				t.Errorf("runner calls = %d, want 0", got)
			}
		})
	}
}

func TestStashRestoreReportsConflictsWithoutLosingTheEntry(t *testing.T) {
	cases := []struct {
		name string
		verb string
		run  func(*Service) error
	}{
		{name: "apply", verb: "apply", run: func(s *Service) error { return s.StashApply(context.Background(), 0) }},
		{name: "pop", verb: "pop", run: func(s *Service) error { return s.StashPop(context.Background(), 0) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			applied := false

			service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
				if args[0] != "stash" {
					if applied {
						return Result{Stdout: fixture(t, "status_v2_unmerged.z")}, nil
					}

					return Result{}, nil
				}

				applied = true

				// git exits non-zero when the restore leaves conflicts.
				return Result{Code: 1}, &RunError{Args: args, Code: 1, Stderr: "CONFLICT (content): Merge conflict in conflict.txt"}
			})

			err := tc.run(service)
			if !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("Stash%s() error = %v, want %v", tc.name, err, domain.ErrConflict)
			}
			if got := callWith(runner, "stash"); !slices.Equal(got, []string{"stash", tc.verb, "stash@{0}"}) {
				t.Errorf("args = %q, want the %s of stash@{0}", got, tc.verb)
			}
			if slices.Contains(callWith(runner, "stash"), "drop") {
				t.Error("a conflicted restore dropped the entry, want it kept")
			}
		})
	}
}

func TestStashRestoreWithoutConflictsReturnsNil(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "status" {
			return Result{}, nil
		}

		return Result{}, nil
	})

	if err := service.StashApply(context.Background(), 0); err != nil {
		t.Fatalf("StashApply() error = %v, want nil", err)
	}

	calls := runner.Calls()
	if len(calls) != 2 {
		t.Fatalf("runner calls = %d, want the apply and its check", len(calls))
	}
	if got := calls[1].Args; !slices.Equal(got, statusArgs) {
		t.Errorf("check args = %q, want %q", got, statusArgs)
	}
}
