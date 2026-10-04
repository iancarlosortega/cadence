// Package config loads and validates cadence's TOML configuration. It is
// owned by the daemon, not by any adapter (specs/daemon-configuration).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"

	"cadence/daemon/internal/session"
)

// Defaults, per specs/daemon-configuration.
const (
	DefaultFocus      = 50 * time.Minute
	DefaultBreak      = 10 * time.Minute
	DefaultIdlePause  = 3 * time.Minute
	DefaultIdleCredit = 10 * time.Minute

	// Camera policy for a held break (specs/session-timer, "Tier Gating").
	DefaultPromptRetry = 5 * time.Minute
	DefaultPromptCap   = 3
)

// fileSchema mirrors the on-disk TOML shape. Durations are minutes in the
// file, converted to time.Duration on load.
type fileSchema struct {
	Timer struct {
		FocusMinutes int `toml:"focus_minutes"`
		BreakMinutes int `toml:"break_minutes"`
	} `toml:"timer"`
	Idle struct {
		PauseAfterMinutes       int `toml:"pause_after_minutes"`
		CreditBreakAfterMinutes int `toml:"credit_break_after_minutes"`
	} `toml:"idle"`
	Camera struct {
		PromptEveryMinutes int `toml:"prompt_every_minutes"`
		PromptLimit        int `toml:"prompt_limit"`
	} `toml:"camera"`
}

// Defaults returns the configuration used when no file exists, and the base
// that a file's present keys overlay (specs/daemon-configuration, "Defaults
// And Validation"). A deleted file reloads to exactly this (design D3).
func Defaults() session.Durations {
	return session.Durations{
		Focus:       DefaultFocus,
		Break:       DefaultBreak,
		IdlePause:   DefaultIdlePause,
		IdleCredit:  DefaultIdleCredit,
		PromptRetry: DefaultPromptRetry,
		PromptCap:   DefaultPromptCap,
	}
}

// Path returns the config file location: ~/.config/cadence/config.toml.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "cadence", "config.toml"), nil
}

// Load reads the config file at path. A missing file yields defaults, not
// an error (specs/daemon-configuration, "No config file"). Unknown keys
// and non-positive durations are rejected with the offending key named
// (specs/daemon-configuration, "Unknown key rejected").
func Load(path string) (session.Durations, error) {
	d := Defaults()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return d, nil
	} else if err != nil {
		return d, fmt.Errorf("config: stat %s: %w", path, err)
	}

	var f fileSchema
	meta, err := toml.DecodeFile(path, &f)
	if err != nil {
		return d, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return d, fmt.Errorf("config: unknown key %q in %s", undecoded[0].String(), path)
	}

	// A present key must be positive; an absent key keeps its default. The
	// two are told apart with meta.IsDefined, not by comparing with zero:
	// gating on `!= 0` made a literal 0 indistinguishable from "absent" and
	// silently accepted it (design D2; specs/daemon-configuration, "Zero
	// rejected").
	var verr error
	read := func(section, key string, v int) (int, bool) {
		if verr != nil || !meta.IsDefined(section, key) {
			return 0, false
		}
		if v <= 0 {
			verr = fmt.Errorf("config: %s.%s must be positive, got %d", section, key, v)
			return 0, false
		}
		return v, true
	}
	minutes := func(section, key string, v int, dst *time.Duration) {
		if n, ok := read(section, key, v); ok {
			*dst = time.Duration(n) * time.Minute
		}
	}
	minutes("timer", "focus_minutes", f.Timer.FocusMinutes, &d.Focus)
	minutes("timer", "break_minutes", f.Timer.BreakMinutes, &d.Break)
	minutes("idle", "pause_after_minutes", f.Idle.PauseAfterMinutes, &d.IdlePause)
	minutes("idle", "credit_break_after_minutes", f.Idle.CreditBreakAfterMinutes, &d.IdleCredit)
	minutes("camera", "prompt_every_minutes", f.Camera.PromptEveryMinutes, &d.PromptRetry)
	if n, ok := read("camera", "prompt_limit", f.Camera.PromptLimit); ok {
		d.PromptCap = n
	}
	if verr != nil {
		return Defaults(), verr
	}

	return d, nil
}
