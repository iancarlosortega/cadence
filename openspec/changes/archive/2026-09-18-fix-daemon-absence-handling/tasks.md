# Tasks: Handling absences the daemon did not witness

## Review Workload Forecast

Estimated changed lines: **~110**.

| Artifact | Estimate |
|----------|----------|
| `daemon/internal/dbusapi/service.go` | ~12 (publish returns error, apply wraps it) |
| `daemon/cmd/cadenced/main.go` | ~6 (apply the startup gap) |
| `packaging/cadenced.service` | ~3 |
| `daemon/internal/session/machine_test.go` | ~55 (three table tests) |
| `daemon/internal/dbusapi/service_test.go` | ~20 (publish error path) |
| runbook | ~40 (markdown, not code) |

Delivery strategy: `single-pr`.

Decision needed before apply: **No**.
Chained PRs recommended: No
Chain strategy: null
400-line budget risk: **None**.

Estimate padded — M2's forecast came in 35% low. Even at double this it stays well inside budget.

### Suggested Work Units

One. Per design Decision 4 the two fixes must ship together: binding the unit to the graphical
session converts a long logout into daemon downtime, which is exactly what the F4 fix interprets
correctly. Shipping F3 alone would bill a three-hour logout as focus work.

## Phase 1: F3 — stop panicking

- [x] 1.1 `daemon/internal/dbusapi/service.go` — `publish()` becomes `publish() error`, using `prop.Properties.Set` instead of `SetMust` for all six properties; return the first error
- [x] 1.2 `daemon/internal/dbusapi/service.go` — the `EffectNotify` arm of `apply()` wraps a publish failure in `dbus.MakeFailedError`, mirroring how the `EffectPersist` arm already wraps a store failure
- [x] 1.3 Confirm no other caller of `publish()` needs updating (`New()` seeds properties through it)

## Phase 2: F3 — stop outliving the session

- [x] 2.1 `packaging/cadenced.service` — add `PartOf=graphical-session.target`, keep `After=`, change `[Install] WantedBy=` to `graphical-session.target`
- [x] 2.2 Keep `Restart=on-failure` and `RestartSec=2` unchanged — a clean stop at logout is not a failure, and a real crash should still restart

## Phase 3: F4 — one rule for unwitnessed time

- [x] 3.1 `daemon/cmd/cadenced/main.go` — after `dbusapi.New(...)` and only when a session was resumed, call `svc.ApplySuspend(session.EventSuspended{From: initial.LastObserved, To: now})`
- [x] 3.2 One line at the call site naming why a suspend event carries downtime — the non-obvious constraint, per the project's comment policy. Not a narration of the code

## Phase 4: Tests

- [x] 4.1 `machine_test.go` — long downtime credits a break: 12m elapsed, 1h gap, expect `focus` with 0 elapsed (spec: *Long downtime credits a break*)
- [x] 4.2 `machine_test.go` — short downtime does not credit: 12m elapsed, 90s gap, expect elapsed unchanged (spec: *Short downtime does not credit*)
- [x] 4.3 `machine_test.go` — downtime and suspend agree: same phase and elapsed reached by both paths for the same duration, asserted directly rather than by inspection (spec: *Downtime and suspend agree*)
- [x] 4.4 `service_test.go` — `publish()` returns an error rather than panicking when the connection is closed; assert the error surfaces through `Tick()`

## Phase 5: Verification

- [x] 5.1 `go test ./...` in `daemon/` — all packages pass
- [x] 5.2 **Mutation check** — as written this was WRONG: removing the startup call broke no test, because 4.1/4.3 call `Apply` directly and never touch `main.go`'s wiring. The check found a real hole in this plan. Fixed by extracting `startupGap()` with its own tests in `cmd/cadenced/main_test.go`; mutating it to always skip now fails `TestStartupGapReplaysTheAbsence`
- [x] 5.3 **Mutation check** — removing the `recover` from `publish` fails 4.4 with "publish panicked on a closed connection instead of returning an error". Note the fix is not `Set`: see the Deviation below
- [x] 5.4 `git diff --stat` — only the four intended files plus tests; `internal/session/machine.go`, `internal/store/` and `extension/` untouched
- [x] 5.5 `gjs -m extension/test-render.js` — 17/17 still pass; the extension is unaffected
- [x] 5.6 Write `verification-runbook.md` in fish, covering the one-time `disable`/`enable` re-wiring, a real logout, and a real long-gap restart
- [x] 5.7 Live: `make -C packaging install-daemon`, then `systemctl --user disable cadenced` and `enable` once so the new `[Install]` section takes effect
- [x] 5.8 Live: stop the daemon for longer than the idle-credit threshold, start it, confirm a **fresh focus** rather than a break (spec: *Overnight shutdown does not open on a break*)
- [x] 5.9 Live: brief stop and restart, confirm elapsed does not jump by the downtime
- [x] 5.10 Verified 2026-09-18 on a real logout/login cycle (17:04:18 → 17:16:22, 12m04s out). Panic count unchanged at 1 (the pre-fix crash at 16:04:50); `NRestarts=0`, so the unit was stopped rather than crashed; journal shows `cadenced: shutting down` immediately **before** `Stopped target graphical-session.target`, which is `PartOf` propagation directly observed; started automatically at login with no intervention. The 12m gap also exercised F4 end to end: resumed as a **fresh focus**, `RemainingSeconds=180`, elapsed 0s, where an uncharged-gap bug would have given `break`

## Note on what only a logout can prove

Tasks 5.10 and the real-shutdown case in 5.8 cannot be simulated. `systemctl --user stop` exercises
the downtime rule but not the `PartOf` propagation, which is the half of F3 that stops the daemon
before its bus disappears. That one needs a genuine session cycle, exactly as M2 did.
