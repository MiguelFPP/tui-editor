//go:build integration

package git

import (
	"context"
	"errors"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// assertFileAt reads one path at one revision through the adapter and compares its text.
func assertFileAt(t *testing.T, service *Service, rev domain.FileRev, path domain.RelPath, want string) {
	t.Helper()

	content, err := service.FileAt(context.Background(), rev, path)
	if err != nil {
		t.Fatalf("FileAt(%v, %q) error = %v, want nil", rev, path, err)
	}
	if content.Path != path || content.Rev != rev {
		t.Errorf("FileAt(%v, %q) = %+v, want the requested path and revision", rev, path, content)
	}
	if content.Text != want {
		t.Errorf("FileAt(%v, %q) text = %q, want %q", rev, path, content.Text, want)
	}
}

// assertFileAtMissing expects the not-found sentinel for one path at one revision.
func assertFileAtMissing(t *testing.T, service *Service, rev domain.FileRev, path domain.RelPath) {
	t.Helper()

	if _, err := service.FileAt(context.Background(), rev, path); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("FileAt(%v, %q) error = %v, want %v", rev, path, err, domain.ErrNotFound)
	}
}

func TestIntegrationFileAt(t *testing.T) {
	r := newRepo(t)
	r.write("tracked.txt", "uno\n")
	r.commit("initial commit")

	service := r.service()

	t.Run("worktree change stays out of both revisions", func(t *testing.T) {
		r.write("tracked.txt", "uno\ndos\n")

		assertFileAt(t, service, domain.RevHead, "tracked.txt", "uno\n")
		assertFileAt(t, service, domain.RevIndex, "tracked.txt", "uno\n")
	})

	t.Run("staging splits the index from HEAD", func(t *testing.T) {
		r.run("add", "tracked.txt")

		assertFileAt(t, service, domain.RevIndex, "tracked.txt", "uno\ndos\n")
		assertFileAt(t, service, domain.RevHead, "tracked.txt", "uno\n")
	})

	t.Run("a new file only exists in the index", func(t *testing.T) {
		r.write("nuevo.txt", "nuevo\n")
		r.run("add", "-A")

		assertFileAtMissing(t, service, domain.RevHead, "nuevo.txt")
		assertFileAt(t, service, domain.RevIndex, "nuevo.txt", "nuevo\n")
	})

	t.Run("a staged deletion is missing from the index only", func(t *testing.T) {
		r.run("rm", "-q", "-f", "tracked.txt")

		assertFileAtMissing(t, service, domain.RevIndex, "tracked.txt")
		assertFileAt(t, service, domain.RevHead, "tracked.txt", "uno\n")
	})

	t.Run("a rename keeps the pre-image in HEAD and the post-image in the index", func(t *testing.T) {
		r.write("viejo.txt", "v\n")
		r.commit("añade viejo")
		r.run("mv", "viejo.txt", "renombrado.txt")

		assertFileAt(t, service, domain.RevHead, "viejo.txt", "v\n")
		assertFileAtMissing(t, service, domain.RevHead, "renombrado.txt")
		assertFileAt(t, service, domain.RevIndex, "renombrado.txt", "v\n")
	})
}
