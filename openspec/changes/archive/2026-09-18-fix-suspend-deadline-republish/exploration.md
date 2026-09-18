# Exploration: Republish the deadline when it moves without a phase change

## Current State

M2 shipped a GNOME Shell indicator that derives its countdown from `PhaseEndsAt`, an absolute unix
second published by the daemon. Invariant I1 makes the daemon the sole authority for state and the
client a pure renderer. That is sound — **provided `PhaseEndsAt` is republished every time the
effective deadline moves.** It is not.

### The defect

`applySuspend` (`daemon/internal/session/machine.go:123-139`):

```go
// Short suspend: ordinary idle, elapsed does not advance. Still
// persisted so the on-disk LastObserved reflects the resume instant.
ns := s
ns.LastObserved = ev.To
return ns, []Effect{EffectPersist{Reason: "resumed from short suspend"}}
```

Declining to charge the suspended time is correct — it is M1 product decision P1, "suspended time is
time away from the desk". But not charging it **moves the effective deadline forward by the suspend
duration**, and the returned effects contain no `EffectNotify`. `publish()` only runs from an
`EffectNotify` (`daemon/internal/dbusapi/service.go:169-188`), so `PhaseEndsAt` is never
recalculated. Every client keeps counting to a deadline the daemon no longer believes in, until some
later transition happens to republish it.

The long-suspend path (`creditBreak`) does emit `EffectNotify`. Only the short path omits it.

### Measured on the running daemon, 2026-09-18

`systemd-logind` recorded a 9-second suspend (15:03:57 → 15:04:06). Afterwards, the value the
extension renders (`PhaseEndsAt - now`) sat against the daemon's elapsed-derived truth:

| sample | daemon true | extension renders | offset |
|--------|-------------|-------------------|--------|
| t+0s   | 2659s       | 2656s             | 3s     |
| t+20s  | 2639s       | 2636s             | 3s     |
| t+40s  | 2619s       | 2616s             | 3s     |

A **constant** offset, not converging drift — so not measurement noise. It never closes on its own.

The offset is 3s rather than the full 9s because a periodic `Tick` can land between resume and the
`EventSuspended` being applied, charging part of the gap. The un-charged remainder is what leaks.

### Why it reached production undetected

`TestShortSuspendDoesNotCredit` (`machine_test.go:124-133`) discards the effects:

```go
next, _ := Apply(s, EventSuspended{From: now, To: resumeAt}, resumeAt)
```

It asserts on `next.ElapsedInPhase` and never looks at what was emitted. Its sibling
`TestLongSuspendCreditsBreak` *does* capture `effects` and assert `len(effects) == 0` fails, and 19
references to `effects` exist across the file — so the convention was there. This one test simply
opted out of it, and the missing notification became invisible.

## Affected Areas

- `daemon/internal/session/machine.go` — `applySuspend`, and see Decision 2 below for `applyTick`.
- `daemon/internal/session/machine_test.go` — the short-suspend test must assert on effects.
- `openspec/specs/daemon-control/` — the Change Notification requirement needs a delta.
- `daemon/internal/dbusapi/` — expected untouched; `publish()` already computes the deadline
  correctly from the current state (`endsAt = now + Remaining()`), so emitting is all that is needed.

## Approaches

### Decision 1 — What the spec should actually require

`specs/daemon-control` currently says:

> Property changes MUST emit `PropertiesChanged` on transitions only, and MUST NOT emit per second.

Read strictly, the implementation already violates it: the correct value of `PhaseEndsAt` changed and
nothing was emitted. But "on transitions only" is doing ambiguous work — a resume from a short
suspend is not a *phase* transition, which is plainly how the code read it.

**Recommended:** amend the requirement so the trigger is the observable value, not the word
"transition" — every change to a published property's correct value MUST emit, and the prohibition
stays scoped to what it was actually protecting against (a quiet tick). This closes the ambiguity
rather than patching one instance of it.

Rejected: leaving the spec alone and fixing only the code. The next person writing an effect list
faces the identical ambiguity, and the spec would still not describe the system.

### Decision 2 — Scope: the idle-pause path has the same defect

`applyTick` (`machine.go:100-105`):

```go
case ev.IdleFor >= s.Durations.IdlePause:
    // Idle beyond the pause threshold but short of the credit
    // threshold: elapsed time does not advance, nothing observable
    // changed, so no effect is emitted.
    ns.LastObserved = now
    return ns, nil
```

The comment's claim — "nothing observable changed" — is **false for exactly the same reason**. If
elapsed does not advance while wall time passes, the correct deadline moves forward, and a client
counting from the published one drifts. It also emits no `EffectPersist`, so `LastObserved` is not
durable here.

It is not reachable today: `IdleSource` is stubbed to zero idle until M4. But `Apply` is a pure
function, so the path is fully testable now by passing `EventTick{IdleFor: ...}` — this is not
hypothetical future-proofing, it is an existing provable defect in existing code with an existing
test seam.

This is a genuine scope fork and is put to the user below.

### Decision 3 — Does emitting here violate "no per-second traffic"?

No. A suspend/resume is a rare, discrete event. The idle-pause case (if included) emits once when the
threshold is crossed, not once per tick — the state stops advancing, so repeated ticks in the idle
window produce no further change. The requirement's intent is to forbid a heartbeat, not to forbid
reporting real changes.

## Recommendation

Add `EffectNotify` to the short-suspend return, amend the Change Notification requirement so the
trigger is a changed value rather than the ambiguous word "transition", and fix
`TestShortSuspendDoesNotCredit` to assert on effects rather than discarding them. The test fix
matters as much as the code fix: it is the reason this shipped.

## Risks

- **Low blast radius.** The domain is a pure function with existing table tests, and the change adds
  an effect rather than altering a state computation.
- **A regression here is silent**, exactly as the original defect was. The new test must assert the
  effect is present, not merely that the run did not error.
- **The `sleep.go` cold-start edge** is adjacent but out of scope: `sleepStarted` initialises to
  `clock.Now()` at goroutine start, so a daemon that starts *during* a suspend and sees only the
  `false` (resume) signal computes `From` = daemon start. Worth its own look; it is not this bug.

## Ready for Proposal

Yes, once Decision 2 is settled. The mechanism, the evidence, the fix and the test gap are all
established; the only open question is whether the idle-pause path is in scope.
