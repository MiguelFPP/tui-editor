package gitpanel

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// updateGold rewrites the golden files: make test-run PKG='./internal/tui/components/gitpanel/...' RUN=TestGolden -update.
var updateGold = flag.Bool("update", false, "reescribe los golden files de testdata")

// The goldens fix the panel size so they never depend on the terminal of the environment.
const (
	goldenWidth  = 80
	goldenHeight = 16
)

// checkGolden compares the visible text of one view against its golden file.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name)
	if *updateGold {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("creando testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
			t.Fatalf("escribiendo %s: %v", path, err)
		}

		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leyendo %s: %v", path, err)
	}

	if want := strings.TrimSuffix(string(data), "\n"); got != want {
		t.Errorf("%s:\n got %q\nwant %q", name, got, want)
	}
}

func TestGoldenCommitModal(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(goldenWidth, goldenHeight)

	m = editKey(t, m, key('c'))
	m = typeText(t, m, "arregla el panel de git")

	checkGolden(t, "commit_modal.golden", plain(m.View()))
}

func TestGoldenBranchPrompt(t *testing.T) {
	m := goTo(t, open(t, gitFake(t)), Branches)
	m.SetSize(goldenWidth, goldenHeight)

	m = settle(t, m, typed('n'))
	m = typeText(t, m, "feature-ui")

	checkGolden(t, "branch_prompt.golden", plain(m.View()))
}

func TestGoldenDeleteConfirmation(t *testing.T) {
	m := goTo(t, open(t, gitFake(t)), Branches)
	m.SetSize(goldenWidth, goldenHeight)

	m = settle(t, m, key(tea.KeyDown))
	m = settle(t, m, typed('x'))

	checkGolden(t, "confirm_delete.golden", plain(m.View()))
}

func TestGoldenConflictBanner(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(goldenWidth, goldenHeight)

	checkGolden(t, "conflict_banner.golden", plain(m.View()))
}

func TestGoldenStableAcrossRenders(t *testing.T) {
	m := open(t, gitFake(t))
	m.SetSize(goldenWidth, goldenHeight)

	first := plain(m.View())
	for range 3 {
		if again := plain(m.View()); again != first {
			t.Fatalf("View() = %q, want the stable %q", again, first)
		}
	}
}
