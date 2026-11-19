package docker

import "time"

// clock isolates the debounce timers so the tests never sleep.
type clock interface {
	Now() time.Time
	NewTimer(d time.Duration) timer
}

// timer is the one-shot timer the debounce uses.
type timer interface {
	C() <-chan time.Time
	Stop() bool
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTimer(d time.Duration) timer { return realTimer{timer: time.NewTimer(d)} }

type realTimer struct {
	timer *time.Timer
}

func (t realTimer) C() <-chan time.Time { return t.timer.C }

func (t realTimer) Stop() bool { return t.timer.Stop() }
