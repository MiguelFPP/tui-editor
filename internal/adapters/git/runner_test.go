package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// helperEnv turns the test binary into a fake git process; TestMain routes on it.
const helperEnv = "TUI_EDITOR_GIT_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		os.Exit(helperProcess(mode))
	}

	os.Exit(m.Run())
}

// helperProcess emulates the behaviours the runner must survive without needing a real git.
func helperProcess(mode string) int {
	switch mode {
	case "args":
		fmt.Println(strings.Join(os.Args[1:], "\n"))

		return 0
	case "env":
		for _, entry := range os.Environ() {
			fmt.Println(entry)
		}

		return 0
	case "fail":
		fmt.Fprintln(os.Stderr, "fatal: not a git repository (or any of the parent directories): .git")

		return 128
	case "flood":
		for range 64 {
			fmt.Print(strings.Repeat("x", 1024))
		}

		return 0
	case "flooderr":
		for range 64 {
			fmt.Fprint(os.Stderr, strings.Repeat("e", 1024))
		}

		return 0
	case "wait":
		time.Sleep(time.Minute)

		return 0
	case "cat":
		if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)

			return 1
		}

		return 0
	case "drop":
		return 0 // exits without reading stdin, so the parent copy hits a broken pipe
	default:
		fmt.Fprintf(os.Stderr, "modo de ayuda desconocido: %s\n", mode)

		return 2
	}
}

// helperRunner builds an ExecRunner that runs the test binary instead of git.
func helperRunner(t *testing.T, mode string) *ExecRunner {
	t.Helper()

	t.Setenv(helperEnv, mode)

	root, err := domain.NewRepoPath(t.TempDir())
	if err != nil {
		t.Fatalf("NewRepoPath() error = %v, want nil", err)
	}

	runner := NewExecRunner(root)
	runner.Git = os.Args[0]

	return runner
}

func TestExecRunnerBuildsTheExactGlobalPrefix(t *testing.T) {
	runner := helperRunner(t, "args")
	root := runner.Root.String()

	result, err := runner.Run(context.Background(), "status", "--porcelain=v2", "-z")
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if result.Code != 0 || result.Truncated {
		t.Errorf("result = %+v, want a clean exit", result)
	}

	args := strings.Split(strings.TrimSpace(string(result.Stdout)), "\n")
	want := []string{
		"-C", root,
		"--no-pager",
		"-c", "core.quotepath=false",
		"-c", "core.fsmonitor=false",
		"-c", "color.ui=false",
		"-c", "safe.directory=" + root,
		"status", "--porcelain=v2", "-z",
	}
	if !slices.Equal(args, want) {
		t.Errorf("argv = %q\nwant   %q", args, want)
	}
	for _, arg := range args {
		if strings.Contains(arg, "safe.directory=*") {
			t.Fatalf("argv = %q must never allow every directory", args)
		}
	}
}

func TestExecRunnerFiltersTheEnvironment(t *testing.T) {
	for _, name := range []string{
		"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE", "GIT_CEILING_DIRECTORIES", "GIT_COMMON_DIR",
	} {
		t.Setenv(name, "/tmp/otro-repositorio")
	}
	t.Setenv("GIT_OPTIONAL_LOCKS", "1")

	runner := helperRunner(t, "env")

	result, err := runner.Run(context.Background(), "status")
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	env := strings.Split(string(result.Stdout), "\n")
	for _, name := range []string{
		"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE", "GIT_CEILING_DIRECTORIES", "GIT_COMMON_DIR",
	} {
		if strings.Contains(string(result.Stdout), name+"=") {
			t.Errorf("the environment kept %s: %v", name, env)
		}
	}

	for _, want := range []string{
		"GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat",
		"PAGER=cat", "LC_ALL=C", "LANG=C", "LC_MESSAGES=C",
	} {
		if got := countEntries(env, want); got != 1 {
			t.Errorf("the environment has %d entries for %q, want exactly 1: the parent value must not survive", got, want)
		}
	}
}

func TestExecRunnerDefaultsTheSSHCommandToBatchMode(t *testing.T) {
	t.Run("sin configurar", func(t *testing.T) {
		runner := helperRunner(t, "env")

		result, err := runner.Run(context.Background(), "fetch")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}

		env := strings.Split(string(result.Stdout), "\n")
		if got := countEntries(env, "GIT_SSH_COMMAND="+sshNoPrompt); got != 1 {
			t.Errorf("the environment has %d batch-mode entries, want exactly 1", got)
		}
	})

	t.Run("configurado por el usuario", func(t *testing.T) {
		t.Setenv("GIT_SSH_COMMAND", "ssh -i /tmp/clave")

		runner := helperRunner(t, "env")

		result, err := runner.Run(context.Background(), "fetch")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}

		env := strings.Split(string(result.Stdout), "\n")
		if got := countEntries(env, "GIT_SSH_COMMAND=ssh -i /tmp/clave"); got != 1 {
			t.Errorf("the environment has %d entries for the user command, want exactly 1", got)
		}
		if got := countEntries(env, "GIT_SSH_COMMAND="+sshNoPrompt); got != 0 {
			t.Errorf("the environment has %d batch-mode entries, want none: the user command wins", got)
		}
	})
}

func TestExecRunnerReportsExitStatusAndStderr(t *testing.T) {
	runner := helperRunner(t, "fail")

	result, err := runner.Run(context.Background(), "status")

	var runErr *RunError
	if !errors.As(err, &runErr) {
		t.Fatalf("Run() error = %v, want *RunError", err)
	}
	if runErr.Code != 128 {
		t.Errorf("Code = %d, want 128", runErr.Code)
	}
	if !strings.Contains(runErr.Stderr, "not a git repository") {
		t.Errorf("Stderr = %q, want the git message", runErr.Stderr)
	}
	if !strings.Contains(runErr.Error(), "salida 128") {
		t.Errorf("Error() = %q, want the exit status", runErr.Error())
	}
	if result.Code != 128 {
		t.Errorf("Result.Code = %d, want 128", result.Code)
	}
	if runErr.Err != nil {
		t.Errorf("RunError.Err = %v, want nil: the exit status is not an error chain", runErr.Err)
	}
	if len(runErr.Args) != 1 || runErr.Args[0] != "status" {
		t.Errorf("Args = %q, want the subcommand only", runErr.Args)
	}
}

func TestExecRunnerMissingBinary(t *testing.T) {
	root, err := domain.NewRepoPath(t.TempDir())
	if err != nil {
		t.Fatalf("NewRepoPath() error = %v, want nil", err)
	}

	runner := NewExecRunner(root)
	runner.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }

	_, err = runner.Run(context.Background(), "status")

	var runErr *RunError
	if !errors.As(err, &runErr) {
		t.Fatalf("Run() error = %v, want *RunError", err)
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("Run() error = %v, want it to wrap %v", err, exec.ErrNotFound)
	}
	if !strings.Contains(runErr.Error(), "executable file not found") {
		t.Errorf("Error() = %q, want the lookup failure", runErr.Error())
	}

	runner.LookPath = func(string) (string, error) { return "", nil }
	if _, err := runner.Run(context.Background(), "status"); !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("Run() with an empty binary path = %v, want %v", err, exec.ErrNotFound)
	}
}

func TestExecRunnerBoundsOutputAndMarksTruncation(t *testing.T) {
	t.Run("stdout", func(t *testing.T) {
		runner := helperRunner(t, "flood")
		runner.MaxOut = 100

		result, err := runner.Run(context.Background(), "diff")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if len(result.Stdout) != 100 {
			t.Errorf("len(stdout) = %d, want 100", len(result.Stdout))
		}
		if !result.Truncated {
			t.Error("Truncated = false, want true")
		}
	})

	t.Run("stderr", func(t *testing.T) {
		runner := helperRunner(t, "flooderr")
		runner.MaxErr = 32

		result, err := runner.Run(context.Background(), "diff")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if len(result.Stderr) != 32 || len(result.Stdout) != 0 {
			t.Errorf("stderr = %d bytes, stdout = %d bytes, want 32 and 0", len(result.Stderr), len(result.Stdout))
		}
		if !result.Truncated {
			t.Error("Truncated = false, want true")
		}
	})

	t.Run("unbounded", func(t *testing.T) {
		runner := helperRunner(t, "args")

		result, err := runner.Run(context.Background(), "status")
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if result.Truncated {
			t.Error("Truncated = true for a small output, want false")
		}
	})
}

func TestExecRunnerTimeoutAndCancellation(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		runner := helperRunner(t, "wait")
		runner.Timeout = 50 * time.Millisecond

		_, err := runner.Run(context.Background(), "status")

		var runErr *RunError
		if !errors.As(err, &runErr) {
			t.Fatalf("Run() error = %v, want *RunError", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Run() error = %v, want %v", err, context.DeadlineExceeded)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		runner := helperRunner(t, "wait")
		runner.Timeout = time.Minute

		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		_, err := runner.Run(ctx, "status")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want %v", err, context.Canceled)
		}
	})
}

func TestExecRunnerRejectsUnsafeArguments(t *testing.T) {
	root, err := domain.NewRepoPath(t.TempDir())
	if err != nil {
		t.Fatalf("NewRepoPath() error = %v, want nil", err)
	}

	cases := []struct {
		name string
		args []string
		root domain.RepoPath
	}{
		{name: "no subcommand", args: nil, root: root},
		{name: "leading dash", args: []string{"--exec=rm"}, root: root},
		{name: "empty subcommand", args: []string{""}, root: root},
		{name: "nul in argument", args: []string{"status", "a\x00b"}, root: root},
		{name: "zero root", args: []string{"status"}, root: domain.RepoPath{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := NewExecRunner(tc.root)
			runner.Git = os.Args[0]

			_, err := runner.Run(context.Background(), tc.args...)
			if tc.root.IsZero() {
				if !errors.Is(err, domain.ErrInvalidPath) {
					t.Fatalf("Run() error = %v, want %v", err, domain.ErrInvalidPath)
				}

				return
			}
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Fatalf("Run() error = %v, want %v", err, domain.ErrInvalidArgument)
			}

			var runErr *RunError
			if errors.As(err, &runErr) {
				t.Errorf("Run() error = %v, want a validation error, not a *RunError", err)
			}
		})
	}
}

func TestRunErrorUnwrapsItsCause(t *testing.T) {
	cause := errors.New("causa")
	runErr := &RunError{Args: []string{"log"}, Code: 1, Stderr: "algo\n", Err: cause}

	if !errors.Is(runErr, cause) {
		t.Error("Unwrap() does not expose the cause")
	}

	message := runErr.Error()
	for _, want := range []string{"git log", "salida 1", "algo", "causa"} {
		if !strings.Contains(message, want) {
			t.Errorf("Error() = %q, want it to contain %q", message, want)
		}
	}

	empty := (&RunError{Args: []string{"status"}}).Error()
	if empty != "git status" {
		t.Errorf("Error() = %q, want %q", empty, "git status")
	}
}

func TestRunErrorSnippetsLongStderr(t *testing.T) {
	runErr := &RunError{Args: []string{"log"}, Code: 128, Stderr: strings.Repeat("e", errorSnippet+50)}

	if got := len(runErr.Error()); got > errorSnippet+40 {
		t.Errorf("len(Error()) = %d, want a bounded message", got)
	}
	if !strings.HasSuffix(runErr.Error(), "…") {
		t.Errorf("Error() = %q, want the truncation marker", runErr.Error())
	}
}

func countEntries(env []string, entry string) int {
	count := 0
	for _, value := range env {
		if value == entry {
			count++
		}
	}

	return count
}

func TestExecRunnerRunInputFeedsStdin(t *testing.T) {
	runner := helperRunner(t, "cat")
	payload := []byte("titulo\n\ncuerpo con ñ\n")

	result, err := runner.RunInput(context.Background(), payload, "commit", "-F", "-")
	if err != nil {
		t.Fatalf("RunInput() error = %v, want nil", err)
	}
	if string(result.Stdout) != string(payload) {
		t.Errorf("stdout = %q, want the payload back", result.Stdout)
	}
	if result.Code != 0 || result.Truncated {
		t.Errorf("result = %+v, want a clean exit", result)
	}
}

func TestExecRunnerWithoutInputClosesStdin(t *testing.T) {
	runner := helperRunner(t, "cat")

	result, err := runner.Run(context.Background(), "status")
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if len(result.Stdout) != 0 {
		t.Errorf("stdout = %q, want nothing: Run never writes to stdin", result.Stdout)
	}
}

func TestExecRunnerRejectsUnusableInput(t *testing.T) {
	cases := []struct {
		name  string
		stdin []byte
		want  error
	}{
		{name: "nul", stdin: []byte("titulo\x00cuerpo"), want: domain.ErrInvalidArgument},
		{name: "too large", stdin: bytes.Repeat([]byte("x"), MaxInputBytes+1), want: domain.ErrTooLarge},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := helperRunner(t, "cat")

			_, err := runner.RunInput(context.Background(), tc.stdin, "commit", "-F", "-")
			if !errors.Is(err, tc.want) {
				t.Fatalf("RunInput() error = %v, want %v", err, tc.want)
			}

			var runErr *RunError
			if errors.As(err, &runErr) {
				t.Errorf("RunInput() error = %v, want a validation error, not a *RunError", err)
			}
		})
	}
}

func TestExecRunnerAcceptsTheInputLimit(t *testing.T) {
	runner := helperRunner(t, "cat")
	payload := bytes.Repeat([]byte("x"), MaxInputBytes)

	result, err := runner.RunInput(context.Background(), payload, "commit", "-F", "-")
	if err != nil {
		t.Fatalf("RunInput() error = %v, want nil", err)
	}
	if len(result.Stdout) != len(payload) {
		t.Errorf("len(stdout) = %d, want %d", len(result.Stdout), len(payload))
	}
}

func TestExecRunnerIgnoresABrokenPipeOfAProcessThatExitedCleanly(t *testing.T) {
	runner := helperRunner(t, "drop")

	result, err := runner.RunInput(context.Background(), bytes.Repeat([]byte("x"), MaxInputBytes), "commit", "-F", "-")
	if err != nil {
		t.Fatalf("RunInput() error = %v, want nil: git closed stdin and still exited cleanly", err)
	}
	if result.Code != 0 {
		t.Errorf("Code = %d, want 0", result.Code)
	}
}

func TestExecRunnerKeepsTheCallerDeadline(t *testing.T) {
	runner := helperRunner(t, "wait")
	runner.Timeout = time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := runner.RunInput(ctx, []byte("datos"), "commit", "-F", "-")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunInput() error = %v, want %v", err, context.DeadlineExceeded)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("RunInput() took %v, want the caller deadline to win over the runner timeout", elapsed)
	}
}
