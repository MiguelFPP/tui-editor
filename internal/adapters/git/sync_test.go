package git

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// ctxRunner answers every call with an empty success and records the deadline it received.
type ctxRunner struct {
	deadlines []time.Time
	without   int
}

func (r *ctxRunner) record(ctx context.Context) {
	deadline, ok := ctx.Deadline()
	if !ok {
		r.without++

		return
	}

	r.deadlines = append(r.deadlines, deadline)
}

func (r *ctxRunner) Run(ctx context.Context, _ ...string) (Result, error) {
	r.record(ctx)

	return Result{}, nil
}

func (r *ctxRunner) RunInput(ctx context.Context, _ []byte, _ ...string) (Result, error) {
	r.record(ctx)

	return Result{}, nil
}

func TestSyncNetworkBudgetIsTwoMinutes(t *testing.T) {
	if NetworkTimeout != 2*time.Minute {
		t.Errorf("NetworkTimeout = %v, want 2m", NetworkTimeout)
	}
}

func TestSyncFetchPrunesAndStaysQuiet(t *testing.T) {
	service, runner := writeService(t, ok)

	if err := service.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}

	want := []string{"fetch", "--prune", "--quiet"}
	if got := runner.last(); !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1", got)
	}
}

func TestSyncPushChecksTheUpstreamFirst(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		return Result{Code: 1}, &RunError{Args: args, Code: 1}
	})

	if err := service.Push(context.Background()); !errors.Is(err, domain.ErrNoUpstream) {
		t.Fatalf("Push() error = %v, want %v", err, domain.ErrNoUpstream)
	}

	calls := runner.Calls()
	if len(calls) != 1 {
		t.Fatalf("runner calls = %d, want 1: a branch without upstream never pushes", len(calls))
	}
	if got, want := calls[0].Args, []string{"rev-parse", "--verify", "--quiet", "@{upstream}"}; !slices.Equal(got, want) {
		t.Errorf("check args = %q, want %q", got, want)
	}
}

func TestSyncPushRunsQuiet(t *testing.T) {
	service, runner := writeService(t, ok)

	if err := service.Push(context.Background()); err != nil {
		t.Fatalf("Push() error = %v, want nil", err)
	}

	if got, want := callWith(runner, "push"), []string{"push", "--quiet"}; !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestSyncPullRequiresAnUpstream(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		return Result{Code: 1}, &RunError{Args: args, Code: 1}
	})

	if _, err := service.Pull(context.Background()); !errors.Is(err, domain.ErrNoUpstream) {
		t.Fatalf("Pull() error = %v, want %v", err, domain.ErrNoUpstream)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1", got)
	}
}

func TestSyncPullClassifiesTheOutcome(t *testing.T) {
	cases := []struct {
		name          string
		before, after string
		target        string
		want          domain.MergeStatus
	}{
		{name: "al día", before: "aaa", after: "aaa", target: "aaa", want: domain.UpToDate},
		{name: "avance rápido", before: "aaa", after: "bbb", target: "bbb", want: domain.FastForward},
		{name: "commit de fusión", before: "aaa", after: "ccc", target: "bbb", want: domain.Clean},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			heads := []string{tc.before, tc.after}
			headIndex := 0

			service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
				if args[0] != "rev-parse" {
					return Result{}, nil
				}
				if args[len(args)-1] == "HEAD" {
					id := heads[min(headIndex, len(heads)-1)]
					headIndex++

					return Result{Stdout: []byte(id + "\n")}, nil
				}

				return Result{Stdout: []byte(tc.target + "\n")}, nil
			})

			result, err := service.Pull(context.Background())
			if err != nil {
				t.Fatalf("Pull() error = %v, want nil", err)
			}
			if result.Status != tc.want {
				t.Errorf("Status = %d, want %d", result.Status, tc.want)
			}
			if want := []string{"pull", "--no-rebase", "--no-edit"}; !slices.Equal(callWith(runner, "pull"), want) {
				t.Errorf("args = %q, want %q", callWith(runner, "pull"), want)
			}
		})
	}
}

func TestSyncPullReportsConflictsWithoutAnError(t *testing.T) {
	service, _ := writeService(t, func(_ []byte, args []string) (Result, error) {
		switch {
		case args[0] == "rev-parse":
			return Result{Stdout: []byte("abc\n")}, nil
		case args[0] == "status":
			return Result{Stdout: fixture(t, "status_v2_unmerged.z")}, nil
		}

		return Result{Code: 1}, &RunError{
			Args: args, Code: 1, Stderr: "CONFLICT (content): Merge conflict in conflict.txt",
		}
	})

	result, err := service.Pull(context.Background())
	if err != nil {
		t.Fatalf("Pull() error = %v, want nil: a conflict is an outcome, not a failure", err)
	}
	if result.Status != domain.Conflicted || len(result.Conflicts) != 1 {
		t.Errorf("result = %+v, want one conflict", result)
	}
}

func TestSyncTranslatesNetworkFailures(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   error
	}{
		{
			name:   "non-fast-forward",
			stderr: "! [rejected]        main -> main (non-fast-forward)\nerror: failed to push some refs to 'origin'",
			want:   domain.ErrNonFastForward,
		},
		{
			name:   "fetch first",
			stderr: "hint: Updates were rejected because the remote contains work that you do not have\nfatal: fetch first",
			want:   domain.ErrNonFastForward,
		},
		{
			name:   "auth de ssh",
			stderr: "git@github.com: Permission denied (publickey).\nfatal: Could not read from remote repository.",
			want:   domain.ErrAuthRequired,
		},
		{
			name:   "auth de https",
			stderr: "fatal: Authentication failed for 'https://github.com/user/repo.git/'",
			want:   domain.ErrAuthRequired,
		},
		{
			name:   "prompts desactivados",
			stderr: "fatal: could not read Username for 'https://github.com': terminal prompts disabled",
			want:   domain.ErrAuthRequired,
		},
		{
			name:   "host desconocido",
			stderr: "fatal: unable to access 'https://github.com/user/repo.git/': Could not resolve host: github.com",
			want:   domain.ErrRemoteUnreachable,
		},
		{
			name:   "ssh inalcanzable",
			stderr: "ssh: connect to host github.com port 22: Operation timed out\nfatal: Could not read from remote repository.",
			want:   domain.ErrRemoteUnreachable,
		},
		{
			name:   "conexión rechazada",
			stderr: "ssh: connect to host localhost port 22: Connection refused",
			want:   domain.ErrRemoteUnreachable,
		},
		{
			name:   "sin upstream",
			stderr: "fatal: The current branch main has no upstream branch.\nTo push the current branch and set the remote as upstream, use\n\n    git push --set-upstream origin main\n",
			want:   domain.ErrNoUpstream,
		},
		{
			name:   "sin tracking",
			stderr: "There is no tracking information for the current branch.\nPlease specify which branch you want to merge with.",
			want:   domain.ErrNoUpstream,
		},
		{
			name:   "árbol sucio",
			stderr: "error: Your local changes to the following files would be overwritten by merge:\n\ta.txt\nPlease commit your changes or stash them before you merge.",
			want:   domain.ErrDirtyWorktree,
		},
		{
			name:   "merge sin concluir",
			stderr: "fatal: You have not concluded your merge (MERGE_HEAD exists).",
			want:   domain.ErrMergeInProgress,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{err: &RunError{Args: []string{"fetch"}, Code: 1, Stderr: tc.stderr}}
			service := newTestService(t, runner)

			if err := service.Fetch(context.Background()); !errors.Is(err, tc.want) {
				t.Fatalf("Fetch() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSyncDoesNotRetryANetworkFailure(t *testing.T) {
	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		return Result{Code: 128}, &RunError{
			Args: args, Code: 128, Stderr: "fatal: Unable to create '.git/index.lock': File exists.",
		}
	})

	if err := service.Fetch(context.Background()); !errors.Is(err, domain.ErrGitIndexLocked) {
		t.Fatalf("Fetch() error = %v, want %v", err, domain.ErrGitIndexLocked)
	}
	if got := runner.count(); got != 1 {
		t.Errorf("runner calls = %d, want 1: a network write is never retried", got)
	}
}

func TestSyncGivesTheNetworkABudgetUnlessTheCallerSetsOne(t *testing.T) {
	t.Run("sin deadline del llamante", func(t *testing.T) {
		runner := &ctxRunner{}
		service := newTestService(t, runner)

		if err := service.Fetch(context.Background()); err != nil {
			t.Fatalf("Fetch() error = %v, want nil", err)
		}
		if runner.without != 0 {
			t.Fatalf("calls without a deadline = %d, want none", runner.without)
		}
		if len(runner.deadlines) == 0 {
			t.Fatal("no call received a deadline, want the network budget")
		}

		remaining := time.Until(runner.deadlines[0])
		if remaining <= 0 || remaining > NetworkTimeout {
			t.Errorf("remaining budget = %v, want it inside %v", remaining, NetworkTimeout)
		}
	})

	t.Run("con deadline del llamante", func(t *testing.T) {
		runner := &ctxRunner{}
		service := newTestService(t, runner)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		want, ok := ctx.Deadline()
		if !ok {
			t.Fatal("the caller context has no deadline")
		}

		if err := service.Fetch(ctx); err != nil {
			t.Fatalf("Fetch() error = %v, want nil", err)
		}
		if runner.without != 0 {
			t.Fatalf("calls without a deadline = %d, want none", runner.without)
		}
		for i, deadline := range runner.deadlines {
			if !deadline.Equal(want) {
				t.Errorf("call %d deadline = %v, want the caller deadline %v", i, deadline, want)
			}
		}
	})
}

func TestSyncNeverForcesOrOverridesTheRemote(t *testing.T) {
	banned := []string{"-f", "--force", "--all", "--mirror", "--tags", "--no-verify", "--include-untracked", "-D", "--set-upstream"}

	service, runner := writeService(t, func(_ []byte, args []string) (Result, error) {
		if args[0] == "rev-parse" {
			return Result{Stdout: []byte("abc\n")}, nil
		}

		return Result{}, nil
	})
	ctx := context.Background()

	if err := service.Fetch(ctx); err != nil {
		t.Fatalf("Fetch() error = %v, want nil", err)
	}
	if err := service.Push(ctx); err != nil {
		t.Fatalf("Push() error = %v, want nil", err)
	}
	if _, err := service.Pull(ctx); err != nil {
		t.Fatalf("Pull() error = %v, want nil", err)
	}

	for _, call := range runner.Calls() {
		switch call.Args[0] {
		case "fetch", "push", "pull":
		default:
			continue
		}

		for _, flag := range banned {
			if slices.Contains(call.Args, flag) {
				t.Errorf("args = %q must never contain %q", call.Args, flag)
			}
		}
	}
}
