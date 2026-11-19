package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
)

// Service implements the Git ports the TUI consumes on top of a Runner.
type Service struct {
	root   domain.RepoPath
	runner Runner

	// write serialises the mutations of the index; lockDelay backs off before the single retry.
	write     chan struct{}
	lockDelay time.Duration
}

var _ ports.GitService = (*Service)(nil)

// New validates the root and the runner without executing git.
func New(root domain.RepoPath, runner Runner) (*Service, error) {
	if root.IsZero() {
		return nil, fmt.Errorf("%w: falta la raíz del repositorio", domain.ErrInvalidPath)
	}
	if runner == nil {
		return nil, fmt.Errorf("%w: falta el runner de git", domain.ErrInvalidArgument)
	}

	return &Service{
		root:      root,
		runner:    runner,
		write:     make(chan struct{}, 1),
		lockDelay: defaultLockDelay,
	}, nil
}

// Status reads the bounded snapshot of the working tree.
func (s *Service) Status(ctx context.Context) (domain.GitStatus, error) {
	result, err := s.runner.Run(ctx, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=normal")
	if err != nil {
		return domain.GitStatus{}, translateError("Status", err)
	}

	status, err := parseStatus(ctx, result.Stdout)
	if err != nil {
		return domain.GitStatus{}, err
	}
	status.Truncated = status.Truncated || result.Truncated

	return status, nil
}

// Diff reads the unified diff of one path, staged or from the worktree.
func (s *Service) Diff(ctx context.Context, req domain.DiffRequest) (domain.Diff, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--no-prefix", "--unified=3"}
	if req.Staged {
		args = append(args, "--cached")
	}
	args = append(args, "--")
	if !req.Path.IsZero() {
		if err := validatePath(req.Path); err != nil {
			return domain.Diff{}, err
		}
		args = append(args, string(req.Path))
	}

	result, err := s.runner.Run(ctx, args...)
	if err != nil {
		return domain.Diff{}, translateError("Diff", err)
	}

	files := parsePatch(string(result.Stdout))
	if len(files) == 0 {
		return domain.Diff{Path: req.Path, Staged: req.Staged, Truncated: result.Truncated}, nil
	}

	diff := files[0]
	diff.Staged = req.Staged
	diff.Truncated = diff.Truncated || result.Truncated

	return diff, nil
}

// DiffStat reads the bounded numstat summary of the index or of the worktree.
func (s *Service) DiffStat(ctx context.Context, staged bool) (domain.DiffStat, error) {
	args := []string{"diff", "--numstat", "-z", "--no-color", "--no-ext-diff", "--no-textconv"}
	if staged {
		args = append(args, "--cached")
	}

	result, err := s.runner.Run(ctx, args...)
	if err != nil {
		return domain.DiffStat{}, translateError("DiffStat", err)
	}

	stat := parseNumStat(result.Stdout)
	stat.Truncated = stat.Truncated || result.Truncated

	return stat, nil
}

// Branches reads the local branches, most recently committed first.
func (s *Service) Branches(ctx context.Context) ([]domain.Branch, error) {
	result, err := s.runner.Run(ctx, "for-each-ref", "--sort=-committerdate",
		"--format=%(refname:short)\t%(upstream:short)\t%(HEAD)\t%(upstream:track,nobracket)", "refs/heads")
	if err != nil {
		return nil, translateError("Branches", err)
	}

	return parseBranches(result.Stdout), nil
}

// Stashes reads the stash list, ordered by index.
func (s *Service) Stashes(ctx context.Context) ([]domain.StashEntry, error) {
	result, err := s.runner.Run(ctx, "stash", "list", "--format=%gd%x00%gs%x00%cI%x00")
	if err != nil {
		return nil, translateError("Stashes", err)
	}

	return parseStashes(result.Stdout), nil
}

// Commits reads one bounded page of history.
func (s *Service) Commits(ctx context.Context, query domain.CommitQuery) (domain.CommitPage, error) {
	query = query.Normalized()

	if err := validateRef(query.Ref); err != nil {
		return domain.CommitPage{}, err
	}
	if err := validatePath(query.Path); err != nil {
		return domain.CommitPage{}, err
	}

	args := []string{"log", "--no-color", "--no-show-signature", "-n", strconv.Itoa(query.Limit + 1)}
	if query.Skip > 0 {
		args = append(args, "--skip="+strconv.Itoa(query.Skip))
	}
	args = append(args, "--format=%H%x00%h%x00%an%x00%ae%x00%cI%x00%s%x00")
	if query.Ref != "" {
		args = append(args, query.Ref)
	}
	if !query.Path.IsZero() {
		args = append(args, "--", string(query.Path))
	}

	result, err := s.runner.Run(ctx, args...)
	if err != nil {
		if isNoCommits(err) {
			return domain.CommitPage{}, nil // an unborn branch answers with an error, not an empty log
		}

		return domain.CommitPage{}, translateError("Commits", err)
	}

	page := parseCommits(result.Stdout, query.Limit)
	page.Truncated = page.Truncated || result.Truncated

	return page, nil
}

// Show reads one commit with its message body and the bounded diff of every file it touched.
func (s *Service) Show(ctx context.Context, hash string) (domain.CommitDetail, error) {
	if err := validateHash(hash); err != nil {
		return domain.CommitDetail{}, err
	}

	result, err := s.runner.Run(ctx, "show", "--no-color", "--no-ext-diff", "--no-textconv", "--no-prefix",
		"--unified=3", "--no-show-signature",
		"--format=%H%x00%h%x00%an%x00%ae%x00%cI%x00%s%x00%b%x00", hash, "--")
	if err != nil {
		return domain.CommitDetail{}, translateError("Show", err)
	}

	detail := parseShow(result.Stdout)
	detail.Truncated = detail.Truncated || result.Truncated

	return detail, nil
}

// FileAt reads the blob of one path at the given revision.
func (s *Service) FileAt(ctx context.Context, rev domain.FileRev, path domain.RelPath) (domain.RevisionContent, error) {
	spec, err := blobSpec(rev, path)
	if err != nil {
		return domain.RevisionContent{}, err
	}

	result, err := s.runner.Run(ctx, "show", "--no-color", "--no-ext-diff", "--no-textconv", "--no-patch", "--format=", spec)
	if err != nil {
		return domain.RevisionContent{}, fileAtError(err)
	}

	return parseBlob(path, rev, result.Stdout, result.Truncated)
}

// translateError maps the technical failures of git onto the domain sentinels.
func translateError(op string, err error) error {
	if err == nil {
		return nil
	}

	var runErr *RunError
	if !errors.As(err, &runErr) {
		return fmt.Errorf("git: %s: %w", op, err)
	}

	if runErr.Err != nil {
		switch {
		case errors.Is(runErr.Err, exec.ErrNotFound):
			return fmt.Errorf("%w: %s", domain.ErrGitUnavailable, op)
		case errors.Is(runErr.Err, context.Canceled), errors.Is(runErr.Err, context.DeadlineExceeded):
			return fmt.Errorf("git: %s: %w", op, runErr.Err)
		}
	}

	switch stderr := strings.ToLower(runErr.Stderr); {
	case strings.Contains(stderr, "index.lock"):
		return fmt.Errorf("%w: %s", domain.ErrGitIndexLocked, op)
	case strings.Contains(stderr, "not a git repository"):
		return fmt.Errorf("%w: %s", domain.ErrNotRepository, op)
	case strings.Contains(stderr, "merge_head exists"), strings.Contains(stderr, "concluded your merge"):
		return fmt.Errorf("%w: %s", domain.ErrMergeInProgress, op)
	case strings.Contains(stderr, "non-fast-forward"), strings.Contains(stderr, "fetch first"):
		return fmt.Errorf("%w: %s", domain.ErrNonFastForward, op)
	case strings.Contains(stderr, "could not resolve host"), strings.Contains(stderr, "connection timed out"),
		strings.Contains(stderr, "connection refused"), strings.Contains(stderr, "network is unreachable"),
		strings.Contains(stderr, "no route to host"), strings.Contains(stderr, "operation timed out"),
		strings.Contains(stderr, "failed to connect"), strings.Contains(stderr, "name resolution"):
		return fmt.Errorf("%w: %s", domain.ErrRemoteUnreachable, op)
	case strings.Contains(stderr, "authentication failed"), strings.Contains(stderr, "authentication required"),
		strings.Contains(stderr, "permission denied (publickey"), strings.Contains(stderr, "invalid username or password"),
		strings.Contains(stderr, "terminal prompts disabled"), strings.Contains(stderr, "could not read from remote repository"),
		strings.Contains(stderr, "no anonymous write access"):
		return fmt.Errorf("%w: %s", domain.ErrAuthRequired, op)
	case strings.Contains(stderr, "has no upstream branch"), strings.Contains(stderr, "no tracking information"),
		strings.Contains(stderr, "no upstream"):
		return fmt.Errorf("%w: %s", domain.ErrNoUpstream, op)
	case strings.Contains(stderr, "your local changes"), strings.Contains(stderr, "would be overwritten"):
		return fmt.Errorf("%w: %s", domain.ErrDirtyWorktree, op)
	case strings.Contains(stderr, "permission denied"), strings.Contains(stderr, "operation not permitted"):
		return fmt.Errorf("%w: %s", domain.ErrPermissionDenied, op)
	case strings.Contains(stderr, "no such remote"), strings.Contains(stderr, "no configured push destination"):
		return fmt.Errorf("%w: %s", domain.ErrNoRemote, op)
	}

	return fmt.Errorf("git: %s: %w", op, runErr)
}

// worktreeStatus reads the tracked status of the repository through the porcelain v2 parser.
func (s *Service) worktreeStatus(ctx context.Context, op string) (domain.GitStatus, error) {
	result, err := s.runner.Run(ctx, "status", "--porcelain=v2", "-z", "--untracked-files=no")
	if err != nil {
		return domain.GitStatus{}, translateError(op, err)
	}

	status, err := parseStatus(ctx, result.Stdout)
	if err != nil {
		return domain.GitStatus{}, err
	}
	status.Truncated = status.Truncated || result.Truncated

	return status, nil
}

// revParse resolves one revision to its object name; an unknown revision is a not-found error.
func (s *Service) revParse(ctx context.Context, op, ref string) (string, error) {
	result, err := s.runner.Run(ctx, "rev-parse", "--verify", "--quiet", ref)
	if err != nil {
		if isExitCode(err, 1) {
			return "", fmt.Errorf("%w: %s no resuelve a un objeto de git", domain.ErrNotFound, op)
		}

		return "", translateError(op, err)
	}

	name := strings.TrimSpace(string(result.Stdout))
	if name == "" {
		return "", fmt.Errorf("%w: %s no resuelve a un objeto de git", domain.ErrNotFound, op)
	}

	return name, nil
}

// isNoCommits detects the markers git uses for a branch without commits.
func isNoCommits(err error) bool {
	var runErr *RunError
	if !errors.As(err, &runErr) || runErr.Err != nil {
		return false
	}

	stderr := strings.ToLower(runErr.Stderr)

	return strings.Contains(stderr, "does not have any commits") ||
		strings.Contains(stderr, "unknown revision") ||
		strings.Contains(stderr, "bad revision")
}

// validatePath rejects paths that are not clean repository-relative paths.
func validatePath(path domain.RelPath) error {
	if path.IsZero() {
		return nil
	}

	clean, err := domain.NewRelPath(string(path))
	if err != nil {
		return err
	}
	if clean != path {
		return fmt.Errorf("%w: %q no está normalizada", domain.ErrInvalidPath, path)
	}

	return nil
}

// validateRef accepts anything git would read as a revision and never as an option.
func validateRef(ref string) error {
	if ref == "" {
		return nil
	}
	if strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, " \t\r\n") ||
		strings.Contains(ref, "..") || strings.Contains(ref, "@{") {
		return fmt.Errorf("%w: %q no es una referencia válida", domain.ErrInvalidArgument, ref)
	}

	return nil
}

// validateHash accepts an abbreviated or full object name.
func validateHash(hash string) error {
	if !isHash(hash) {
		return fmt.Errorf("%w: %q no es un hash de commit", domain.ErrInvalidArgument, hash)
	}

	return nil
}

func isHash(text string) bool {
	if len(text) < 40 || len(text) > 64 {
		return false
	}

	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}

	return true
}
