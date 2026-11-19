package domain

import (
	"errors"
	"testing"
)

func TestNewRepoPathRejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "blank", raw: "   "},
		{name: "relative", raw: "repo"},
		{name: "relative dot", raw: "./repo"},
		{name: "nul", raw: "/tmp/repo\x00"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewRepoPath(tc.raw); !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("NewRepoPath(%q) error = %v, want %v", tc.raw, err, ErrInvalidPath)
			}
		})
	}
}

func TestNewRepoPathCleans(t *testing.T) {
	root, err := NewRepoPath("/tmp/./repo/sub/../")
	if err != nil {
		t.Fatalf("NewRepoPath() error = %v, want nil", err)
	}

	if got, want := root.String(), "/tmp/repo"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if root.IsZero() {
		t.Error("IsZero() = true, want false")
	}
}

func TestRepoPathZero(t *testing.T) {
	zero := RepoPath{}

	if !zero.IsZero() {
		t.Error("IsZero() = false, want true")
	}
	if got := zero.String(); got != "" {
		t.Errorf("String() = %q, want empty", got)
	}
	if _, err := zero.Resolve("file.txt"); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("Resolve() error = %v, want %v", err, ErrInvalidPath)
	}
}

func TestRepoPathResolve(t *testing.T) {
	root, err := NewRepoPath("/tmp/repo")
	if err != nil {
		t.Fatalf("NewRepoPath() error = %v, want nil", err)
	}

	cases := []struct {
		name string
		rel  RelPath
		want string
	}{
		{name: "root", rel: "", want: "/tmp/repo"},
		{name: "dot", rel: ".", want: "/tmp/repo"},
		{name: "file", rel: "main.go", want: "/tmp/repo/main.go"},
		{name: "nested", rel: "dir/sub/file.txt", want: "/tmp/repo/dir/sub/file.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := root.Resolve(tc.rel)
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v, want nil", tc.rel, err)
			}
			if got != tc.want {
				t.Errorf("Resolve(%q) = %q, want %q", tc.rel, got, tc.want)
			}
		})
	}
}

func TestRepoPathResolveRejectsEscape(t *testing.T) {
	root, err := NewRepoPath("/tmp/repo")
	if err != nil {
		t.Fatalf("NewRepoPath() error = %v, want nil", err)
	}

	// The values below bypass NewRelPath on purpose: a parser could hand them over.
	cases := []struct {
		name string
		rel  RelPath
	}{
		{name: "parent", rel: RelPath("../repo2/secret.txt")},
		{name: "sibling prefix", rel: RelPath("../repo2")},
		{name: "deep parent", rel: RelPath("../../etc/passwd")},
		{name: "nested parent", rel: RelPath("dir/../../outside.txt")},
		{name: "absolute", rel: RelPath("/etc/passwd")},
		{name: "windows absolute", rel: RelPath(`C:\Windows\system32`)},
		{name: "nul", rel: RelPath("dir/\x00/file")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := root.Resolve(tc.rel)
			if !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("Resolve(%q) error = %v, want %v", tc.rel, err, ErrInvalidPath)
			}
			if got != "" {
				t.Errorf("Resolve(%q) = %q, want empty", tc.rel, got)
			}
		})
	}
}

func TestNewRelPathNormalizes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want RelPath
	}{
		{name: "empty", raw: "", want: ""},
		{name: "dot", raw: ".", want: ""},
		{name: "dot slash", raw: "./", want: ""},
		{name: "dot backslash", raw: `.\`, want: ""},
		{name: "file", raw: "main.go", want: "main.go"},
		{name: "nested", raw: "dir/sub/file.txt", want: "dir/sub/file.txt"},
		{name: "trailing slash", raw: "dir/sub/", want: "dir/sub"},
		{name: "double slash", raw: "dir//sub", want: "dir/sub"},
		{name: "inner dot", raw: "dir/./sub", want: "dir/sub"},
		{name: "backslashes", raw: `dir\sub\file.txt`, want: "dir/sub/file.txt"},
		{name: "dot backslash prefix", raw: `.\dir\sub`, want: "dir/sub"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewRelPath(tc.raw)
			if err != nil {
				t.Fatalf("NewRelPath(%q) error = %v, want nil", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("NewRelPath(%q) = %q, want %q", tc.raw, got, tc.want)
			}
			if (got == "") != got.IsZero() {
				t.Errorf("IsZero() and the empty value disagree for %q", got)
			}
			if got != "" && got.String() != string(got) {
				t.Errorf("String() = %q, want %q", got.String(), string(got))
			}
		})
	}
}

func TestNewRelPathRejects(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "nul", raw: "dir/\x00/file"},
		{name: "unix absolute", raw: "/etc/passwd"},
		{name: "unix absolute nested", raw: "/dir/file.txt"},
		{name: "windows absolute", raw: `C:\dir\file.txt`},
		{name: "windows absolute slashed", raw: "C:/dir/file.txt"},
		{name: "windows drive only", raw: `c:\`},
		{name: "home", raw: "~"},
		{name: "home path", raw: "~/dir"},
		{name: "parent", raw: ".."},
		{name: "parent path", raw: "../dir"},
		{name: "inner parent", raw: "dir/../other"},
		{name: "inner parent backslash", raw: `dir\..\other`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewRelPath(tc.raw)
			if !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("NewRelPath(%q) error = %v, want %v", tc.raw, err, ErrInvalidPath)
			}
			if got != "" {
				t.Errorf("NewRelPath(%q) = %q, want empty", tc.raw, got)
			}
		})
	}
}

func TestRelPathJoin(t *testing.T) {
	cases := []struct {
		name     string
		base     RelPath
		segments []string
		want     RelPath
	}{
		{name: "none", base: "", segments: nil, want: ""},
		{name: "from root", base: "", segments: []string{"file.txt"}, want: "file.txt"},
		{name: "from dir", base: "dir", segments: []string{"file.txt"}, want: "dir/file.txt"},
		{name: "nested", base: "dir", segments: []string{"sub", "file.txt"}, want: "dir/sub/file.txt"},
		{name: "trailing segment", base: "dir", segments: []string{"sub"}, want: "dir/sub"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.base.Join(tc.segments...)
			if err != nil {
				t.Fatalf("Join(%q) error = %v, want nil", tc.segments, err)
			}
			if got != tc.want {
				t.Errorf("Join(%q) = %q, want %q", tc.segments, got, tc.want)
			}
		})
	}
}

func TestRelPathJoinRejects(t *testing.T) {
	for _, segment := range []string{"", ".", "..", "~", "a/b", `a\b`, "a\x00b"} {
		t.Run(segment, func(t *testing.T) {
			if _, err := RelPath("dir").Join(segment); !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("Join(%q) error = %v, want %v", segment, err, ErrInvalidPath)
			}
		})
	}
}
