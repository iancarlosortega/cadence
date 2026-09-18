package dbusapi

import (
	"testing"
	"time"

	godbus "github.com/godbus/dbus/v5"

	"cadence/daemon/internal/session"
)

type memStore struct{ saved session.State }

func (m *memStore) Load() (session.State, bool, error) { return session.State{}, false, nil }
func (m *memStore) Save(s session.State) error          { m.saved = s; return nil }

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
	conn, err := godbus.SessionBus()
	if err != nil {
		t.Skipf("no session bus available, skipping D-Bus integration test: %v", err)
	}

	initial := session.State{Durations: testDurations(), LastObserved: clock.Now()}
	svc, err := New(conn, initial, &memStore{}, clock, fixedTier{}, zeroIdle{})
	if err != nil {
		t.Skipf("could not claim %s (another cadenced running?): %v", BusName, err)
	}
	t.Cleanup(func() { conn.ReleaseName(BusName) })
	return svc, conn
}

func getProp[T any](t *testing.T, conn *godbus.Conn, name string) T {
	t.Helper()
	var v T
	obj := conn.Object(BusName, ObjectPath)
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
