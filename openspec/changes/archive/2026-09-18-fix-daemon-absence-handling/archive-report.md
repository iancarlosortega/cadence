# Archive Report: fix-daemon-absence-handling

**Change**: fix-daemon-absence-handling
**Project**: cadence
**Archived**: 2026-09-18
**Verdict at verify**: pass — 2/2 requirements, 6/6 scenarios, 0 blockers, 3 warnings

## What shipped

Two defects found while verifying M2 on real hardware, fixed together because they compose.

| File | Diff |
|------|------|
| `daemon/internal/session/machine_test.go` | +61 |
| `daemon/cmd/cadenced/main_test.go` | new, +38 |
| `daemon/internal/dbusapi/service_test.go` | +30 |
| `daemon/internal/dbusapi/service.go` | +21 / −2 |
| `daemon/cmd/cadenced/main.go` | +17 |
| `packaging/cadenced.service` | +2 / −1 |

129 changed lines against a 250 budget. `internal/session/machine.go`, `internal/store/` and
`extension/` untouched — the domain rule already existed and was reused rather than copied.

**F3** — `publish()` converts prop's panic contract into an error with a narrow `recover`, and the
unit binds to `graphical-session.target` so the daemon stops before its bus disappears.

**F4** — startup replays the gap since `LastObserved` as `EventSuspended`, so downtime and suspend
obey one rule.

## Spec changes merged

`openspec/specs/session-persistence/spec.md`:

- **MODIFIED** `Durable State` — its "Restart resumes position" scenario gained
  `AND the daemon was down for less than the idle-credit threshold`. Without that qualification the
  spec would have contradicted itself, because a long downtime now deliberately resets to a fresh
  focus rather than resuming position.
- **ADDED** `Downtime Is Time Away`, mirroring `session-timer`'s "Suspend Is Time Away", with the
  60s measurement tolerance stated in the requirement rather than left as folklore.

Nothing was removed. Requirement count went from 1 to 2.

## Why these two shipped together

Binding the unit to the graphical session makes the daemon **stop at logout**, which converts a long
logout into daemon downtime — precisely what F4 mishandled. Fixing F3 alone would have made F4 more
visible, not less: a twelve-minute logout would have been billed as focus work at the next login.

The verification proved exactly that interaction. One logout produced the evidence for both.

## Evidence at close

Real logout/login cycle, 17:04:18 → 17:16:22, 12m04s out:

```
17:04:18  cadenced: shutting down
17:04:18  Stopped cadenced.service
17:04:18  Stopped target graphical-session.target
17:16:22  Reached target graphical-session.target
17:16:22  Started cadenced.service
```

- **No new panic** — count unchanged at 1, which is the pre-fix crash at 16:04:50.
- **`NRestarts=0`** — stopped, not crashed and restarted.
- **`PartOf` propagation directly observed** — `cadenced` stops *before* the target finishes stopping.
- **Automatic start at login**, no intervention.
- **F4 end to end** — 12m04s against a 10m credit resumed as a fresh `focus`, `RemainingSeconds=180`,
  elapsed 0s. The defect would have produced a `break`.
- Bounded live checks: 20s downtime not charged; 70s downtime against a 60s credit gave elapsed 0s.
- `go test ./...` all five packages, `go vet` clean, both mutation checks fail correctly, extension
  17/17 unaffected.

## Deviations from design

1. **Decision 1 was wrong and was corrected mid-apply.** `prop.Properties.Set` enforces the
   `Writable` flag — false for all six properties — so it broke `New()` with `ErrReadOnly` on the
   first run. The non-panicking `p.set` is unexported, so no exported non-panicking internal write
   exists. `publish()` keeps `SetMust` and catches the panic at the boundary, which the design had
   explicitly rejected before anyone read the library's surface. The design was amended rather than
   left contradicting the code.
2. **Task 5.2's mutation check was invalid as written** and the check itself revealed that: removing
   the startup `ApplySuspend` broke no test, because the domain tests call `Apply` directly and never
   exercise `main.go`. Fixed by extracting `startupGap()` with its own tests in a package that had
   none before.
3. **A tautological test was rewritten.** `TestDowntimeAndSuspendAgree` first compared `Apply`
   against the same event on both sides — it could never fail. It now contrasts the suspend path
   against the tick path and asserts they still differ, pinning why the startup indirection exists.

## Warnings carried forward

1. F4 under-charges by up to 60s after a clean restart, bounded by the heartbeat and within the
   tolerance `Durable State` allows. It replaces an error that was unbounded.
2. Live checks ran on a 3-minute focus and 1-minute break test config; the 50-minute defaults were
   not themselves observed.
3. `recover` in `publish` converts *any* future panic there into an error, not only a closed
   connection. Intended at this boundary, but an unexpected panic now surfaces as a logged tick error
   rather than a crash.

## Findings

F3 and F4 resolved. No new findings. **F2** — the idle-pause republish path — remains open and owned
by M4.

## Process notes worth keeping

- Three defects this session were found by running the system, none by reading it. This change fixed
  two of them and was itself verified by running rather than arguing: the `PartOf` ordering in the
  journal is the kind of evidence no amount of reasoning substitutes for.
- The mutation checks earned their place twice — once by passing, once by exposing a hole in the test
  plan that wrote them.
- A near-miss worth remembering: marking tasks complete by string replacement matched `5.1` inside
  `5.10` and briefly claimed evidence that did not exist. Caught by re-counting open items.

## Delivery

Not committed at time of writing. Commit and push remain separate human decisions.
