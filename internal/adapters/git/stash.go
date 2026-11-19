package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// StashPush saves the tracked changes, with an optional one-line message; untracked files stay out.
func (s *Service) StashPush(ctx context.Context, message string) error {
	clean, err := stashMessage(message)
	if err != nil {
		return err
	}

	_, err = s.runWrite(ctx, "StashPush", true, func() (Result, error) {
		// The check runs inside the turn: another write cannot slip in between.
		status, err := s.worktreeStatus(ctx, "StashPush")
		if err != nil {
			return Result{}, err
		}
		if !status.HasChanges() {
			return Result{}, domain.ErrNoChanges
		}

		args := []string{"stash", "push"}
		if clean != "" {
			args = append(args, "-m", clean)
		}

		return s.runner.Run(ctx, args...)
	})

	return err
}

// StashApply restores one entry and keeps it in the list.
func (s *Service) StashApply(ctx context.Context, index int) error {
	return s.stashRestore(ctx, "StashApply", "apply", index)
}

// StashPop restores one entry and drops it when nothing conflicts.
func (s *Service) StashPop(ctx context.Context, index int) error {
	return s.stashRestore(ctx, "StashPop", "pop", index)
}

// StashDrop forgets one entry without touching the worktree.
func (s *Service) StashDrop(ctx context.Context, index int) error {
	ref, err := stashRef(index)
	if err != nil {
		return err
	}

	_, err = s.runWrite(ctx, "StashDrop", true, func() (Result, error) {
		return s.runner.Run(ctx, "stash", "drop", ref)
	})

	return err
}

// stashRestore applies or pops one entry; a conflicted result keeps the entry and is reported.
func (s *Service) stashRestore(ctx context.Context, op, verb string, index int) error {
	ref, err := stashRef(index)
	if err != nil {
		return err
	}

	_, runErr := s.runWrite(ctx, op, true, func() (Result, error) {
		return s.runner.Run(ctx, "stash", verb, ref)
	})

	status, statusErr := s.worktreeStatus(ctx, op)
	if statusErr != nil {
		if runErr != nil {
			return runErr
		}

		return statusErr
	}
	if conflicts := status.Conflicts(); len(conflicts) > 0 {
		return fmt.Errorf("%w: %s dejó %d conflictos sin resolver", domain.ErrConflict, ref, len(conflicts))
	}

	return runErr
}

// stashRef validates one stash position and renders the reference git expects.
func stashRef(index int) (string, error) {
	if index < 0 || index >= domain.MaxStashes {
		return "", fmt.Errorf("%w: el índice %d del stash está fuera de rango", domain.ErrInvalidArgument, index)
	}

	return fmt.Sprintf("stash@{%d}", index), nil
}

// stashMessage validates the optional message of a stash, which travels as one argument.
func stashMessage(message string) (string, error) {
	clean := strings.TrimSpace(message)
	if clean == "" {
		return "", nil
	}
	if len(clean) > domain.MaxCommitMessageBytes {
		return "", fmt.Errorf("%w: el mensaje del stash supera %d bytes", domain.ErrTooLarge, domain.MaxCommitMessageBytes)
	}
	if strings.ContainsRune(clean, 0) || strings.ContainsAny(clean, "\r\n") {
		return "", fmt.Errorf("%w: el mensaje del stash debe ser una sola línea", domain.ErrInvalidArgument)
	}

	return clean, nil
}
