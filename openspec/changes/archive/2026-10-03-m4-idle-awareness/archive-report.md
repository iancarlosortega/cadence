# Archive Report: m4-idle-awareness

**Change**: m4-idle-awareness (Milestone 4)
**Project**: cadence
**Archived**: 2026-10-03
**Verdict at verify**: pass with warnings — 10/10 requirements, 38/38 scenarios, 0 blockers, 4 warnings

## What shipped

Idle detection is real. The zero-stub `IdleSource` is replaced by an adapter over
`org.gnome.Mutter.IdleMonitor.GetIdletime`, and the idle path now tells clients the truth. Focus
freezes after 3 minutes idle. A 10-minute absence credits exactly one break. A new `Idle` property
publishes the window as a frozen interval, and the extension renders it.

| File | Lines |
|------|-------|
| `daemon/internal/dbusapi/idle.go` | new, 98 |
| `daemon/internal/dbusapi/idle_test.go` | new, 54 |
| `daemon/internal/dbusapi/service.go` | +107 / −30 |
| `daemon/internal/dbusapi/service_test.go` | +325 / −5 |
| `daemon/internal/session/machine.go` | +69 / −9 |
| `daemon/internal/session/machine_test.go` | +212 / −2 |
| `daemon/internal/session/state.go` | +25 / −4 |
| `daemon/internal/store/file_test.go` | +33 |
| `daemon/cmd/cadenced/main.go` | +3 / −6 |
| `extension/render.js` | +22 / −3 |
| `extension/test-render.js` | +36 |
| `extension/extension.js` | +2 |

**About 990 changed lines**, under `size:exception` accepted at preflight. The forecast was ~430,
so it came in 130% over, after M2's 35%. Same cause as before, but worse: test code dominates, and
the post-apply fixes F7–F9 added about 250 lines of service code and tests.

## Spec changes merged

| Capability | Modified | Added |
|---|---|---|
| `session-timer` | Idle Credit, Suspend Is Time Away | Idle Source Availability |
| `daemon-control` | Control Surface, Change Notification | Idle Publication |
| `panel-indicator` | Countdown Derivation, Presentation States, Break Warning, Control Actions | — |

The trailing note in `daemon-control` "Change Notification" is removed. It had said idle gating was
unimplemented and that M4 owed the requirement for the idle paths. M4 now discharges it, along with
all four obligations inherited from `fix-suspend-deadline-republish` finding F2.

## The defect found at design

`applyTick`'s credit branch was unconditional. A real idle reading grows for the whole absence and
is never reset by a credit, so every tick past 10 minutes would have reset the phase, written state
and emitted a signal. It had stayed unreachable only because the source was stubbed to zero, which
was exactly the condition M4 removed. The `IdleCredited` latch fixes it. Live check 8.8 confirmed
the fix: a 16.5-minute absence credited once, and `session.json` was written only on the 60s
heartbeat plus the three window edges.

## Defects found in live verification, all fixed

| ID | Defect | Found by |
|---|---|---|
| F7 | Each idle edge emitted 7 `PropertiesChanged` instead of one | `dbus-monitor` capture against the spec |
| F8 | Countdown jumped back 1s at idle exit | The user watching the panel |
| F9 | dbusapi tests silently skipped whenever a real `cadenced` was running | Investigating why F7 had passed the tests |

F9 is the one that matters. For as long as the installed daemon was running, the suite had been
reporting green without running the bus tests at all.

## Research questions answered

- **C7, the lock screen**: Mutter's idle timer runs unbroken across a lock (it reached 703s). No
  screensaver or logind source is needed. An earlier run that suggested otherwise had been
  contaminated by input, which shows a locked-screen test is only valid if nothing touches the
  input devices.
- **C8, double credit across suspend**: **unanswered live.** This hardware wakes from S3 within
  7–9s on IRQ 7 `pinctrl_amd`, even with ACPI `XH00` and USB device wakeup disabled. The user
  waived task 8.6. The latch covers the risk structurally, and
  `TestSuspendAndIdleCreditTheSameAbsenceOnce` pins it.

## Carried forward

- **Re-test C8** on hardware that holds S3.
- **Idle source loss and return mid-session** is covered by inspection only (verify W2).
- **F10**: `cadence status` prints a stale `RemainingSeconds`. The panel spec allows that
  staleness, but the CLI should derive from `PhaseEndsAt` the way the panel does. Its own fix.
- `daemon/internal/config/config.go` fails `gofmt`. Pre-existing, noted at apply.

## Next milestone

M5, tier detection.
