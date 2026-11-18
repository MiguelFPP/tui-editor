package fs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

func newService(t *testing.T, dir string) *Service {
	t.Helper()

	root, err := domain.NewRepoPath(dir)
	if err != nil {
		t.Fatalf("NewRepoPath(%q) error = %v, want nil", dir, err)
	}

	service, err := New(root)
	if err != nil {
		t.Fatalf("New(%q) error = %v, want nil", dir, err)
	}

	return service
}

func writeFile(t *testing.T, file, content string) {
	t.Helper()

	writeMode(t, file, content, 0o644)
}

func writeMode(t *testing.T, file, content string, mode os.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v, want nil", filepath.Dir(file), err)
	}
	if err := os.WriteFile(file, []byte(content), mode); err != nil {
		t.Fatalf("WriteFile(%q) error = %v, want nil", file, err)
	}
}

func mkdir(t *testing.T, dirs ...string) {
	t.Helper()

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v, want nil", dir, err)
		}
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()

	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink(%q, %q) error = %v, want nil", target, link, err)
	}
}

func names(entries []domain.FileNode) []string {
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name)
	}

	return got
}

func nodeKind(t *testing.T, entries []domain.FileNode, name string) domain.FileKind {
	t.Helper()

	for _, entry := range entries {
		if entry.Name == name {
			return entry.Kind
		}
	}

	t.Fatalf("entry %q not found in %v", name, names(entries))

	return domain.File
}

func TestNewValidatesRoot(t *testing.T) {
	dir := t.TempDir()

	if _, err := New(domain.RepoPath{}); !errors.Is(err, domain.ErrInvalidPath) {
		t.Errorf("New(zero) error = %v, want %v", err, domain.ErrInvalidPath)
	}

	file := filepath.Join(dir, "file.txt")
	writeFile(t, file, "x")

	root, err := domain.NewRepoPath(file)
	if err != nil {
		t.Fatalf("NewRepoPath(%q) error = %v, want nil", file, err)
	}
	if _, err := New(root); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("New(file) error = %v, want %v", err, domain.ErrInvalidArgument)
	}

	missing, err := domain.NewRepoPath(filepath.Join(dir, "missing"))
	if err != nil {
		t.Fatalf("NewRepoPath() error = %v, want nil", err)
	}
	if _, err := New(missing); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("New(missing) error = %v, want %v", err, domain.ErrNotFound)
	}
}

func TestReadDirSortsDirectoriesFirstAndIgnoresCase(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "zeta"), filepath.Join(dir, "Alpha"))
	writeFile(t, filepath.Join(dir, "Beta.txt"), "b")
	writeFile(t, filepath.Join(dir, "aardvark.go"), "a")

	service := newService(t, dir)

	listing, err := service.ReadDir(context.Background(), "")
	if err != nil {
		t.Fatalf("ReadDir() error = %v, want nil", err)
	}

	want := []string{"Alpha", "zeta", "aardvark.go", "Beta.txt"}
	if got := names(listing.Entries); !slices.Equal(got, want) {
		t.Errorf("ReadDir() names = %v, want %v", got, want)
	}
	if listing.Dir != "" {
		t.Errorf("ReadDir() dir = %q, want empty", listing.Dir)
	}
	if listing.Truncated {
		t.Error("ReadDir() truncated = true, want false")
	}
	if kind := nodeKind(t, listing.Entries, "Alpha"); kind != domain.Dir {
		t.Errorf("Alpha kind = %v, want %v", kind, domain.Dir)
	}
	if kind := nodeKind(t, listing.Entries, "Beta.txt"); kind != domain.File {
		t.Errorf("Beta.txt kind = %v, want %v", kind, domain.File)
	}
	if entry := listing.Entries[3]; entry.Path != "Beta.txt" || entry.Size != 1 || entry.ModTime.IsZero() {
		t.Errorf("Beta.txt = %+v, want path, size and modtime", entry)
	}
}

func TestReadDirAppliesDefaultIgnores(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{".git", ".idea", ".venv", "__pycache__", "build", "dist", "node_modules", "target", "vendor"} {
		mkdir(t, filepath.Join(dir, name))
	}
	writeFile(t, filepath.Join(dir, ".DS_Store"), "x")
	writeFile(t, filepath.Join(dir, "main.go"), "x")
	mkdir(t, filepath.Join(dir, "src"))

	service := newService(t, dir)

	listing, err := service.ReadDir(context.Background(), "")
	if err != nil {
		t.Fatalf("ReadDir() error = %v, want nil", err)
	}

	want := []string{"src", "main.go"}
	if got := names(listing.Entries); !slices.Equal(got, want) {
		t.Errorf("ReadDir() names = %v, want %v", got, want)
	}
}

func TestReadDirAppliesGitignoreRules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore"), "# comentario\n\n*.log\n!keep.log\nbuild/\nnested/*.tmp\n")
	writeFile(t, filepath.Join(dir, "drop.log"), "x")
	writeFile(t, filepath.Join(dir, "keep.log"), "x")
	writeFile(t, filepath.Join(dir, "nested", "drop.tmp"), "x")
	writeFile(t, filepath.Join(dir, "nested", "keep.txt"), "x")
	writeFile(t, filepath.Join(dir, "other", "nested", "drop.tmp"), "x")
	writeFile(t, filepath.Join(dir, "build", "out.bin"), "x")
	writeFile(t, filepath.Join(dir, "deep", "build", "out.bin"), "x")

	service := newService(t, dir)

	root, err := service.ReadDir(context.Background(), "")
	if err != nil {
		t.Fatalf("ReadDir() error = %v, want nil", err)
	}
	want := []string{"deep", "nested", "other", ".gitignore", "keep.log"}
	if got := names(root.Entries); !slices.Equal(got, want) {
		t.Errorf("ReadDir() names = %v, want %v", got, want)
	}

	deep, err := service.ReadDir(context.Background(), "deep")
	if err != nil {
		t.Fatalf("ReadDir(deep) error = %v, want nil", err)
	}
	if got := names(deep.Entries); len(got) != 0 {
		t.Errorf("ReadDir(deep) names = %v, want none", got)
	}

	nested, err := service.ReadDir(context.Background(), "nested")
	if err != nil {
		t.Fatalf("ReadDir(nested) error = %v, want nil", err)
	}
	if got, want := names(nested.Entries), []string{"keep.txt"}; !slices.Equal(got, want) {
		t.Errorf("ReadDir(nested) names = %v, want %v", got, want)
	}
	if nested.Entries[0].Path != "nested/keep.txt" {
		t.Errorf("path = %q, want %q", nested.Entries[0].Path, "nested/keep.txt")
	}

	inner, err := service.ReadDir(context.Background(), "other/nested")
	if err != nil {
		t.Fatalf("ReadDir(other/nested) error = %v, want nil", err)
	}
	if got, want := names(inner.Entries), []string{"drop.tmp"}; !slices.Equal(got, want) {
		t.Errorf("the anchored pattern leaked: names = %v, want %v", got, want)
	}
}

func TestReadDirGitignoreRootAnchored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore"), "/root.txt\n")
	writeFile(t, filepath.Join(dir, "root.txt"), "x")
	writeFile(t, filepath.Join(dir, "sub", "root.txt"), "x")

	service := newService(t, dir)

	root, err := service.ReadDir(context.Background(), "")
	if err != nil {
		t.Fatalf("ReadDir() error = %v, want nil", err)
	}
	if got, want := names(root.Entries), []string{"sub", ".gitignore"}; !slices.Equal(got, want) {
		t.Errorf("ReadDir() names = %v, want %v", got, want)
	}

	sub, err := service.ReadDir(context.Background(), "sub")
	if err != nil {
		t.Fatalf("ReadDir(sub) error = %v, want nil", err)
	}
	if got, want := names(sub.Entries), []string{"root.txt"}; !slices.Equal(got, want) {
		t.Errorf("ReadDir(sub) names = %v, want %v", got, want)
	}
}

func TestReadDirDirOnlyRuleIgnoresDirectoriesOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore"), "logs/\n")
	writeFile(t, filepath.Join(dir, "logs"), "not a directory")
	mkdir(t, filepath.Join(dir, "logs.d"))

	service := newService(t, dir)

	listing, err := service.ReadDir(context.Background(), "")
	if err != nil {
		t.Fatalf("ReadDir() error = %v, want nil", err)
	}

	want := []string{"logs.d", ".gitignore", "logs"}
	if got := names(listing.Entries); !slices.Equal(got, want) {
		t.Errorf("ReadDir() names = %v, want %v", got, want)
	}
}

func TestReadDirNegationCannotReopenIgnoredDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".gitignore"), "logs\n!logs/keep.txt\n")
	writeFile(t, filepath.Join(dir, "logs", "keep.txt"), "x")
	writeFile(t, filepath.Join(dir, "logs", "drop.txt"), "x")

	service := newService(t, dir)

	listing, err := service.ReadDir(context.Background(), "")
	if err != nil {
		t.Fatalf("ReadDir() error = %v, want nil", err)
	}

	if got, want := names(listing.Entries), []string{".gitignore"}; !slices.Equal(got, want) {
		t.Errorf("ReadDir() names = %v, want %v", got, want)
	}
}

func TestReadDirDoesNotFollowSymlinks(t *testing.T) {
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "s")

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "real", "file.txt"), "r")
	symlink(t, outside, filepath.Join(dir, "escape"))
	symlink(t, filepath.Join(dir, "real"), filepath.Join(dir, "link"))

	service := newService(t, dir)

	listing, err := service.ReadDir(context.Background(), "")
	if err != nil {
		t.Fatalf("ReadDir() error = %v, want nil", err)
	}

	if kind := nodeKind(t, listing.Entries, "escape"); kind != domain.Symlink {
		t.Errorf("escape kind = %v, want %v", kind, domain.Symlink)
	}
	if kind := nodeKind(t, listing.Entries, "link"); kind != domain.Symlink {
		t.Errorf("link kind = %v, want %v", kind, domain.Symlink)
	}

	cases := []struct {
		name string
		run  func() error
	}{
		{name: "list symlinked directory", run: func() error {
			_, err := service.ReadDir(context.Background(), "escape")

			return err
		}},
		{name: "list intermediate symlink", run: func() error {
			_, err := service.ReadDir(context.Background(), "link")

			return err
		}},
		{name: "read through symlinked directory", run: func() error {
			_, err := service.ReadFile(context.Background(), "escape/secret.txt")

			return err
		}},
		{name: "read through intermediate symlink", run: func() error {
			_, err := service.ReadFile(context.Background(), "link/file.txt")

			return err
		}},
		{name: "write through symlinked directory", run: func() error {
			_, err := service.WriteFile(context.Background(), "escape/secret.txt", "x", domain.FileStamp{})

			return err
		}},
		{name: "read symlink itself", run: func() error {
			_, err := service.ReadFile(context.Background(), "link")

			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, domain.ErrInvalidPath) {
				t.Fatalf("error = %v, want %v", err, domain.ErrInvalidPath)
			}
		})
	}
}

func TestPathsRejectTraversalAndAliasing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "real", "file.txt"), "r")

	service := newService(t, dir)

	paths := []domain.RelPath{"..", "../other", "../../etc/passwd", "/etc/passwd", "real/../../outside", "real/./file.txt", "real//file.txt"}
	for _, rel := range paths {
		t.Run(string(rel), func(t *testing.T) {
			if _, err := service.ReadDir(context.Background(), rel); !errors.Is(err, domain.ErrInvalidPath) {
				t.Errorf("ReadDir(%q) error = %v, want %v", rel, err, domain.ErrInvalidPath)
			}
			if _, err := service.ReadFile(context.Background(), rel); !errors.Is(err, domain.ErrInvalidPath) {
				t.Errorf("ReadFile(%q) error = %v, want %v", rel, err, domain.ErrInvalidPath)
			}
			if _, err := service.WriteFile(context.Background(), rel, "x", domain.FileStamp{}); !errors.Is(err, domain.ErrInvalidPath) {
				t.Errorf("WriteFile(%q) error = %v, want %v", rel, err, domain.ErrInvalidPath)
			}
		})
	}
}

func TestReadDirErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "file.txt"), "x")

	service := newService(t, dir)

	if _, err := service.ReadDir(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ReadDir(missing) error = %v, want %v", err, domain.ErrNotFound)
	}
	if _, err := service.ReadDir(context.Background(), "file.txt"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("ReadDir(file) error = %v, want %v", err, domain.ErrInvalidArgument)
	}
}

func TestReadDirTruncatesAtMaxEntries(t *testing.T) {
	dir := t.TempDir()
	for i := range domain.MaxDirEntries + 3 {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("f%05d.txt", i)), "")
	}

	service := newService(t, dir)

	listing, err := service.ReadDir(context.Background(), "")
	if err != nil {
		t.Fatalf("ReadDir() error = %v, want nil", err)
	}

	if !listing.Truncated {
		t.Error("Truncated = false, want true")
	}
	if len(listing.Entries) != domain.MaxDirEntries {
		t.Errorf("len(entries) = %d, want %d", len(listing.Entries), domain.MaxDirEntries)
	}
}

func TestReadFileRejectsBinary(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "nul.dat"), "abc\x00def")
	writeFile(t, filepath.Join(dir, "tail.dat"), strings.Repeat("a", domain.MaxPreviewBytes)+"\x00")

	service := newService(t, dir)

	for _, name := range []string{"nul.dat", "tail.dat"} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.ReadFile(context.Background(), domain.RelPath(name)); !errors.Is(err, domain.ErrBinary) {
				t.Fatalf("ReadFile(%q) error = %v, want %v", name, err, domain.ErrBinary)
			}
		})
	}
}

func TestReadFileTruncatesBytes(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("a", domain.MaxPreviewBytes+16)
	writeFile(t, filepath.Join(dir, "long.txt"), long)
	writeFile(t, filepath.Join(dir, "exact.txt"), strings.Repeat("b", domain.MaxPreviewBytes))
	writeFile(t, filepath.Join(dir, "rune.txt"), strings.Repeat("a", domain.MaxPreviewBytes-1)+"é"+"b")

	service := newService(t, dir)

	content, err := service.ReadFile(context.Background(), "long.txt")
	if err != nil {
		t.Fatalf("ReadFile() error = %v, want nil", err)
	}
	if len(content.Text) != domain.MaxPreviewBytes {
		t.Errorf("len(text) = %d, want %d", len(content.Text), domain.MaxPreviewBytes)
	}
	if !content.Truncated {
		t.Error("Truncated = false, want true")
	}
	if content.Stamp.Size != int64(len(long)) {
		t.Errorf("stamp size = %d, want %d", content.Stamp.Size, len(long))
	}
	if content.Stamp.ModTime.IsZero() {
		t.Error("stamp modtime is zero")
	}

	exact, err := service.ReadFile(context.Background(), "exact.txt")
	if err != nil {
		t.Fatalf("ReadFile(exact) error = %v, want nil", err)
	}
	if exact.Truncated {
		t.Error("a file of exactly MaxPreviewBytes was reported as truncated")
	}

	runes, err := service.ReadFile(context.Background(), "rune.txt")
	if err != nil {
		t.Fatalf("ReadFile(rune) error = %v, want nil", err)
	}
	if !utf8.ValidString(runes.Text) {
		t.Error("text is not valid UTF-8, the byte limit cut a rune in half")
	}
	if len(runes.Text) != domain.MaxPreviewBytes-1 {
		t.Errorf("len(text) = %d, want %d", len(runes.Text), domain.MaxPreviewBytes-1)
	}
}

func TestReadFileTruncatesLines(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("line\n", domain.MaxPreviewLines+10)
	writeFile(t, filepath.Join(dir, "long.txt"), long)
	writeFile(t, filepath.Join(dir, "exact.txt"), strings.Repeat("line\n", domain.MaxPreviewLines))

	service := newService(t, dir)

	content, err := service.ReadFile(context.Background(), "long.txt")
	if err != nil {
		t.Fatalf("ReadFile() error = %v, want nil", err)
	}
	if got := strings.Count(content.Text, "\n"); got != domain.MaxPreviewLines {
		t.Errorf("lines = %d, want %d", got, domain.MaxPreviewLines)
	}
	if !content.Truncated {
		t.Error("Truncated = false, want true")
	}
	if content.Stamp.Size != int64(len(long)) {
		t.Errorf("stamp size = %d, want %d", content.Stamp.Size, len(long))
	}
	if !strings.HasSuffix(content.Text, "\n") {
		t.Error("the last line was cut in half")
	}

	exact, err := service.ReadFile(context.Background(), "exact.txt")
	if err != nil {
		t.Fatalf("ReadFile(exact) error = %v, want nil", err)
	}
	if exact.Truncated {
		t.Error("a file of exactly MaxPreviewLines lines was reported as truncated")
	}
}

func TestReadFileErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "real", "file.txt"), "hola")
	symlink(t, filepath.Join(dir, "real", "file.txt"), filepath.Join(dir, "link.txt"))

	service := newService(t, dir)

	if _, err := service.ReadFile(context.Background(), ""); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("ReadFile(root) error = %v, want %v", err, domain.ErrInvalidArgument)
	}
	if _, err := service.ReadFile(context.Background(), "real"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("ReadFile(dir) error = %v, want %v", err, domain.ErrInvalidArgument)
	}
	if _, err := service.ReadFile(context.Background(), "missing.txt"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ReadFile(missing) error = %v, want %v", err, domain.ErrNotFound)
	}
	if _, err := service.ReadFile(context.Background(), "link.txt"); !errors.Is(err, domain.ErrInvalidPath) {
		t.Errorf("ReadFile(symlink) error = %v, want %v", err, domain.ErrInvalidPath)
	}
}

func TestWriteFileCreatesAtomically(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "nested"))

	service := newService(t, dir)

	stamp, err := service.WriteFile(context.Background(), "nested/new.txt", "hola", domain.FileStamp{})
	if err != nil {
		t.Fatalf("WriteFile() error = %v, want nil", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "nested", "new.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v, want nil", err)
	}
	if string(data) != "hola" {
		t.Errorf("content = %q, want %q", data, "hola")
	}
	if stamp.Size != 4 || stamp.ModTime.IsZero() {
		t.Errorf("stamp = %+v, want size 4 and a modtime", stamp)
	}

	info, err := os.Stat(filepath.Join(dir, "nested", "new.txt"))
	if err != nil {
		t.Fatalf("Stat() error = %v, want nil", err)
	}
	if got := info.Mode().Perm(); got != fileMode {
		t.Errorf("perm = %v, want %v", got, fileMode)
	}

	listing, err := service.ReadDir(context.Background(), "nested")
	if err != nil {
		t.Fatalf("ReadDir() error = %v, want nil", err)
	}
	if got, want := names(listing.Entries), []string{"new.txt"}; !slices.Equal(got, want) {
		t.Errorf("a temporary file survived the write: names = %v, want %v", got, want)
	}
}

func TestWriteFileKeepsModeAndConfirmsStamp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "script.sh")
	writeMode(t, target, "old", 0o600)

	service := newService(t, dir)

	first, err := service.WriteFile(context.Background(), "script.sh", "new", domain.FileStamp{})
	if err != nil {
		t.Fatalf("WriteFile() error = %v, want nil", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat() error = %v, want nil", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("perm = %v, want %v", got, os.FileMode(0o600))
	}

	second, err := service.WriteFile(context.Background(), "script.sh", "newer", first)
	if err != nil {
		t.Fatalf("WriteFile(matching stamp) error = %v, want nil", err)
	}
	if second.Size != int64(len("newer")) {
		t.Errorf("stamp size = %d, want %d", second.Size, len("newer"))
	}
}

func TestWriteFileRejectsStaleStamp(t *testing.T) {
	dir := t.TempDir()
	service := newService(t, dir)

	stamp, err := service.WriteFile(context.Background(), "file.txt", "first", domain.FileStamp{})
	if err != nil {
		t.Fatalf("WriteFile() error = %v, want nil", err)
	}

	writeFile(t, filepath.Join(dir, "file.txt"), "external change with another size")

	if _, err := service.WriteFile(context.Background(), "file.txt", "second", stamp); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("WriteFile(stale) error = %v, want %v", err, domain.ErrConflict)
	}
	if _, err := service.WriteFile(context.Background(), "missing.txt", "x", stamp); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("WriteFile(missing, stamp) error = %v, want %v", err, domain.ErrNotFound)
	}
}

func TestWriteFileRejectsUnsafeTargets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "real", "file.txt"), "r")
	symlink(t, filepath.Join(dir, "real", "file.txt"), filepath.Join(dir, "link.txt"))
	symlink(t, filepath.Join(dir, "real"), filepath.Join(dir, "linkdir"))

	service := newService(t, dir)

	cases := []struct {
		name    string
		rel     domain.RelPath
		wantErr error
	}{
		{name: "empty path", rel: "", wantErr: domain.ErrInvalidArgument},
		{name: "symlink target", rel: "link.txt", wantErr: domain.ErrInvalidPath},
		{name: "symlink parent", rel: "linkdir/new.txt", wantErr: domain.ErrInvalidPath},
		{name: "missing parent", rel: "ghost/new.txt", wantErr: domain.ErrNotFound},
		{name: "traversal", rel: "../new.txt", wantErr: domain.ErrInvalidPath},
		{name: "directory target", rel: "real", wantErr: domain.ErrInvalidArgument},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.WriteFile(context.Background(), tc.rel, "x", domain.FileStamp{}); !errors.Is(err, tc.wantErr) {
				t.Fatalf("WriteFile(%q) error = %v, want %v", tc.rel, err, tc.wantErr)
			}
		})
	}
}

func TestContextCancellation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "file.txt"), "x")

	service := newService(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := service.ReadDir(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Errorf("ReadDir() error = %v, want %v", err, context.Canceled)
	}
	if _, err := service.ReadFile(ctx, "file.txt"); !errors.Is(err, context.Canceled) {
		t.Errorf("ReadFile() error = %v, want %v", err, context.Canceled)
	}
	if _, err := service.WriteFile(ctx, "file.txt", "x", domain.FileStamp{}); !errors.Is(err, context.Canceled) {
		t.Errorf("WriteFile() error = %v, want %v", err, context.Canceled)
	}
}

func TestParseIgnoreRule(t *testing.T) {
	cases := []struct {
		name string
		line string
		want ignoreRule
		ok   bool
	}{
		{name: "empty", line: "   ", ok: false},
		{name: "comment", line: "# *.log", ok: false},
		{name: "name", line: "*.log", want: ignoreRule{pattern: "*.log"}, ok: true},
		{name: "crlf", line: "dist/\r", want: ignoreRule{pattern: "dist", dirOnly: true}, ok: true},
		{name: "negation", line: "!keep.log", want: ignoreRule{pattern: "keep.log", negate: true}, ok: true},
		{name: "anchored", line: "/build", want: ignoreRule{pattern: "build", anchor: true}, ok: true},
		{name: "nested pattern", line: "nested/*.tmp", want: ignoreRule{pattern: "nested/*.tmp", anchor: true}, ok: true},
		{name: "bare slash", line: "/", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseIgnoreRule(tc.line)
			if ok != tc.ok {
				t.Fatalf("parseIgnoreRule(%q) ok = %v, want %v", tc.line, ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("parseIgnoreRule(%q) = %+v, want %+v", tc.line, got, tc.want)
			}
		})
	}
}
