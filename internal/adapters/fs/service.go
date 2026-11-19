package fs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/MiguelFPP/tui-editor/internal/domain"
	"github.com/MiguelFPP/tui-editor/internal/ports"
)

const (
	gitignoreFile = ".gitignore"
	fileMode      = iofs.FileMode(0o644)
	tempPattern   = ".tui-editor-*.tmp"
)

// defaultIgnores are always hidden, whatever .gitignore says.
var defaultIgnores = map[string]bool{
	".git":         true,
	".DS_Store":    true,
	".idea":        true,
	".venv":        true,
	"__pycache__":  true,
	"build":        true,
	"dist":         true,
	"node_modules": true,
	"target":       true,
	"vendor":       true,
}

// Service is the ports.FileService adapter over one repository root.
type Service struct {
	root  domain.RepoPath
	rules []ignoreRule
}

var _ ports.FileService = (*Service)(nil)

// New validates root and loads the top-level .gitignore rules.
func New(root domain.RepoPath) (*Service, error) {
	if root.IsZero() {
		return nil, fmt.Errorf("%w: falta la raíz del repositorio", domain.ErrInvalidPath)
	}

	info, err := os.Stat(root.String())
	if err != nil {
		return nil, wrapError(err, "")
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %q no es un directorio", domain.ErrInvalidArgument, root)
	}

	rules, err := loadIgnoreRules(filepath.Join(root.String(), gitignoreFile))
	if err != nil {
		return nil, err
	}

	return &Service{root: root, rules: rules}, nil
}

// ReadDir lists one directory: symlink-free, ignored entries removed, sorted and capped.
func (s *Service) ReadDir(ctx context.Context, dir domain.RelPath) (domain.DirListing, error) {
	if err := ctx.Err(); err != nil {
		return domain.DirListing{}, fmt.Errorf("fs: ReadDir: %w", err)
	}

	abs, err := s.safePath(dir)
	if err != nil {
		return domain.DirListing{}, err
	}

	info, err := os.Lstat(abs)
	if err != nil {
		return domain.DirListing{}, wrapError(err, dir)
	}
	if !info.IsDir() {
		return domain.DirListing{}, fmt.Errorf("%w: %q no es un directorio", domain.ErrInvalidArgument, display(dir))
	}

	raw, err := os.ReadDir(abs)
	if err != nil {
		return domain.DirListing{}, wrapError(err, dir)
	}

	kept := make([]os.DirEntry, 0, len(raw))
	for _, entry := range raw {
		if err := ctx.Err(); err != nil {
			return domain.DirListing{}, fmt.Errorf("fs: ReadDir: %w", err)
		}

		rel, err := dir.Join(entry.Name())
		if err != nil {
			continue // a name that cannot be represented safely
		}
		if s.ignored(rel, entry.Name(), entry.IsDir()) {
			continue
		}

		kept = append(kept, entry)
	}

	sortEntries(kept)

	listing := domain.DirListing{Dir: dir}
	if len(kept) > domain.MaxDirEntries {
		kept = kept[:domain.MaxDirEntries]
		listing.Truncated = true
	}

	entries := make([]domain.FileNode, 0, len(kept))
	for _, entry := range kept {
		node, ok, err := s.node(dir, entry)
		if err != nil {
			return domain.DirListing{}, err
		}
		if !ok {
			continue
		}

		entries = append(entries, node)
	}

	listing.Entries = entries

	return listing, nil
}

// ReadFile returns a bounded text preview plus the stamp of the whole file.
func (s *Service) ReadFile(ctx context.Context, file domain.RelPath) (domain.FileContent, error) {
	if err := ctx.Err(); err != nil {
		return domain.FileContent{}, fmt.Errorf("fs: ReadFile: %w", err)
	}
	if file.IsZero() {
		return domain.FileContent{}, fmt.Errorf("%w: %q no es un archivo", domain.ErrInvalidArgument, display(file))
	}

	abs, err := s.safePath(file)
	if err != nil {
		return domain.FileContent{}, err
	}

	info, err := os.Lstat(abs)
	if err != nil {
		return domain.FileContent{}, wrapError(err, file)
	}
	if !info.Mode().IsRegular() {
		return domain.FileContent{}, fmt.Errorf("%w: %q no es un archivo regular", domain.ErrInvalidArgument, display(file))
	}

	handle, err := os.Open(abs)
	if err != nil {
		return domain.FileContent{}, wrapError(err, file)
	}

	data, readErr := readPrefix(handle, domain.MaxPreviewBytes)
	closeErr := handle.Close()

	if readErr != nil {
		return domain.FileContent{}, fmt.Errorf("fs: leyendo %q: %w", display(file), readErr)
	}
	if closeErr != nil {
		return domain.FileContent{}, fmt.Errorf("fs: cerrando %q: %w", display(file), closeErr)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return domain.FileContent{}, fmt.Errorf("%w: %q", domain.ErrBinary, display(file))
	}

	text := string(data)
	truncated := false
	if len(data) > domain.MaxPreviewBytes {
		text = trimPartialRune(text[:domain.MaxPreviewBytes])
		truncated = true
	}
	if clipped, cut := truncateLines(text, domain.MaxPreviewLines); cut {
		text = clipped
		truncated = true
	}

	return domain.FileContent{
		Path:      file,
		Text:      text,
		Stamp:     domain.FileStamp{Size: info.Size(), ModTime: info.ModTime()},
		Truncated: truncated,
	}, nil
}

// WriteFile replaces one file atomically, honouring the optimistic-locking stamp.
func (s *Service) WriteFile(ctx context.Context, file domain.RelPath, content string, expected domain.FileStamp) (domain.FileStamp, error) {
	if err := ctx.Err(); err != nil {
		return domain.FileStamp{}, fmt.Errorf("fs: WriteFile: %w", err)
	}
	if file.IsZero() {
		return domain.FileStamp{}, fmt.Errorf("%w: %q no es un archivo", domain.ErrInvalidArgument, display(file))
	}

	abs, err := s.root.Resolve(file)
	if err != nil {
		return domain.FileStamp{}, err
	}
	if err := validateSegments(file); err != nil {
		return domain.FileStamp{}, err
	}

	dir := parentOf(file)
	dirAbs, err := s.safePath(dir)
	if err != nil {
		return domain.FileStamp{}, err
	}

	dirInfo, err := os.Lstat(dirAbs)
	if err != nil {
		return domain.FileStamp{}, wrapError(err, dir)
	}
	if !dirInfo.IsDir() {
		return domain.FileStamp{}, fmt.Errorf("%w: %q no es un directorio", domain.ErrNotFound, display(dir))
	}

	mode := fileMode
	switch info, statErr := os.Lstat(abs); {
	case statErr == nil:
		if info.Mode()&iofs.ModeSymlink != 0 {
			return domain.FileStamp{}, fmt.Errorf("%w: %q es un symlink", domain.ErrInvalidPath, display(file))
		}
		if !info.Mode().IsRegular() {
			return domain.FileStamp{}, fmt.Errorf("%w: %q no es un archivo regular", domain.ErrInvalidArgument, display(file))
		}
		if !expected.IsZero() && !sameStamp(info, expected) {
			return domain.FileStamp{}, fmt.Errorf("%w: %q cambió en disco", domain.ErrConflict, display(file))
		}
		mode = info.Mode().Perm()
	case errors.Is(statErr, iofs.ErrNotExist):
		if !expected.IsZero() {
			return domain.FileStamp{}, fmt.Errorf("%w: %q", domain.ErrNotFound, display(file))
		}
	default:
		return domain.FileStamp{}, wrapError(statErr, file)
	}

	if err := writeAtomic(dirAbs, abs, content, mode, file); err != nil {
		return domain.FileStamp{}, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return domain.FileStamp{}, wrapError(err, file)
	}

	return domain.FileStamp{Size: info.Size(), ModTime: info.ModTime()}, nil
}

// safePath joins rel to the root and rejects aliasing segments and symlinks in any component.
func (s *Service) safePath(rel domain.RelPath) (string, error) {
	abs, err := s.root.Resolve(rel)
	if err != nil {
		return "", err
	}
	if err := validateSegments(rel); err != nil {
		return "", err
	}

	current := s.root.String()
	if !rel.IsZero() {
		for _, segment := range strings.Split(string(rel), "/") {
			current = filepath.Join(current, filepath.FromSlash(segment))

			info, err := os.Lstat(current)
			if err != nil {
				return "", wrapError(err, rel)
			}
			if info.Mode()&iofs.ModeSymlink != 0 {
				return "", fmt.Errorf("%w: %q atraviesa un symlink", domain.ErrInvalidPath, display(rel))
			}
		}
	}

	return abs, nil
}

// node converts one directory entry, never following symlinks.
func (s *Service) node(dir domain.RelPath, entry os.DirEntry) (domain.FileNode, bool, error) {
	rel, err := dir.Join(entry.Name())
	if err != nil {
		return domain.FileNode{}, false, nil
	}

	info, err := entry.Info()
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return domain.FileNode{}, false, nil // deleted while listing
		}

		return domain.FileNode{}, false, wrapError(err, rel)
	}

	return domain.FileNode{
		Name:    entry.Name(),
		Path:    rel,
		Kind:    kindOf(info.Mode()),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, true, nil
}

// ignored applies the hard defaults and then the .gitignore rules, last match wins.
func (s *Service) ignored(rel domain.RelPath, name string, isDir bool) bool {
	if defaultIgnores[name] {
		return true
	}

	ignored := false
	for _, rule := range s.rules {
		if rule.dirOnly && !isDir {
			continue
		}

		target := name
		if rule.anchor {
			target = string(rel)
		}
		if ok, err := doublestar.Match(rule.pattern, target); err != nil || !ok {
			continue
		}

		ignored = !rule.negate
	}

	return ignored
}

func writeAtomic(dirAbs, abs, content string, mode iofs.FileMode, file domain.RelPath) error {
	tmp, err := os.CreateTemp(dirAbs, tempPattern)
	if err != nil {
		return wrapError(err, parentOf(file))
	}
	name := tmp.Name()

	if err := tmp.Chmod(mode); err != nil {
		return discard(tmp, name, fmt.Errorf("fs: ajustando permisos de %q: %w", display(file), err))
	}
	if _, err := tmp.WriteString(content); err != nil {
		return discard(tmp, name, fmt.Errorf("fs: escribiendo %q: %w", display(file), err))
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)

		return fmt.Errorf("fs: cerrando %q: %w", display(file), err)
	}
	if err := os.Rename(name, abs); err != nil {
		_ = os.Remove(name)

		return wrapError(err, file)
	}

	return nil
}

func discard(tmp *os.File, name string, err error) error {
	_ = tmp.Close()
	_ = os.Remove(name)

	return err
}

func readPrefix(reader io.Reader, limit int) ([]byte, error) {
	buf := make([]byte, limit+1)

	n, err := io.ReadFull(reader, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}

	return buf[:n], nil
}

// truncateLines keeps at most maxLines complete lines.
func truncateLines(text string, maxLines int) (string, bool) {
	if maxLines <= 0 {
		return "", text != ""
	}

	lines := 0
	for i, r := range text {
		if r != '\n' {
			continue
		}

		lines++
		if lines == maxLines && i+1 < len(text) {
			return text[:i+1], true
		}
	}

	return text, false
}

// trimPartialRune drops a trailing multi-byte rune cut in half by the byte limit.
func trimPartialRune(text string) string {
	for len(text) > 0 {
		r, size := utf8.DecodeLastRuneInString(text)
		if r != utf8.RuneError || size > 1 {
			return text
		}

		text = text[:len(text)-1]
	}

	return text
}

func sortEntries(entries []os.DirEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.IsDir() != b.IsDir() {
			return a.IsDir()
		}

		lower, other := strings.ToLower(a.Name()), strings.ToLower(b.Name())
		if lower != other {
			return lower < other
		}

		return a.Name() < b.Name()
	})
}

func kindOf(mode iofs.FileMode) domain.FileKind {
	switch {
	case mode.IsDir():
		return domain.Dir
	case mode&iofs.ModeSymlink != 0:
		return domain.Symlink
	case mode.IsRegular():
		return domain.File
	default:
		return domain.Other
	}
}

func sameStamp(info iofs.FileInfo, want domain.FileStamp) bool {
	return info.Size() == want.Size && info.ModTime().Equal(want.ModTime)
}

func parentOf(rel domain.RelPath) domain.RelPath {
	dir := path.Dir(string(rel))
	if dir == "." || dir == "/" {
		return ""
	}

	return domain.RelPath(dir)
}

func simpleSegment(segment string) bool {
	switch segment {
	case "", ".", "..":
		return false
	}

	return !strings.ContainsRune(segment, 0)
}

// validateSegments rejects aliasing segments before any syscall touches the path.
func validateSegments(rel domain.RelPath) error {
	if rel.IsZero() {
		return nil
	}

	for _, segment := range strings.Split(string(rel), "/") {
		if !simpleSegment(segment) {
			return fmt.Errorf("%w: %q no es una ruta relativa simple", domain.ErrInvalidPath, display(rel))
		}
	}

	return nil
}

func display(rel domain.RelPath) string {
	if rel.IsZero() {
		return "."
	}

	return string(rel)
}

func wrapError(err error, rel domain.RelPath) error {
	switch {
	case errors.Is(err, iofs.ErrNotExist):
		return fmt.Errorf("%w: %q", domain.ErrNotFound, display(rel))
	case errors.Is(err, iofs.ErrPermission):
		return fmt.Errorf("%w: %q", domain.ErrPermissionDenied, display(rel))
	case errors.Is(err, iofs.ErrInvalid):
		return fmt.Errorf("%w: %q", domain.ErrInvalidPath, display(rel))
	default:
		return fmt.Errorf("fs: %q: %w", display(rel), err)
	}
}
