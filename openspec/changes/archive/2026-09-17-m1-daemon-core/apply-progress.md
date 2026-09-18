# Apply Progress: M1 — Daemon Core

**Mode**: Standard (`strict_tdd: false`)
**Batch**: 1 of 1 — all 23 tasks completed in one pass.

## Completed Tasks

All 23 tasks across Phases 1-6 are complete — see `tasks.md` for the checked list.

## Files Changed

| File | Action | What Was Done |
|---|---|---|
| `daemon/go.mod`, `go.sum` | Created | Module `cadence/daemon`, Go 1.26; `godbus/dbus/v5` v5.2.2, `BurntSushi/toml` v1.6.0 |
| `daemon/internal/session/state.go` | Created | `Phase`, `Tier`, `Durations`, `State`, `Effect` types |
| `daemon/internal/session/ports.go` | Created | `Clock`, `TierSource`, `IdleSource`, `Store` interfaces |
| `daemon/internal/session/event.go` | Created | `Event` types (Tick, StartSession, StopSession, Pause, Resume, SkipBreak, Suspended) |
| `daemon/internal/session/machine.go` | Created | `Apply` pure reducer: phase cycle, idle-credit, pause-as-remaining, suspend-as-idle, T0 tier gating |
| `daemon/internal/session/clock.go` | Created | `RealClock`, `FakeClock` |
| `daemon/internal/session/machine_test.go` | Created | 10 tests: all 8 `session-timer` scenarios + 2 supporting cases |
| `daemon/internal/config/config.go` | Created | TOML load, defaults, `Undecoded()`-based unknown-key rejection |
| `daemon/internal/config/config_test.go` | Created | 4 tests: `daemon-configuration` scenarios + overrides + non-positive rejection |
| `daemon/internal/store/file.go` | Created | Write-temp-then-rename JSON persistence; stores elapsed-in-phase, never a deadline |
| `daemon/internal/store/file_test.go` | Created | 3 tests: `session-persistence` scenarios + no-partial-file check |
| `daemon/internal/dbusapi/service.go` | Created | D-Bus export (`dev.ian.Cadence`/`/dev/ian/Cadence`/`dev.ian.Cadence1`), methods via `ExportMethodTable`, properties via `prop.Export`, manual introspection export |
| `daemon/internal/dbusapi/sleep.go` | Created | `login1.Manager.PrepareForSleep` subscriber on the system bus |
| `daemon/internal/dbusapi/service_test.go` | Created | 3 tests against the real session bus: start-then-inspect, start-noop, no-quiet-tick-traffic |
| `daemon/cmd/cadenced/main.go` | Created | Daemon entry point: config+store load, D-Bus export, tick/heartbeat loop, sleep watcher |
| `daemon/cmd/cadence/main.go` | Created | CLI client: `start\|stop\|status\|skip\|pause\|resume` |
| `daemon/README.md` | Created | Build/run/install, D-Bus interface, config, state, scope |
| `packaging/cadenced.service` | Created | systemd user unit, `Restart=on-failure` |

## Deviations from Design

- **Introspection export was initially omitted**, following an ambiguous documentation excerpt suggesting it is "automatically managed." Running the real daemon and `busctl --user introspect` immediately disproved this (`does not implement ... Introspectable`). Fixed by fetching the actual godbus example source and restoring an explicit `introspect.Node` + `conn.Export(introspect.NewIntrospectable(node), ...)`, then re-verified against the real bus. Not a design deviation in intent — the design never specified introspection mechanics — but worth recording as the one place initial code shipped a real, caught bug.
- Method export uses `ExportMethodTable` rather than `Export`, to guarantee `Tick`, `Heartbeat`, and `ApplySuspend` (internal-only Go methods) can never be reached as D-Bus method calls regardless of Go export casing. Not specified in design but consistent with its stated D-Bus trust-boundary decision.
- No other deviations — implementation matches design.md.

## Issues Found

None outstanding. One issue (missing introspection) was found and fixed during this batch; see Deviations.

## Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and result | `go test ./...` from `daemon/` — 20 tests, all pass (10 session + 4 config + 3 store + 3 dbusapi) |
| Runtime harness command/scenario and result | Real daemon+CLI run against the live session bus: `cadence start/status/pause/resume/stop` all correct; `busctl --user introspect dev.ian.Cadence /dev/ian/Cadence` lists all 5 methods and 6 properties; simulated crash (`kill -9`) followed by restart correctly resumed the persisted session |
| Rollback boundary | `rm -rf daemon packaging`; no shared system state touched; no prior commits exist |

## Native Runtime Attempt Ledger

- Acquired: `request-id apply-m1-daemon-core-batch1-20260914`, `max-changed-lines 1600`
- Settled: `outcome passed`, `changed_lines 1706` (exceeded the 1600 cap set at acquire; forecast was 900-1300)
- Reset by maintainer (`ian`, personal project) citing the pre-accepted `size:exception` — `decision_required: false` after reset, work preserved (reset candidate tree matches the finished candidate tree)

## Status

23/23 tasks complete. **Ready for verify.**
