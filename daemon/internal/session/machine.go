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
		// Paused outranks Idle, so a window open at this instant closes
		// here rather than publishing both frozen conditions at once
		// (specs/daemon-control, "Idle Publication").
		ns.Idle = false
		ns.IdleCredited = false
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
		ns := release(s) // a held break is released with its count (design D5)
		ns.Phase = PhaseFocus
		ns.ElapsedInPhase = 0
		ns.Paused = false
		ns.LastObserved = now
		return ns, []Effect{EffectPersist{Reason: "break skipped"}, EffectNotify{Reason: "break skipped"}}

	case EventTick:
		ns, effects := applyTick(s, ev, now)
		// A tier-only change publishes (specs/daemon-control, "Change
		// Notification": "A tier change alone emits once"; design D6). Doing
		// it here, once, covers every branch of applyTick, including the
		// ones that return no effects, rather than patching each. Nothing is
		// persisted: tier is sampled live (design D7).
		if ns.Tier != s.Tier && !hasNotifyEffect(effects) {
			effects = append(effects, EffectNotify{Reason: "tier changed"})
		}
		return ns, effects

	case EventSuspended:
		return applySuspend(s, ev)

	case EventConfigChanged:
		return applyConfig(s, ev)
	}

	return s, nil
}

// hasNotifyEffect reports whether effects already carries a publish.
func hasNotifyEffect(effects []Effect) bool {
	for _, e := range effects {
		if _, ok := e.(EffectNotify); ok {
			return true
		}
	}
	return false
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
	// Presenting ends a break in progress, exactly as skipping it does
	// (specs/session-timer, "Tier Gating": "Presenting during a break ends
	// it"; design D5). It is the first case so that presenting wins over
	// every idle rule on the same tick: a break must not outlive a screen
	// share. It reads the tick's tier, not the stored one, so there is no
	// one-tick lag.
	case s.Phase == PhaseBreak && ev.Tier == TierT3:
		ns = release(ns) // a held break ends here too (design D5)
		ns.Phase = PhaseFocus
		ns.ElapsedInPhase = 0
		ns.LastObserved = now
		return ns, []Effect{
			EffectPersist{Reason: "break ended by screen share"},
			EffectNotify{Reason: "break ended by screen share"},
		}

	// The latch is the whole reason this case is conditional. ev.IdleFor
	// grows for as long as the user is away and crediting does not reset
	// it, so an unconditional credit fires on every tick of the absence
	// (specs/session-timer, "Idle Credit": a single idle window credits at
	// most one break).
	case ev.IdleFor >= s.Durations.IdleCredit && !s.IdleCredited:
		credited, effects := creditBreak(s, now)
		credited.Idle = true
		credited.IdleCredited = true
		return credited, effects

	// Lift (specs/session-timer, "Tier Gating": "The camera turning off
	// starts the break"; design D4 case 3). It sits after idle credit so an
	// absence is credited like any other, and before the re-hold and retry
	// cases so a hold never survives a tier below T2. The held gap is not
	// charged: the break resumes from this instant with the remainder it was
	// held at. The prompt count is kept so a flapping camera cannot escape
	// the cap (design D3).
	case s.Phase == PhaseBreak && s.Held() && ev.Tier != TierT2:
		ns.Hold = HoldNone
		ns.SincePrompt = 0
		ns.LastObserved = now
		return ns, []Effect{
			EffectPersist{Reason: "hold lifted"},
			EffectNotify{Reason: "hold lifted"},
		}

	// Re-hold: a break already running when the camera turns on becomes held
	// with its elapsed time intact ("The camera turning on mid-break holds
	// it"; design D4 case 4). hold() resumes the prompt count rather than
	// resetting it, so re-entry at the cap goes straight to the pill.
	case s.Phase == PhaseBreak && !s.Held() && ev.Tier == TierT2:
		ns = hold(ns)
		ns.LastObserved = now
		return ns, []Effect{
			EffectPersist{Reason: "break held"},
			EffectNotify{Reason: "break held"},
		}

	// Retry clock (design D4 case 5). Elapsed does not advance; only the
	// time since the last prompt does. Ticks between prompts emit nothing
	// (specs/daemon-control, "Hold Publication": the ticks between prompts
	// must not emit), so only the edges below return effects. Paused ticks
	// never reach here, which freezes the clock for free (design D6).
	case s.Phase == PhaseBreak && s.Held():
		ns.LastObserved = now
		if s.Hold == HoldPill {
			return ns, nil // nothing further happens past the cap
		}
		ns.SincePrompt = s.SincePrompt + max(now.Sub(s.LastObserved), 0)
		if ns.SincePrompt < s.Durations.PromptRetry {
			return ns, nil
		}
		ns.SincePrompt = 0
		if ns.Prompts < s.Durations.PromptCap {
			ns.Prompts++
			return ns, []Effect{
				EffectPersist{Reason: "break prompt"},
				EffectNotify{Reason: "break prompt"},
			}
		}
		ns.Hold = HoldPill
		return ns, []Effect{
			EffectPersist{Reason: "hold became pill"},
			EffectNotify{Reason: "hold became pill"},
		}

	// Focus only. A break is time away from the desk by design, so going
	// idle during one is the user doing exactly what it asked: the break
	// must keep running on wall-clock time and finish while they are gone.
	// Freezing it would leave the remainder owed on their return, so a
	// break taken properly would be the one that never completes
	// (specs/session-timer, "Idle Credit").
	case ev.IdleFor >= s.Durations.IdlePause && s.Phase == PhaseFocus:
		// Elapsed time does not advance here, which moves the effective
		// deadline for as long as the window stays open. The window's
		// edges are therefore observable and are published; the ticks
		// between them are not (specs/daemon-control, "Change
		// Notification"). Emitting per tick here would be the per-second
		// heartbeat that requirement forbids.
		ns.LastObserved = now
		if s.Idle {
			return ns, nil
		}
		ns.Idle = true
		return ns, []Effect{
			EffectPersist{Reason: "idle window opened"},
			EffectNotify{Reason: "idle window opened"},
		}

	default:
		// Spend the latch only on real activity. Reaching here while
		// still idle means the phase is a break, which does not freeze —
		// the absence is not over, so its credit must not be re-armed.
		// Clearing on activity rather than only on the window-close edge
		// also covers a break credited by applySuspend, which sets the
		// latch without opening a window.
		if ev.IdleFor < s.Durations.IdlePause {
			ns.IdleCredited = false
		}

		if s.Idle {
			// Closing edge. The gap since the last observation was idle
			// time and is not charged; the phase resumes from this
			// instant with the remainder it was frozen at
			// (specs/session-timer, "Idle Credit").
			ns.Idle = false
			ns.LastObserved = now
			return ns, []Effect{
				EffectPersist{Reason: "idle window closed"},
				EffectNotify{Reason: "idle window closed"},
			}
		}

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
		credited, effects := creditBreak(s, ev.To)
		// The same absence may also be visible to the idle source on the
		// next tick. Latching here means it is credited once whether or
		// not the compositor's idle reading accrues across suspend, which
		// is undocumented upstream (design.md, Decision 2). Idle stays
		// false: the user may well be back at the keyboard on resume, and
		// the next tick decides that from a real reading.
		credited.IdleCredited = true
		return credited, effects
	}

	// Declining to charge the suspend moves the phase deadline, so clients
	// counting from the published PhaseEndsAt must be told (specs/daemon-control).
	ns := s
	ns.LastObserved = ev.To
	return ns, []Effect{
		EffectPersist{Reason: "resumed from short suspend"},
		EffectNotify{Reason: "resumed from short suspend"},
	}
}

// applyConfig swaps in a reloaded policy (specs/daemon-configuration, "Live
// Reload"; design D5). It never transitions: a new length at or below the time
// already worked is caught by the next tick's default branch, which already
// ends a phase once ElapsedInPhase >= PhaseDuration. That keeps one
// transition path, and an idle-frozen phase keeps its frozen semantics.
func applyConfig(s State, ev EventConfigChanged) (State, []Effect) {
	if ev.Durations == s.Durations {
		return s, nil // a save that changes nothing emits nothing
	}
	ns := s
	ns.Durations = ev.Durations
	if s.Paused {
		// The elapsed time at the pause is the old length less what was left;
		// it is what must survive, so the frozen remainder is recomputed from
		// the new length ("A paused phase keeps its elapsed time").
		elapsed := s.Durations.PhaseDuration(s.Phase) - s.PausedRemaining
		ns.PausedRemaining = max(ev.Durations.PhaseDuration(s.Phase)-elapsed, 0)
	}
	// PhaseEndsAt, RemainingSeconds and Config all changed, so publish; persist
	// because the stored record carries the four duration minutes.
	return ns, []Effect{EffectPersist{Reason: "config changed"}, EffectNotify{Reason: "config changed"}}
}

// creditBreak resets to a fresh focus phase, as if the user had just taken
// (and finished) a break by being away — idle or suspended — for at least
// the credit threshold.
func creditBreak(s State, at time.Time) (State, []Effect) {
	ns := release(s) // idle, suspend and downtime credit all release a hold (design D5)
	ns.Phase = PhaseFocus
	ns.ElapsedInPhase = 0
	ns.Paused = false
	ns.PausedRemaining = 0
	ns.LastObserved = at
	return ns, []Effect{EffectPersist{Reason: "break credited"}, EffectNotify{Reason: "break credited"}}
}

// hold puts a break that has just started, or is already running, into a hold
// (specs/session-timer, "Tier Gating"; design D3). It is the only place a
// prompt is counted on entry, and it resumes the count rather than
// resetting it: past the cap the hold starts as a pill, with no new prompt.
// The caller has already set Phase to break.
func hold(ns State) State {
	if ns.Prompts < ns.Durations.PromptCap {
		ns.Prompts++
		ns.Hold = HoldPrompt
	} else {
		ns.Hold = HoldPill
	}
	ns.SincePrompt = 0
	return ns
}

// release clears every hold field. It is the one place that ends a hold for
// good, called from every path that ends or credits a break (design D5):
// creditBreak, skip, T3, and a break that completes normally. Stopping a
// session builds a fresh State and so releases implicitly.
func release(ns State) State {
	ns.Hold = HoldNone
	ns.Prompts = 0
	ns.SincePrompt = 0
	return ns
}

// transitionPhase flips focus<->break once the current phase's duration is
// exhausted through active ticking. Tier gating (specs/session-timer,
// "Tier Gating") is consulted here: T3 (presenting) withholds the break and
// the phase stays focus with a fresh block; T2 (on camera) starts the break
// held, so the user is asked rather than interrupted (design D3); T0 and T1
// start it normally.
func transitionPhase(s State, now time.Time) (State, []Effect) {
	ns := s
	if s.Phase == PhaseFocus {
		if s.Tier != TierT3 {
			ns.Phase = PhaseBreak
			if s.Tier == TierT2 {
				ns = hold(ns)
			}
		}
	} else {
		ns = release(ns) // a normal completion, possibly after a lift (design D5)
		ns.Phase = PhaseFocus
	}
	ns.ElapsedInPhase = 0
	ns.LastObserved = now
	return ns, []Effect{EffectPersist{Reason: "phase transition"}, EffectNotify{Reason: "phase transition"}}
}
