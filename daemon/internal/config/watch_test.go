package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// watchFixture writes content to a config file, seeds a watcher with the
// identity of what it just wrote (as cmd/cadenced does after its startup
// load), and returns both.
func watchFixture(t *testing.T, content string) (string, *Watcher) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path, NewWatcher(path, Identify(path))
}

var mtimeCounter int

// save rewrites the file and forces a distinct mtime, so a same-second write
// is still seen as a change on filesystems with coarse timestamps.
func save(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("save: %v", err)
	}
	mtimeCounter++
	at := time.Date(2026, 1, 1, 0, 0, mtimeCounter, 0, time.UTC)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

// specs/daemon-configuration, "Live Reload", Scenario "Saving without a
// change emits nothing": an unchanged file is not even read.
func TestWatcherUnchangedFileDoesNotReload(t *testing.T) {
	_, w := watchFixture(t, "[timer]\nfocus_minutes = 30\n")

	for i := 0; i < 3; i++ {
		if _, changed, err := w.Poll(); changed || err != nil {
			t.Fatalf("poll %d: changed=%v err=%v, want neither", i, changed, err)
		}
	}
}

// Scenario "Saving a new focus length".
func TestWatcherChangedContentReloads(t *testing.T) {
	path, w := watchFixture(t, "[timer]\nfocus_minutes = 30\n")

	save(t, path, "[timer]\nfocus_minutes = 40\n")
	d, changed, err := w.Poll()

	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v, want a reload", changed, err)
	}
	if d.Focus != 40*time.Minute {
		t.Fatalf("focus = %s, want 40m", d.Focus)
	}
	if _, changed, _ := w.Poll(); changed {
		t.Fatal("a second poll with no further save reloaded again")
	}
}

// Scenario "An invalid file is ignored": reported once, not on every poll.
func TestWatcherInvalidSaveErrorsOnce(t *testing.T) {
	path, w := watchFixture(t, "[timer]\nfocus_minutes = 30\n")

	save(t, path, "[timer]\nfocus_minutes = 0\n")
	_, changed, err := w.Poll()
	if err == nil || changed {
		t.Fatalf("changed=%v err=%v, want an error and no change", changed, err)
	}
	if !strings.Contains(err.Error(), "timer.focus_minutes") {
		t.Fatalf("error %q does not name timer.focus_minutes", err)
	}

	for i := 0; i < 3; i++ {
		if _, changed, err := w.Poll(); changed || err != nil {
			t.Fatalf("repeat poll %d: changed=%v err=%v, want silence", i, changed, err)
		}
	}
}

// Scenario "Fixing the file applies it".
func TestWatcherFixAfterInvalidReloads(t *testing.T) {
	path, w := watchFixture(t, "[timer]\nfocus_minutes = 30\n")
	save(t, path, "[timer]\nfocus_minutes = 0\n")
	w.Poll()

	save(t, path, "[timer]\nfocus_minutes = 40\n")
	d, changed, err := w.Poll()

	if err != nil || !changed || d.Focus != 40*time.Minute {
		t.Fatalf("focus=%s changed=%v err=%v, want 40m reloaded", d.Focus, changed, err)
	}
}

// Scenario "Deleting the file reverts to defaults".
func TestWatcherDeletionRevertsToDefaults(t *testing.T) {
	path, w := watchFixture(t, "[timer]\nfocus_minutes = 30\n")

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	d, changed, err := w.Poll()

	if err != nil || !changed || d != Defaults() {
		t.Fatalf("d=%+v changed=%v err=%v, want the defaults", d, changed, err)
	}
	if _, changed, _ := w.Poll(); changed {
		t.Fatal("a second poll of the still-missing file reloaded again")
	}
}

// A file that appears after startup with none present is a change.
func TestWatcherCreationAfterAbsenceReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	w := NewWatcher(path, Identify(path))
	if _, changed, err := w.Poll(); changed || err != nil {
		t.Fatalf("changed=%v err=%v, want quiet while the file is absent", changed, err)
	}

	save(t, path, "[timer]\nfocus_minutes = 35\n")
	d, changed, err := w.Poll()

	if err != nil || !changed || d.Focus != 35*time.Minute {
		t.Fatalf("focus=%s changed=%v err=%v, want 35m", d.Focus, changed, err)
	}
}

// Editors that save by writing a temp file and renaming it over the path
// replace the inode; polling the path still sees it (design D3).
func TestWatcherRenameOverSaveReloads(t *testing.T) {
	path, w := watchFixture(t, "[timer]\nfocus_minutes = 30\n")

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte("[timer]\nfocus_minutes = 45\n"), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	at := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(tmp, at, at); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("rename: %v", err)
	}

	d, changed, err := w.Poll()

	if err != nil || !changed || d.Focus != 45*time.Minute {
		t.Fatalf("focus=%s changed=%v err=%v, want 45m", d.Focus, changed, err)
	}
}
