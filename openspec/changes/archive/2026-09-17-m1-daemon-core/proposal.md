# Proposal: M1 — Daemon Core

## Intent

cadence must remind the user to leave the chair. Nothing exists yet. M1 builds the authoritative timer daemon so later UI is a pure adapter over a correct domain — one surviving `gnome-shell --replace`, crashes, and suspend without losing state.

## Scope

### In Scope

- Go module `daemon/` (Go 1.26).
- State machine: focus/break phases, idle-credit, deferral, tier gating.
- Injected `Clock` port; rules testable without real time.
- TOML config `~/.config/cadence/config.toml`; unknown keys rejected.
- Persistence across restart and suspend.
- D-Bus adapter: `dev.ian.Cadence`, `/dev/ian/Cadence`, `dev.ian.Cadence1`.
- CLI `cadence start|stop|status|skip`.
- systemd user unit `cadenced.service`.

### Out of Scope

- All UI: extension, indicator, overlay, corner panel (M2/M3).
- Tier detection — PipeWire, portal, camera (M5); `TierSource` returns `T0`.
- Mutter idle detection (M4); port exists, adapter stubbed.
- Config hot reload (M6). Statistics.

## Capabilities

### New Capabilities

- `session-timer`: phases, durations, idle-credit, deferral, tier gating, suspend.
- `session-persistence`: durable state across restart and suspend.
- `daemon-control`: D-Bus interface and CLI surface.
- `daemon-configuration`: TOML schema, defaults, validation.

### Modified Capabilities

None.

## Approach

Hexagonal. `internal/session` is the domain, importing no D-Bus, GNOME, or OS clock. Ports: `Clock`, `TierSource`, `IdleSource`, `Store`.

D-Bus publishes `Phase`, `PhaseEndsAt`, `Paused`, `RemainingSeconds`, `Tier`, `SessionActive` via `prop.Export` (`Emit: EmitTrue`), emitting only on transitions; clients tick locally.

Suspend is time away: subscribe to `login1` `PrepareForSleep`, persist on `true`, re-evaluate on `false`. Suspend past the idle-credit threshold credits a break; shorter is ordinary idle. Persist elapsed-in-phase, never a deadline — Go's monotonic clock is system-dependent across sleep.

## Affected Areas

All new:

- `daemon/internal/session/` — domain, ports
- `daemon/internal/config/` — TOML
- `daemon/internal/store/` — persistence
- `daemon/internal/dbusapi/` — D-Bus adapter
- `daemon/cmd/cadenced/` — daemon
- `daemon/cmd/cadence/` — CLI
- `packaging/cadenced.service` — systemd unit

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Suspend differs on Fedora 42 | Med | Test before design |
| Pause-as-deadline keeps expiring | Med | Store remaining; table tests |
| Bus name is durable | Low | Fix `dev.ian.Cadence` now |
| Over 400-line review budget | High | `ask-on-risk` at tasks |

## Rollback Plan

`systemctl --user disable --now cadenced.service`; delete unit; `rm -rf ~/.config/cadence ~/.local/state/cadence`. No shared system state touched. Code revert is `git reset`.

## Dependencies

- `github.com/godbus/dbus/v5` v5.2.2
- `github.com/BurntSushi/toml` v1.6.0
- systemd user session; D-Bus session bus


## Success Criteria

- [ ] `go test ./...` passes; rules covered by fake-clock table tests.
- [ ] `cadence start` then `status` reports active focus.
- [ ] Restart mid-focus resumes within 60s.
- [ ] Simulated suspend past threshold credits a break.
- [ ] `busctl --user introspect dev.ian.Cadence /dev/ian/Cadence` lists the interface.
