package git

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

func TestParseBranchesFixture(t *testing.T) {
	branches := parseBranches(fixture(t, "refs.tsv"))

	if len(branches) != 3 {
		t.Fatalf("len(branches) = %d, want 3", len(branches))
	}

	main := branches[0]
	if main.Name != "main" || main.Upstream != "origin/main" {
		t.Errorf("main = %+v, want the upstream origin/main", main)
	}
	if !main.Current || main.Ahead != 3 || main.Behind != 0 || main.Gone {
		t.Errorf("main = %+v, want the current branch three commits ahead", main)
	}

	feature := branches[1]
	if feature.Name != "feature-x" || !feature.Gone || feature.Current {
		t.Errorf("feature-x = %+v, want a gone upstream", feature)
	}
	if feature.Upstream != "origin/feature-x" {
		t.Errorf("feature-x upstream = %q, want origin/feature-x", feature.Upstream)
	}

	side := branches[2]
	if side.Name != "side" || side.Upstream != "" || side.Gone || side.Ahead != 0 {
		t.Errorf("side = %+v, want a branch without an upstream", side)
	}
}

func TestParseBranchesTrackingStates(t *testing.T) {
	raw := []byte("main\torigin/main\t*\tahead 2, behind 3\n" +
		"solo\t\t \t\n" +
		"gone\torigin/gone\t \tgone\n" +
		"roto\t\t \tno es una pista\n" +
		"\t\t \t\n" +
		"corto\n")

	branches := parseBranches(raw)
	if len(branches) != 4 {
		t.Fatalf("len(branches) = %d, want 4", len(branches))
	}

	if got := branches[0]; got.Ahead != 2 || got.Behind != 3 || got.Gone {
		t.Errorf("main = %+v, want ahead 2 and behind 3", got)
	}
	if got := branches[1]; got.Ahead != 0 || got.Behind != 0 || got.Gone {
		t.Errorf("solo = %+v, want no tracking", got)
	}
	if !branches[2].Gone {
		t.Errorf("gone = %+v, want Gone", branches[2])
	}
	if got := branches[3]; got.Ahead != 0 || got.Gone {
		t.Errorf("roto = %+v, want the unparsable track ignored", got)
	}
}

func TestParseBranchesTruncatesAtMaxBranches(t *testing.T) {
	var raw strings.Builder
	for i := range domain.MaxBranches + 2 {
		fmt.Fprintf(&raw, "rama-%05d\t\t \t\n", i)
	}

	branches := parseBranches([]byte(raw.String()))
	if len(branches) != domain.MaxBranches {
		t.Errorf("len(branches) = %d, want %d", len(branches), domain.MaxBranches)
	}
}

func TestParseStashesFixture(t *testing.T) {
	entries := parseStashes(fixture(t, "stash.z"))

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}

	custom := entries[0]
	if custom.Index != 0 || custom.Branch != "main" || custom.Message != "trabajo a medias" {
		t.Errorf("stash@{0} = %+v, want the custom message on main", custom)
	}
	if custom.Ref() != "stash@{0}" {
		t.Errorf("Ref() = %q, want stash@{0}", custom.Ref())
	}

	wip := entries[1]
	if wip.Index != 1 || wip.Branch != "main" {
		t.Errorf("stash@{1} = %+v, want the default message on main", wip)
	}
	if !strings.Contains(wip.Message, "merge side into main") {
		t.Errorf("stash@{1} message = %q, want the stashed subject", wip.Message)
	}

	for _, entry := range entries {
		if entry.Created.IsZero() {
			t.Errorf("%s has no creation date", entry.Ref())
		}
	}
}

func TestParseStashesSortsAndSkipsMalformed(t *testing.T) {
	stamp := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC).Format(time.RFC3339)
	raw := []byte("stash@{2}\x00On main: tercera\x00" + stamp + "\x00" +
		"basura\x00On main: sin ref\x00" + stamp + "\x00" +
		"stash@{0}\x00On main: primera\x00sin fecha\x00" +
		"stash@{1}\x00sin formato\x00" + stamp + "\x00")

	entries := parseStashes(raw)
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}

	for i, want := range []int{0, 1, 2} {
		if entries[i].Index != want {
			t.Errorf("entries[%d].Index = %d, want %d", i, entries[i].Index, want)
		}
	}

	first := entries[0]
	if first.Branch != "main" || first.Message != "primera" {
		t.Errorf("entries[0] = %+v, want the parsed subject", first)
	}
	if !first.Created.IsZero() {
		t.Errorf("entries[0].Created = %v, want the unparsable date ignored", first.Created)
	}

	created := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	for _, entry := range entries[1:] {
		if !entry.Created.Equal(created) {
			t.Errorf("%s Created = %v, want %v", entry.Ref(), entry.Created, created)
		}
	}
}

func TestParseStashesTruncatesAtMaxStashes(t *testing.T) {
	var raw strings.Builder
	for i := range domain.MaxStashes + 2 {
		fmt.Fprintf(&raw, "stash@{%d}\x00On main: st %d\x002026-02-03T04:05:06Z\x00", i, i)
	}

	entries := parseStashes([]byte(raw.String()))
	if len(entries) != domain.MaxStashes {
		t.Errorf("len(entries) = %d, want %d", len(entries), domain.MaxStashes)
	}
}

func TestParseStashSubject(t *testing.T) {
	cases := []struct {
		subject string
		branch  string
		message string
	}{
		{subject: "WIP on main: 1234abc asunto", branch: "main", message: "1234abc asunto"},
		{subject: "On feature/x: mensaje libre", branch: "feature/x", message: "mensaje libre"},
		{subject: "On main: con: dos puntos", branch: "main", message: "con: dos puntos"},
		{subject: "sin formato", branch: "", message: "sin formato"},
	}

	for _, tc := range cases {
		t.Run(tc.subject, func(t *testing.T) {
			branch, message := splitStashSubject(tc.subject)
			if branch != tc.branch || message != tc.message {
				t.Errorf("splitStashSubject(%q) = %q/%q, want %q/%q", tc.subject, branch, message, tc.branch, tc.message)
			}
		})
	}
}
