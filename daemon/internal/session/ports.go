package session

import "time"

// Clock is the domain's only source of time. Production code supplies the
// real wall clock; tests supply FakeClock. Elapsed time is always measured
// as a wall-clock difference (now - lastTick), never via time.Since or a
// monotonic reading, because the monotonic clock's behavior across system
// suspend is system-dependent (research.md C1).
type Clock interface {
	Now() time.Time
}

// TierSource reports what other people can currently see. dbusapi.TierDetector
// implements it from the compositor, /proc and PipeWire; a signal that cannot
// be read counts as absent, so the answer can only err toward T0.
type TierSource interface {
	CurrentTier() Tier
}

// IdleSource reports how long the user has been idle at the input level
// (keyboard/mouse), independent of system suspend. M1 stubs this to always
// report zero idle; the Mutter IdleMonitor adapter is M4.
type IdleSource interface {
	IdleFor(now time.Time) time.Duration
}

// Store persists and restores durable session state. It stores
// ElapsedInPhase, never a deadline (see state.go).
type Store interface {
	Load() (State, bool, error)
	Save(State) error
}
