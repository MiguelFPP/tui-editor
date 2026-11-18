package git

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// defaultLockDelay is the pause before the single retry that follows a locked index.
const defaultLockDelay = 100 * time.Millisecond

// runnerCall is one git invocation run inside the write turn.
type runnerCall func() (Result, error)

// Stage adds paths to the index.
func (s *Service) Stage(ctx context.Context, paths []domain.RelPath) error {
	clean, err := stageablePaths(paths)
	if err != nil {
		return err
	}

	_, err = s.runWrite(ctx, "Stage", func() (Result, error) {
		return s.runner.Run(ctx, append([]string{"add", "--"}, clean...)...)
	})

	return err
}

// Unstage takes paths out of the index; an unborn branch has no HEAD to restore from.
func (s *Service) Unstage(ctx context.Context, paths []domain.RelPath) error {
	clean, err := stageablePaths(paths)
	if err != nil {
		return err
	}

	_, err = s.runWrite(ctx, "Unstage", func() (Result, error) {
		hasHead, err := s.hasHead(ctx)
		if err != nil {
			return Result{}, err
		}
		if !hasHead {
			return s.runner.Run(ctx, append([]string{"rm", "--cached", "--quiet", "--"}, clean...)...)
		}

		return s.runner.Run(ctx, append([]string{"restore", "--staged", "--"}, clean...)...)
	})

	return err
}

// StageHunks applies the patch of the selected hunks to the index.
func (s *Service) StageHunks(ctx context.Context, diff domain.Diff, hunks []int) error {
	return s.applyHunks(ctx, "StageHunks", diff, hunks, []string{"apply", "--cached", "-p1", "--whitespace=nowarn", "-"})
}

// UnstageHunks reverses the patch of the selected hunks out of the index.
func (s *Service) UnstageHunks(ctx context.Context, diff domain.Diff, hunks []int) error {
	return s.applyHunks(ctx, "UnstageHunks", diff, hunks,
		[]string{"apply", "--cached", "--reverse", "-p1", "--whitespace=nowarn", "-"})
}

// Commit records the index with the given message, which travels through stdin.
func (s *Service) Commit(ctx context.Context, message string) error {
	payload, err := domain.NormalizeCommitMessage(message)
	if err != nil {
		return err
	}

	_, err = s.runWrite(ctx, "Commit", func() (Result, error) {
		staged, err := s.hasStagedChanges(ctx)
		if err != nil {
			return Result{}, err
		}
		if !staged {
			return Result{}, domain.ErrNoChanges
		}

		return s.runner.RunInput(ctx, []byte(payload),
			"commit", "--quiet", "--cleanup=whitespace", "--no-status", "-F", "-")
	})

	return err
}

func (s *Service) applyHunks(ctx context.Context, op string, diff domain.Diff, hunks []int, args []string) error {
	payload, err := buildPatch(diff, hunks)
	if err != nil {
		return err
	}

	_, err = s.runWrite(ctx, op, func() (Result, error) {
		return s.runner.RunInput(ctx, payload, args...)
	})

	return err
}

// runWrite serialises one index write, retrying a locked index once inside the same turn.
func (s *Service) runWrite(ctx context.Context, op string, run runnerCall) (Result, error) {
	release, err := s.acquireWrite(ctx)
	if err != nil {
		return Result{}, err
	}
	defer release()

	result, err := run()
	if err == nil {
		return result, nil
	}

	failure := writeError(op, err)
	if !errors.Is(failure, domain.ErrGitIndexLocked) {
		return result, failure
	}
	if err := s.retryDelay(ctx); err != nil {
		return result, err
	}

	result, err = run()
	if err == nil {
		return result, nil
	}

	return result, writeError(op, err)
}

// acquireWrite takes the single write turn, or gives up when the caller is done.
func (s *Service) acquireWrite(ctx context.Context) (func(), error) {
	select {
	case s.write <- struct{}{}:
		return func() { <-s.write }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("git: %w", ctx.Err())
	}
}

// retryDelay waits the short backoff of the retry, still honouring the caller deadline.
func (s *Service) retryDelay(ctx context.Context) error {
	if s.lockDelay <= 0 {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("git: %w", err)
		}

		return nil
	}

	timer := time.NewTimer(s.lockDelay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("git: %w", ctx.Err())
	}
}

// hasHead reports whether the repository already has a commit.
func (s *Service) hasHead(ctx context.Context) (bool, error) {
	if _, err := s.runner.Run(ctx, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		if isExitCode(err, 1) {
			return false, nil
		}

		return false, translateError("Unstage", err)
	}

	return true, nil
}

// hasStagedChanges reports whether the index differs from HEAD.
func (s *Service) hasStagedChanges(ctx context.Context) (bool, error) {
	if _, err := s.runner.Run(ctx, "diff", "--cached", "--quiet", "--no-ext-diff"); err != nil {
		if isExitCode(err, 1) {
			return true, nil
		}

		return false, translateError("Commit", err)
	}

	return false, nil
}

// writeError translates a failed write and keeps the domain errors a pre-check returned.
func writeError(op string, err error) error {
	var runErr *RunError
	if !errors.As(err, &runErr) {
		return err
	}

	return translateError(op, err)
}

// isExitCode reports one specific exit status; a signal or a missing binary is not one.
func isExitCode(err error, code int) bool {
	var runErr *RunError
	if !errors.As(err, &runErr) || runErr.Err != nil {
		return false
	}

	return runErr.Code == code
}

// stageablePaths validates the path list of a stage or unstage call.
func stageablePaths(paths []domain.RelPath) ([]string, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("%w: falta al menos una ruta", domain.ErrInvalidArgument)
	}

	clean := make([]string, 0, len(paths))
	for _, path := range paths {
		if err := validatePath(path); err != nil {
			return nil, err
		}
		if path.IsZero() {
			return nil, fmt.Errorf("%w: no se puede preparar la raíz del repositorio", domain.ErrInvalidArgument)
		}

		clean = append(clean, string(path))
	}

	return clean, nil
}
