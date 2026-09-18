# Archive Report: fix-suspend-deadline-republish

**Change**: fix-suspend-deadline-republish
**Project**: cadence
**Archived**: 2026-09-18
**Verdict at verify**: pass (0 blockers, 0 critical findings, 1 warning)

## What shipped

A four-line domain change and its tests. `applySuspend`'s short-suspend path now returns
`EffectNotify` alongside the `EffectPersist` it already returned, so the daemon republishes
`PhaseEndsAt` when it declines to charge a suspend to the current phase.

| File | Diff |
|------|------|
| `daemon/internal/session/machine.go` | +6 / -3 |
| `daemon/internal/session/machine_test.go` | +38 / -1 |

## Spec changes merged

`openspec/specs/daemon-control/spec.md`, Requirement "Change Notification" — MODIFIED.

Was:

> Property changes MUST emit `PropertiesChanged` on transitions only, and MUST NOT emit per second.

Now keys on the observable value rather than the word "transition", because "on transitions only"
was read as "only on phase transitions" — which is precisely how the defect came to be written and
reviewed without anyone noticing. The "No per-second traffic" scenario is preserved byte-for-byte.
Two scenarios added: short suspend republishes the deadline, and a client never drifts across
suspends and transitions.

The `Control Surface` requirement is untouched. Requirement count unchanged at 2. Nothing was removed
— this was an additive rewrite of one requirement body.

## Why this change existed

M2 shipped a GNOME indicator deriving its countdown from `PhaseEndsAt`. Verifying M2 task 6.3 on a
live daemon showed the panel a constant 3 seconds ahead of the daemon after a 9-second suspend, and
the gap never closed. The daemon was correctly declining to charge suspended time (M1 decision P1)
but never telling anyone the deadline had moved. The error was bounded by the idle-credit threshold,
so a suspend just under 10 minutes could have left the panel roughly nine minutes wrong about when to
stand up.

It was found by measuring a running system, not by reading code.

## Evidence at close

- `go test ./...` in `daemon/` passes; `go vet` clean.
- Mutation check: with `EffectNotify` removed both new tests fail; restored, both pass. The
  regression tests have been observed failing.
- 60s of `dbus-monitor` during an active unpaused `focus` with no transition: **0**
  `PropertiesChanged` — the preserved scenario verified against the live fixed daemon.
- Real 9s suspend, full-precision offset `+0.319s` constant across three samples, versus a constant
  3 whole seconds before the fix.
- D-Bus surface unchanged: 5 methods, 6 properties. `internal/dbusapi/`, `internal/store/` and
  `extension/` show 0 changed files.

## Carried forward

- **F2 — idle-pause path, owner M4, deferred.** Same defect class, unreachable today because
  `IdleSource` is stubbed to zero. Cannot be fixed correctly without deciding what a client displays
  during an idle window, since the deadline advances continuously throughout it rather than at its
  edges. M4 also owes: delete the branch's false "nothing observable changed" comment, and either
  revive or remove `IdleSince` (`state.go:57`), declared and documented since M1 but never read or
  written.
- **Warning — `PhaseEndsAt` has whole-second resolution.** `now.Add(remaining).Unix()` truncates, so
  a client sits permanently up to 1s below exact truth. Contract resolution, not drift: it does not
  accumulate and resets at every publish. Success criterion 3's literal "offset zero" is
  unachievable at full precision.
- **Pre-existing**: `daemon/internal/session/state.go` fails `gofmt -l` on a comment-alignment nit.
  Unrelated, deliberately not bundled.

## Effect on M2

M2 finding F1 is resolved. M2 task 6.3 is no longer blocked by this defect; it remains gated on a
Wayland session restart, which is M2's own business.

## Process notes worth keeping

- The defect lived in a discarded return value. `Apply` returns `(State, []Effect)` and
  `TestShortSuspendDoesNotCredit` asserted on the first half only, for the entire life of M1. A
  reducer test that writes `next, _ :=` is testing half its contract.
- Scope was cut at design (rescope R1) after the user had already approved a wider scope on my
  analysis, which turned out to be wrong. The earlier decision is recorded as superseded rather than
  silently honoured.
- Three acceptance criteria written during this work were sharper than the system can be. Criterion 8
  of M2 compared against a transition-only snapshot; the "never drifts" scenario promised a universal
  property; criterion 3 here demanded an offset of exactly zero from a whole-second value. All three
  were corrected rather than quietly passed.

## Delivery

Not committed. Commit and push remain separate human decisions under ordinary repository policy.
