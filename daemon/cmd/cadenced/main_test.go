package main

import (
	"testing"
	"time"

	"cadence/daemon/internal/session"
)

// The startup gap must reach the domain as a suspend, not as a tick: a tick
// charges the whole absence as elapsed work, which is the defect
// specs/session-persistence "Downtime Is Time Away" exists to prevent.
func TestStartupGapReplaysTheAbsence(t *testing.T) {
	lastObserved := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	now := lastObserved.Add(8 * time.Hour)
	active := session.State{Active: true, Phase: session.PhaseFocus, LastObserved: lastObserved}

	ev, ok := startupGap(active, true, now)
	if !ok {
		t.Fatal("want a startup gap for a resumed active session")
	}
	if !ev.From.Equal(lastObserved) || !ev.To.Equal(now) {
		t.Fatalf("gap = %s..%s, want %s..%s", ev.From, ev.To, lastObserved, now)
	}
}

func TestStartupGapSkippedWithoutASession(t *testing.T) {
	now := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)

	if _, ok := startupGap(session.State{}, false, now); ok {
		t.Error("no persisted state: want no gap replayed")
	}
	if _, ok := startupGap(session.State{Active: false, LastObserved: now.Add(-time.Hour)}, true, now); ok {
		t.Error("persisted but inactive session: want no gap replayed")
	}
}
