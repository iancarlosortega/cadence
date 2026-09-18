# Design: Republish the deadline after a short suspend

## Technical Approach

Add `EffectNotify` to the one effect list that is missing it. `publish()` already recomputes
`PhaseEndsAt` from current state on every notify (`daemon/internal/dbusapi/service.go:194-202`), so
no new computation, field, or adapter change is required. The work that earns its keep here is the
test, and the reasoning about why the neighbouring path is deliberately untouched.

## Architecture Decisions

### Decision 1 — Emit from the domain, not the adapter

The fix goes in `applySuspend`'s returned effects, not in `ApplySuspend` on the D-Bus service.

The domain is the only place that knows whether the deadline moved. `Service.apply` is a dumb
executor: it walks the effect list and performs what it is told
(`daemon/internal/dbusapi/service.go:169-180`). Teaching the adapter to notice "this was a suspend,
so publish" would move a domain rule into an adapter and give the daemon two places that decide when
clients are told things — which is how the defect arose in the first place.

### Decision 2 — `EffectNotify` alone, no `EffectPersist` change

The short-suspend return already carries `EffectPersist{Reason: "resumed from short suspend"}`. That
stays exactly as is. Only the notification was missing; persistence was already correct.

### Decision 3 — The test asserts the effect, not just the state

`TestShortSuspendDoesNotCredit` currently reads:

```go
next, _ := Apply(s, EventSuspended{From: now, To: resumeAt}, resumeAt)
```

The `_` is the defect's hiding place. `Apply` returns `(State, []Effect)` — a two-part contract — and
this test asserted on one part for the whole life of M1. It is amended to bind `effects` and assert
that an `EffectNotify` is present, matching the convention its sibling `TestLongSuspendCreditsBreak`
already follows.

A second test pins the *consequence* rather than the mechanism: after a short suspend, the deadline
a client would derive equals the daemon's own remaining. That one fails even if someone later
"fixes" the effect list by emitting the wrong thing.

Rejected: asserting `len(effects) == 2`. Effect counts are brittle and say nothing about which effect
is present. Assert on the presence of the type.

### Decision 4 — The idle-pause path is left alone, on purpose

Recorded as Finding F2 in the proposal and as an explicit obligation on M4 in the delta spec. Two
reasons, and the second is the real one:

1. That branch runs on every tick while idle, so emitting there unguarded is a per-second heartbeat.
   It needs edge detection, and the natural mechanism — `IdleSince` (`state.go:57`) — is declared but
   never read or written anywhere, so it would have to be brought to life first.
2. During idle, `LastObserved` advances while `ElapsedInPhase` is frozen, so the correct deadline
   moves *continuously* across the idle window, not once at each edge. Edge emission would bound the
   error instead of removing it. Removing it requires deciding what a client shows while the user is
   idle, which changes the D-Bus contract M2 consumes and belongs to M4.

The branch is unreachable today (`IdleSource` stubbed to zero), so nothing ships broken.

## Data Flow

```
login1 PrepareForSleep(false)
   → WatchSleep goroutine (sleep.go)
   → Service.ApplySuspend(EventSuspended{From, To})
   → Service.apply → session.Apply → applySuspend
        duration < IdleCredit
        elapsed unchanged, LastObserved = resume instant
        effects: [EffectPersist, EffectNotify]   ← EffectNotify is the fix
   → Service.apply walks effects
        EffectPersist → store.Save
        EffectNotify  → publish()
   → publish: PhaseEndsAt = now + Remaining()    ← already correct, just never ran
   → PropertiesChanged on the session bus
   → client recomputes; offset returns to zero
```

## File Changes

| File | Change |
|------|--------|
| `daemon/internal/session/machine.go` | `applySuspend` short path: append `EffectNotify`; correct the comment |
| `daemon/internal/session/machine_test.go` | Amend `TestShortSuspendDoesNotCredit` to assert on effects; add a deadline-consequence test |
| `openspec/changes/.../specs/daemon-control/spec.md` | Delta, already written |
| `daemon/internal/dbusapi/` | Untouched |
| `extension/` | Untouched |

## Interfaces / Contracts

Unchanged. No new property, no new method, no new signal, no change to the persisted JSON shape. The
D-Bus surface M2 consumes is identical — it simply receives a `PropertiesChanged` it should always
have received.

## Testing Strategy

- **Unit, deterministic:** `Apply` is pure and takes `now` as a parameter, so both tests construct an
  exact suspend window with no clock dependency and no sleeping.
- **Mutation check, manual and explicit:** remove the `EffectNotify` line, confirm both new
  assertions go red, restore it. A regression test that has never been seen to fail is a guess.
- **Live confirmation:** the measurement that found the defect is the one that closes it — start a
  session, suspend briefly, resume, then sample `PhaseEndsAt - now` against the daemon's
  elapsed-derived remaining three times about 20s apart. Before the fix this held a constant 3s
  offset; after it the offset must be zero and stay zero.
- **No new harness.** The existing table tests and `go test ./...` cover this.

## Migration / Rollout

None. Behaviour change is additive: clients receive one more `PropertiesChanged` than before, on an
event that already existed. An unpatched client is unaffected; a patched daemon with the shipped
extension is strictly more correct. Rollback is a single-commit `git revert`.

## Open Questions

None for this change. One deliberately deferred to M4: what a client displays while the user is
idle, which determines how the idle-pause path satisfies the Change Notification requirement.
