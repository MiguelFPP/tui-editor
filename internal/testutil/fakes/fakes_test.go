package fakes_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/testutil/fakes"
)

func TestFileServiceRecordsCallsAndReturnsHookValues(t *testing.T) {
	svc := fakes.NewFileService()
	svc.OnReadFile = func(ctx context.Context, path domain.RelPath) (domain.FileContent, error) {
		return domain.FileContent{Path: path, Text: "hola"}, nil
	}

	content, err := svc.ReadFile(context.Background(), "dir/file.txt")
	if err != nil {
		t.Fatalf("ReadFile() error = %v, want nil", err)
	}
	if content.Path != domain.RelPath("dir/file.txt") || content.Text != "hola" {
		t.Errorf("ReadFile() = %+v, want the hook content", content)
	}

	dir, err := svc.ReadDir(context.Background(), "dir")
	if err != nil {
		t.Fatalf("ReadDir() with a nil hook error = %v, want nil", err)
	}
	if dir.Dir != "" || dir.Truncated || len(dir.Entries) != 0 {
		t.Errorf("ReadDir() = %+v, want the zero listing", dir)
	}

	stamp, err := svc.WriteFile(context.Background(), "dir/file.txt", "nuevo", domain.FileStamp{Size: 4})
	if err != nil {
		t.Fatalf("WriteFile() with a nil hook error = %v, want nil", err)
	}
	if !stamp.IsZero() {
		t.Errorf("WriteFile() = %+v, want the zero stamp", stamp)
	}

	if got, want := svc.CallCount("ReadFile"), 1; got != want {
		t.Errorf("CallCount(ReadFile) = %d, want %d", got, want)
	}
	if got, want := svc.CallCount("ReadDir"), 1; got != want {
		t.Errorf("CallCount(ReadDir) = %d, want %d", got, want)
	}
	if got, want := svc.CallCount("WriteFile"), 1; got != want {
		t.Errorf("CallCount(WriteFile) = %d, want %d", got, want)
	}

	calls := svc.Calls()
	if len(calls) != 3 {
		t.Fatalf("len(Calls()) = %d, want 3", len(calls))
	}
	for i, want := range []string{"ReadFile", "ReadDir", "WriteFile"} {
		if calls[i].Method != want {
			t.Errorf("calls[%d].Method = %q, want %q", i, calls[i].Method, want)
		}
	}
	if got, want := calls[0].Args[0], domain.RelPath("dir/file.txt"); got != want {
		t.Errorf("calls[0].Args[0] = %#v, want %#v", got, want)
	}
	if got, want := calls[2].Args[1], "nuevo"; got != want {
		t.Errorf("calls[2].Args[1] = %#v, want %#v", got, want)
	}
	wantStamp := domain.FileStamp{Size: 4}
	if got := calls[2].Args[2]; got != wantStamp {
		t.Errorf("calls[2].Args[2] = %#v, want %#v", got, wantStamp)
	}

	calls[0].Method = "mutated"
	if got := svc.Calls()[0].Method; got != "ReadFile" {
		t.Errorf("Calls() must return a copy, got %q", got)
	}

	svc.Reset()
	if len(svc.Calls()) != 0 || svc.CallCount("ReadFile") != 0 {
		t.Error("Reset() did not clear the recorded calls")
	}
}

func TestFakeSkipsHookWhenContextIsDone(t *testing.T) {
	cases := []struct {
		name string
		ctx  func(t *testing.T) context.Context
		want error
	}{
		{
			name: "canceled",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				return ctx
			},
			want: context.Canceled,
		},
		{
			name: "deadline exceeded",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), time.Time{})
				t.Cleanup(cancel)

				return ctx
			},
			want: context.DeadlineExceeded,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := fakes.NewFileService()
			called := false
			svc.OnReadFile = func(ctx context.Context, path domain.RelPath) (domain.FileContent, error) {
				called = true

				return domain.FileContent{Text: "nunca"}, nil
			}

			content, err := svc.ReadFile(tc.ctx(t), "dir/file.txt")
			if !errors.Is(err, tc.want) {
				t.Fatalf("ReadFile() error = %v, want %v", err, tc.want)
			}
			if content != (domain.FileContent{}) {
				t.Errorf("ReadFile() = %+v, want the zero content", content)
			}
			if called {
				t.Error("the hook ran with a context that was already done")
			}
			if !strings.HasPrefix(err.Error(), "ReadFile: ") {
				t.Errorf("error %q must be prefixed with the method name", err)
			}
			if got, want := svc.CallCount("ReadFile"), 1; got != want {
				t.Errorf("CallCount(ReadFile) = %d, want %d: the call is recorded first", got, want)
			}
		})
	}
}

func TestGitServiceHooksErrorsAndZeroValues(t *testing.T) {
	svc := fakes.NewGitService()

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() with a nil hook error = %v, want nil", err)
	}
	if status.Branch != "" || status.HasChanges() {
		t.Errorf("Status() = %+v, want the zero status", status)
	}

	branches, err := svc.Branches(context.Background())
	if err != nil || len(branches) != 0 {
		t.Errorf("Branches() = %v, %v, want none and nil", branches, err)
	}

	diff, err := svc.Diff(context.Background(), domain.DiffRequest{Path: "main.go"})
	if err != nil || !diff.IsEmpty() {
		t.Errorf("Diff() = %+v, %v, want an empty diff and nil", diff, err)
	}

	stashes, err := svc.Stashes(context.Background())
	if err != nil || len(stashes) != 0 {
		t.Errorf("Stashes() = %v, %v, want none and nil", stashes, err)
	}

	if err := svc.Stage(context.Background(), []domain.RelPath{"main.go"}); err != nil {
		t.Errorf("Stage() error = %v, want nil", err)
	}
	if err := svc.Fetch(context.Background()); err != nil {
		t.Errorf("Fetch() error = %v, want nil", err)
	}

	result, err := svc.Merge(context.Background(), "main")
	if err != nil || result.HasConflicts() {
		t.Errorf("Merge() = %+v, %v, want a clean result and nil", result, err)
	}

	svc.OnStatus = func(ctx context.Context) (domain.GitStatus, error) {
		return domain.GitStatus{Branch: "main", Upstream: "origin/main", Ahead: 1}, nil
	}
	svc.OnCommit = func(ctx context.Context, message string) error {
		return domain.ErrConflict
	}

	status, err = svc.Status(context.Background())
	if err != nil || status.Branch != "main" || status.Ahead != 1 {
		t.Errorf("Status() = %+v, %v, want the hook status and nil", status, err)
	}

	if err := svc.Commit(context.Background(), "mensaje"); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("Commit() error = %v, want %v", err, domain.ErrConflict)
	}
	if got, want := svc.CallCount("Commit"), 1; got != want {
		t.Errorf("CallCount(Commit) = %d, want %d", got, want)
	}

	calls := svc.Calls()
	if got, want := calls[len(calls)-1].Args[0], "mensaje"; got != want {
		t.Errorf("Commit arg = %#v, want %#v", got, want)
	}
}

func TestGitServiceRecordsEveryWriterCall(t *testing.T) {
	svc := fakes.NewGitService()

	writer := []struct {
		method string
		call   func() error
	}{
		{method: "Stage", call: func() error { return svc.Stage(context.Background(), nil) }},
		{method: "Unstage", call: func() error { return svc.Unstage(context.Background(), nil) }},
		{method: "StageHunks", call: func() error { return svc.StageHunks(context.Background(), "a.go", nil) }},
		{method: "UnstageHunks", call: func() error { return svc.UnstageHunks(context.Background(), "a.go", nil) }},
		{method: "Commit", call: func() error { return svc.Commit(context.Background(), "m") }},
		{method: "Checkout", call: func() error { return svc.Checkout(context.Background(), "main") }},
		{method: "CreateBranch", call: func() error { return svc.CreateBranch(context.Background(), "feat", "main") }},
		{method: "DeleteBranch", call: func() error { return svc.DeleteBranch(context.Background(), "feat", true) }},
		{method: "StashPush", call: func() error { return svc.StashPush(context.Background(), "wip") }},
		{method: "StashApply", call: func() error { return svc.StashApply(context.Background(), 0) }},
		{method: "StashPop", call: func() error { return svc.StashPop(context.Background(), 1) }},
		{method: "StashDrop", call: func() error { return svc.StashDrop(context.Background(), 2) }},
		{method: "AbortMerge", call: func() error { return svc.AbortMerge(context.Background()) }},
		{method: "ResolveConflict", call: func() error {
			return svc.ResolveConflict(context.Background(), "a.go", domain.Ours)
		}},
		{method: "Fetch", call: func() error { return svc.Fetch(context.Background()) }},
		{method: "Push", call: func() error { return svc.Push(context.Background()) }},
	}

	for _, tc := range writer {
		t.Run(tc.method, func(t *testing.T) {
			if err := tc.call(); err != nil {
				t.Fatalf("%s() error = %v, want nil", tc.method, err)
			}
			if got, want := svc.CallCount(tc.method), 1; got != want {
				t.Errorf("CallCount(%s) = %d, want %d", tc.method, got, want)
			}
		})
	}

	if _, err := svc.Pull(context.Background()); err != nil {
		t.Errorf("Pull() error = %v, want nil", err)
	}
	if _, err := svc.Merge(context.Background(), "main"); err != nil {
		t.Errorf("Merge() error = %v, want nil", err)
	}

	if got := svc.CallCount("Pull"); got != 1 {
		t.Errorf("CallCount(Pull) = %d, want 1", got)
	}
	if got := svc.CallCount("Merge"); got != 1 {
		t.Errorf("CallCount(Merge) = %d, want 1", got)
	}
}

func TestEditorServiceHookAndZeroValue(t *testing.T) {
	svc := fakes.NewEditorService()

	cmd, err := svc.Resolve(context.Background(), domain.EditorTarget{Path: "dir/file.txt", Line: 12})
	if err != nil {
		t.Fatalf("Resolve() with a nil hook error = %v, want nil", err)
	}
	if !cmd.IsZero() {
		t.Errorf("Resolve() = %+v, want the zero command", cmd)
	}

	svc.OnResolve = func(ctx context.Context, target domain.EditorTarget) (domain.EditorCommand, error) {
		return domain.EditorCommand{Binary: "vim", Args: []string{"+12", target.Path.String()}}, nil
	}

	cmd, err = svc.Resolve(context.Background(), domain.EditorTarget{Path: "dir/file.txt", Line: 12})
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if got, want := cmd.String(), "vim +12 dir/file.txt"; got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
	if got := svc.CallCount("Resolve"); got != 2 {
		t.Errorf("CallCount(Resolve) = %d, want 2", got)
	}

	calls := svc.Calls()
	target, ok := calls[1].Args[0].(domain.EditorTarget)
	if !ok {
		t.Fatalf("Resolve arg type = %T, want domain.EditorTarget", calls[1].Args[0])
	}
	if target.Line != 12 || target.Path != domain.RelPath("dir/file.txt") {
		t.Errorf("Resolve arg = %+v, want the requested target", target)
	}
}

func TestFakesRecordConcurrentCalls(t *testing.T) {
	const goroutines = 32

	svc := fakes.NewFileService()
	svc.OnReadDir = func(ctx context.Context, dir domain.RelPath) (domain.DirListing, error) {
		return domain.DirListing{Dir: dir}, nil
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)

	paths := make([]domain.RelPath, goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()

			dir, err := svc.ReadDir(context.Background(), "dir")
			if err != nil {
				t.Errorf("ReadDir() error = %v, want nil", err)
			}
			paths[i] = dir.Dir
		}()
	}
	wg.Wait()

	if got, want := svc.CallCount("ReadDir"), goroutines; got != want {
		t.Errorf("CallCount(ReadDir) = %d, want %d", got, want)
	}
	if got, want := len(svc.Calls()), goroutines; got != want {
		t.Errorf("len(Calls()) = %d, want %d", got, want)
	}
	for i, path := range paths {
		if path != "dir" {
			t.Errorf("goroutine %d saw dir %q, want %q", i, path, "dir")
		}
	}
	if slices.Contains(paths, "") {
		t.Error("a goroutine saw an empty directory")
	}
}
