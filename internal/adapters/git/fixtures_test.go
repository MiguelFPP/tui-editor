package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// fixture reads one real git output captured in testdata.
func fixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("leyendo testdata/%s: %v", name, err)
	}

	return data
}

// diffPaths lists the new path of every parsed file.
func diffPaths(files []domain.Diff) []domain.RelPath {
	got := make([]domain.RelPath, 0, len(files))
	for _, file := range files {
		got = append(got, file.Path)
	}

	return got
}

// texts lists the text of every line of the first hunk.
func texts(diff domain.Diff) []string {
	if len(diff.Hunks) == 0 {
		return nil
	}

	got := make([]string, 0, len(diff.Hunks[0].Lines))
	for _, line := range diff.Hunks[0].Lines {
		got = append(got, line.Text)
	}

	return got
}
