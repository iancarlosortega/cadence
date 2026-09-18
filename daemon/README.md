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
`RemainingSeconds`, `Paused`, `Tier`.

## Configuration

`~/.config/cadence/config.toml` (optional — missing file uses defaults):

```toml
[timer]
focus_minutes = 50
break_minutes = 10

[idle]
pause_after_minutes = 3
credit_break_after_minutes = 10
```

Unknown keys and non-positive durations are rejected at startup, naming the
offending key.

## State

`~/.local/state/cadence/session.json` — elapsed-in-phase, never a deadline.
Written on every transition and on a 60s heartbeat.

## Scope

This is M1: the daemon core only. No GNOME extension, no UI. Tier
detection (`TierSource`) is stubbed to always report `T0`; idle detection
(`IdleSource`) is stubbed to always report zero. Both ports exist so later
milestones (M4 idle, M5 tiers) plug in without touching `internal/session`.
