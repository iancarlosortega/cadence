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
	d := session.Durations{
		Focus:      DefaultFocus,
		Break:      DefaultBreak,
		IdlePause:  DefaultIdlePause,
		IdleCredit: DefaultIdleCredit,
	}

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

	if f.Timer.FocusMinutes != 0 {
		if f.Timer.FocusMinutes <= 0 {
			return d, fmt.Errorf("config: timer.focus_minutes must be positive, got %d", f.Timer.FocusMinutes)
		}
		d.Focus = time.Duration(f.Timer.FocusMinutes) * time.Minute
	}
	if f.Timer.BreakMinutes != 0 {
		if f.Timer.BreakMinutes <= 0 {
			return d, fmt.Errorf("config: timer.break_minutes must be positive, got %d", f.Timer.BreakMinutes)
		}
		d.Break = time.Duration(f.Timer.BreakMinutes) * time.Minute
	}
	if f.Idle.PauseAfterMinutes != 0 {
		if f.Idle.PauseAfterMinutes <= 0 {
			return d, fmt.Errorf("config: idle.pause_after_minutes must be positive, got %d", f.Idle.PauseAfterMinutes)
		}
		d.IdlePause = time.Duration(f.Idle.PauseAfterMinutes) * time.Minute
	}
	if f.Idle.CreditBreakAfterMinutes != 0 {
		if f.Idle.CreditBreakAfterMinutes <= 0 {
			return d, fmt.Errorf("config: idle.credit_break_after_minutes must be positive, got %d", f.Idle.CreditBreakAfterMinutes)
		}
		d.IdleCredit = time.Duration(f.Idle.CreditBreakAfterMinutes) * time.Minute
	}

	return d, nil
}
