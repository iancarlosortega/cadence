package session

import "time"

// RealClock is the production Clock: the OS wall clock.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// FakeClock is a test double. It never reads the OS clock, so every
// session-timer scenario runs instantly and deterministically.
type FakeClock struct {
	now time.Time
}

// NewFakeClock returns a FakeClock starting at t.
func NewFakeClock(t time.Time) *FakeClock { return &FakeClock{now: t} }

func (c *FakeClock) Now() time.Time { return c.now }

// Advance moves the fake clock forward by d and returns the new time.
func (c *FakeClock) Advance(d time.Duration) time.Time {
	c.now = c.now.Add(d)
	return c.now
}
