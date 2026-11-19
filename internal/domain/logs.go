package domain

import "time"

type LogStream uint8

const (
	Stdout LogStream = iota
	Stderr
)

type LogLine struct {
	Timestamp time.Time
	Stream    LogStream
	Text      string
}

// LogOptions is the pagination window of a log read; there is no follow mode.
type LogOptions struct {
	Tail   int
	Before time.Time
}

// Normalized clamps Tail into the domain range, leaving Before untouched.
func (o LogOptions) Normalized() LogOptions {
	tail := o.Tail
	switch {
	case tail <= 0:
		tail = DefaultLogTail
	case tail > MaxLogLines:
		tail = MaxLogLines
	}

	return LogOptions{Tail: tail, Before: o.Before}
}

// LogPage is one page of logs; NextBefore is the cursor of the previous page.
type LogPage struct {
	Lines      []LogLine
	NextBefore time.Time
	Truncated  bool
}
