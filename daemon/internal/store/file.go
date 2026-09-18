// Package store persists session.State to disk. It stores elapsed-in-phase,
// never an absolute deadline — a deadline computed before a suspend would
// resume already-expired (see openspec/changes/m1-daemon-core/design.md,
// Decision "Persisted quantity").
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cadence/daemon/internal/session"
)

// record is the on-disk JSON shape. Field names are explicit and stable —
// this file is read back across daemon restarts and OS upgrades.
type record struct {
	Active          bool          `json:"active"`
	Phase           session.Phase `json:"phase"`
	ElapsedInPhase  time.Duration `json:"elapsed_in_phase_ns"`
	Paused          bool          `json:"paused"`
	PausedRemaining time.Duration `json:"paused_remaining_ns"`
	Tier            session.Tier  `json:"tier"`
	LastObserved    time.Time     `json:"last_observed"`
	FocusMinutes    int           `json:"focus_minutes"`
	BreakMinutes    int           `json:"break_minutes"`
	IdlePauseMin    int           `json:"idle_pause_minutes"`
	IdleCreditMin   int           `json:"idle_credit_minutes"`
}

// FileStore implements session.Store as a single JSON file, written by
// write-temp-then-rename so a crash mid-write never corrupts the previous
// good state (specs/session-persistence, "Restart resumes position").
type FileStore struct {
	path string
}

// Path returns the default state file location:
// ~/.local/state/cadence/session.json.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("store: resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "cadence", "session.json"), nil
}

// NewFileStore returns a FileStore rooted at path. The parent directory is
// created on first Save if it does not exist.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Load reads the persisted state. A missing file returns (zero-State,
// false, nil) — "no session to resume"
// (specs/session-persistence, "No session to resume") — not an error.
func (fs *FileStore) Load() (session.State, bool, error) {
	data, err := os.ReadFile(fs.path)
	if errors.Is(err, os.ErrNotExist) {
		return session.State{}, false, nil
	}
	if err != nil {
		return session.State{}, false, fmt.Errorf("store: read %s: %w", fs.path, err)
	}

	var r record
	if err := json.Unmarshal(data, &r); err != nil {
		return session.State{}, false, fmt.Errorf("store: parse %s: %w", fs.path, err)
	}

	s := session.State{
		Active:          r.Active,
		Phase:           r.Phase,
		ElapsedInPhase:  r.ElapsedInPhase,
		Paused:          r.Paused,
		PausedRemaining: r.PausedRemaining,
		Tier:            r.Tier,
		LastObserved:    r.LastObserved,
		Durations: session.Durations{
			Focus:      time.Duration(r.FocusMinutes) * time.Minute,
			Break:      time.Duration(r.BreakMinutes) * time.Minute,
			IdlePause:  time.Duration(r.IdlePauseMin) * time.Minute,
			IdleCredit: time.Duration(r.IdleCreditMin) * time.Minute,
		},
	}
	return s, true, nil
}

// Save durably writes s. It writes to a temp file in the same directory
// then renames over the target, so readers never observe a partial write
// and a crash mid-save leaves the previous file intact.
func (fs *FileStore) Save(s session.State) error {
	dir := filepath.Dir(fs.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("store: create %s: %w", dir, err)
	}

	r := record{
		Active:          s.Active,
		Phase:           s.Phase,
		ElapsedInPhase:  s.ElapsedInPhase,
		Paused:          s.Paused,
		PausedRemaining: s.PausedRemaining,
		Tier:            s.Tier,
		LastObserved:    s.LastObserved,
		FocusMinutes:    int(s.Durations.Focus.Minutes()),
		BreakMinutes:    int(s.Durations.Break.Minutes()),
		IdlePauseMin:    int(s.Durations.IdlePause.Minutes()),
		IdleCreditMin:   int(s.Durations.IdleCredit.Minutes()),
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("store: marshal state: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".session-*.json.tmp")
	if err != nil {
		return fmt.Errorf("store: create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("store: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("store: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, fs.path); err != nil {
		return fmt.Errorf("store: rename into place: %w", err)
	}
	return nil
}
