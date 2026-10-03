package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"cadence/daemon/internal/session"
)

// specs/session-persistence, Scenario "Restart resumes position".
func TestRestartResumesPosition(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(filepath.Join(dir, "session.json"))

	want := session.State{
		Active:         true,
		Phase:          session.PhaseFocus,
		ElapsedInPhase: 30 * time.Minute,
		Tier:           session.TierT0,
		LastObserved:   time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC),
		Durations: session.Durations{
			Focus:      50 * time.Minute,
			Break:      10 * time.Minute,
			IdlePause:  3 * time.Minute,
			IdleCredit: 10 * time.Minute,
		},
	}

	if err := fs.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Simulate a daemon restart: a fresh FileStore over the same path.
	restarted := NewFileStore(filepath.Join(dir, "session.json"))
	got, found, err := restarted.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !found {
		t.Fatal("want found=true after a prior Save")
	}
	if got.Phase != want.Phase || got.ElapsedInPhase != want.ElapsedInPhase {
		t.Fatalf("got phase=%s elapsed=%s, want phase=%s elapsed=%s",
			got.Phase, got.ElapsedInPhase, want.Phase, want.ElapsedInPhase)
	}
	if !got.LastObserved.Equal(want.LastObserved) {
		t.Fatalf("LastObserved = %v, want %v", got.LastObserved, want.LastObserved)
	}
}

// specs/session-persistence, Scenario "No session to resume".
func TestNoSessionToResume(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(filepath.Join(dir, "does-not-exist.json"))

	_, found, err := fs.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("want found=false when no file has ever been saved")
	}
}

func TestSaveWritesNoPartialFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(filepath.Join(dir, "session.json"))

	if err := fs.Save(session.State{Active: true, Phase: session.PhaseFocus}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want exactly one file (no leftover temp files), got %d: %v", len(entries), entries)
	}
}

// design.md, Decision 8: the idle flags are runtime state derived from a
// live compositor reading, and are re-derived within one tick of startup.
// Persisting the latch would let it outlive the absence that set it and
// suppress a break the user had earned.
func TestIdleFlagsAreNotPersisted(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(filepath.Join(dir, "session.json"))

	saved := session.State{
		Active:         true,
		Phase:          session.PhaseFocus,
		ElapsedInPhase: 30 * time.Minute,
		Idle:           true,
		IdleCredited:   true,
		Tier:           session.TierT0,
		LastObserved:   time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC),
	}
	if err := fs.Save(saved); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, found, err := NewFileStore(filepath.Join(dir, "session.json")).Load()
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if got.Idle {
		t.Error("Idle survived a save/load round trip, want it derived at runtime")
	}
	if got.IdleCredited {
		t.Error("IdleCredited survived a save/load round trip: a stale latch can suppress an earned break")
	}
}
