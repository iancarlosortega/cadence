# Tasks: M1 — Daemon Core

## Review Workload Forecast

Estimated changed lines: ~900-1300 (reducer, config, store, D-Bus adapter, sleep subscriber, 2 binaries, unit, tests). Delivery strategy: exception-ok. User accepted `size:exception` up front (personal project, no push — GitHub CLI unconfigured).

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: High

### Suggested Work Units

1. **Domain** — `internal/session` reducer+ports. Test: `go test ./internal/session/...`. Harness: N/A, pure function. Rollback: `rm -rf daemon/internal/session`.
2. **Config+Store** — adapters. Test: `go test ./internal/config/... ./internal/store/...`. Harness: N/A, file I/O only. Rollback: `rm -rf daemon/internal/config daemon/internal/store`.
3. **D-Bus** — adapter+sleep subscriber. Test: `go test ./internal/dbusapi/...`. Harness: `busctl --user introspect dev.ian.Cadence /dev/ian/Cadence`. Rollback: `rm -rf daemon/internal/dbusapi`.
4. **Binaries** — CLI+daemon+unit. Test: `go build ./...`. Harness: `cadence start && cadence status`. Rollback: `rm -rf daemon/cmd packaging/cadenced.service`.

All four ship as one PR under the accepted exception.

## Phase 1: Foundation

- [x] 1.1 `daemon/go.mod` — module `cadence/daemon`, Go 1.26; add `godbus/dbus/v5` v5.2.2, `BurntSushi/toml` v1.6.0
- [x] 1.2 `daemon/internal/session/state.go` — `Phase`, `State`, `Event`, `Effect` types (no imports beyond stdlib)
- [x] 1.3 `daemon/internal/session/ports.go` — `Clock`, `TierSource`, `IdleSource`, `Store` interfaces

## Phase 2: Domain (reducer + tests)

- [x] 2.1 `daemon/internal/session/machine.go` — `Apply(State, Event, time.Time) (State, []Effect)`: phase cycle, idle-credit, pause-as-remaining, suspend-as-idle, T0 tier gating
- [x] 2.2 `daemon/internal/session/clock.go` — `FakeClock` test double
- [x] 2.3 `daemon/internal/session/machine_test.go` — table test per `session-timer` scenario (8 scenarios: focus/break elapse, short/long idle, pause-survives-3h, long/short suspend, T0 starts break)
- [x] 2.4 Verify: `go test ./internal/session/...` — all 8 pass

## Phase 3: Config and Persistence

- [x] 3.1 `daemon/internal/config/config.go` — TOML load, defaults (focus 50m, break 10m, idle-pause 3m, idle-credit 10m), `Undecoded()` rejection with offending key named
- [x] 3.2 `daemon/internal/config/config_test.go` — no-file defaults; unknown-key rejection (`daemon-configuration` scenarios)
- [x] 3.3 `daemon/internal/store/file.go` — write-temp-then-rename to `~/.local/state/cadence/session.json`; stores `elapsed_in_phase`, never a deadline
- [x] 3.4 `daemon/internal/store/file_test.go` — restart-resumes-position; no-persisted-state (`session-persistence` scenarios)
- [x] 3.5 Verify: `go test ./internal/config/... ./internal/store/...`

## Phase 4: D-Bus Adapter

- [x] 4.1 `daemon/internal/dbusapi/service.go` — export `dev.ian.Cadence1` at `/dev/ian/Cadence`, `RequestName("dev.ian.Cadence")`; methods `StartSession/StopSession/Pause/Resume/SkipBreak`
- [x] 4.2 `daemon/internal/dbusapi/service.go` — `prop.Export` for `SessionActive/Phase/PhaseEndsAt/RemainingSeconds/Paused/Tier`, all `Emit: prop.EmitTrue`; emit only on transitions
- [x] 4.3 `daemon/internal/dbusapi/sleep.go` — subscribe `org.freedesktop.login1.Manager.PrepareForSleep`; persist on `true`, re-evaluate elapsed on `false`
- [x] 4.4 `daemon/internal/dbusapi/service_test.go` — start-then-inspect; start-when-active-noop; no `PropertiesChanged` over 60s idle (`daemon-control` scenarios)
- [x] 4.5 Verify: `go test ./internal/dbusapi/...` against a real session bus

## Phase 5: Binaries and Unit

- [x] 5.1 `daemon/cmd/cadenced/main.go` — wire Store→session→dbusapi, real `Clock`, stub `TierSource` returning `T0`
- [x] 5.2 `daemon/cmd/cadence/main.go` — CLI `start|stop|status|skip` as thin D-Bus client
- [x] 5.3 `packaging/cadenced.service` — `WantedBy=default.target`, `Restart=on-failure`
- [x] 5.4 Verify: `go build ./...`; `cadence start` then `cadence status` shows `focus`; `busctl --user introspect dev.ian.Cadence /dev/ian/Cadence`

## Phase 6: Cleanup

- [x] 6.1 `daemon/README.md` — build/run/install steps, bus name, CLI usage
- [x] 6.2 Confirm no file under `internal/session/` imports `dbus`, GNOME, or calls `time.Now` directly
