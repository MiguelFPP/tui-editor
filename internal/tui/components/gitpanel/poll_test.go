package gitpanel

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/tui/keymap"
)

// active begins a quiet poll, plus the tick of the next period.
func activate(t *testing.T, m Model) Model {
	t.Helper()

	m.interval = 0
	cmd := m.SetActive(true)

	return answers(t, m, cmd)
}

// pollOnce feeds one tick of the running generation and applies the answer it produces.
func pollOnce(t *testing.T, m Model) Model {
	t.Helper()

	next, cmd := m.Update(tickMsg{gen: m.tickGen})

	return answers(t, next, cmd)
}

func TestInactiveTickNoPoll(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)
	git.Reset()

	next, cmd := m.Update(tickMsg{gen: m.tickGen})

	if cmd != nil {
		t.Error("inactive tick scheduled a command, want none")
	}
	if got := git.CallCount("Status"); got != 0 {
		t.Errorf("Status calls = %d, want 0 while the tab is hidden", got)
	}
	if next.polling {
		t.Error("polling = true, want no read started by a hidden tick")
	}
}

func TestInactiveTickStaleGeneration(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)
	m.interval = 0
	m = answers(t, m, m.SetActive(true))

	gen := m.tickGen
	m = answers(t, m, m.SetActive(false))
	git.Reset()

	next, cmd := m.Update(tickMsg{gen: gen})

	if cmd != nil {
		t.Error("stale tick scheduled a command, want none")
	}
	if got := git.CallCount("Status"); got != 0 {
		t.Errorf("Status calls = %d, want 0 for the generation of a hidden tab", got)
	}
	if next.polling {
		t.Error("polling = true, want no read after a stale tick")
	}
}

func TestSetActiveStartsAndStops(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)
	m.interval = 0
	git.Reset()

	next, cmd := m.Update(tickMsg{gen: 0}) // a tick of the initial generation

	if cmd != nil || next.polling {
		t.Error("a tick before SetActive scheduled work, want none")
	}

	next = answers(t, next, next.SetActive(true))
	if !next.pollOn {
		t.Error("pollOn = false, want the poller running after SetActive(true)")
	}
	if got := git.CallCount("Status"); got != 1 {
		t.Errorf("Status calls = %d, want the immediate read of entering the tab", got)
	}

	next = answers(t, next, next.SetActive(false))
	if next.pollOn {
		t.Error("pollOn = true, want the poller stopped after SetActive(false)")
	}

	next, cmd = next.Update(tickMsg{gen: next.tickGen})
	if cmd != nil {
		t.Error("a tick after leaving the tab scheduled work, want none")
	}
	if next.polling {
		t.Error("polling = true, want no read after leaving the tab")
	}
}

func TestTickPollsAndRearms(t *testing.T) {
	git := gitFake(t)
	m := activate(t, open(t, git))
	git.Reset()

	next, cmd := m.Update(tickMsg{gen: m.tickGen})

	if cmd == nil {
		t.Fatal("the tick scheduled no command, want the answer and the next period")
	}
	if !next.polling {
		t.Error("polling = false, want the read in flight")
	}

	answers(t, next, cmd)

	if got := git.CallCount("Status"); got != 1 {
		t.Errorf("Status calls = %d, want one read per tick", got)
	}
}

func TestTickSkippedWhilePolling(t *testing.T) {
	git := gitFake(t)
	m := activate(t, open(t, git))
	git.Reset()

	m.polling = true // a read is still in flight
	next, cmd := m.Update(tickMsg{gen: m.tickGen})

	if got := git.CallCount("Status"); got != 0 {
		t.Errorf("Status calls = %d, want no stacked read while one is in flight", got)
	}
	if cmd == nil {
		t.Error("the skipped tick armed no next period, want the poller to keep running")
	}
	if next.polling != true {
		t.Error("polling = false, want the read in flight to stay untouched")
	}
}

func TestTickSkippedWhileWriting(t *testing.T) {
	git := gitFake(t)
	m := activate(t, open(t, git))
	git.Reset()

	m.writing = true // a stage or a commit owns the repository
	next, cmd := m.Update(tickMsg{gen: m.tickGen})

	if got := git.CallCount("Status"); got != 0 {
		t.Errorf("Status calls = %d, want no read while a write is in flight", got)
	}
	if cmd == nil {
		t.Error("the skipped tick armed no next period, want the poller to keep running")
	}
	if next.polling {
		t.Error("polling = true, want no read started by the skipped tick")
	}
}

func TestTickSkippedWhileAManualReadIsInFlight(t *testing.T) {
	git := gitFake(t)
	m := activate(t, open(t, git))
	git.Reset()

	m.loading.status = true // an r or a write started a manual read of the same path
	next, cmd := m.Update(tickMsg{gen: m.tickGen})

	if got := git.CallCount("Status"); got != 0 {
		t.Errorf("Status calls = %d, want no duplicate read while a manual one is in flight", got)
	}
	if cmd == nil {
		t.Error("the skipped tick armed no next period, want the poller to keep running")
	}
	if next.polling {
		t.Error("polling = true, want no poll started over a manual read")
	}
}

func TestPollDoesNotSpin(t *testing.T) {
	git := gitFake(t)
	m := activate(t, open(t, git))

	next := pollOnce(t, m)

	if next.loading.status {
		t.Error("loading.status = true after a poll, want the quiet read to go unnoticed")
	}
	if next.Loading() {
		t.Error("Loading() = true after a poll, want the status bar spinner stopped")
	}
	if next.errs.status != nil {
		t.Errorf("errs.status = %v, want the manual error untouched by a poll", next.errs.status)
	}
	if next.polling {
		t.Error("polling = true, want the answer to clear the flag")
	}
}

func TestPollAppliesExternalChange(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	// A file edited outside the TUI: the fake starts answering a different snapshot.
	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		changed := status()
		changed.Files = append(changed.Files, domain.GitFileStatus{
			Path: "externo.go", Index: domain.GitUnmodified, Worktree: domain.GitModified,
		})

		return changed, nil
	}

	next := pollOnce(t, activate(t, m))

	if !next.status.HasChanges() {
		t.Fatal("HasChanges() = false, want the external edit to reach the panel")
	}
	if got := len(next.status.Files); got != len(status().Files)+1 {
		t.Errorf("files = %d, want %d after the external edit", got, len(status().Files)+1)
	}
	if !slices.ContainsFunc(next.status.Files, func(f domain.GitFileStatus) bool {
		return string(f.Path) == "externo.go"
	}) {
		t.Error("externo.go is missing, want the file edited outside the TUI listed")
	}
}

func TestPollIgnoresUnchangedStatus(t *testing.T) {
	git := gitFake(t)
	m := activate(t, open(t, git))

	m.cursors[Changes] = 1
	m.offsets[Changes] = 1
	before := m

	next := pollOnce(t, m)

	if !next.status.Equal(before.status) {
		t.Error("status changed, want the identical snapshot to be kept as is")
	}
	if next.cursors[Changes] != before.cursors[Changes] || next.offsets[Changes] != before.offsets[Changes] {
		t.Error("the cursor or the offset moved, want an unchanged snapshot to touch nothing")
	}
}

func TestPollIgnoresTransientError(t *testing.T) {
	git := gitFake(t)
	m := activate(t, open(t, git))
	before := m.status

	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		return domain.GitStatus{}, errors.New("git status: resource temporarily unavailable")
	}

	next := pollOnce(t, m)

	if !next.status.Equal(before) {
		t.Error("status changed, want a failed poll to keep the last good snapshot")
	}
	if next.errs.status != nil {
		t.Errorf("errs.status = %v, want a transient failure dropped instead of shown", next.errs.status)
	}
	if next.Err() != nil {
		t.Errorf("Err() = %v, want no error shown after a failed poll", next.Err())
	}
	if next.polling {
		t.Error("polling = true, want a failed poll to clear the flag")
	}
}

func TestPollKeepsItsAnswerWhenAManualReadTookOver(t *testing.T) {
	git := gitFake(t)
	m := activate(t, open(t, git))

	m.seqs.poll++
	stale := pollStatusMsg{seq: m.seqs.poll, epoch: m.seqs.status, status: domain.GitStatus{Branch: "otra"}}
	m.seqs.status++ // a manual refresh started after the poll

	next, cmd := m.Update(stale)

	if cmd != nil {
		t.Error("a stale poll scheduled a command, want none")
	}
	if next.status.Branch != "main" {
		t.Errorf("branch = %q, want the manual read to win over a stale poll", next.status.Branch)
	}
}

func TestPollDropsAnOldAnswer(t *testing.T) {
	git := gitFake(t)
	m := activate(t, open(t, git))
	before := m.status

	m.seqs.poll++ // a newer poll is already running
	next, _ := m.Update(pollStatusMsg{seq: m.seqs.poll - 1, status: domain.GitStatus{Branch: "vieja"}})

	if !next.status.Equal(before) {
		t.Error("status changed, want the answer of an old poll dropped")
	}
}

func TestPollReloadsBranchesOnlyWhenTheRefsChange(t *testing.T) {
	git := gitFake(t)
	m := activate(t, open(t, git))

	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		changed := status()
		changed.Files = append(changed.Files, domain.GitFileStatus{Path: "otro.go", Worktree: domain.GitModified})

		return changed, nil
	}
	git.Reset()
	pollOnce(t, m)

	if got := git.CallCount("Branches"); got != 0 {
		t.Errorf("Branches calls = %d, want none when only the worktree changed", got)
	}

	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		changed := status()
		changed.Branch = "otra-rama"

		return changed, nil
	}
	git.Reset()
	pollOnce(t, m)

	if got := git.CallCount("Branches"); got != 1 {
		t.Errorf("Branches calls = %d, want one when the branch changed", got)
	}
}

func TestPollReloadsTheOpenDiff(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	cmd := m.loadDiff("normal.txt")
	m = answers(t, m, cmd)
	git.Reset()

	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		changed := status()
		changed.Files = append(changed.Files, domain.GitFileStatus{Path: "otro.go", Worktree: domain.GitModified})

		return changed, nil
	}

	pollOnce(t, activate(t, m))

	if got := git.CallCount("Diff"); got != 1 {
		t.Errorf("Diff calls = %d, want the open diff reloaded after a poll", got)
	}
}

func TestPollLeavesTheDiffAloneWhenNothingIsOpen(t *testing.T) {
	git := gitFake(t)
	m := activate(t, open(t, git))

	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		changed := status()
		changed.Files = append(changed.Files, domain.GitFileStatus{Path: "otro.go", Worktree: domain.GitModified})

		return changed, nil
	}
	git.Reset()
	pollOnce(t, m)

	if got := git.CallCount("Diff"); got != 0 {
		t.Errorf("Diff calls = %d, want none when no diff was ever requested", got)
	}
}

func TestPollKeepsTheMarkedHunksOfAnUnchangedDiff(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	cmd := m.loadDiff("normal.txt") // mutates m, so the answer must be applied to the result
	m = answers(t, m, cmd)

	m.marked[0] = true
	m.hunkCursor = 0

	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		changed := status()
		changed.Files = append(changed.Files, domain.GitFileStatus{Path: "otro.go", Worktree: domain.GitModified})

		return changed, nil
	}

	next := pollOnce(t, activate(t, m))

	if !next.marked[0] {
		t.Error("the marked hunk was dropped, want the poll to keep the selection of an unchanged diff")
	}
}

func TestPollResetTheMarkedHunksWhenTheDiffChanged(t *testing.T) {
	git := gitFake(t)
	m := open(t, git)

	cmd := m.loadDiff("normal.txt")
	m = answers(t, m, cmd)

	m.marked[0] = true

	git.OnStatus = func(context.Context) (domain.GitStatus, error) {
		changed := status()
		changed.Files = append(changed.Files, domain.GitFileStatus{Path: "otro.go", Worktree: domain.GitModified})

		return changed, nil
	}
	git.OnDiff = func(_ context.Context, req domain.DiffRequest) (domain.Diff, error) {
		changed := sampleDiff()
		changed.Hunks = append(changed.Hunks, domain.DiffHunk{
			Header: "@@ -9,1 +9,1 @@", OldStart: 9, OldLines: 1, NewStart: 9, NewLines: 1,
			Lines: []domain.DiffLine{{Kind: domain.Added, Text: "nuevo", NewLine: 9}},
		})

		return changed, nil
	}

	next := pollOnce(t, activate(t, m))

	if next.marked[0] {
		t.Error("the marked hunk survived, want a changed diff to drop the selection")
	}
}

func TestPollIntervalIsOneSecond(t *testing.T) {
	m := New(gitFake(t), keymap.Default())

	if m.interval != time.Second {
		t.Errorf("interval = %v, want %v", m.interval, time.Second)
	}
}
