package domain

import (
	"testing"
	"time"
)

func TestLogOptionsNormalized(t *testing.T) {
	cases := []struct {
		name     string
		tail     int
		wantTail int
	}{
		{name: "zero", tail: 0, wantTail: DefaultLogTail},
		{name: "negative", tail: -5, wantTail: DefaultLogTail},
		{name: "kept", tail: 250, wantTail: 250},
		{name: "one", tail: 1, wantTail: 1},
		{name: "at max", tail: MaxLogLines, wantTail: MaxLogLines},
		{name: "above max", tail: MaxLogLines + 1, wantTail: MaxLogLines},
		{name: "huge", tail: 1 << 30, wantTail: MaxLogLines},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LogOptions{Tail: tc.tail}.Normalized()
			if got.Tail != tc.wantTail {
				t.Errorf("Tail = %d, want %d", got.Tail, tc.wantTail)
			}
			if !got.Before.IsZero() {
				t.Errorf("Before = %v, want zero", got.Before)
			}
		})
	}
}

func TestLogOptionsNormalizedKeepsBefore(t *testing.T) {
	before := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)

	got := LogOptions{Tail: MaxLogLines * 2, Before: before}.Normalized()
	if !got.Before.Equal(before) {
		t.Errorf("Before = %v, want %v", got.Before, before)
	}
	if got.Tail != MaxLogLines {
		t.Errorf("Tail = %d, want %d", got.Tail, MaxLogLines)
	}
}

func TestLogDefaultsAreSane(t *testing.T) {
	if DefaultLogTail <= 0 {
		t.Errorf("DefaultLogTail = %d, want a positive default", DefaultLogTail)
	}
	if DefaultLogTail > MaxLogLines {
		t.Errorf("DefaultLogTail = %d exceeds MaxLogLines = %d", DefaultLogTail, MaxLogLines)
	}
}
