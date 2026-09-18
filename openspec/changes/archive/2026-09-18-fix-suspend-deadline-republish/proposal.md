# Proposal: Republish the deadline when it moves without a phase change

## Intent

Make `PhaseEndsAt` mean what every client already assumes it means. The daemon publishes an absolute
deadline and expects clients to derive their countdown from it — that is M1 decision D1 and M2
invariant I1. But two code paths move the effective deadline without republishing, so clients drift
away from the daemon permanently and silently. Fix both, close the spec ambiguity that allowed them,
and fix the test that hid one of them.

## Scope

### In Scope

- `applySuspend` short-suspend path: emit `EffectNotify` alongside the existing `EffectPersist`.
- `TestShortSuspendDoesNotCredit`: stop discarding effects; assert the notification is emitted.
- A new table test pinning the republished deadline after a short suspend.
- A delta on `daemon-control`'s Change Notification requirement so the trigger is a changed published
  value rather than the ambiguous word "transition".

### Out of Scope

- The GNOME extension. It is correct as written; it drifts because it is told to. No M2 file changes.
- The `sleep.go` cold-start edge, where `sleepStarted` initialises to `clock.Now()` so a daemon
  starting *during* a suspend computes `From` = daemon start. Real, adjacent, separate.
- **The idle-pause path, rescoped out at design on 2026-09-18.** It carries the same defect in
  principle, but it is not the same fix and it is unreachable today — see Finding F2 below.
- Any change to how long a suspend is charged. M1 decision P1 stands untouched — this is about
  telling clients what happened, not about changing what happened.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `daemon-control` — the Change Notification requirement is rewritten to key on a changed published
  value, with scenarios for both paths.

## Approach

One line, in one path. `publish()` already derives the deadline correctly from current state
(`endsAt = now + Remaining()`, `service.go:194-202`), so emitting the effect is the entire fix — no
new computation, no new field, no adapter change.

The spec change is the substantive part. Today:

> Property changes MUST emit `PropertiesChanged` on transitions only, and MUST NOT emit per second.

"On transitions only" was read as "only on phase transitions", which is how both defects were
written without anyone noticing. The requirement becomes: a published property MUST emit whenever its
correct value changes, and a tick that changes nothing MUST stay silent. That forbids the heartbeat
the original clause was protecting against while no longer permitting a silent deadline shift.

### Finding F2 — why the idle-pause path was rescoped out

The original scope claimed the idle branch of `applyTick` was the same one-line fix. Design proved
that wrong on two counts.

First, that branch runs on **every tick** while idle, so an unguarded `EffectNotify` there emits once
per second — precisely the heartbeat the preserved "No per-second traffic" scenario forbids. It needs
edge detection. The natural mechanism is `IdleSince`, which is declared in `State`
(`daemon/internal/session/state.go:57`) and **never read or written anywhere** — dead since M1.

Second, and decisively: during idle, `LastObserved` advances while `ElapsedInPhase` is frozen, so the
correct deadline moves *continuously* for the whole idle window, not once at each edge. Edge-only
emission still leaves a client wrong for up to the idle-credit threshold. Being right requires
deciding what a client displays while the user is idle — freeze as `Paused` does, or publish a new
`Idle` property — which changes the D-Bus contract M2 consumes and is squarely M4's design work.

The branch is unreachable today because `IdleSource` is stubbed to zero, so nothing ships broken by
leaving it. M4 inherits it as a named requirement rather than a surprise.

### Why the test change carries equal weight

`TestShortSuspendDoesNotCredit` asserted the state and discarded the effects. The bug lived in the
discarded half for the entire life of M1. Both new and amended tests assert on the effect list, so a
future edit that drops a notification fails loudly instead of shipping a silent client drift.

## Affected Areas

- `daemon/internal/session/machine.go` — two effect lists, one comment deleted.
- `daemon/internal/session/machine_test.go` — one test amended, one added.
- `openspec/changes/fix-suspend-deadline-republish/specs/daemon-control/` — delta spec.
- `daemon/internal/dbusapi/` — untouched.
- `extension/` — untouched.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| A future edit silently drops the notification again | Med | Both tests assert on the effect list, which is precisely what was missing |
| The idle-pause path stays defective | Certain | Unreachable today (`IdleSource` stubbed); recorded as Finding F2 and as an explicit obligation on M4 in the delta spec |
| Fix is correct but M2's task 6.3 still cannot run | Certain | 6.3 needs a Wayland session restart regardless; unblocking it is the point, running it is M2's business |

## Rollback Plan

`git revert` of a single commit. The change adds effects to two returns and touches no persisted
shape, no D-Bus signature, and no client. A daemon running the old code and a client expecting the
new behaviour is exactly today's situation, which is the bug — there is no new incompatibility to
roll back into.

## Dependencies

None. No new Go modules, no schema change, no coordination with the extension.

## Success Criteria

1. `go test ./...` in `daemon/` passes, including the amended and new tests.
2. `TestShortSuspendDoesNotCredit` fails if `EffectNotify` is removed from the short-suspend path —
   verified by removing it once and watching the test go red.
3. On a live daemon: start a session, suspend briefly, resume, then sample `PhaseEndsAt - now`
   against the daemon's elapsed-derived remaining three times ~20s apart. The offset is zero and
   stays zero — the same measurement that produced a constant 3s offset before the fix.
4. M2's task 6.3 becomes runnable: it is blocked on this defect, not on anything in M2.
