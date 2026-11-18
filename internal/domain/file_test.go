package domain

import (
	"testing"
	"time"
)

func TestFileNodeExpandable(t *testing.T) {
	if !(FileNode{Kind: Dir}).Expandable() {
		t.Error("Dir.Expandable() = false, want true")
	}

	for _, kind := range []FileKind{File, Symlink, Other} {
		if (FileNode{Kind: kind}).Expandable() {
			t.Errorf("FileKind(%d).Expandable() = true, want false", kind)
		}
	}
}

func TestFileStampIsZero(t *testing.T) {
	modTime := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)

	cases := []struct {
		name  string
		stamp FileStamp
		want  bool
	}{
		{name: "zero", stamp: FileStamp{}, want: true},
		{name: "size only", stamp: FileStamp{Size: 12}, want: false},
		{name: "time only", stamp: FileStamp{ModTime: modTime}, want: false},
		{name: "empty file", stamp: FileStamp{Size: 0, ModTime: modTime}, want: false},
		{name: "full", stamp: FileStamp{Size: 12, ModTime: modTime}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.stamp.IsZero(); got != tc.want {
				t.Errorf("IsZero() = %v, want %v", got, tc.want)
			}
		})
	}
}
