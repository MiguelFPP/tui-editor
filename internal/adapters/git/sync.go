package git

import (
	"context"
	"fmt"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// NetworkTimeout bounds a fetch, a pull or a push when the caller sets no deadline.
const NetworkTimeout = 2 * time.Minute

// upstreamRef is the revision that names the branch the current one tracks.
const upstreamRef = "@{upstream}"

// Fetch downloads the remote refs and prunes the ones the remotes removed.
func (s *Service) Fetch(ctx context.Context) error {
	ctx, cancel := networkContext(ctx)
	defer cancel()

	_, err := s.runWrite(ctx, "Fetch", false, func() (Result, error) {
		return s.runner.Run(ctx, "fetch", "--prune", "--quiet")
	})

	return err
}

// Push publishes the current branch to its upstream; it never forces.
func (s *Service) Push(ctx context.Context) error {
	ctx, cancel := networkContext(ctx)
	defer cancel()

	if err := s.requireUpstream(ctx, "Push"); err != nil {
		return err
	}

	_, err := s.runWrite(ctx, "Push", false, func() (Result, error) {
		return s.runner.Run(ctx, "push", "--quiet")
	})

	return err
}

// Pull fuses the upstream into the current branch; a conflict is reported, never resolved.
func (s *Service) Pull(ctx context.Context) (domain.MergeResult, error) {
	ctx, cancel := networkContext(ctx)
	defer cancel()

	if err := s.requireUpstream(ctx, "Pull"); err != nil {
		return domain.MergeResult{}, err
	}

	return s.mergeWith(ctx, "Pull", upstreamRef, func() (Result, error) {
		return s.runner.Run(ctx, "pull", "--no-rebase", "--no-edit")
	})
}

// requireUpstream fails before touching the network when the branch tracks nothing.
func (s *Service) requireUpstream(ctx context.Context, op string) error {
	_, err := s.runner.Run(ctx, "rev-parse", "--verify", "--quiet", upstreamRef)

	switch {
	case err == nil:
		return nil
	case isExitCode(err, 1):
		return fmt.Errorf("%w: %s", domain.ErrNoUpstream, op)
	default:
		return translateError(op, err)
	}
}

// networkContext bounds a network operation; a deadline set by the caller always wins.
func networkContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, NetworkTimeout)
}
