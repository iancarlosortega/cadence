# cadence daemon

The authoritative timer for cadence. Owns all session state; the GNOME
Shell extension (a later milestone) is a pure consumer over D-Bus.

## Build

```bash
cd daemon
go build ./...
```

Produces two binaries: `cmd/cadenced` (the daemon) and `cmd/cadence` (CLI).

## Run

```bash
go run ./cmd/cadenced &
go run ./cmd/cadence start
go run ./cmd/cadence status
go run ./cmd/cadence pause
go run ./cmd/cadence resume
go run ./cmd/cadence skip
go run ./cmd/cadence stop
```

## Install (systemd user service)

```bash
go build -o ~/.local/bin/cadenced ./cmd/cadenced
go build -o ~/.local/bin/cadence  ./cmd/cadence
mkdir -p ~/.config/systemd/user
cp ../packaging/cadenced.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now cadenced.service
```

## D-Bus interface

| | |
|---|---|
| Bus name | `dev.ian.Cadence` |
| Object path | `/dev/ian/Cadence` |
| Interface | `dev.ian.Cadence1` |

```bash
busctl --user introspect dev.ian.Cadence /dev/ian/Cadence
busctl --user get-property dev.ian.Cadence /dev/ian/Cadence dev.ian.Cadence1 Phase
```

Methods: `StartSession`, `StopSession`, `Pause`, `Resume`, `SkipBreak`.
Properties (`org.freedesktop.DBus.Properties`, `EmitsChangedSignal=true`,
emitted only on real transitions): `SessionActive`, `Phase`, `PhaseEndsAt`,
`RemainingSeconds`, `Paused`, `Tier`, `Idle`, `Hold`, `Prompts`, and `Config`
(`a{si}`, read-only: every configuration key as `section.key`, such as
`timer.focus_minutes`, mapped to its active value; republished when a reload
changes it).

## Configuration

`~/.config/cadence/config.toml` (optional — missing file uses defaults):

```toml
[timer]
focus_minutes = 50
break_minutes = 10

[idle]
pause_after_minutes = 3
credit_break_after_minutes = 10

[camera]
prompt_every_minutes = 5   # minutes between prompts while a break is held on camera
prompt_limit = 3           # prompts shown before the hold becomes a quiet pill
```

Every key is optional; an absent key takes the default shown. Unknown keys and
non-positive values are rejected, naming the offending key as `section.key`.
A key set to `0` is non-positive and is rejected too: earlier versions treated
a literal `0` as "use the default", so a file that contains one now fails at
startup with a diagnostic, and the fix is to delete the line.

### Live reload

The daemon notices a saved change within one tick (5s) and applies it to the
running session, with no restart:

- The phase in progress keeps its elapsed time and is measured against the new
  length. Shortening a phase to at or below the time already worked ends it on
  the next tick. A paused phase keeps its elapsed time too.
- A file that fails to load (unknown key, non-positive value, malformed TOML)
  is ignored: the active configuration stays as it was, and one line is logged
  naming the problem. Saving again retries.
- Deleting the file reverts to the defaults, as a missing file does at startup.
- Saving without changing any value does nothing.

Startup is still strict: an invalid file stops the daemon from starting.

## State

`~/.local/state/cadence/session.json` — elapsed-in-phase, never a deadline.
Written on every transition and on a 60s heartbeat.

## Scope

This is M1: the daemon core only. No GNOME extension, no UI. Tier
detection (`TierSource`) is stubbed to always report `T0`; idle detection
(`IdleSource`) is stubbed to always report zero. Both ports exist so later
milestones (M4 idle, M5 tiers) plug in without touching `internal/session`.
