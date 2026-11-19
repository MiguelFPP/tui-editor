package domain

import "testing"

func TestFileRevValid(t *testing.T) {
	cases := []struct {
		name string
		rev  FileRev
		want bool
	}{
		{name: "index", rev: RevIndex, want: true},
		{name: "head", rev: RevHead, want: true},
		{name: "zero", rev: FileRev(0), want: false},
		{name: "unknown", rev: FileRev(99), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rev.Valid(); got != tc.want {
				t.Errorf("Valid() got = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFileRevString(t *testing.T) {
	cases := []struct {
		name string
		rev  FileRev
		want string
	}{
		{name: "index", rev: RevIndex, want: "index"},
		{name: "head", rev: RevHead, want: "HEAD"},
		{name: "zero", rev: FileRev(0), want: ""},
		{name: "unknown", rev: FileRev(99), want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rev.String(); got != tc.want {
				t.Errorf("String() got = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRevisionContentIsEmpty(t *testing.T) {
	cases := []struct {
		name    string
		content RevisionContent
		want    bool
	}{
		{name: "zero", content: RevisionContent{}, want: true},
		{
			name:    "text",
			content: RevisionContent{Path: "main.go", Rev: RevHead, Text: "uno\n"},
			want:    false,
		},
		{
			name:    "empty text with revision",
			content: RevisionContent{Path: "main.go", Rev: RevIndex},
			want:    true,
		},
		{
			name:    "empty text truncated",
			content: RevisionContent{Path: "main.go", Rev: RevIndex, Truncated: true},
			want:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.content.IsEmpty(); got != tc.want {
				t.Errorf("IsEmpty() got = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFileRevZeroIsInvalid(t *testing.T) {
	var zero FileRev

	if zero.Valid() {
		t.Error("zero FileRev.Valid() got = true, want false")
	}
	if got := zero.String(); got != "" {
		t.Errorf("zero FileRev.String() got = %q, want %q", got, "")
	}
	if RevIndex == 0 || RevHead == 0 {
		t.Errorf("RevIndex = %d, RevHead = %d, want both outside the zero value", RevIndex, RevHead)
	}
}
