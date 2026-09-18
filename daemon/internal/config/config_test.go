package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// specs/daemon-configuration, Scenario "No config file".
func TestNoConfigFileYieldsDefaults(t *testing.T) {
	dir := t.TempDir()
	d, err := Load(filepath.Join(dir, "does-not-exist.toml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Focus != DefaultFocus || d.Break != DefaultBreak {
		t.Fatalf("got focus=%s break=%s, want defaults 50m/10m", d.Focus, d.Break)
	}
}

// specs/daemon-configuration, Scenario "Unknown key rejected".
func TestUnknownKeyRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("focus_mins = 45\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("want an error for an unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "focus_mins") {
		t.Fatalf("error %q does not name the offending key focus_mins", err)
	}
}

func TestOverridesApply(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := "[timer]\nfocus_minutes = 45\nbreak_minutes = 8\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	d, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Focus.Minutes() != 45 || d.Break.Minutes() != 8 {
		t.Fatalf("got focus=%s break=%s, want 45m/8m", d.Focus, d.Break)
	}
	// Unset fields keep their defaults.
	if d.IdlePause != DefaultIdlePause {
		t.Fatalf("idle pause changed unexpectedly: %s", d.IdlePause)
	}
}

func TestNonPositiveDurationRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[timer]\nfocus_minutes = -5\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("want an error for a non-positive duration, got nil")
	}
}
