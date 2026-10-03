# Tasks: M4 — Idle awareness

## Review Workload Forecast

Estimated changed lines: **~430**.

| Artifact | Estimate |
|----------|----------|
| `daemon/internal/dbusapi/idle.go` | ~70 (new: adapter, fail-safe, transition logging) |
| `daemon/internal/session/machine.go` | ~40 (latch, edge logic, pause clears idle, comment removal) |
| `daemon/internal/session/state.go` | ~10 (delete `IdleSince`, add two booleans) |
| `daemon/internal/dbusapi/service.go` | ~12 (property map, introspection, frozen condition) |
| `daemon/cmd/cadenced/main.go` | ~8 (delete stub, wire real source) |
| `daemon/internal/session/machine_test.go` | ~110 (edge, latch, re-credit, pause precedence) |
| `daemon/internal/dbusapi/service_test.go` | ~80 (idle publication, quiet window, frozen deadline) |
| `extension/render.js` | ~25 (idle freeze, warning suppression) |
| `extension/extension.js` | ~6 (XML, property cache) |
| `extension/test-render.js` | ~65 (freeze, resume, warning, menu sensitivity) |
| `daemon/internal/store/file_test.go` | ~4 (assert idle flags are not persisted) |

400-line budget risk: **High**. Estimated above the 400 ceiling, and M2's forecast came in 35% low,
so the real figure could reach ~580.

Chained PRs recommended: **No** — see below.
Decision needed before apply: **No**.

Delivery strategy: `exception-ok`. The user accepted `size:exception` up front at preflight
(2026-09-19) and develops this personally, pushing straight to main. **This run uses
`size:exception`.** The forecast is reported rather than acted on; no prompt is owed.

Splitting was considered and rejected on correctness grounds, not convenience. Design "Migration /
Rollout" requires the daemon and extension halves to land together: a daemon publishing `Idle` to an
extension that does not read it freezes `PhaseEndsAt` at `0` with no client-side branch to handle
it. A split would ship a known-broken intermediate state, which is worse than a large review.

Roughly 45% of the estimate is test code, which reviews faster than the logic it covers.

### Suggested Work Units

1. Domain — state fields, latch, edge logic, and their tests.
2. Adapter — `idle.go`, wiring, publication, and their tests.
3. Extension — XML, cache, render, and their tests.
4. Live verification — the two checks that need a real session.

## Phase 1: Domain state and edge logic

- [x] 1.1 Delete `IdleSince` from `State` (`state.go:53-57`); add `Idle bool` and `IdleCredited bool` with comments stating what each means and that neither is persisted.
- [x] 1.2 Confirm `store/file.go` persists neither new field, and that `State` round-trips unchanged.
- [x] 1.3 Latch the credit branch in `applyTick`: credit only when `ev.IdleFor >= IdleCredit && !s.IdleCredited`; set `Idle` and `IdleCredited` on the credited state.
- [x] 1.4 Add the window-open edge to the idle-pause branch: when `!s.Idle`, set `Idle` and emit persist + notify; when `s.Idle`, stay silent.
- [x] 1.5 Add the window-close edge to the active branch: when `s.Idle`, clear `Idle` and `IdleCredited` and emit persist + notify before advancing elapsed time.
- [x] 1.6 Delete the false "nothing observable changed" comment (`machine.go:101-103`) and replace it with one describing the edge behavior.
- [x] 1.7 Set `Idle = false` and `IdleCredited = false` in `EventPause`, enforcing the `Paused`-over-`Idle` precedence from `daemon-control` "Idle Publication".
- [x] 1.8 Set the same flags in `applySuspend` when it credits a break, so a following tick sees an already-credited window.

## Phase 2: Domain tests

- [x] 2.1 Invert `TestShortIdlePauses` (`machine_test.go:55-67`): elapsed still unchanged, but the effects list now carries notify. Reference the amended `session-timer` "Short idle pauses".
- [x] 2.2 Test that a second tick inside an open window emits nothing.
- [x] 2.3 Test that a long absence credits exactly one break: apply repeated ticks with growing `IdleFor` past the credit threshold, assert one credit and silence after it.
- [x] 2.4 Test that a second absence credits again after the user returns in between.
- [x] 2.5 Test the window-close edge: elapsed resumes advancing from the return instant, not from the window's start.
- [x] 2.6 Test that `EventPause` during an open window clears `Idle`.
- [x] 2.7 Test that a credited suspend followed by a tick with large `IdleFor` credits only once — the C8 risk, pinned in the domain.
- [x] 2.8 **Mutation check**: remove the `IdleCredited` guard; confirm 2.3 and 2.7 go red; restore.
- [x] 2.9 **Mutation check**: remove the `!s.Idle` guard from the window-open branch; confirm 2.2 goes red; restore.

## Phase 3: D-Bus adapter

- [x] 3.1 Create `daemon/internal/dbusapi/idle.go` with a `mutterIdleSource` calling `org.gnome.Mutter.IdleMonitor.GetIdletime` at `/org/gnome/Mutter/IdleMonitor/Core` on the session bus.
- [x] 3.2 Convert the returned `uint64` explicitly as milliseconds; do not rely on an implicit unit.
- [x] 3.3 Return zero idle on any error, per `session-timer` "Idle Source Availability". Never propagate the error to callers and never terminate.
- [x] 3.4 Track last availability; log only when it changes, not per failed call.
- [x] 3.5 Add `Idle` to the property map and to the introspection node in `dbusapi.New`.
- [x] 3.6 Extend `publish` so `Paused || Idle` suppresses `PhaseEndsAt` to `0`; keep `PausedRemaining` used only when `Paused`.
- [x] 3.7 Replace `noIdleSource` in `cmd/cadenced/main.go` with the real source; delete the stub type.

## Phase 4: Adapter tests

- [x] 4.1 Test that a known millisecond value converts to the expected `time.Duration` — the C5 unit guard.
- [x] 4.2 Test that an unreachable interface yields zero idle and no error.
- [x] 4.3 Test against the real bus that entering a window emits exactly one `PropertiesChanged`, following `TestNoPropertiesChangedOnQuietTick`'s pattern.
- [x] 4.4 Test that ticks inside an open window emit nothing.
- [x] 4.5 Test that `Idle` true publishes `PhaseEndsAt` as `0` and `RemainingSeconds` as the frozen remainder.
- [x] 4.6 Test that `Paused` and `Idle` are never both true.
- [x] 4.7 **Mutation check**: remove `Idle` from `publish`'s frozen condition; confirm 4.5 goes red; restore.
- [x] 4.8 Add a `store` test asserting the idle flags do not survive a save/load round trip.

## Phase 5: Extension

- [x] 5.1 Add `<property name="Idle" type="b" access="read"/>` to `IFACE_XML` (`extension.js:25-41`).
- [x] 5.2 Add `idle: !!p.Idle` to the property cache in `_refresh` (`extension.js:120-131`) and to `DISCONNECTED` in `render.js`.
- [x] 5.3 Branch `computeDisplay` on `state.idle` alongside `state.paused`, reading `remainingSeconds`.
- [x] 5.4 Suppress the break warning while idle, per `panel-indicator` "Break Warning".
- [x] 5.5 Leave `menuSensitivity` deriving from `paused` only — add a comment stating that this is required, not an oversight, and why `Resume` must not be offered for an idle freeze.

## Phase 6: Extension tests

- [x] 6.1 Test that `Idle` true freezes the countdown on `remainingSeconds` and never renders negative.
- [x] 6.2 Test that the countdown resumes from `PhaseEndsAt` when `Idle` goes false.
- [x] 6.3 Test that the warning is not applied while idle.
- [x] 6.4 Test that `menuSensitivity` offers `Pause` and withholds `Resume` while `Idle` is true and `Paused` is false.
- [x] 6.5 Run `gjs extension/test-render.js`; all cases pass.

## Phase 7: Static verification

- [x] 7.1 `go vet ./...` clean.
- [x] 7.2 `go test ./...` passes in `daemon/`.
- [x] 7.3 `gofmt -l` reports no file this change touched. The pre-existing `state.go` alignment nit noted in `fix-suspend-deadline-republish` is fixed here, since this change edits that file anyway.
- [x] 7.4 `busctl --user introspect dev.ian.Cadence /dev/ian/Cadence` shows 5 methods and 7 properties.
- [x] 7.5 Confirm no `IdleSince` reference survives anywhere: `rg IdleSince` returns nothing.

## Phase 8: Live verification

These need a real session and cannot be unit tested. They carry research C7 and C8 and proposal
success criteria 5 and 6.

- [x] 8.1 Start a session, stop touching input, and confirm the indicator freezes within one tick of the pause threshold.
- [x] 8.2 Run `dbus-monitor` across a full idle window; confirm exactly one `PropertiesChanged` at entry and one at exit, and none between.
- [x] 8.3 Return from idle and confirm the countdown resumes from the frozen value and then tracks the daemon.
- [x] 8.4 Open the indicator menu while idle; confirm `Pause` is offered and `Resume` is not.
- [x] 8.5 **C7**: lock the screen, wait past the credit threshold, unlock. Record whether `Idle` tracked the lock and whether a break was credited. Document the answer in the verify report either way — this is the first observation of a behavior no documentation states.
- [x] 8.6 **C8** (WAIVED 2026-10-03 by user: not observed live, hardware blocked — see note): suspend past the credit threshold and resume. Confirm exactly one break was credited, not two. This is the direct observation of the risk Decision 2 addresses by argument.
  - **Waived — blocked (hardware), 2026-10-03.** Carried into the verify report as an unverified risk; coverage is the unit tests plus the 2026-09-19 accidental 9s short-suspend observation. Four attempts (`systemctl suspend`, `rtcwake -m mem -s 660` x3) all woke within 7-9s per `journalctl -k` (`PM: suspend entry` -> `exit`). Wake IRQ 7 `pinctrl_amd` (AMD GPIO); persisted with ACPI `XH00` and USB wakeup on `5-2`, `7-1.3`, `7-1.4` disabled. Not a cadence defect. Each 11-minute window was an awake-idle window and credited exactly once.
- [x] 8.7 Stop `gnome-shell`'s ownership of the idle interface if feasible, or otherwise confirm by inspection, that the daemon keeps running and reports zero idle without log spam.
- [x] 8.8 Leave a session idle past the credit threshold for at least 15 minutes; confirm from the journal and the state file's mtime that exactly one credit occurred and state was not rewritten every tick — the direct check on the Decision 2 defect.
