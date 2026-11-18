package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// fakeRunner is the test double of Runner; it records the arguments, the stdin and the answers.
type fakeRunner struct {
	mu      sync.Mutex
	calls   [][]string
	inputs  [][]byte
	result  Result
	err     error
	handler func(stdin []byte, args []string) (Result, error)
}

func (f *fakeRunner) Run(_ context.Context, args ...string) (Result, error) {
	return f.invoke(nil, args)
}

func (f *fakeRunner) RunInput(_ context.Context, stdin []byte, args ...string) (Result, error) {
	return f.invoke(stdin, args)
}

func (f *fakeRunner) invoke(stdin []byte, args []string) (Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), args...))
	f.inputs = append(f.inputs, append([]byte(nil), stdin...))
	handler, result, err := f.handler, f.result, f.err
	f.mu.Unlock()

	if handler != nil {
		return handler(stdin, args)
	}

	return result, err
}

func (f *fakeRunner) last() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.calls) == 0 {
		return nil
	}

	return f.calls[len(f.calls)-1]
}

// recordedCall is one invocation with the arguments and the stdin it received.
type recordedCall struct {
	Args  []string
	Stdin []byte
}

func (f *fakeRunner) Calls() []recordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]recordedCall, 0, len(f.calls))
	for i, args := range f.calls {
		out = append(out, recordedCall{
			Args:  append([]string(nil), args...),
			Stdin: append([]byte(nil), f.inputs[i]...),
		})
	}

	return out
}

func (f *fakeRunner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.calls)
}

func (f *fakeRunner) lastInput() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.inputs) == 0 {
		return nil
	}

	return f.inputs[len(f.inputs)-1]
}

// newTestService builds a Service over a fake runner.
func newTestService(t *testing.T, runner Runner) *Service {
	t.Helper()

	root, err := domain.NewRepoPath(t.TempDir())
	if err != nil {
		t.Fatalf("NewRepoPath() error = %v, want nil", err)
	}

	service, err := New(root, runner)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	return service
}

func TestNewValidatesRootAndRunner(t *testing.T) {
	if _, err := New(domain.RepoPath{}, &fakeRunner{}); !errors.Is(err, domain.ErrInvalidPath) {
		t.Errorf("New(zero root) error = %v, want %v", err, domain.ErrInvalidPath)
	}

	root, err := domain.NewRepoPath(t.TempDir())
	if err != nil {
		t.Fatalf("NewRepoPath() error = %v, want nil", err)
	}
	if _, err := New(root, nil); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("New(nil runner) error = %v, want %v", err, domain.ErrInvalidArgument)
	}

	runner := &fakeRunner{}
	service, err := New(root, runner)
	if err != nil || service == nil {
		t.Fatalf("New() = %v, %v, want a service and nil", service, err)
	}
	if len(runner.calls) != 0 {
		t.Error("New() executed git, want no invocation")
	}
}

func TestServiceArguments(t *testing.T) {
	hash := strings.Repeat("a", 40)

	cases := []struct {
		name string
		run  func(t *testing.T, s *Service)
		want []string
	}{
		{
			name: "status",
			run:  func(t *testing.T, s *Service) { _, _ = s.Status(context.Background()) },
			want: []string{"status", "--porcelain=v2", "-z", "--branch", "--untracked-files=normal"},
		},
		{
			name: "diff worktree",
			run: func(t *testing.T, s *Service) {
				_, _ = s.Diff(context.Background(), domain.DiffRequest{Path: "dir/file.txt"})
			},
			want: []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--no-prefix", "--unified=3", "--", "dir/file.txt"},
		},
		{
			name: "diff staged without a path",
			run: func(t *testing.T, s *Service) {
				_, _ = s.Diff(context.Background(), domain.DiffRequest{Staged: true})
			},
			want: []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--no-prefix", "--unified=3", "--cached", "--"},
		},
		{
			name: "diffstat worktree",
			run:  func(t *testing.T, s *Service) { _, _ = s.DiffStat(context.Background(), false) },
			want: []string{"diff", "--numstat", "-z", "--no-color", "--no-ext-diff", "--no-textconv"},
		},
		{
			name: "diffstat staged",
			run:  func(t *testing.T, s *Service) { _, _ = s.DiffStat(context.Background(), true) },
			want: []string{"diff", "--numstat", "-z", "--no-color", "--no-ext-diff", "--no-textconv", "--cached"},
		},
		{
			name: "branches",
			run:  func(t *testing.T, s *Service) { _, _ = s.Branches(context.Background()) },
			want: []string{
				"for-each-ref", "--sort=-committerdate",
				"--format=%(refname:short)\t%(upstream:short)\t%(HEAD)\t%(upstream:track,nobracket)",
				"refs/heads",
			},
		},
		{
			name: "stashes",
			run:  func(t *testing.T, s *Service) { _, _ = s.Stashes(context.Background()) },
			want: []string{"stash", "list", "--format=%gd%x00%gs%x00%cI%x00"},
		},
		{
			name: "commits with ref skip and path",
			run: func(t *testing.T, s *Service) {
				_, _ = s.Commits(context.Background(), domain.CommitQuery{
					Ref: "main", Path: "dir/file.txt", Limit: 10, Skip: 5,
				})
			},
			want: []string{
				"log", "--no-color", "--no-show-signature", "-n", "11", "--skip=5",
				"--format=%H%x00%h%x00%an%x00%ae%x00%cI%x00%s%x00", "main", "--", "dir/file.txt",
			},
		},
		{
			name: "commits with defaults",
			run:  func(t *testing.T, s *Service) { _, _ = s.Commits(context.Background(), domain.CommitQuery{}) },
			want: []string{
				"log", "--no-color", "--no-show-signature", "-n", "51",
				"--format=%H%x00%h%x00%an%x00%ae%x00%cI%x00%s%x00",
			},
		},
		{
			name: "commits clamped to the maximum",
			run: func(t *testing.T, s *Service) {
				_, _ = s.Commits(context.Background(), domain.CommitQuery{Limit: domain.MaxCommits + 10, Skip: -1})
			},
			want: []string{
				"log", "--no-color", "--no-show-signature", "-n", "201",
				"--format=%H%x00%h%x00%an%x00%ae%x00%cI%x00%s%x00",
			},
		},
		{
			name: "show",
			run:  func(t *testing.T, s *Service) { _, _ = s.Show(context.Background(), hash) },
			want: []string{
				"show", "--no-color", "--no-ext-diff", "--no-textconv", "--no-prefix", "--unified=3",
				"--no-show-signature", "--format=%H%x00%h%x00%an%x00%ae%x00%cI%x00%s%x00%b%x00", hash, "--",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{}
			tc.run(t, newTestService(t, runner))

			if got := runner.last(); !slices.Equal(got, tc.want) {
				t.Errorf("args = %q\nwant   %q", got, tc.want)
			}
		})
	}
}

func TestServiceReadsFixturesThroughTheRunner(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		runner := &fakeRunner{result: Result{Stdout: fixture(t, "status_v2_changes.z")}}
		status, err := newTestService(t, runner).Status(context.Background())
		if err != nil {
			t.Fatalf("Status() error = %v, want nil", err)
		}
		if status.Branch != "main" || !status.HasChanges() {
			t.Errorf("Status() = %+v, want the parsed fixture", status)
		}
	})

	t.Run("diff", func(t *testing.T) {
		runner := &fakeRunner{result: Result{Stdout: fixture(t, "diff_basic.patch")}}
		diff, err := newTestService(t, runner).Diff(context.Background(), domain.DiffRequest{Path: "code.go", Staged: true})
		if err != nil {
			t.Fatalf("Diff() error = %v, want nil", err)
		}
		if diff.Path != "code.go" || !diff.Staged {
			t.Errorf("Diff() = %+v, want the fixture and the staged flag", diff)
		}
	})

	t.Run("diff without changes", func(t *testing.T) {
		runner := &fakeRunner{}
		diff, err := newTestService(t, runner).Diff(context.Background(), domain.DiffRequest{Path: "code.go"})
		if err != nil {
			t.Fatalf("Diff() error = %v, want nil", err)
		}
		if !diff.IsEmpty() || diff.Path != "code.go" {
			t.Errorf("Diff() = %+v, want an empty diff for the requested path", diff)
		}
	})

	t.Run("diffstat", func(t *testing.T) {
		runner := &fakeRunner{result: Result{Stdout: fixture(t, "numstat.z")}}
		stat, err := newTestService(t, runner).DiffStat(context.Background(), true)
		if err != nil {
			t.Fatalf("DiffStat() error = %v, want nil", err)
		}
		if len(stat.Files) != 8 || stat.Additions != 6 {
			t.Errorf("DiffStat() = %+v, want the parsed fixture", stat)
		}
	})

	t.Run("branches", func(t *testing.T) {
		runner := &fakeRunner{result: Result{Stdout: fixture(t, "refs.tsv")}}
		branches, err := newTestService(t, runner).Branches(context.Background())
		if err != nil {
			t.Fatalf("Branches() error = %v, want nil", err)
		}
		if len(branches) != 3 || !branches[0].Current {
			t.Errorf("Branches() = %+v, want the parsed fixture", branches)
		}
	})

	t.Run("stashes", func(t *testing.T) {
		runner := &fakeRunner{result: Result{Stdout: fixture(t, "stash.z")}}
		stashes, err := newTestService(t, runner).Stashes(context.Background())
		if err != nil {
			t.Fatalf("Stashes() error = %v, want nil", err)
		}
		if len(stashes) != 2 || stashes[0].Message != "trabajo a medias" {
			t.Errorf("Stashes() = %+v, want the parsed fixture", stashes)
		}
	})

	t.Run("commits", func(t *testing.T) {
		runner := &fakeRunner{result: Result{Stdout: fixture(t, "log.z")}}
		page, err := newTestService(t, runner).Commits(context.Background(), domain.CommitQuery{Limit: 3})
		if err != nil {
			t.Fatalf("Commits() error = %v, want nil", err)
		}
		if len(page.Commits) != 3 || !page.Truncated {
			t.Errorf("Commits() = %+v, want three commits and a truncation flag", page)
		}
	})

	t.Run("show", func(t *testing.T) {
		runner := &fakeRunner{result: Result{Stdout: fixture(t, "show_commit.txt")}}
		detail, err := newTestService(t, runner).Show(context.Background(), strings.Repeat("a", 40))
		if err != nil {
			t.Fatalf("Show() error = %v, want nil", err)
		}
		if len(detail.Files) != 2 || detail.Body == "" {
			t.Errorf("Show() = %+v, want the parsed fixture", detail)
		}
	})
}

func TestServiceTranslatesFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "not a repository", err: &RunError{Code: 128, Stderr: "fatal: not a git repository (or any of the parent directories): .git"}, want: domain.ErrNotRepository},
		{name: "not a repository from diff", err: &RunError{Code: 129, Stderr: "warning: Not a git repository. Use --no-index to compare two paths outside a working tree"}, want: domain.ErrNotRepository},
		{name: "locked index", err: &RunError{Code: 128, Stderr: "fatal: Unable to create '/repo/.git/index.lock': File exists."}, want: domain.ErrGitIndexLocked},
		{name: "permission", err: &RunError{Code: 128, Stderr: "fatal: cannot open '.git/objects': Permission denied"}, want: domain.ErrPermissionDenied},
		{name: "no remote", err: &RunError{Code: 128, Stderr: "fatal: no such remote 'origin'"}, want: domain.ErrNoRemote},
		{name: "missing binary", err: &RunError{Err: fmt.Errorf("git: %w", exec.ErrNotFound)}, want: domain.ErrGitUnavailable},
		{name: "deadline", err: &RunError{Err: context.DeadlineExceeded}, want: context.DeadlineExceeded},
		{name: "cancellation", err: &RunError{Err: context.Canceled}, want: context.Canceled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{err: tc.err}

			if _, err := newTestService(t, runner).Status(context.Background()); !errors.Is(err, tc.want) {
				t.Fatalf("Status() error = %v, want %v", err, tc.want)
			}
			if _, err := newTestService(t, runner).DiffStat(context.Background(), false); !errors.Is(err, tc.want) {
				t.Errorf("DiffStat() error = %v, want %v", err, tc.want)
			}
			if _, err := newTestService(t, runner).Branches(context.Background()); !errors.Is(err, tc.want) {
				t.Errorf("Branches() error = %v, want %v", err, tc.want)
			}
			if _, err := newTestService(t, runner).Stashes(context.Background()); !errors.Is(err, tc.want) {
				t.Errorf("Stashes() error = %v, want %v", err, tc.want)
			}
			if _, err := newTestService(t, runner).Show(context.Background(), strings.Repeat("a", 40)); !errors.Is(err, tc.want) {
				t.Errorf("Show() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestServiceKeepsUnknownFailuresWrapped(t *testing.T) {
	runner := &fakeRunner{err: &RunError{Args: []string{"status"}, Code: 1, Stderr: "algo raro"}}

	_, err := newTestService(t, runner).Status(context.Background())

	var runErr *RunError
	if !errors.As(err, &runErr) {
		t.Fatalf("Status() error = %v, want it to wrap the *RunError", err)
	}
	if !strings.Contains(err.Error(), "algo raro") {
		t.Errorf("Status() error = %q, want the technical stderr preserved for logs", err)
	}
}

func TestServiceKeepsAnEmptyHistoryOnAnUnbornBranch(t *testing.T) {
	cases := []string{
		"fatal: your current branch 'main' does not have any commits yet",
		"fatal: ambiguous argument 'main': unknown revision or path not in the working tree.",
		"fatal: bad revision 'HEAD'",
	}

	for _, stderr := range cases {
		t.Run(stderr, func(t *testing.T) {
			runner := &fakeRunner{err: &RunError{Args: []string{"log"}, Code: 128, Stderr: stderr}}

			page, err := newTestService(t, runner).Commits(context.Background(), domain.CommitQuery{})
			if err != nil {
				t.Fatalf("Commits() error = %v, want nil", err)
			}
			if len(page.Commits) != 0 || page.Truncated {
				t.Errorf("Commits() = %+v, want an empty page", page)
			}
		})
	}
}

func TestServiceRejectsInvalidArguments(t *testing.T) {
	runner := &fakeRunner{}
	service := newTestService(t, runner)
	hash := strings.Repeat("a", 40)

	refs := []string{"-x", "--all", "main..dev", "main@{1}", "con espacio", "salto\nlinea"}
	for _, ref := range refs {
		t.Run("ref "+ref, func(t *testing.T) {
			_, err := service.Commits(context.Background(), domain.CommitQuery{Ref: ref})
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("Commits(%q) error = %v, want %v", ref, err, domain.ErrInvalidArgument)
			}
			if len(runner.calls) != 0 {
				t.Error("an invalid ref reached git")
			}
		})
	}

	paths := []domain.RelPath{"../fuera.txt", "/etc/passwd", "dir/./file.txt", "dir//file.txt"}
	for _, path := range paths {
		t.Run("path "+string(path), func(t *testing.T) {
			if _, err := service.Commits(context.Background(), domain.CommitQuery{Path: path}); !errors.Is(err, domain.ErrInvalidPath) {
				t.Errorf("Commits(%q) error = %v, want %v", path, err, domain.ErrInvalidPath)
			}
			if _, err := service.Diff(context.Background(), domain.DiffRequest{Path: path}); !errors.Is(err, domain.ErrInvalidPath) {
				t.Errorf("Diff(%q) error = %v, want %v", path, err, domain.ErrInvalidPath)
			}
			if len(runner.calls) != 0 {
				t.Error("an invalid path reached git")
			}
		})
	}

	hashes := []string{"", "abc", strings.Repeat("g", 40), strings.Repeat("a", 39), strings.Repeat("a", 65), hash + " "}
	for _, value := range hashes {
		t.Run("hash "+value, func(t *testing.T) {
			if _, err := service.Show(context.Background(), value); !errors.Is(err, domain.ErrInvalidArgument) {
				t.Errorf("Show(%q) error = %v, want %v", value, err, domain.ErrInvalidArgument)
			}
			if len(runner.calls) != 0 {
				t.Error("an invalid hash reached git")
			}
		})
	}

	if _, err := service.Show(context.Background(), hash); err != nil {
		t.Errorf("Show(valid) error = %v, want nil", err)
	}
	if _, err := service.Show(context.Background(), strings.Repeat("A", 64)); err != nil {
		t.Errorf("Show(uppercase sha256) error = %v, want nil", err)
	}
}

func TestServiceWrapsErrorsThatAreNotRunErrors(t *testing.T) {
	runner := &fakeRunner{err: errors.New("ruido interno")}

	_, err := newTestService(t, runner).Status(context.Background())
	if err == nil {
		t.Fatal("Status() error = nil, want the runner failure")
	}
	for _, want := range []string{"git: Status:", "ruido interno"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Status() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestServicePropagatesRunnerTruncation(t *testing.T) {
	runner := &fakeRunner{result: Result{Truncated: true}}
	service := newTestService(t, runner)
	ctx := context.Background()

	status, err := service.Status(ctx)
	if err != nil || !status.Truncated {
		t.Errorf("Status() = %+v, %v, want truncated and nil", status, err)
	}

	diff, err := service.Diff(ctx, domain.DiffRequest{Path: "file.txt"})
	if err != nil || !diff.Truncated {
		t.Errorf("Diff() = %+v, %v, want truncated and nil", diff, err)
	}

	stat, err := service.DiffStat(ctx, false)
	if err != nil || !stat.Truncated {
		t.Errorf("DiffStat() = %+v, %v, want truncated and nil", stat, err)
	}

	page, err := service.Commits(ctx, domain.CommitQuery{})
	if err != nil || !page.Truncated {
		t.Errorf("Commits() = %+v, %v, want truncated and nil", page, err)
	}

	detail, err := service.Show(ctx, strings.Repeat("a", 40))
	if err != nil || !detail.Truncated {
		t.Errorf("Show() = %+v, %v, want truncated and nil", detail, err)
	}
}
