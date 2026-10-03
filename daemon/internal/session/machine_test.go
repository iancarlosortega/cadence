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
//
// This assertion was inverted by M4. It previously required no effect at
// all, on the reasoning that nothing observable had changed. That was
// wrong: elapsed stops advancing while wall time does not, so the
// effective deadline moves for the whole window and a client counting
// down from the last published PhaseEndsAt drifts by its full width.
func TestShortIdlePauses(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 20*time.Minute)

	next, effects := Apply(s, EventTick{IdleFor: testDurations().IdlePause + time.Minute, Tier: TierT0}, now.Add(time.Minute))

	if next.ElapsedInPhase != 20*time.Minute {
		t.Fatalf("elapsed = %s, want unchanged 20m", next.ElapsedInPhase)
	}
	if !next.Idle {
		t.Fatal("want Idle true: the window is open")
	}
	if !hasNotify(effects) {
		t.Fatal("want EffectNotify on the opening edge: the deadline moved")
	}
}

// specs/daemon-control, Requirement "Change Notification", Scenario
// "An idle window emits exactly twice". This is the half that forbids a
// per-tick heartbeat inside the window.
func TestTicksInsideAnIdleWindowAreSilent(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 20*time.Minute)
	idle := testDurations().IdlePause + time.Minute

	s, _ = Apply(s, EventTick{IdleFor: idle, Tier: TierT0}, now.Add(time.Minute))

	for i := 1; i <= 5; i++ {
		at := now.Add(time.Minute + time.Duration(i)*5*time.Second)
		next, effects := Apply(s, EventTick{IdleFor: idle + time.Duration(i)*5*time.Second, Tier: TierT0}, at)
		if len(effects) != 0 {
			t.Fatalf("tick %d inside an open window emitted %d effects, want 0", i, len(effects))
		}
		s = next
	}
}

// specs/session-timer, Requirement "Idle Credit", Scenario "A long
// absence credits exactly one break".
//
// The defect this pins: observed idle grows for as long as the user is
// away and crediting never resets it, so an unlatched credit branch fires
// on every tick of the absence — resetting the phase and writing state
// once per tick for its whole duration.
func TestLongAbsenceCreditsExactlyOneBreak(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 20*time.Minute)
	credit := testDurations().IdleCredit

	credits := 0
	// One hour of absence at the daemon's 5s tick interval.
	for i := 0; i <= 720; i++ {
		at := now.Add(time.Duration(i) * 5 * time.Second)
		idleFor := time.Duration(i) * 5 * time.Second
		next, effects := Apply(s, EventTick{IdleFor: idleFor, Tier: TierT0}, at)
		if idleFor >= credit && next.ElapsedInPhase == 0 && s.ElapsedInPhase != 0 {
			credits++
		}
		if idleFor > credit+time.Minute && len(effects) != 0 {
			t.Fatalf("tick at idle=%s emitted %d effects after the credit, want 0", idleFor, len(effects))
		}
		s = next
	}

	if credits != 1 {
		t.Fatalf("credited %d breaks across a one-hour absence, want exactly 1", credits)
	}
}

// specs/session-timer, Requirement "Idle Credit", Scenario "A second
// absence credits again". The latch must be spent by returning, not
// permanent.
func TestSecondAbsenceCreditsAgain(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 20*time.Minute)
	credit := testDurations().IdleCredit

	s, _ = Apply(s, EventTick{IdleFor: credit, Tier: TierT0}, now.Add(credit))
	if !s.IdleCredited {
		t.Fatal("want the latch set after the first credit")
	}

	back := now.Add(credit + 5*time.Second)
	s, _ = Apply(s, EventTick{IdleFor: 0, Tier: TierT0}, back)
	if s.IdleCredited || s.Idle {
		t.Fatalf("want both flags cleared on return, got idle=%v credited=%v", s.Idle, s.IdleCredited)
	}

	s.ElapsedInPhase = 20 * time.Minute
	next, _ := Apply(s, EventTick{IdleFor: credit, Tier: TierT0}, back.Add(credit))
	if next.ElapsedInPhase != 0 {
		t.Fatalf("elapsed = %s after a second absence, want a second credited break", next.ElapsedInPhase)
	}
}

// specs/session-timer, Requirement "Idle Credit", Scenario "Returning
// before the credit threshold resumes the same phase".
func TestReturnBeforeCreditResumesSamePhase(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 20*time.Minute)

	s, _ = Apply(s, EventTick{IdleFor: testDurations().IdlePause, Tier: TierT0}, now.Add(3*time.Minute))

	back := now.Add(8 * time.Minute)
	next, effects := Apply(s, EventTick{IdleFor: 0, Tier: TierT0}, back)

	if next.Idle {
		t.Fatal("want the window closed on return")
	}
	if next.ElapsedInPhase != 20*time.Minute {
		t.Fatalf("elapsed = %s on return, want the frozen 20m — idle time must not be charged", next.ElapsedInPhase)
	}
	if !hasNotify(effects) {
		t.Fatal("want EffectNotify on the closing edge: the deadline is live again")
	}

	after := back.Add(time.Minute)
	resumed, _ := Apply(next, EventTick{IdleFor: 0, Tier: TierT0}, after)
	if resumed.ElapsedInPhase != 21*time.Minute {
		t.Fatalf("elapsed = %s, want 21m — the timer resumes from the return instant", resumed.ElapsedInPhase)
	}
}

// specs/daemon-control, Requirement "Idle Publication", Scenario "Pause
// during an idle window". Paused outranks Idle so only one frozen
// condition is ever on the wire.
func TestPauseDuringIdleWindowClearsIdle(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 20*time.Minute)

	s, _ = Apply(s, EventTick{IdleFor: testDurations().IdlePause, Tier: TierT0}, now.Add(3*time.Minute))
	if !s.Idle {
		t.Fatal("precondition: want an open window")
	}

	next, _ := Apply(s, EventPause{}, now.Add(4*time.Minute))

	if !next.Paused {
		t.Fatal("want Paused true")
	}
	if next.Idle {
		t.Fatal("want Idle false: Paused takes precedence")
	}
}

// specs/session-timer, Requirement "Suspend Is Time Away", Scenario "A
// suspend is not credited twice".
//
// This is research risk C8 pinned where it is deterministic. Whether the
// compositor's idle reading accrues across suspend is undocumented
// upstream, so the domain is built to be correct either way.
func TestSuspendAndIdleCreditTheSameAbsenceOnce(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 12*time.Minute)

	resumeAt := now.Add(time.Hour)
	s, _ = Apply(s, EventSuspended{From: now, To: resumeAt}, resumeAt)
	if s.ElapsedInPhase != 0 {
		t.Fatalf("precondition: want the suspend to credit, elapsed = %s", s.ElapsedInPhase)
	}

	s.ElapsedInPhase = 4 * time.Minute // the fresh focus phase has begun

	// The idle source reports the whole absence, as it would if idle
	// accrued across suspend.
	next, effects := Apply(s, EventTick{IdleFor: time.Hour, Tier: TierT0}, resumeAt.Add(5*time.Second))

	if next.ElapsedInPhase == 0 {
		t.Fatal("the same absence credited a second break: the suspend latch did not hold")
	}
	if !next.Idle {
		t.Fatal("want an idle window opened instead of a second credit")
	}
	if !hasNotify(effects) {
		t.Fatal("want EffectNotify on the opening edge")
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

// specs/session-persistence, Requirement "Downtime Is Time Away", Scenario
// "Long downtime credits a break". Downtime reaches the domain as the gap
// between the persisted LastObserved and startup, carried by EventSuspended.
func TestLongDowntimeCreditsBreak(t *testing.T) {
	now := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 12*time.Minute)

	restartAt := now.Add(time.Hour)
	next, effects := Apply(s, EventSuspended{From: now, To: restartAt}, restartAt)

	if next.Phase != PhaseFocus || next.ElapsedInPhase != 0 {
		t.Fatalf("got phase=%s elapsed=%s, want a fresh focus after 1h of downtime", next.Phase, next.ElapsedInPhase)
	}
	if !hasNotify(effects) {
		t.Fatal("want EffectNotify so clients learn the session was reset")
	}
}

// specs/session-persistence, Requirement "Downtime Is Time Away", Scenario
// "Short downtime does not credit".
func TestShortDowntimeDoesNotCredit(t *testing.T) {
	now := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 12*time.Minute)

	restartAt := now.Add(90 * time.Second)
	next, _ := Apply(s, EventSuspended{From: now, To: restartAt}, restartAt)

	if next.ElapsedInPhase != 12*time.Minute {
		t.Fatalf("elapsed = %s, want unchanged 12m after 90s of downtime", next.ElapsedInPhase)
	}
	if next.Phase != PhaseFocus {
		t.Fatalf("phase = %s, want focus", next.Phase)
	}
}

// specs/session-persistence, Requirement "Downtime Is Time Away", Scenario
// "Downtime and suspend agree".
//
// The two paths agree only because cmd/cadenced routes the startup gap through
// EventSuspended. Routing it through a tick instead charges the whole absence
// as elapsed work, which is the defect this change fixes. This test pins that
// difference so the startup path cannot quietly regress to a tick.
func TestDowntimeAndSuspendAgree(t *testing.T) {
	now := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	gap := 8 * time.Hour
	resumeAt := now.Add(gap)

	viaSuspend, _ := Apply(startedFocus(now, 12*time.Minute), EventSuspended{From: now, To: resumeAt}, resumeAt)
	viaTick, _ := Apply(startedFocus(now, 12*time.Minute), EventTick{Tier: TierT0}, resumeAt)

	if viaSuspend.Phase != PhaseFocus || viaSuspend.ElapsedInPhase != 0 {
		t.Fatalf("suspend path gave %s/%s, want a fresh focus", viaSuspend.Phase, viaSuspend.ElapsedInPhase)
	}
	if viaTick.Phase == viaSuspend.Phase {
		t.Fatalf("tick path now agrees with the suspend path (%s); if applyTick learned the "+
			"idle-credit rule, delete this test and the startup indirection with it", viaTick.Phase)
	}
	t.Logf("8h absence: suspend path -> %s, tick path -> %s (startup must use the suspend path)",
		viaSuspend.Phase, viaTick.Phase)
}

// specs/session-timer, Requirement "Idle Credit": the freeze is scoped to
// focus. A break is time away by design, so idling through one is the
// intended use, not a reason to stop its clock.
func TestBreakDoesNotFreezeWhileIdle(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 0)
	s.Phase = PhaseBreak
	s.ElapsedInPhase = 2 * time.Minute

	idle := testDurations().IdlePause + time.Minute
	next, effects := Apply(s, EventTick{IdleFor: idle, Tier: TierT0}, now.Add(time.Minute))

	if next.Idle {
		t.Fatal("want Idle false during a break: the freeze is focus-only")
	}
	if next.ElapsedInPhase != 3*time.Minute {
		t.Fatalf("elapsed = %s, want 3m — a break advances on wall-clock time while idle", next.ElapsedInPhase)
	}
	if len(effects) != 0 {
		t.Fatalf("want a quiet tick, got %d effects", len(effects))
	}
}

// The break a user actually takes must finish while they are away.
func TestBreakCompletesWhileTheUserIsAway(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	s := startedFocus(now, 0)
	s.Phase = PhaseBreak
	s.ElapsedInPhase = testDurations().Break - time.Minute // one minute left

	// Idle past the pause threshold but short of credit, so only the
	// break's own clock can end it.
	idle := testDurations().IdlePause + time.Minute
	next, _ := Apply(s, EventTick{IdleFor: idle, Tier: TierT0}, now.Add(time.Minute))

	if next.Phase != PhaseFocus {
		t.Fatalf("phase = %s, want focus — the break must complete while the user is away", next.Phase)
	}
}
