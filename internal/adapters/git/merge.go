package git

import (
	"context"
	"fmt"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// Merge fuses branch into the current one; a conflict is reported, never resolved.
func (s *Service) Merge(ctx context.Context, branch string) (domain.MergeResult, error) {
	if err := domain.ValidateBranchName(branch); err != nil {
		return domain.MergeResult{}, err
	}

	running, err := s.mergeInProgress(ctx, "Merge")
	if err != nil {
		return domain.MergeResult{}, err
	}
	if running {
		return domain.MergeResult{}, fmt.Errorf("%w: Merge", domain.ErrMergeInProgress)
	}

	return s.mergeWith(ctx, "Merge", branch, func() (Result, error) {
		return s.runner.Run(ctx, "merge", "--no-edit", "--no-stat", branch)
	})
}

// AbortMerge undoes the merge in progress, restoring HEAD and the index.
func (s *Service) AbortMerge(ctx context.Context) error {
	running, err := s.mergeInProgress(ctx, "AbortMerge")
	if err != nil {
		return err
	}
	if !running {
		return fmt.Errorf("%w: AbortMerge", domain.ErrNoMergeInProgress)
	}

	_, err = s.runWrite(ctx, "AbortMerge", false, func() (Result, error) {
		return s.runner.Run(ctx, "merge", "--abort")
	})

	return err
}

// ResolveConflict stages one side of a conflicted path; the choice is always the caller's.
func (s *Service) ResolveConflict(ctx context.Context, path domain.RelPath, resolution domain.Resolution) error {
	if err := validatePath(path); err != nil {
		return err
	}
	if path.IsZero() {
		return fmt.Errorf("%w: falta la ruta del conflicto", domain.ErrInvalidArgument)
	}

	flag, err := resolutionFlag(resolution)
	if err != nil {
		return err
	}

	_, err = s.runWrite(ctx, "ResolveConflict", true, func() (Result, error) {
		if _, err := s.runner.Run(ctx, "checkout", flag, "--", string(path)); err != nil {
			return Result{}, err
		}

		return s.runner.Run(ctx, "add", "--all", "--", string(path))
	})

	return err
}

// mergeWith runs one merge-like command and classifies what it did to HEAD.
func (s *Service) mergeWith(ctx context.Context, op, target string, run runnerCall) (domain.MergeResult, error) {
	before, err := s.revParse(ctx, op, "HEAD")
	if err != nil {
		return domain.MergeResult{}, err
	}
	if _, err := s.revParse(ctx, op, target); err != nil {
		return domain.MergeResult{}, err
	}

	if _, err := s.runWrite(ctx, op, false, run); err != nil {
		status, statusErr := s.worktreeStatus(ctx, op)
		if statusErr != nil {
			return domain.MergeResult{}, err
		}
		if conflicts := status.Conflicts(); len(conflicts) > 0 {
			return domain.MergeResult{Status: domain.Conflicted, Conflicts: conflicts}, nil
		}

		return domain.MergeResult{}, err
	}

	after, err := s.revParse(ctx, op, "HEAD")
	if err != nil {
		return domain.MergeResult{}, err
	}

	// The target is read again: a pull advances the upstream with the same command.
	reached, err := s.revParse(ctx, op, target)
	if err != nil {
		reached = ""
	}

	return classifyMerge(before, reached, after), nil
}

// classifyMerge compares the resulting HEAD with the bounds of the merge.
func classifyMerge(before, reached, after string) domain.MergeResult {
	switch {
	case after == before:
		return domain.MergeResult{Status: domain.UpToDate}
	case after == reached:
		return domain.MergeResult{Status: domain.FastForward}
	default:
		return domain.MergeResult{Status: domain.Clean}
	}
}

// mergeInProgress reports whether MERGE_HEAD exists, which is how git marks a merge in flight.
func (s *Service) mergeInProgress(ctx context.Context, op string) (bool, error) {
	_, err := s.runner.Run(ctx, "rev-parse", "--verify", "--quiet", "MERGE_HEAD")

	switch {
	case err == nil:
		return true, nil
	case isExitCode(err, 1):
		return false, nil
	default:
		return false, translateError(op, err)
	}
}

// resolutionFlag maps one domain resolution onto the checkout flag of git.
func resolutionFlag(resolution domain.Resolution) (string, error) {
	switch resolution {
	case domain.Ours:
		return "--ours", nil
	case domain.Theirs:
		return "--theirs", nil
	default:
		return "", fmt.Errorf("%w: %d no es una resolución de conflicto", domain.ErrInvalidArgument, resolution)
	}
}
