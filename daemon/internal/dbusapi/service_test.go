package dbusapi

import (
	"testing"
	"time"

	godbus "github.com/godbus/dbus/v5"

	"cadence/daemon/internal/session"
)

type memStore struct{ saved session.State }

func (m *memStore) Load() (session.State, bool, error) { return session.State{}, false, nil }
func (m *memStore) Save(s session.State) error         { m.saved = s; return nil }

type fixedTier struct{}

func (fixedTier) CurrentTier() session.Tier { return session.TierT0 }

type zeroIdle struct{}

func (zeroIdle) IdleFor(time.Time) time.Duration { return 0 }

func testDurations() session.Durations {
	return session.Durations{
		Focus:      50 * time.Minute,
		Break:      10 * time.Minute,
		IdlePause:  3 * time.Minute,
		IdleCredit: 10 * time.Minute,
	}
}

// newTestService connects to the real session bus (confirmed reachable in
// this environment: unix:path=/run/user/1000/bus) and exports a Service
// under the production BusName. It skips rather than fails when the bus is
// unavailable or the name is already held — e.g. by a real cadenced.
func newTestService(t *testing.T, clock session.Clock) (*Service, *godbus.Conn) {
	t.Helper()
	return newTestServiceWithIdle(t, clock, zeroIdle{})
}

// settableIdle stands in for the compositor so a test can drive a window
// open and closed without a real desk.
type settableIdle struct{ d time.Duration }

func (s *settableIdle) IdleFor(time.Time) time.Duration { return s.d }

// newTestServiceWithIdle exports a Service on a PRIVATE connection under no
// well-known name. Both halves of that matter (F9):
//
// Private, because godbus.SessionBus returns a cached shared connection. Every
// test exporting on the same ObjectPath over one connection overwrites the
// previous test's handlers.
//
// Nameless, because claiming the production BusName fails whenever a real
// cadenced is running, which is the normal state of a developer machine. The
// old harness answered that with t.Skip, so `go test ./...` printed ok while
// skipping 7 of 9 tests — and F7 and F8 shipped behind that green result.
// The service is still fully reachable here: conn's unique name addresses it.
//
// A missing session bus is the one honest skip, and it is kept below.
func newTestServiceWithIdle(t *testing.T, clock session.Clock, idle session.IdleSource) (*Service, *godbus.Conn) {
	t.Helper()
	return newTestServiceWithSources(t, clock, idle, fixedTier{})
}

// settableTier stands in for the tier detector so a test can move the tier
// between ticks.
type settableTier struct{ t session.Tier }

func (s *settableTier) CurrentTier() session.Tier { return s.t }

// newTestServiceWithSources is newTestServiceWithIdle with the tier source
// injectable. See that helper for why the connection is private and nameless.
func newTestServiceWithSources(t *testing.T, clock session.Clock, idle session.IdleSource, tier session.TierSource) (*Service, *godbus.Conn) {
	t.Helper()
	conn, err := godbus.SessionBusPrivate()
	if err != nil {
		t.Skipf("no session bus available, skipping D-Bus integration test: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.Auth(nil); err != nil {
		t.Fatalf("authenticating on a private session bus connection: %v", err)
	}
	if err := conn.Hello(); err != nil {
		t.Fatalf("Hello on a private session bus connection: %v", err)
	}

	initial := session.State{Durations: testDurations(), LastObserved: clock.Now()}
	svc, err := newService(conn, "", initial, &memStore{}, clock, tier, idle)
	if err != nil {
		t.Fatalf("exporting the test service: %v", err)
	}
	return svc, conn
}

// testDest is the connection's own unique name, which addresses a service
// exported on it without any well-known name.
func testDest(t *testing.T, conn *godbus.Conn) string {
	t.Helper()
	dest := conn.Names()
	if len(dest) == 0 {
		t.Fatal("connection has no unique name; was Hello called?")
	}
	return dest[0]
}

func getProp[T any](t *testing.T, conn *godbus.Conn, name string) T {
	t.Helper()
	var v T
	obj := conn.Object(testDest(t, conn), ObjectPath)
	if err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, InterfaceName, name).Store(&v); err != nil {
		t.Fatalf("Get %s: %v", name, err)
	}
	return v
}

// specs/daemon-control, Scenario "Start then inspect".
func TestStartThenInspect(t *testing.T) {
	svc, conn := newTestService(t, session.RealClock{})

	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if !getProp[bool](t, conn, "SessionActive") {
		t.Fatal("want SessionActive=true after StartSession")
	}
	if phase := getProp[string](t, conn, "Phase"); phase != string(session.PhaseFocus) {
		t.Fatalf("Phase = %q, want %q", phase, session.PhaseFocus)
	}
}

// specs/daemon-control, Scenario "Start when already active".
func TestStartWhenAlreadyActiveNoop(t *testing.T) {
	svc, _ := newTestService(t, session.RealClock{})

	if err := svc.StartSession(); err != nil {
		t.Fatalf("first StartSession: %v", err)
	}
	svc.mu.Lock()
	elapsedBefore := svc.state.ElapsedInPhase
	svc.mu.Unlock()

	if err := svc.StartSession(); err != nil {
		t.Fatalf("second StartSession: %v", err)
	}
	svc.mu.Lock()
	elapsedAfter := svc.state.ElapsedInPhase
	svc.mu.Unlock()

	if elapsedBefore != elapsedAfter {
		t.Fatalf("elapsed changed on a redundant StartSession: %s -> %s", elapsedBefore, elapsedAfter)
	}
}

// specs/daemon-control, Scenario "No per-second traffic". Ticks with no
// transition must not touch PropertiesChanged — verified against the real
// bus, not by inspecting the Go return value alone.
func TestNoPropertiesChangedOnQuietTick(t *testing.T) {
	clock := session.NewFakeClock(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC))
	svc, conn := newTestService(t, clock)

	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	signals := make(chan *godbus.Signal, 8)
	conn.Signal(signals)
	matchCall := conn.BusObject().Call(
		"org.freedesktop.DBus.AddMatch", 0,
		"type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='"+string(ObjectPath)+"'",
	)
	if matchCall.Err != nil {
		t.Fatalf("AddMatch: %v", matchCall.Err)
	}

	// Drain the StartSession transition's own signal before observing.
	drainFor(signals, 200*time.Millisecond)

	clock.Advance(30 * time.Second) // well short of any threshold
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	select {
	case sig := <-signals:
		t.Fatalf("want no PropertiesChanged from a quiet tick, got %v", sig.Body)
	case <-time.After(500 * time.Millisecond):
		// expected: nothing arrived
	}
}

func drainFor(ch <-chan *godbus.Signal, d time.Duration) {
	deadline := time.After(d)
	for {
		select {
		case <-ch:
		case <-deadline:
			return
		}
	}
}

// A closed connection is an ordinary condition at logout. publish must return
// that as an error so cmd/cadenced can log it, instead of panicking through
// prop.SetMust and taking the process down (F3).
func TestPublishOnClosedConnectionErrorsNotPanics(t *testing.T) {
	clock := session.NewFakeClock(time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC))
	svc, conn := newTestService(t, clock)

	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("closing the connection: %v", err)
	}

	// publish must have something to announce for the closed connection to
	// matter: it emits only changed properties, and correctly stays silent
	// (and error-free) when a transition changed nothing.
	clock.Advance(30 * time.Second)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("publish panicked on a closed connection instead of returning an error: %v", r)
		}
	}()

	err := svc.publish()
	if err == nil {
		t.Fatal("publish returned nil on a closed connection, want an error")
	}
	t.Logf("publish reported: %v", err)
}

// specs/daemon-control, Requirement "Idle Publication", Scenario
// "Entering an idle window". The deadline is suppressed rather than
// republished, because no single deadline stays true for the window.
func TestIdleWindowPublishesAFrozenInterval(t *testing.T) {
	clock := session.NewFakeClock(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC))
	idle := &settableIdle{}
	svc, conn := newTestServiceWithIdle(t, clock, idle)

	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	idle.d = testDurations().IdlePause
	clock.Advance(testDurations().IdlePause)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if !getProp[bool](t, conn, "Idle") {
		t.Fatal("want Idle true once the pause threshold is reached")
	}
	if got := getProp[int64](t, conn, "PhaseEndsAt"); got != 0 {
		t.Fatalf("PhaseEndsAt = %d while idle, want 0", got)
	}
	if got := getProp[int64](t, conn, "RemainingSeconds"); got != int64(testDurations().Focus.Seconds()) {
		t.Fatalf("RemainingSeconds = %d, want the frozen %d", got, int64(testDurations().Focus.Seconds()))
	}
	if getProp[bool](t, conn, "Paused") {
		t.Fatal("want Paused false: Paused and Idle are never both true")
	}
}

// specs/daemon-control, Requirement "Change Notification", Scenario
// "An idle window emits exactly twice" — the silent middle.
func TestNoPropertiesChangedInsideAnIdleWindow(t *testing.T) {
	clock := session.NewFakeClock(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC))
	idle := &settableIdle{}
	svc, conn := newTestServiceWithIdle(t, clock, idle)

	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	idle.d = testDurations().IdlePause
	clock.Advance(testDurations().IdlePause)
	if err := svc.Tick(); err != nil { // opening edge
		t.Fatalf("Tick: %v", err)
	}

	signals := make(chan *godbus.Signal, 8)
	conn.Signal(signals)
	matchCall := conn.BusObject().Call(
		"org.freedesktop.DBus.AddMatch", 0,
		"type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='"+string(ObjectPath)+"'",
	)
	if matchCall.Err != nil {
		t.Fatalf("AddMatch: %v", matchCall.Err)
	}
	drainFor(signals, 200*time.Millisecond)

	for i := 0; i < 4; i++ {
		idle.d += 5 * time.Second
		clock.Advance(5 * time.Second)
		if err := svc.Tick(); err != nil {
			t.Fatalf("Tick %d: %v", i, err)
		}
	}

	select {
	case sig := <-signals:
		t.Fatalf("want silence inside an open idle window, got %v", sig.Body)
	case <-time.After(500 * time.Millisecond):
	}
}

// specs/daemon-control, Requirement "Idle Publication", Scenario
// "Leaving an idle window".
func TestLeavingIdleRepublishesALiveDeadline(t *testing.T) {
	clock := session.NewFakeClock(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC))
	idle := &settableIdle{}
	svc, conn := newTestServiceWithIdle(t, clock, idle)

	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	idle.d = testDurations().IdlePause
	clock.Advance(testDurations().IdlePause)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	idle.d = 0
	clock.Advance(5 * time.Second)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if getProp[bool](t, conn, "Idle") {
		t.Fatal("want Idle false after returning")
	}
	endsAt := getProp[int64](t, conn, "PhaseEndsAt")
	want := clock.Now().Add(testDurations().Focus).Unix()
	if endsAt != want {
		t.Fatalf("PhaseEndsAt = %d on return, want %d — the frozen remainder from the return instant", endsAt, want)
	}
}

// countFor reports how many signals arrive within d.
func countFor(ch <-chan *godbus.Signal, d time.Duration) int {
	n := 0
	deadline := time.After(d)
	for {
		select {
		case <-ch:
			n++
		case <-deadline:
			return n
		}
	}
}

// watchSignals subscribes to PropertiesChanged on ObjectPath and drains
// whatever is already in flight, so the caller observes only what follows.
func watchSignals(t *testing.T, conn *godbus.Conn) chan *godbus.Signal {
	t.Helper()
	signals := make(chan *godbus.Signal, 32)
	conn.Signal(signals)
	matchCall := conn.BusObject().Call(
		"org.freedesktop.DBus.AddMatch", 0,
		"type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='"+string(ObjectPath)+"'",
	)
	if matchCall.Err != nil {
		t.Fatalf("AddMatch: %v", matchCall.Err)
	}
	drainFor(signals, 200*time.Millisecond)
	return signals
}

// specs/daemon-control, Requirement "Change Notification", Scenario
// "An idle window emits exactly twice" — the two edges themselves.
//
// Regression for F7. prop.Properties emits one signal per Set with a
// single-entry map and no old/new comparison, so a SetMust-per-property
// publish sent seven signals per edge and republished SessionActive, Phase,
// Tier and Paused unchanged. Observed live on 2026-09-19 before the fix.
func TestIdleEdgesEachEmitExactlyOneSignal(t *testing.T) {
	clock := session.NewFakeClock(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC))
	idle := &settableIdle{}
	svc, conn := newTestServiceWithIdle(t, clock, idle)

	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	signals := watchSignals(t, conn)

	idle.d = testDurations().IdlePause
	clock.Advance(testDurations().IdlePause)
	if err := svc.Tick(); err != nil { // opening edge
		t.Fatalf("Tick: %v", err)
	}
	if n := countFor(signals, 500*time.Millisecond); n != 1 {
		t.Fatalf("opening edge emitted %d PropertiesChanged, want exactly 1", n)
	}

	idle.d = 0
	clock.Advance(5 * time.Second)
	if err := svc.Tick(); err != nil { // closing edge
		t.Fatalf("Tick: %v", err)
	}
	if n := countFor(signals, 500*time.Millisecond); n != 1 {
		t.Fatalf("closing edge emitted %d PropertiesChanged, want exactly 1", n)
	}
}

// The one signal carries every property that changed, and nothing that did not.
func TestIdleEdgeSignalCarriesOnlyWhatChanged(t *testing.T) {
	clock := session.NewFakeClock(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC))
	idle := &settableIdle{}
	svc, conn := newTestServiceWithIdle(t, clock, idle)

	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	signals := watchSignals(t, conn)

	idle.d = testDurations().IdlePause
	clock.Advance(testDurations().IdlePause)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	var sig *godbus.Signal
	select {
	case sig = <-signals:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no PropertiesChanged on the opening edge")
	}

	changed, ok := sig.Body[1].(map[string]godbus.Variant)
	if !ok {
		t.Fatalf("signal body[1] = %T, want map[string]dbus.Variant", sig.Body[1])
	}
	if _, ok := changed["Idle"]; !ok {
		t.Error("Idle changed on the opening edge but is absent from the signal")
	}
	if _, ok := changed["PhaseEndsAt"]; !ok {
		t.Error("PhaseEndsAt is suppressed to 0 on the opening edge but is absent from the signal")
	}
	for _, unchanged := range []string{"SessionActive", "Phase", "Tier", "Paused"} {
		if _, ok := changed[unchanged]; ok {
			t.Errorf("%s did not change across the opening edge but was republished", unchanged)
		}
	}
}

// specs/daemon-control, Requirement "Change Notification", Scenario
// "A client never drifts across suspends, idle windows and transitions".
//
// Regression for F8. A client counts down as PhaseEndsAt minus its own
// floor(now), and reads the frozen RemainingSeconds while the window is
// open, so the two must agree exactly at the instant the window closes.
// Deriving the deadline from a float now instead of the same truncated
// integer left them a second apart, and the panel rendered 47:20 on leaving
// an idle window it had spent frozen at 47:19 — a countdown running
// backwards. Observed live on 2026-09-19 before the fix.
//
// The fractional advance is the point: a whole-second clock makes the two
// formulas identical, which is why the original suite never caught this.
func TestCountdownDoesNotJumpBackwardsLeavingIdle(t *testing.T) {
	clock := session.NewFakeClock(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC))
	idle := &settableIdle{}
	svc, conn := newTestServiceWithIdle(t, clock, idle)

	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Work a fractional stretch first. The remainder is what exposes the
	// defect, and it can only come from time actually charged to the phase:
	// an idle window that covers the whole session charges nothing and
	// leaves remaining at a whole 50m.
	idle.d = 0
	clock.Advance(100*time.Second + 930*time.Millisecond)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	idle.d = testDurations().IdlePause
	clock.Advance(testDurations().IdlePause)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	frozen := getProp[int64](t, conn, "RemainingSeconds")
	if got := getProp[int64](t, conn, "PhaseEndsAt"); got != 0 {
		t.Fatalf("PhaseEndsAt = %d while idle, want 0", got)
	}

	idle.d = 0
	clock.Advance(5 * time.Second)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	endsAt := getProp[int64](t, conn, "PhaseEndsAt")
	live := endsAt - clock.Now().Unix() // exactly what the extension computes
	if live != frozen {
		t.Fatalf("countdown jumped from %ds frozen to %ds live on leaving idle (PhaseEndsAt=%d); the client must not see the remaining time change across the edge", frozen, live, endsAt)
	}
}

// specs/daemon-control, Requirement "Change Notification", Scenario "A tier
// change alone emits once" (design D6). The tick changes nothing but the
// tier: no phase edge, no idle window. Before the fix that tick returned no
// effects, so the extension kept showing a stale tier until something else
// happened to publish.
func TestTierChangeAloneEmitsOnce(t *testing.T) {
	clock := session.NewFakeClock(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC))
	tier := &settableTier{t: session.TierT0}
	svc, conn := newTestServiceWithSources(t, clock, zeroIdle{}, tier)

	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	signals := watchSignals(t, conn)

	tier.t = session.TierT3
	clock.Advance(5 * time.Second)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	var sig *godbus.Signal
	select {
	case sig = <-signals:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no PropertiesChanged on a tier-only change")
	}
	changed, ok := sig.Body[1].(map[string]godbus.Variant)
	if !ok {
		t.Fatalf("signal body[1] = %T, want map[string]dbus.Variant", sig.Body[1])
	}
	if v, ok := changed["Tier"]; !ok || v.Value() != string(session.TierT3) {
		t.Fatalf("Tier = %v (present=%v), want T3 in the signal", v, ok)
	}
	// RemainingSeconds is allowed: it is derived from elapsed time, which the
	// 5s the tick advanced legitimately moved. What must not appear is any
	// property the tier change did not touch.
	for _, unchanged := range []string{"SessionActive", "Phase", "Paused", "Idle", "PhaseEndsAt"} {
		if _, ok := changed[unchanged]; ok {
			t.Errorf("%s did not change but was republished with the tier", unchanged)
		}
	}
	if n := countFor(signals, 300*time.Millisecond); n != 0 {
		t.Fatalf("%d extra signals after the tier edge, want 0", n)
	}

	// The following ticks at the same tier are silent.
	for i := 0; i < 3; i++ {
		clock.Advance(5 * time.Second)
		if err := svc.Tick(); err != nil {
			t.Fatalf("Tick: %v", err)
		}
	}
	if n := countFor(signals, 500*time.Millisecond); n != 0 {
		t.Fatalf("ticks at an unchanged tier emitted %d signals, want 0", n)
	}
}

// countingTier records how often the tier detector is sampled.
type countingTier struct{ calls int }

func (c *countingTier) CurrentTier() session.Tier { c.calls++; return session.TierT0 }

// specs/session-timer, Requirement "Tier Gating": "The tier MUST be sampled
// on every tick while a session is active and not paused. Outside that, the
// tier is not sampled." Sampling costs a pw-dump spawn and a /proc walk, so
// an idle daemon must not pay it every tick.
func TestTierIsSampledOnlyWhileActiveAndUnpaused(t *testing.T) {
	clock := session.NewFakeClock(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC))
	tier := &countingTier{}
	svc, _ := newTestServiceWithSources(t, clock, zeroIdle{}, tier)

	tick := func() {
		t.Helper()
		clock.Advance(5 * time.Second)
		if err := svc.Tick(); err != nil {
			t.Fatalf("Tick: %v", err)
		}
	}

	tick()
	if tier.calls != 0 {
		t.Fatalf("sampled %d times with no session, want 0", tier.calls)
	}

	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	tick()
	if tier.calls != 1 {
		t.Fatalf("sampled %d times during an active session tick, want 1", tier.calls)
	}

	if err := svc.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	tick()
	if tier.calls != 1 {
		t.Fatalf("sampled %d times in total after a paused tick, want still 1", tier.calls)
	}
}

// heldService starts a session and advances it to a held break under T2,
// returning the service, its connection, the fake clock and the tier source
// so a test can keep driving it. The signal watcher is attached by the
// caller AFTER this returns, so setup traffic is not counted.
func heldService(t *testing.T) (*Service, *godbus.Conn, *session.FakeClock, *settableTier) {
	t.Helper()
	clock := session.NewFakeClock(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC))
	tier := &settableTier{t: session.TierT0}
	svc, conn := newTestServiceWithSources(t, clock, zeroIdle{}, tier)
	if err := svc.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	return svc, conn, clock, tier
}

// nextChanged waits for one PropertiesChanged and returns its changed map.
func nextChanged(t *testing.T, signals <-chan *godbus.Signal) map[string]godbus.Variant {
	t.Helper()
	select {
	case sig := <-signals:
		changed, ok := sig.Body[1].(map[string]godbus.Variant)
		if !ok {
			t.Fatalf("signal body[1] = %T, want map[string]dbus.Variant", sig.Body[1])
		}
		return changed
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no PropertiesChanged within 500ms")
		return nil
	}
}

// specs/daemon-control, Requirement "Hold Publication", Scenario "Entering a
// hold": one signal carries Phase, Hold, Prompts = 1 and PhaseEndsAt = 0.
func TestEnteringAHoldEmitsOneSignal(t *testing.T) {
	svc, conn, clock, tier := heldService(t)
	signals := watchSignals(t, conn)

	tier.t = session.TierT2
	clock.Advance(testDurations().Focus)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	changed := nextChanged(t, signals)
	if v, ok := changed["Phase"]; !ok || v.Value() != "break" {
		t.Errorf("Phase = %v (present=%v), want break", v, ok)
	}
	if v, ok := changed["Hold"]; !ok || v.Value() != "prompt" {
		t.Errorf("Hold = %v (present=%v), want prompt", v, ok)
	}
	if v, ok := changed["Prompts"]; !ok || v.Value() != int32(1) {
		t.Errorf("Prompts = %v (present=%v), want int32 1", v, ok)
	}
	if v, ok := changed["PhaseEndsAt"]; !ok || v.Value() != int64(0) {
		t.Errorf("PhaseEndsAt = %v (present=%v), want 0 while held", v, ok)
	}
	if n := countFor(signals, 300*time.Millisecond); n != 0 {
		t.Fatalf("%d extra signals after entering the hold, want exactly 1 in total", n)
	}
	if got := getProp[int64](t, conn, "RemainingSeconds"); got != int64(testDurations().Break.Seconds()) {
		t.Fatalf("RemainingSeconds = %d, want the whole break frozen", got)
	}
}

// specs/daemon-control, "Hold Publication", Scenario "A further prompt":
// the ticks in between emit nothing; the retry emits Prompts = 2 alone.
func TestAFurtherPromptEmitsOnlyPrompts(t *testing.T) {
	svc, conn, clock, tier := heldService(t)
	tier.t = session.TierT2
	clock.Advance(testDurations().Focus)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	signals := watchSignals(t, conn)

	for i := 0; i < 59; i++ { // 295s: one tick short of the retry interval
		clock.Advance(5 * time.Second)
		if err := svc.Tick(); err != nil {
			t.Fatalf("Tick: %v", err)
		}
	}
	if n := countFor(signals, 300*time.Millisecond); n != 0 {
		t.Fatalf("ticks between prompts emitted %d signals, want 0", n)
	}

	clock.Advance(5 * time.Second)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	changed := nextChanged(t, signals)
	if v, ok := changed["Prompts"]; !ok || v.Value() != int32(2) {
		t.Fatalf("Prompts = %v (present=%v), want int32 2", v, ok)
	}
	for _, unchanged := range []string{"Phase", "Hold", "PhaseEndsAt", "Tier", "SessionActive"} {
		if _, ok := changed[unchanged]; ok {
			t.Errorf("%s did not change but was republished with the prompt", unchanged)
		}
	}
}

// specs/daemon-control, "Hold Publication", Scenario "Lifting a hold": one
// signal carries Hold = none and a live PhaseEndsAt of lift instant + the
// frozen remainder.
func TestLiftingAHoldRepublishesALiveDeadline(t *testing.T) {
	svc, conn, clock, tier := heldService(t)
	tier.t = session.TierT2
	clock.Advance(testDurations().Focus)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	signals := watchSignals(t, conn)

	tier.t = session.TierT0
	clock.Advance(5 * time.Second)
	if err := svc.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	changed := nextChanged(t, signals)
	if v, ok := changed["Hold"]; !ok || v.Value() != "none" {
		t.Errorf("Hold = %v (present=%v), want none", v, ok)
	}
	want := clock.Now().Unix() + int64(testDurations().Break.Seconds())
	if v, ok := changed["PhaseEndsAt"]; !ok || v.Value() != want {
		t.Errorf("PhaseEndsAt = %v (present=%v), want %d", v, ok, want)
	}
	if n := countFor(signals, 300*time.Millisecond); n != 0 {
		t.Fatalf("%d extra signals on the lift, want exactly 1", n)
	}
}

// specs/daemon-control, Requirement "Control Surface", Scenario "Hold is
// published from the first connection".
func TestHoldIsPublishedFromTheFirstConnection(t *testing.T) {
	_, conn := newTestService(t, session.NewFakeClock(time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)))

	if got := getProp[string](t, conn, "Hold"); got != "none" {
		t.Fatalf("Hold = %q, want none", got)
	}
	if got := getProp[int32](t, conn, "Prompts"); got != 0 {
		t.Fatalf("Prompts = %d, want 0", got)
	}
}
