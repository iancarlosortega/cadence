// Package session is the domain of cadence: a phase-cycling timer with
// idle-credit, pause, and suspend semantics. It imports nothing beyond the
// standard library — no D-Bus, no GNOME, no direct call to time.Now. All
// time enters through the Clock port and the now parameter of Apply.
package session

import "time"

// Phase is the current stage of a session.
type Phase string

const (
	PhaseNone  Phase = "none" // no active session
	PhaseFocus Phase = "focus"
	PhaseBreak Phase = "break"
)

// Tier describes what other people can currently see (T0-T3, see
// specs/session-timer). M1 only ever observes TierT0; T1-T3 are defined
// here so the type is stable across milestones, but no rule in Apply
// branches on them yet beyond "T0 starts the break".
type Tier string

const (
	TierT0 Tier = "T0" // free
	TierT1 Tier = "T1" // listening only
	TierT2 Tier = "T2" // on camera
	TierT3 Tier = "T3" // presenting
)

// Durations holds the configured phase and idle thresholds. Populated from
// config; never hardcoded in the state machine.
type Durations struct {
	Focus      time.Duration
	Break      time.Duration
	IdlePause  time.Duration // idle time before the timer stops advancing
	IdleCredit time.Duration // idle time (or suspend) that credits a break
}

// State is the full session state. It is immutable from the caller's point
// of view: Apply returns a new State rather than mutating this one.
type State struct {
	Active bool
	Phase  Phase

	// ElapsedInPhase is the durable quantity. It is never a deadline:
	// deadlines computed before a suspend resume already-expired, because
	// Go's monotonic clock is system-dependent across sleep (see
	// openspec/changes/m1-daemon-core/research.md, claims C1-C2).
	ElapsedInPhase time.Duration

	Paused          bool
	PausedRemaining time.Duration // valid only while Paused

	// Idle reports that an idle window is open: the user has been idle at
	// least Durations.IdlePause, so elapsed time is not advancing and the
	// adapter publishes the phase as a frozen interval (PhaseEndsAt 0 plus
	// a frozen RemainingSeconds), exactly as it does for Paused. No
	// "IdleRemaining" companion is needed: ElapsedInPhase is frozen for the
	// window's duration, so Remaining() is already constant throughout it.
	//
	// Paused takes precedence: Idle is false whenever Paused is true, so
	// only one frozen-interval condition is ever published
	// (specs/daemon-control, "Idle Publication").
	Idle bool

	// IdleCredited reports that the open window has already credited a
	// break. Observed idle time keeps growing while the user is away and is
	// never reset by crediting — it is the compositor's number, not ours —
	// so without this latch every tick past the credit threshold would
	// credit again, resetting the phase and writing state once per tick for
	// the whole absence (specs/session-timer, "Idle Credit").
	IdleCredited bool

	// Neither flag is persisted. Both are derived from a live idle reading
	// and are re-derived within one tick of startup; a persisted latch could
	// outlive the absence that set it and suppress a break the user earned.
	// See store/file.go, which maps State onto an explicit record.

	// LastObserved is the wall-clock instant Apply last advanced this
	// state. Every EventTick and EventSuspended measures elapsed real
	// time as now.Sub(LastObserved) — never via time.Since or a
	// monotonic reading (research.md C1-C2).
	LastObserved time.Time

	Tier Tier

	Durations Durations
}

// Effect is something Apply wants performed outside the domain: persist
// state, or notify a D-Bus client. Apply never performs effects itself.
type Effect interface{ isEffect() }

// EffectPersist asks the caller to durably store the returned State.
type EffectPersist struct{ Reason string }

// EffectNotify asks the caller to publish the returned State's public
// fields (Phase, PhaseEndsAt derived from it, Paused, Tier, SessionActive)
// as a D-Bus PropertiesChanged. Emitted only on real transitions — never
// once per tick (see specs/daemon-control, "No per-second traffic").
type EffectNotify struct{ Reason string }

func (EffectPersist) isEffect() {}
func (EffectNotify) isEffect()  {}

// PhaseDuration returns the configured length of the given phase.
func (d Durations) PhaseDuration(p Phase) time.Duration {
	switch p {
	case PhaseFocus:
		return d.Focus
	case PhaseBreak:
		return d.Break
	default:
		return 0
	}
}

// Remaining returns how much of the current phase is left, given the
// configured duration for that phase. It does not consult a deadline —
// the caller derives PhaseEndsAt as now+Remaining() only when publishing.
func (s State) Remaining() time.Duration {
	total := s.Durations.PhaseDuration(s.Phase)
	left := total - s.ElapsedInPhase
	if left < 0 {
		return 0
	}
	return left
}
