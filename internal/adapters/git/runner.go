package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

const (
	// RunTimeout bounds one git invocation; MaxOutputBytes and MaxErrorBytes bound its output.
	RunTimeout     = 10 * time.Second
	MaxOutputBytes = 8 << 20
	MaxErrorBytes  = 64 << 10

	waitDelay    = 2 * time.Second
	errorSnippet = 300
)

// Result is the bounded output of one git invocation that exited without error.
type Result struct {
	Stdout    []byte
	Stderr    string
	Code      int
	Truncated bool
}

// RunError is a failed invocation; it keeps the arguments and the technical stderr for logs.
type RunError struct {
	Args   []string
	Code   int
	Stderr string
	Err    error
}

func (e *RunError) Error() string {
	parts := []string{"git " + strings.Join(e.Args, " ")}
	if e.Code != 0 {
		parts = append(parts, fmt.Sprintf("salida %d", e.Code))
	}
	if stderr := snippet(e.Stderr); stderr != "" {
		parts = append(parts, stderr)
	}
	if e.Err != nil {
		parts = append(parts, e.Err.Error())
	}

	return strings.Join(parts, ": ")
}

// Unwrap exposes the missing binary or the cancelled context below the exit status.
func (e *RunError) Unwrap() error {
	return e.Err
}

// snippet keeps an error readable without leaking a whole stderr buffer into the UI.
func snippet(text string) string {
	trimmed := strings.TrimSpace(text)
	runes := []rune(trimmed)
	if len(runes) <= errorSnippet {
		return trimmed
	}

	return string(runes[:errorSnippet]) + "…"
}

// Runner runs one git subcommand without a shell.
type Runner interface {
	Run(ctx context.Context, args ...string) (Result, error)
}

// ExecRunner runs the native git binary; NewExecRunner fills the bounded defaults.
type ExecRunner struct {
	Git      string
	Root     domain.RepoPath
	Timeout  time.Duration
	MaxOut   int
	MaxErr   int
	LookPath func(string) (string, error)
}

var _ Runner = (*ExecRunner)(nil)

func NewExecRunner(root domain.RepoPath) *ExecRunner {
	return &ExecRunner{
		Root:     root,
		Timeout:  RunTimeout,
		MaxOut:   MaxOutputBytes,
		MaxErr:   MaxErrorBytes,
		LookPath: exec.LookPath,
	}
}

// Run executes args as one git subcommand, with separated streams and bounded output.
func (r *ExecRunner) Run(ctx context.Context, args ...string) (Result, error) {
	if r.Root.IsZero() {
		return Result{}, fmt.Errorf("%w: falta la raíz del repositorio", domain.ErrInvalidPath)
	}
	if err := validateArgs(args); err != nil {
		return Result{}, err
	}

	binary, err := r.binary()
	if err != nil {
		return Result{}, &RunError{Args: args, Err: err}
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()

	stdout := newSink(r.limit(r.MaxOut, MaxOutputBytes))
	stderr := newSink(r.limit(r.MaxErr, MaxErrorBytes))

	cmd := exec.CommandContext(ctx, binary, r.command(args)...)
	cmd.Env = gitEnv()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = waitDelay

	runErr := cmd.Run()

	result := Result{
		Stdout:    stdout.bytes(),
		Stderr:    stderr.text(),
		Code:      exitCode(runErr),
		Truncated: stdout.truncated || stderr.truncated,
	}
	if runErr == nil {
		return result, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, &RunError{Args: args, Code: result.Code, Stderr: result.Stderr, Err: ctxErr}
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return result, &RunError{Args: args, Code: result.Code, Stderr: result.Stderr}
	}

	return result, &RunError{Args: args, Stderr: result.Stderr, Err: runErr}
}

// command builds the global prefix; only the subcommand arguments are appended to it.
func (r *ExecRunner) command(args []string) []string {
	root := r.Root.String()
	full := make([]string, 0, len(args)+12)

	full = append(full,
		"-C", root,
		"--no-pager",
		"-c", "core.quotepath=false",
		"-c", "core.fsmonitor=false",
		"-c", "color.ui=false",
		"-c", "safe.directory="+root,
	)

	return append(full, args...)
}

func (r *ExecRunner) binary() (string, error) {
	if r.Git != "" {
		return r.Git, nil
	}

	lookPath := r.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	name, err := lookPath("git")
	if err != nil {
		return "", fmt.Errorf("git: %w", err)
	}
	if name == "" {
		return "", fmt.Errorf("git: %w", exec.ErrNotFound)
	}

	return name, nil
}

func (r *ExecRunner) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}

	return RunTimeout
}

func (r *ExecRunner) limit(configured, fallback int) int {
	if configured > 0 {
		return configured
	}

	return fallback
}

// validateArgs rejects anything that would need a shell or change the global prefix.
func validateArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: falta el subcomando de git", domain.ErrInvalidArgument)
	}
	if args[0] == "" || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("%w: %q no es un subcomando de git", domain.ErrInvalidArgument, args[0])
	}

	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return fmt.Errorf("%w: un argumento de git contiene un byte NUL", domain.ErrInvalidArgument)
		}
	}

	return nil
}

// blockedEnv redirects git to another repository or object store; the process env wins otherwise.
var blockedEnv = map[string]bool{
	"GIT_DIR":                          true,
	"GIT_WORK_TREE":                    true,
	"GIT_INDEX_FILE":                   true,
	"GIT_OBJECT_DIRECTORY":             true,
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	"GIT_NAMESPACE":                    true,
	"GIT_CEILING_DIRECTORIES":          true,
	"GIT_COMMON_DIR":                   true,
}

var forcedEnv = []string{
	"GIT_OPTIONAL_LOCKS=0",
	"GIT_TERMINAL_PROMPT=0",
	"GIT_PAGER=cat",
	"PAGER=cat",
	"LC_ALL=C",
	"LANG=C",
	"LC_MESSAGES=C",
}

// forcedNames are replaced, never inherited: the adapter always wins over the parent process.
var forcedNames = map[string]bool{
	"GIT_OPTIONAL_LOCKS":  true,
	"GIT_TERMINAL_PROMPT": true,
	"GIT_PAGER":           true,
	"PAGER":               true,
	"LC_ALL":              true,
	"LANG":                true,
	"LC_MESSAGES":         true,
}

// gitEnv inherits the environment without the variables that would escape the root.
func gitEnv() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env)+len(forcedEnv))

	for _, entry := range env {
		if name, _, _ := strings.Cut(entry, "="); blockedEnv[name] || forcedNames[name] {
			continue
		}

		filtered = append(filtered, entry)
	}

	return append(filtered, forcedEnv...)
}

// sink keeps at most limit bytes and discards the rest, so git never blocks on a full pipe.
type sink struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newSink(limit int) *sink {
	return &sink{limit: max(limit, 0)}
}

func (s *sink) Write(p []byte) (int, error) {
	written := len(p)
	if written == 0 {
		return 0, nil
	}

	room := s.limit - s.buf.Len()
	if room <= 0 {
		s.truncated = true

		return written, nil
	}
	if written > room {
		s.truncated = true
		p = p[:room]
	}

	_, _ = s.buf.Write(p)

	return written, nil
}

func (s *sink) bytes() []byte {
	return bytes.Clone(s.buf.Bytes())
}

func (s *sink) text() string {
	return strings.ToValidUTF8(s.buf.String(), "\uFFFD")
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
		return exitErr.ExitCode()
	}

	return 0
}
