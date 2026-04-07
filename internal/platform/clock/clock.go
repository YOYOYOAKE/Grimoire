package clock

import "time"

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

type FixedClock struct {
	now time.Time
}

func NewSystemClock() SystemClock {
	return SystemClock{}
}

func NewFixedClock(now time.Time) FixedClock {
	return FixedClock{now: now}
}

func (SystemClock) Now() time.Time {
	return time.Now()
}

func (c FixedClock) Now() time.Time {
	return c.now
}
