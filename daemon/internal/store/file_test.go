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

// Design D7: tier is sampled live and never restored. An old state file that
// still carries a "tier" key must load without error and yield T0, so a stale
// T3 cannot suppress a break before the first tick re-samples it.
func TestStoredTierIsIgnoredOnRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	old := `{"active": true, "phase": "focus", "tier": "T3", "last_observed": "2026-09-14T09:00:00Z"}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	got, found, err := NewFileStore(path).Load()
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if got.Tier != session.TierT0 {
		t.Fatalf("Tier = %q, want T0: tier is not persisted", got.Tier)
	}
}

// specs/session-persistence, Requirement "Durable State": a held break
// survives a restart with its stage, its prompt count and its retry clock
// (design D8).
func TestAHeldBreakRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	want := session.State{
		Active:         true,
		Phase:          session.PhaseBreak,
		ElapsedInPhase: 4 * time.Minute,
		Hold:           session.HoldPrompt,
		Prompts:        2,
		SincePrompt:    90 * time.Second,
		LastObserved:   time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC),
	}
	if err := NewFileStore(path).Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, found, err := NewFileStore(path).Load()
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if got.Hold != want.Hold || got.Prompts != want.Prompts || got.SincePrompt != want.SincePrompt {
		t.Fatalf("got hold=%q prompts=%d since=%s, want hold=%q prompts=%d since=%s",
			got.Hold, got.Prompts, got.SincePrompt, want.Hold, want.Prompts, want.SincePrompt)
	}
}

// specs/session-persistence, Requirement "Durable State": a file written
// before M6 has no hold keys and loads as not held (design D8).
func TestAnOldFileLoadsAsNotHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	old := `{"active": true, "phase": "break", "last_observed": "2026-09-14T09:00:00Z"}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	got, found, err := NewFileStore(path).Load()
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if got.Hold != session.HoldNone || got.Prompts != 0 || got.SincePrompt != 0 {
		t.Fatalf("got hold=%q prompts=%d since=%s, want none/0/0", got.Hold, got.Prompts, got.SincePrompt)
	}
}

// An unknown hold value loads as not held, so the published Hold always
// agrees with State.Held() (specs/session-persistence, "Durable State").
func TestUnknownHoldLoadsAsNotHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	data := `{"active":true,"phase":"break","hold":"bogus","prompts":2,"since_prompt_ns":60000000000,` +
		`"focus_minutes":50,"break_minutes":10,"idle_pause_minutes":3,"idle_credit_minutes":10}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	s, ok, err := NewFileStore(path).Load()
	if err != nil || !ok {
		t.Fatalf("Load = ok %v, err %v", ok, err)
	}
	if s.Hold != session.HoldNone || s.Prompts != 0 || s.SincePrompt != 0 {
		t.Fatalf("hold = %q prompts = %d since = %s, want none/0/0", s.Hold, s.Prompts, s.SincePrompt)
	}
}
