package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// writeConfig writes content to a fresh config.toml and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// specs/daemon-configuration, "Defaults And Validation", Scenario "Zero
// rejected": a key set to 0 is non-positive, for every key (design D2).
func TestZeroAndNegativeRejectedForEveryKey(t *testing.T) {
	keys := []struct{ section, key string }{
		{"timer", "focus_minutes"},
		{"timer", "break_minutes"},
		{"idle", "pause_after_minutes"},
		{"idle", "credit_break_after_minutes"},
		{"camera", "prompt_every_minutes"},
		{"camera", "prompt_limit"},
	}
	for _, k := range keys {
		for _, v := range []string{"0", "-5"} {
			t.Run(k.section+"."+k.key+"="+v, func(t *testing.T) {
				path := writeConfig(t, "["+k.section+"]\n"+k.key+" = "+v+"\n")

				_, err := Load(path)
				if err == nil {
					t.Fatalf("want an error for %s.%s = %s, got nil", k.section, k.key, v)
				}
				if !strings.Contains(err.Error(), k.section+"."+k.key) {
					t.Fatalf("error %q does not name %s.%s", err, k.section, k.key)
				}
			})
		}
	}
}

// The exact message format is part of the contract: section.key, then the
// offending value.
func TestZeroRejectionMessageFormat(t *testing.T) {
	_, err := Load(writeConfig(t, "[timer]\nfocus_minutes = 0\n"))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	want := "config: timer.focus_minutes must be positive, got 0"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

// specs/daemon-configuration, "Defaults And Validation": only an absent key
// takes its default, and an empty file has every key absent.
func TestAbsentKeysTakeDefaults(t *testing.T) {
	for name, content := range map[string]string{
		"empty file":        "",
		"unrelated section": "[timer]\nfocus_minutes = 45\n",
	} {
		t.Run(name, func(t *testing.T) {
			d, err := Load(writeConfig(t, content))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := Defaults()
			if name == "unrelated section" {
				want.Focus = 45 * time.Minute
			}
			if d != want {
				t.Fatalf("got %+v, want %+v", d, want)
			}
		})
	}
}

// specs/daemon-configuration, "Defaults And Validation", Scenario "No config
// file": all six defaults.
func TestDefaultsCarryAllSixValues(t *testing.T) {
	d := Defaults()
	if d.Focus != 50*time.Minute || d.Break != 10*time.Minute ||
		d.IdlePause != 3*time.Minute || d.IdleCredit != 10*time.Minute ||
		d.PromptRetry != 5*time.Minute || d.PromptCap != 3 {
		t.Fatalf("defaults = %+v", d)
	}
	missing, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil || missing != d {
		t.Fatalf("missing file = %+v, %v; want the defaults", missing, err)
	}
}

// specs/daemon-configuration, "Defaults And Validation", Scenario "Camera
// policy configured".
func TestCameraKeysLoad(t *testing.T) {
	d, err := Load(writeConfig(t, "[camera]\nprompt_every_minutes = 2\nprompt_limit = 5\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.PromptRetry != 2*time.Minute || d.PromptCap != 5 {
		t.Fatalf("retry=%s cap=%d, want 2m/5", d.PromptRetry, d.PromptCap)
	}
}

// An unknown key inside a known section is still rejected, now that the
// schema has a [camera] section.
func TestUnknownCameraKeyRejected(t *testing.T) {
	_, err := Load(writeConfig(t, "[camera]\nprompt_forever = 1\n"))
	if err == nil || !strings.Contains(err.Error(), "prompt_forever") {
		t.Fatalf("error = %v, want one naming prompt_forever", err)
	}
}
