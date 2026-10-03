# Apply Progress: M4 — Idle awareness

**47 of 55 tasks complete.** All code is written and every check that does not need a live desk has
run. The 8 open tasks are Phase 8 live verification, which needs this build installed and a Wayland
logout to reload the extension.

Runtime attempt: acquired `state: proceed`, token `sha256:4ce1…1a3a`, untracked planning artifacts
excluded from the candidate. Settled `passed`, evidence revision
`sha256:4dbe0c96bbd963692987a93994ab7528b076c7a646fed07de069156e4cd2d2d6`.

## What landed

### Domain (`internal/session`)

- `IdleSince` deleted. `Idle bool` and `IdleCredited bool` added, both documented as runtime-only.
- `applyTick`'s credit branch is now conditional on `!s.IdleCredited`. **This is the repeated-credit
  defect from design Decision 2.** Without the latch, observed idle keeps growing and every tick past
  the credit threshold resets the phase and writes state, for the whole absence.
- The idle-pause branch gained an opening edge (emit once when the window opens, silent thereafter).
- The active branch gained a closing edge, and clears `IdleCredited` unconditionally so a latch set
  by `applySuspend` is spent on return.
- `EventPause` clears both flags, enforcing `Paused`-over-`Idle`.
- `applySuspend` sets `IdleCredited` when it credits, so the same absence cannot be credited twice.
- The false "nothing observable changed" comment is gone.

### Adapter (`internal/dbusapi`)

- New `idle.go`: `MutterIdleSource` over `org.gnome.Mutter.IdleMonitor.GetIdletime`, failing safe to
  zero, logging only on availability transitions.
- `Idle` added to the property map, introspection, and `publish`. `publish` now suppresses
  `PhaseEndsAt` for `Paused || Idle`.
- `noIdleSource` deleted from `cmd/cadenced`; the real source is wired.

### Extension

- `Idle` in the interface XML and property cache; `idle` in `DISCONNECTED`.
- `computeDisplay` freezes on `remainingSeconds` and suppresses the warning while idle.
- `menuSensitivity` left deriving from `paused` only, with a comment recording that this is required
  rather than an oversight.
- **`shouldShowOverlay` fixed — see finding F5.**

Found during apply, not planning. `shouldShowOverlay` (`render.js`) derived from `phaseEndsAt`, which
this change publishes as `0` during an idle window. A user who takes their break — stops touching
input for three minutes, which is the entire point — would have crossed the idle-pause threshold and
had the enforcement overlay torn down as "already expired".

M3's overlay is the enforcement mechanism, so this would have silently disabled it in exactly the
case it exists for. Fixed by giving idle the same branch `Paused` already had, and pinned by two
cases in `test-render.js`.

This is the interaction risk the proposal flagged as "first edit to M2's capability" arriving from an
unexpected direction: the overlay belongs to M3, which the proposal did not list as affected.

## Finding F6 — RESOLVED: the idle freeze is now focus-only

Raised as a product question and ruled on by the user on 2026-09-19: **the freeze applies to `focus`
only.** A break is time away from the desk by design, so idling through one is its intended use. The
break must be able to finish while the user is away; freezing it would leave the remainder owed on
their return, making a break taken properly the one that never completes.

Implemented as a phase guard on the freeze branch, plus a narrowing of when the credit latch is
spent: the latch is now cleared only on real activity, not merely on any unfrozen tick, so a break
that the user idles through does not re-arm a credit while the absence is still running.

Two tests pin it (`TestBreakDoesNotFreezeWhileIdle`, `TestBreakCompletesWhileTheUserIsAway`) and a
fourth mutation check confirms them: removing the `s.Phase == PhaseFocus` guard turns both red.

Both delta specs moved first — `session-timer` "Idle Credit" now states the break rule and carries
two new scenarios, and `daemon-control` "Idle Publication" scopes `Idle` to `focus` with a scenario
of its own.

### F5 was dissolved by this ruling

The overlay teardown described below can no longer occur: `Idle` is never true during a break, so
`PhaseEndsAt` stays live throughout one and `shouldShowOverlay` never sees a frozen deadline there.
The defensive branch added for F5 was **removed** rather than left as unreachable code — the same
standard this change applied to `IdleSince`. The original finding is kept below because the
reasoning that surfaced it is what justified the F6 question in the first place.

## Finding F5 — the break overlay would have vanished mid-break (superseded by the F6 ruling)

With idle real for the first time, a break behaves like this: the user walks away, the break freezes
at the idle-pause threshold, and the remainder is still owed when they return. Take a four-minute
break and the overlay still wants seven more minutes on your return.

Over a *full* absence the arithmetic is right — stay away ten minutes and the credit threshold fires,
crediting the break and starting a fresh focus. It is only the return-before-credit case that feels
wrong, and M3's hold-to-skip is an exit.

This follows directly from the M1 product rule "idle >3 min pauses the timer" applied uniformly to
both phases. It is not a defect against any written requirement, and it is deliberately **not**
fixed here. It is recorded for a ruling at verify: should the idle freeze apply during `break` at
all, or only during `focus`?

## Verification performed

| Check | Result |
|---|---|
| `go vet ./...` | clean |
| `go test ./...` (daemon) | all packages pass |
| `gofmt -l` on every file this change touches | clean |
| `gjs -m extension/test-render.js` | all checks pass, including 7 new idle cases |
| `rg IdleSince` over `*.go` / `*.js` | no source reference survives |

### Mutation checks

| # | Mutation | Expected red | Observed |
|---|---|---|---|
| 2.8 | remove the `!s.IdleCredited` guard | long-absence and suspend-latch tests | **both failed, then passed on restore** |
| 2.9 | remove the `!s.Idle` guard on the opening edge | silent-window test | **failed, then passed on restore** |
| 4.7 | remove `Idle` from `publish`'s frozen condition | frozen-interval test | **failed, then passed on restore** |
| F6 | remove the `s.Phase == PhaseFocus` guard | both break tests | **both failed, then passed on restore** |

All four mutation checks are confirmed. Each was observed red before being observed green.

One assertion I wrote was wrong and was corrected: the suspend-latch test asserted an effect count of
1 where the opening edge correctly emits persist *and* notify. The count was a bad proxy; it now
asserts the properties that matter.

## The skip that was hiding in plain sight

`cadenced` (pid 6343) owned `dev.ian.Cadence`, so every test exporting the service **skipped rather
than passed** — and `go test` reported `ok` for the package regardless. This was **pre-existing**:
`TestNoPropertiesChangedOnQuietTick` had been skipping under the same condition since M2, and its
result was read as a pass for two milestones.

It was caught here only because tests containing two 500ms waits completed in 3ms. `ok` on a package
containing D-Bus integration tests is not evidence; `-v` and a grep for `SKIP` is.

With the user's approval the daemon was stopped, the suite run, and the daemon restarted. **All nine
tests in `internal/dbusapi` pass with zero skips**, including the M2-era test that had never actually
run. The daemon was verified back on the bus afterwards, in the same state it held before (no active
session, so nothing was lost).

### Live D-Bus surface of this build — task 7.4

The new binary was run directly, without installing it, and introspected:

```
.Idle  property  b  false  emits-change
```

5 methods and **7 properties** on `dev.ian.Cadence1`, `Idle` present and emitting change. The
adapter logged no availability transition across the run, which means every `GetIdletime` call
reached the compositor — research claims C1 and C2 confirmed from inside the daemon rather than from
a shell probe.

## Still open — 8 tasks, all of Phase 8

Live verification needs this build installed over the running daemon **and** a Wayland logout to
reload the extension, as M2 did. That includes the two observations research could not settle:
locked-screen behavior (C7, task 8.5) and the real-suspend double-credit check (C8, task 8.6).

## Changed lines

| Scope | Lines |
|---|---|
| Tracked diff (`daemon/`, `extension/`) | 559 (533 added, 26 removed) |
| New untracked source (`idle.go`, `idle_test.go`) | 152 |
| **Honest total** | **711** |

The figure rose from 665 after the F6 ruling landed.

Forecast was ~430, so the estimate came in **55% low**. The same pattern as M2, whose forecast was
35% low, and for the same reason: test code is consistently underestimated. Phases 2, 4, 6 and the
store test account for 369 of the 665 lines — 55% of the change is tests.

`size:exception` applies, accepted by the user at preflight. The runtime attempt's
`--max-changed-lines 600` measures the tracked candidate only (513), which is inside its budget.

## Deviations from the plan

- Task 5.5 grew beyond "leave `menuSensitivity` alone": `shouldShowOverlay` needed the same
  treatment (F5). Same file, same cause, not a scope expansion.
- `idleFromMilliseconds` was extracted as a named function so task 4.1 could pin the unit without a
  bus. The design implied the conversion but did not name it.

## Observed, not touched

- `daemon/internal/config/config.go` fails `gofmt`. Pre-existing, unrelated to idle, and not bundled
  into this diff. Task 7.3 scoped itself to files this change touches, and `state.go`'s own
  pre-existing nit was fixed because this change edits that file anyway.
