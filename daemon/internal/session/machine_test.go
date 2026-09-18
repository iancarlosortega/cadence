package session

import (
	"testing"
	"time"
)

func testDurations() Durations {
	return Durations{
		Focus:      50 * time.Minute,
		Break:      10 * time.Minute,
		IdlePause:  3 * time.Minute,
		IdleCredit: 10 * time.Minute,
	}
}

func startedFocus(now time.Time, elapsed time.Duration) State {
	s := State{Durations: testDurations()}
	s, _ = Apply(s, EventStartSession{}, now)
	s.ElapsedInPhase = elapsed
	s.LastObserved = now
	return s
}

// specs/session-timer, Requirement "Phase Cycle", Scenario "Focus elapses".
func TestFocusElapses(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, testDurations().Focus) // 0s remaining

	next, effects := Apply(s, EventTick{Tier: TierT0}, now.Add(time.Second))

	if next.Phase != PhaseBreak {
		t.Fatalf("phase = %s, want break", next.Phase)
	}
	if len(effects) == 0 {
		t.Fatal("want persist+notify effects on transition")
	}
}

// specs/session-timer, Requirement "Phase Cycle", Scenario "Break elapses".
func TestBreakElapses(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 0)
	s.Phase = PhaseBreak
	s.ElapsedInPhase = testDurations().Break

	next, _ := Apply(s, EventTick{Tier: TierT0}, now.Add(time.Second))

	if next.Phase != PhaseFocus {
		t.Fatalf("phase = %s, want focus", next.Phase)
	}
}

// specs/session-timer, Requirement "Idle Credit", Scenario "Short idle pauses".
func TestShortIdlePauses(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 20*time.Minute)

	next, effects := Apply(s, EventTick{IdleFor: testDurations().IdlePause + time.Minute, Tier: TierT0}, now.Add(time.Minute))

	if next.ElapsedInPhase != 20*time.Minute {
		t.Fatalf("elapsed = %s, want unchanged 20m", next.ElapsedInPhase)
	}
	if len(effects) != 0 {
		t.Fatalf("want no effect for a non-observable idle pause, got %d", len(effects))
	}
}

// specs/session-timer, Requirement "Idle Credit", Scenario "Long idle credits".
func TestLongIdleCredits(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 20*time.Minute)

	next, effects := Apply(s, EventTick{IdleFor: testDurations().IdleCredit, Tier: TierT0}, now.Add(time.Minute))

	if next.Phase != PhaseFocus || next.ElapsedInPhase != 0 {
		t.Fatalf("got phase=%s elapsed=%s, want focus/0m", next.Phase, next.ElapsedInPhase)
	}
	if len(effects) == 0 {
		t.Fatal("want persist+notify on a credited break")
	}
}

// specs/session-timer, Requirement "Pause Semantics", Scenario
// "Paused phase does not expire".
func TestPausedPhaseDoesNotExpire(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 40*time.Minute) // 10m remaining
	s, _ = Apply(s, EventPause{}, now)

	if !s.Paused || s.PausedRemaining != 10*time.Minute {
		t.Fatalf("pause did not capture remaining correctly: paused=%v remaining=%s", s.Paused, s.PausedRemaining)
	}

	next, effects := Apply(s, EventTick{Tier: TierT0}, now.Add(3*time.Hour))

	if next.Phase != PhaseFocus || !next.Paused || next.PausedRemaining != 10*time.Minute {
		t.Fatalf("paused state changed across a 3h gap: %+v", next)
	}
	if len(effects) != 0 {
		t.Fatalf("want no effect from a tick while paused, got %d", len(effects))
	}
}

// specs/session-timer, Requirement "Suspend Is Time Away", Scenario
// "Long suspend credits a break".
func TestLongSuspendCreditsBreak(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 12*time.Minute)

	resumeAt := now.Add(time.Hour)
	next, effects := Apply(s, EventSuspended{From: now, To: resumeAt}, resumeAt)

	if next.Phase != PhaseFocus || next.ElapsedInPhase != 0 {
		t.Fatalf("got phase=%s elapsed=%s, want focus/0m after a 1h suspend", next.Phase, next.ElapsedInPhase)
	}
	if len(effects) == 0 {
		t.Fatal("want persist+notify on a credited break")
	}
}

func hasNotify(effects []Effect) bool {
	for _, e := range effects {
		if _, ok := e.(EffectNotify); ok {
			return true
		}
	}
	return false
}

// specs/session-timer, Requirement "Suspend Is Time Away", Scenario
// "Short suspend does not credit".
func TestShortSuspendDoesNotCredit(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 12*time.Minute)

	resumeAt := now.Add(90 * time.Second)
	next, effects := Apply(s, EventSuspended{From: now, To: resumeAt}, resumeAt)

	if next.ElapsedInPhase != 12*time.Minute {
		t.Fatalf("elapsed = %s, want unchanged 12m after a 90s suspend", next.ElapsedInPhase)
	}
	if !hasNotify(effects) {
		t.Fatal("want EffectNotify: elapsed did not advance, so the deadline moved")
	}
}

// specs/daemon-control, Requirement "Change Notification", Scenario
// "Short suspend republishes the deadline".
func TestShortSuspendRepublishesDeadline(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 12*time.Minute)

	resumeAt := now.Add(90 * time.Second)
	next, effects := Apply(s, EventSuspended{From: now, To: resumeAt}, resumeAt)

	if !hasNotify(effects) {
		t.Fatal("want EffectNotify so PhaseEndsAt is recomputed on resume")
	}

	got := resumeAt.Add(next.Remaining())
	want := resumeAt.Add(testDurations().Focus - 12*time.Minute)
	if !got.Equal(want) {
		t.Fatalf("republished deadline = %s, want %s", got, want)
	}

	stale := now.Add(testDurations().Focus - 12*time.Minute)
	if got.Equal(stale) {
		t.Fatal("deadline did not move: a client would stay 90s ahead of the daemon")
	}
}

// specs/session-timer, Requirement "Tier Gating", Scenario "T0 starts break".
func TestT0StartsBreak(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, testDurations().Focus)

	next, _ := Apply(s, EventTick{Tier: TierT0}, now.Add(time.Second))

	if next.Phase != PhaseBreak {
		t.Fatalf("phase = %s, want break under tier T0", next.Phase)
	}
}

// Not a spec scenario, but the property specs/daemon-control depends on:
// no PropertiesChanged-worthy effect fires from ticks with no transition.
func TestNoEffectOnQuietTick(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 5*time.Minute)

	_, effects := Apply(s, EventTick{Tier: TierT0}, now.Add(30*time.Second))

	if len(effects) != 0 {
		t.Fatalf("want no effect from an ordinary mid-phase tick, got %d", len(effects))
	}
}

func TestStartSessionWhenAlreadyActiveIsNoop(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 7*time.Minute)

	next, effects := Apply(s, EventStartSession{}, now.Add(time.Minute))

	if next.ElapsedInPhase != 7*time.Minute {
		t.Fatalf("elapsed changed on a redundant start: %s", next.ElapsedInPhase)
	}
	if len(effects) != 0 {
		t.Fatalf("want no effect from a redundant start, got %d", len(effects))
	}
}
