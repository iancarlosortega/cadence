package session

import "time"

// Apply is the domain's single entry point: a pure, total function from
// (current state, one event, the wall-clock instant it happened) to (next
// state, effects for the caller to perform). It never calls time.Now,
// never touches D-Bus, and never touches disk — see ports.go for how the
// caller supplies time, tier, and idle observations.
//
// now is the wall-clock instant supplied by the caller's Clock port. Elapsed
// time is always computed as a wall-clock difference against
// State.LastObserved, never via a monotonic reading or a stored deadline,
// because Go's monotonic clock is system-dependent across suspend
// (see openspec/changes/m1-daemon-core/research.md, claims C1-C2).
func Apply(s State, e Event, now time.Time) (State, []Effect) {
	switch ev := e.(type) {

	case EventStartSession:
		if s.Active {
			return s, nil // specs/daemon-control: "Start when already active" is a no-op
		}
		ns := State{
			Active:         true,
			Phase:          PhaseFocus,
			ElapsedInPhase: 0,
			Tier:           TierT0,
			Durations:      s.Durations,
			LastObserved:   now,
		}
		return ns, []Effect{EffectPersist{Reason: "session started"}, EffectNotify{Reason: "session started"}}

	case EventStopSession:
		if !s.Active {
			return s, nil
		}
		ns := State{Active: false, Phase: PhaseNone, Durations: s.Durations}
		return ns, []Effect{EffectPersist{Reason: "session stopped"}, EffectNotify{Reason: "session stopped"}}

	case EventPause:
		if !s.Active || s.Paused {
			return s, nil
		}
		ns := s
		ns.Paused = true
		ns.PausedRemaining = s.Remaining()
		ns.LastObserved = now
		return ns, []Effect{EffectPersist{Reason: "paused"}, EffectNotify{Reason: "paused"}}

	case EventResume:
		if !s.Active || !s.Paused {
			return s, nil
		}
		ns := s
		ns.Paused = false
		total := s.Durations.PhaseDuration(s.Phase)
		ns.ElapsedInPhase = total - s.PausedRemaining
		ns.PausedRemaining = 0
		ns.LastObserved = now
		return ns, []Effect{EffectPersist{Reason: "resumed"}, EffectNotify{Reason: "resumed"}}

	case EventSkipBreak:
		if !s.Active || s.Phase != PhaseBreak {
			return s, nil
		}
		ns := s
		ns.Phase = PhaseFocus
		ns.ElapsedInPhase = 0
		ns.Paused = false
		ns.LastObserved = now
		return ns, []Effect{EffectPersist{Reason: "break skipped"}, EffectNotify{Reason: "break skipped"}}

	case EventTick:
		return applyTick(s, ev, now)

	case EventSuspended:
		return applySuspend(s, ev)
	}

	return s, nil
}

// applyTick advances the clock by whatever real time passed since
// s.LastObserved, subject to the idle-pause and idle-credit rules
// (specs/session-timer, "Idle Credit"). A paused phase never advances —
// it is not represented via a deadline, so a 3-hour gap between ticks
// leaves it untouched (specs/session-timer, "Pause Semantics").
func applyTick(s State, ev EventTick, now time.Time) (State, []Effect) {
	if !s.Active || s.Paused {
		return s, nil
	}

	ns := s
	ns.Tier = ev.Tier

	switch {
	case ev.IdleFor >= s.Durations.IdleCredit:
		return creditBreak(s, now)

	case ev.IdleFor >= s.Durations.IdlePause:
		// Idle beyond the pause threshold but short of the credit
		// threshold: elapsed time does not advance, nothing observable
		// changed, so no effect is emitted.
		ns.LastObserved = now
		return ns, nil

	default:
		delta := max(now.Sub(s.LastObserved), 0)
		ns.ElapsedInPhase = s.ElapsedInPhase + delta
		ns.LastObserved = now

		if ns.ElapsedInPhase >= s.Durations.PhaseDuration(s.Phase) {
			return transitionPhase(ns, now)
		}
		return ns, nil
	}
}

// applySuspend treats a system suspend exactly like idle time of the same
// duration, reusing the credit threshold rather than introducing a second
// concept (specs/session-timer, "Suspend Is Time Away"; confirmed by the
// user: suspended time is time away from the desk).
func applySuspend(s State, ev EventSuspended) (State, []Effect) {
	if !s.Active {
		return s, nil
	}

	duration := max(ev.To.Sub(ev.From), 0)

	if duration >= s.Durations.IdleCredit {
		return creditBreak(s, ev.To)
	}

	// Short suspend: ordinary idle, elapsed does not advance. Still
	// persisted so the on-disk LastObserved reflects the resume instant.
	ns := s
	ns.LastObserved = ev.To
	return ns, []Effect{EffectPersist{Reason: "resumed from short suspend"}}
}

// creditBreak resets to a fresh focus phase, as if the user had just taken
// (and finished) a break by being away — idle or suspended — for at least
// the credit threshold.
func creditBreak(s State, at time.Time) (State, []Effect) {
	ns := s
	ns.Phase = PhaseFocus
	ns.ElapsedInPhase = 0
	ns.Paused = false
	ns.PausedRemaining = 0
	ns.LastObserved = at
	return ns, []Effect{EffectPersist{Reason: "break credited"}, EffectNotify{Reason: "break credited"}}
}

// transitionPhase flips focus<->break once the current phase's duration is
// exhausted through active ticking. Tier gating (specs/session-timer,
// "Tier Gating") is consulted here: M1's TierSource is stubbed to always
// report TierT0, so the break always starts; the T1-T3 policies (silent
// skip, deferred corner panel, muted overlay) are M5 behavior layered on
// top of this same decision point.
func transitionPhase(s State, now time.Time) (State, []Effect) {
	ns := s
	if s.Phase == PhaseFocus {
		if s.Tier == TierT0 {
			ns.Phase = PhaseBreak
		}
		// Non-T0 tiers: M1 has no adapter that ever reports them, so no
		// rule is needed yet. A future milestone extends this switch.
	} else {
		ns.Phase = PhaseFocus
	}
	ns.ElapsedInPhase = 0
	ns.LastObserved = now
	return ns, []Effect{EffectPersist{Reason: "phase transition"}, EffectNotify{Reason: "phase transition"}}
}
