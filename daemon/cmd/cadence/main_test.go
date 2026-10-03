package main

import (
	"testing"
	"time"
)

// F10: RemainingSeconds is republished only on transitions, so between them
// it is a stale snapshot (specs/panel-indicator, "Countdown Derivation").
// The CLI must derive the live countdown from PhaseEndsAt, exactly as the
// panel does, and fall back to the frozen value only while the daemon
// publishes PhaseEndsAt as 0.
func TestRemainingDerivesFromDeadline(t *testing.T) {
	now := time.Unix(1_000_000, 0)

	cases := []struct {
		name string
		snap statusSnapshot
		want time.Duration
	}{
		{
			name: "running phase ignores the stale snapshot",
			snap: statusSnapshot{PhaseEndsAt: now.Unix() + 2993, RemainingSeconds: 3000},
			want: 2993 * time.Second,
		},
		{
			name: "past deadline clamps to zero",
			snap: statusSnapshot{PhaseEndsAt: now.Unix() - 5, RemainingSeconds: 3000},
			want: 0,
		},
		{
			name: "paused reads the frozen remainder",
			snap: statusSnapshot{Paused: true, PhaseEndsAt: 0, RemainingSeconds: 720},
			want: 720 * time.Second,
		},
		{
			name: "idle reads the frozen remainder",
			snap: statusSnapshot{Idle: true, PhaseEndsAt: 0, RemainingSeconds: 720},
			want: 720 * time.Second,
		},
	}
	for _, c := range cases {
		if got := c.snap.remaining(now); got != c.want {
			t.Errorf("%s: remaining = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestStateLabel(t *testing.T) {
	cases := []struct {
		snap statusSnapshot
		want string
	}{
		{statusSnapshot{Phase: "focus"}, "focus"},
		{statusSnapshot{Phase: "focus", Paused: true}, "focus (paused)"},
		{statusSnapshot{Phase: "focus", Idle: true}, "focus (idle)"},
	}
	for _, c := range cases {
		if got := c.snap.label(); got != c.want {
			t.Errorf("label(%+v) = %q, want %q", c.snap, got, c.want)
		}
	}
}
