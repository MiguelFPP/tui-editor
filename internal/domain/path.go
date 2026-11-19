package domain

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// RepoPath is the absolute, cleaned root of the repository. It never touches the filesystem.
type RepoPath struct {
	p string
}

// NewRepoPath validates raw and returns the root without resolving symlinks.
func NewRepoPath(raw string) (RepoPath, error) {
	if strings.TrimSpace(raw) == "" {
		return RepoPath{}, fmt.Errorf("%w: la ruta está vacía", ErrInvalidPath)
	}
	if strings.ContainsRune(raw, 0) {
		return RepoPath{}, fmt.Errorf("%w: contiene un byte NUL", ErrInvalidPath)
	}
	if !filepath.IsAbs(raw) {
		return RepoPath{}, fmt.Errorf("%w: %q no es absoluta", ErrInvalidPath, raw)
	}

	return RepoPath{p: filepath.Clean(raw)}, nil
}

// String returns the native absolute path.
func (r RepoPath) String() string {
	return r.p
}

func (r RepoPath) IsZero() bool {
	return r.p == ""
}

// Resolve joins rel to the root and rejects any result escaping it, including
// sibling roots that only share a name prefix.
func (r RepoPath) Resolve(rel RelPath) (string, error) {
	if r.IsZero() {
		return "", fmt.Errorf("%w: falta la raíz del repositorio", ErrInvalidPath)
	}

	raw := string(rel)
	if strings.ContainsRune(raw, 0) || path.IsAbs(raw) || isWindowsAbs(raw) {
		return "", fmt.Errorf("%w: %q no es una ruta relativa", ErrInvalidPath, raw)
	}

	resolved := r.p
	if !rel.IsZero() {
		resolved = filepath.Join(r.p, filepath.FromSlash(raw))
	}

	inside, err := filepath.Rel(r.p, resolved)
	if err != nil {
		return "", fmt.Errorf("%w: %q no se relaciona con %q", ErrInvalidPath, string(rel), r.p)
	}
	if inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q queda fuera de %q", ErrInvalidPath, string(rel), r.p)
	}

	return resolved, nil
}

// RelPath is a clean, slash-separated path relative to the repository root.
// The zero value is the root itself.
type RelPath string

// NewRelPath normalises separators and validates raw, without touching the filesystem.
func NewRelPath(raw string) (RelPath, error) {
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("%w: contiene un byte NUL", ErrInvalidPath)
	}

	slashed := strings.ReplaceAll(raw, `\`, "/")
	switch {
	case slashed == "" || slashed == ".":
		return "", nil
	case strings.HasPrefix(slashed, "/"):
		return "", fmt.Errorf("%w: %q es una ruta absoluta", ErrInvalidPath, raw)
	case isWindowsAbs(raw):
		return "", fmt.Errorf("%w: %q es una ruta absoluta", ErrInvalidPath, raw)
	case strings.HasPrefix(slashed, "~"):
		return "", fmt.Errorf("%w: %q apunta al directorio home", ErrInvalidPath, raw)
	}

	for _, segment := range strings.Split(slashed, "/") {
		if segment == ".." {
			return "", fmt.Errorf("%w: %q sale de la raíz del repositorio", ErrInvalidPath, raw)
		}
	}

	cleaned := path.Clean(slashed)
	if cleaned == "." || cleaned == "/" {
		return "", nil
	}

	return RelPath(cleaned), nil
}

// String returns the slash-separated relative path.
func (r RelPath) String() string {
	return string(r)
}

func (r RelPath) IsZero() bool {
	return r == ""
}

// Join appends simple segments, rejecting separators, "~", "." and "..".
func (r RelPath) Join(segments ...string) (RelPath, error) {
	parts := make([]string, 0, len(segments)+1)
	if !r.IsZero() {
		parts = append(parts, string(r))
	}

	for _, segment := range segments {
		if !validSegment(segment) {
			return "", fmt.Errorf("%w: el segmento %q no es válido", ErrInvalidPath, segment)
		}
		parts = append(parts, segment)
	}

	if len(parts) == 0 {
		return "", nil
	}

	return RelPath(path.Join(parts...)), nil
}

func validSegment(segment string) bool {
	switch segment {
	case "", ".", "..", "~":
		return false
	}

	return !strings.ContainsRune(segment, 0) && !strings.ContainsAny(segment, `/\`)
}

func isWindowsAbs(raw string) bool {
	if len(raw) < 2 || raw[1] != ':' {
		return false
	}

	c := raw[0]
	if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
		return false
	}

	return len(raw) == 2 || raw[2] == '\\' || raw[2] == '/'
}
