package git

import (
	"context"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// Checkout switches to branch, which must exist locally: no remote counterpart is guessed.
func (s *Service) Checkout(ctx context.Context, branch string) error {
	if err := domain.ValidateBranchName(branch); err != nil {
		return err
	}

	_, err := s.runWrite(ctx, "Checkout", true, func() (Result, error) {
		return s.runner.Run(ctx, "checkout", "--no-guess", branch, "--")
	})

	return err
}

// CreateBranch creates name at startPoint, or at the current HEAD when it is empty, and switches
// to it in the same turn.
func (s *Service) CreateBranch(ctx context.Context, name, startPoint string) error {
	if err := domain.ValidateBranchName(name); err != nil {
		return err
	}
	if err := validateRef(startPoint); err != nil {
		return err
	}

	args := []string{"checkout", "--no-guess", "-b", name}
	if startPoint != "" {
		args = append(args, startPoint)
	}
	args = append(args, "--")

	_, err := s.runWrite(ctx, "CreateBranch", true, func() (Result, error) {
		return s.runner.Run(ctx, args...)
	})

	return err
}

// DeleteBranch removes a local branch; force is the only way to drop one that is not merged.
func (s *Service) DeleteBranch(ctx context.Context, name string, force bool) error {
	if err := domain.ValidateBranchName(name); err != nil {
		return err
	}

	flag := "--delete"
	if force {
		flag = "--force"
	}

	_, err := s.runWrite(ctx, "DeleteBranch", true, func() (Result, error) {
		return s.runner.Run(ctx, "branch", flag, name)
	})

	return err
}
