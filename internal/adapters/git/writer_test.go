package git

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// writeService pairs the service under test with the runner that records every call.
func writeService(t *testing.T, handler func(stdin []byte, args []string) (Result, error)) (*Service, *fakeRunner) {
	t.Helper()

	runner := &fakeRunner{handler: handler}
	service := newTestService(t, runner)
	service.lockDelay = 0

	return service, runner
}

// ok answers every invocation with a clean exit.
func ok([]byte, []string) (Result, error) {
	return Result{}, nil
}

func TestStageRunsAddWithSeparatedArguments(t *testing.T) {
	service, runner := writeService(t, ok)

	if err := service.Stage(context.Background(), []domain.RelPath{"a.go", "dir/b b.txt"}); err != nil {
		t.Fatalf("Stage() error = %v, want nil", err)
	}

	want := []string{"add", "--", "a.go", "dir/b b.txt"}
	if got := runner.last(); !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1", got)
	}
}

func TestUnstageUsesRestoreWithACommit(t *testing.T) {
	service, runner := writeService(t, ok)

	if err := service.Unstage(context.Background(), []domain.RelPath{"a.go"}); err != nil {
		t.Fatalf("Unstage() error = %v, want nil", err)
	}

	calls := runner.Calls()
	if got, want := len(calls), 2; got != want {
		t.Fatalf("runner calls = %d, want %d", got, want)
	}
	if got, want := calls[0].Args, []string{"rev-parse", "--verify", "--quiet", "HEAD"}; !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
	if got, want := calls[1].Args, []string{"restore", "--staged", "--", "a.go"}; !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestUnstageFallsBackToRmCachedOnAnUnbornBranch(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "rev-parse" {
			return Result{Code: 1}, &RunError{Args: args, Code: 1}
		}

		return Result{}, nil
	})

	if err := service.Unstage(context.Background(), []domain.RelPath{"a.go"}); err != nil {
		t.Fatalf("Unstage() error = %v, want nil", err)
	}

	calls := runner.Calls()
	if got, want := len(calls), 2; got != want {
		t.Fatalf("runner calls = %d, want %d", got, want)
	}
	if got, want := calls[1].Args, []string{"rm", "--cached", "--quiet", "--", "a.go"}; !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestUnstageReportsAHeadCheckThatIsNotMissing(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "not a repository",
			err:  &RunError{Code: 128, Stderr: "fatal: not a git repository (or any of the parent directories): .git"},
			want: domain.ErrNotRepository,
		},
		{name: "cancelled", err: &RunError{Err: context.Canceled}, want: context.Canceled},
		{
			name: "permission",
			err:  &RunError{Code: 128, Stderr: "fatal: cannot open '.git/index': Permission denied"},
			want: domain.ErrPermissionDenied,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
				return Result{}, tc.err
			})

			err := service.Unstage(context.Background(), []domain.RelPath{"a.go"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Unstage() error = %v, want %v", err, tc.want)
			}
			if got := runner.count(); got != 1 {
				t.Errorf("runner calls = %d, want 1: the write must not run after a failed check", got)
			}
		})
	}
}

func TestStageHunksAndUnstageHunksPipeThePatch(t *testing.T) {
	cases := []struct {
		name string
		want []string
		run  func(*Service) error
	}{
		{
			name: "stage",
			want: []string{"apply", "--cached", "-p1", "--whitespace=nowarn", "-"},
			run: func(s *Service) error {
				return s.StageHunks(context.Background(), hunkDiff(), []int{0})
			},
		},
		{
			name: "unstage",
			want: []string{"apply", "--cached", "--reverse", "-p1", "--whitespace=nowarn", "-"},
			run: func(s *Service) error {
				return s.UnstageHunks(context.Background(), hunkDiff(), []int{0})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, runner := writeService(t, ok)

			if err := tc.run(service); err != nil {
				t.Fatalf("%s error = %v, want nil", tc.name, err)
			}
			if got := runner.last(); !slices.Equal(got, tc.want) {
				t.Errorf("args = %q, want %q", got, tc.want)
			}

			payload := runner.lastInput()
			if len(payload) == 0 {
				t.Fatal("stdin = empty, want the patch")
			}
			if want := string(mustPatch(t, hunkDiff(), []int{0})); string(payload) != want {
				t.Errorf("stdin =\n%q\nwant\n%q", payload, want)
			}
		})
	}
}

func TestStageHunksRejectsAnUnusableDiff(t *testing.T) {
	service, runner := writeService(t, ok)

	if err := service.StageHunks(context.Background(), domain.Diff{Path: "a.go"}, []int{0}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("StageHunks() error = %v, want %v", err, domain.ErrInvalidArgument)
	}
	if err := service.UnstageHunks(context.Background(), hunkDiff(), nil); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("UnstageHunks() error = %v, want %v", err, domain.ErrInvalidArgument)
	}
	if got := runner.count(); got != 0 {
		t.Errorf("runner calls = %d, want 0: an unusable diff never reaches git", got)
	}
}

func TestStageAndUnstageRejectBadPaths(t *testing.T) {
	cases := []struct {
		name  string
		paths []domain.RelPath
		want  error
	}{
		{name: "no paths", paths: nil, want: domain.ErrInvalidArgument},
		{name: "empty list", paths: []domain.RelPath{}, want: domain.ErrInvalidArgument},
		{name: "zero path", paths: []domain.RelPath{""}, want: domain.ErrInvalidArgument},
		{name: "root", paths: []domain.RelPath{"a.go", "."}, want: domain.ErrInvalidPath},
		{name: "escape", paths: []domain.RelPath{"../fuera.txt"}, want: domain.ErrInvalidPath},
		{name: "absolute", paths: []domain.RelPath{"/etc/passwd"}, want: domain.ErrInvalidPath},
		{name: "unnormalised", paths: []domain.RelPath{"dir//file.txt"}, want: domain.ErrInvalidPath},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, runner := writeService(t, ok)

			if err := service.Stage(context.Background(), tc.paths); !errors.Is(err, tc.want) {
				t.Errorf("Stage() error = %v, want %v", err, tc.want)
			}
			if err := service.Unstage(context.Background(), tc.paths); !errors.Is(err, tc.want) {
				t.Errorf("Unstage() error = %v, want %v", err, tc.want)
			}
			if got := runner.count(); got != 0 {
				t.Errorf("runner calls = %d, want 0: an invalid path never reaches git", got)
			}
		})
	}
}

func TestCommitChecksTheIndexAndPipesTheMessage(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "diff" {
			return Result{Code: 1}, &RunError{Args: args, Code: 1}
		}

		return Result{}, nil
	})

	if err := service.Commit(context.Background(), "titulo\r\n\r\ncuerpo\r\n\r\n"); err != nil {
		t.Fatalf("Commit() error = %v, want nil", err)
	}

	calls := runner.Calls()
	if got, want := len(calls), 2; got != want {
		t.Fatalf("runner calls = %d, want %d", got, want)
	}
	if got, want := calls[0].Args, []string{"diff", "--cached", "--quiet", "--no-ext-diff"}; !slices.Equal(got, want) {
		t.Errorf("check args = %q, want %q", got, want)
	}
	if got, want := calls[1].Args,
		[]string{"commit", "--quiet", "--cleanup=whitespace", "--no-status", "-F", "-"}; !slices.Equal(got, want) {
		t.Errorf("commit args = %q, want %q", got, want)
	}
	if got, want := string(runner.lastInput()), "titulo\n\ncuerpo\n"; got != want {
		t.Errorf("stdin = %q, want %q", got, want)
	}
	for _, banned := range []string{"-m", "-a", "-A", "--no-verify", "--allow-empty", "--amend", "--hard"} {
		if slices.Contains(calls[1].Args, banned) {
			t.Errorf("commit args = %q must never contain %q", calls[1].Args, banned)
		}
	}
}

func TestCommitWithoutStagedChangesDoesNotRun(t *testing.T) {
	service, runner := writeService(t, ok)

	err := service.Commit(context.Background(), "titulo")
	if !errors.Is(err, domain.ErrNoChanges) {
		t.Fatalf("Commit() error = %v, want %v", err, domain.ErrNoChanges)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1: only the check runs", got)
	}
	if got := runner.last(); got[0] != "diff" {
		t.Errorf("args = %q, want the pre-check only", got)
	}
}

func TestCommitRejectsAnEmptyMessageBeforeGit(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    error
	}{
		{name: "empty", message: "", want: domain.ErrInvalidArgument},
		{name: "spaces", message: "   \n", want: domain.ErrInvalidArgument},
		{name: "nul", message: "titulo\x00", want: domain.ErrInvalidArgument},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, runner := writeService(t, ok)

			if err := service.Commit(context.Background(), tc.message); !errors.Is(err, tc.want) {
				t.Errorf("Commit() error = %v, want %v", err, tc.want)
			}
			if got := runner.count(); got != 0 {
				t.Errorf("runner calls = %d, want 0", got)
			}
		})
	}
}

func TestCommitKeepsAFailedHookAsAFailure(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "diff" {
			return Result{Code: 1}, &RunError{Args: args, Code: 1}
		}

		return Result{Code: 1}, &RunError{Args: args, Code: 1, Stderr: "pre-commit hook falló"}
	})

	err := service.Commit(context.Background(), "titulo")
	if err == nil {
		t.Fatal("Commit() error = nil, want the hook failure")
	}
	for _, want := range []string{"Commit", "pre-commit hook falló"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Commit() error = %q, want it to contain %q", err, want)
		}
	}
	if got := runner.count(); got != 2 {
		t.Errorf("runner calls = %d, want 2: a failed hook is never retried", got)
	}
}

func TestWriteRetriesOnceOnALockedIndex(t *testing.T) {
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

	if err := service.Stage(context.Background(), []domain.RelPath{"a.go"}); err != nil {
		t.Fatalf("Stage() error = %v, want the retry to succeed", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
	if got := runner.count(); got != 2 {
		t.Errorf("runner calls = %d, want 2", got)
	}
}

func TestWriteReturnsTheLockErrorAfterASingleRetry(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		return Result{Code: 128}, &RunError{
			Args: args, Code: 128, Stderr: "fatal: Unable to create '.git/index.lock': File exists.",
		}
	})

	err := service.Stage(context.Background(), []domain.RelPath{"a.go"})
	if !errors.Is(err, domain.ErrGitIndexLocked) {
		t.Fatalf("Stage() error = %v, want %v", err, domain.ErrGitIndexLocked)
	}
	if got := runner.count(); got != 2 {
		t.Errorf("runner calls = %d, want exactly 2: one retry, no more", got)
	}
}

func TestWriteDoesNotRetryOtherFailures(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		return Result{Code: 128}, &RunError{Args: args, Code: 128, Stderr: "fatal: not a git repository"}
	})

	err := service.Stage(context.Background(), []domain.RelPath{"a.go"})
	if !errors.Is(err, domain.ErrNotRepository) {
		t.Fatalf("Stage() error = %v, want %v", err, domain.ErrNotRepository)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1", got)
	}
}

func TestWriteGivesUpWhenTheContextExpiresDuringTheBackoff(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		return Result{Code: 128}, &RunError{
			Args: args, Code: 128, Stderr: "fatal: Unable to create '.git/index.lock': File exists.",
		}
	})
	service.lockDelay = 200 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := service.Stage(ctx, []domain.RelPath{"a.go"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stage() error = %v, want %v", err, context.DeadlineExceeded)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1: the retry never ran", got)
	}
}

func TestWritesAreSerialised(t *testing.T) {
	var (
		mu       sync.Mutex
		active   int
		observed int
	)
	service, runner := writeService(t, func(_ []byte, _ []string) (Result, error) {
		mu.Lock()
		active++
		observed = max(observed, active)
		mu.Unlock()

		time.Sleep(time.Millisecond)

		mu.Lock()
		active--
		mu.Unlock()

		return Result{}, nil
	})

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			path := domain.RelPath(fmt.Sprintf("file-%d.txt", i))
			if err := service.Stage(context.Background(), []domain.RelPath{path}); err != nil {
				t.Errorf("Stage(%q) error = %v, want nil", path, err)
			}
		}()
	}
	wg.Wait()

	if observed != 1 {
		t.Errorf("observed %d concurrent writes, want 1: the adapter must serialise them", observed)
	}
	if got := runner.count(); got != 8 {
		t.Errorf("runner calls = %d, want 8", got)
	}
}

func TestWriteWaitingForTheTurnHonoursCancellation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})

	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "add" {
			close(entered)
			<-release
		}

		return Result{}, nil
	})

	held := make(chan error, 1)
	go func() { held <- service.Stage(context.Background(), []domain.RelPath{"a.go"}) }()
	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := service.Stage(ctx, []domain.RelPath{"b.go"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stage() error = %v, want %v", err, context.DeadlineExceeded)
	}

	close(release)
	if err := <-held; err != nil {
		t.Errorf("the first write error = %v, want nil", err)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1: the cancelled write never reached git", got)
	}
}

func TestReadsAreNotSerialisedWithWrites(t *testing.T) {
	service, runner := writeService(t, ok)

	if _, err := service.Status(context.Background()); err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if got := runner.last(); got[0] != "status" {
		t.Errorf("args = %q, want the read subcommand", got)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1", got)
	}
}

func mustPatch(t *testing.T, diff domain.Diff, hunks []int) []byte {
	t.Helper()

	patch, err := buildPatch(diff, hunks)
	if err != nil {
		t.Fatalf("buildPatch() error = %v, want nil", err)
	}

	return patch
}
