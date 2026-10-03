package dbusapi

import (
	"testing"
	"time"

	godbus "github.com/godbus/dbus/v5"
)

// The unit guard. Mutter's interface declares no unit (research.md, claim
// C5), so milliseconds rests on measurement alone. If that ever changes,
// this fails instead of silently rescaling every threshold.
func TestIdleFromMillisecondsUsesMilliseconds(t *testing.T) {
	cases := []struct {
		ms   uint64
		want time.Duration
	}{
		{0, 0},
		{1, time.Millisecond},
		{19216, 19216 * time.Millisecond}, // an observed reading, 2026-09-19
		{180000, 3 * time.Minute},         // the default idle-pause threshold
		{600000, 10 * time.Minute},        // the default idle-credit threshold
	}
	for _, c := range cases {
		if got := idleFromMilliseconds(c.ms); got != c.want {
			t.Fatalf("idleFromMilliseconds(%d) = %s, want %s", c.ms, got, c.want)
		}
	}
}

// specs/session-timer, Requirement "Idle Source Availability", Scenario
// "Idle source absent at startup". Zero is the safe value: it degrades to
// charging all elapsed time as work rather than freezing a timer or
// crediting a break nobody earned.
func TestUnavailableIdleSourceReportsZero(t *testing.T) {
	conn, err := godbus.SessionBus()
	if err != nil {
		t.Skipf("no session bus available: %v", err)
	}

	src := NewMutterIdleSource(conn)
	// Point at a name nothing owns, standing in for a Shell that has not
	// started or has just died.
	src.obj = conn.Object("dev.ian.CadenceNoSuchIdleService", idleMonitorPath)

	if got := src.IdleFor(time.Now()); got != 0 {
		t.Fatalf("IdleFor with an unreachable source = %s, want 0", got)
	}
	// Repeated failures must stay silent after the first transition; this
	// call simply must not panic or block.
	if got := src.IdleFor(time.Now()); got != 0 {
		t.Fatalf("second IdleFor = %s, want 0", got)
	}
}
