# Design: Make idle detection real and honest to clients

## Technical Approach

A real `IdleSource` polls `org.gnome.Mutter.IdleMonitor.GetIdletime` once per tick and feeds the
existing `EventTick{IdleFor}`. The domain gains two booleans and edge logic; the adapter gains a
frozen-interval branch it already has for `Paused`. The extension gains one property.

The shape of the change is deliberately unremarkable. Every mechanism it needs — a stubbed port to
fill, a freeze convention on the wire, a pure reducer over events — already exists and was built for
this milestone. The interesting work is in two places: a latent defect the stub has been hiding, and
the decision not to build edge detection the compositor already offers.

## Architecture Decisions

### Decision 1 — Poll `GetIdletime`, do not subscribe to idle watches

Research C6 found that Mutter offers `AddIdleWatch(t)`, `AddUserActiveWatch()` and `WatchFired`, so
the compositor can push edges instead of the daemon polling for them. `sleep.go` is precedent for
exactly that adapter shape.

Polling wins, and the deciding fact is the tick interval. `tickInterval` is **5 seconds**
(`cmd/cadenced/main.go:25`), not one second. Polling therefore costs **12 D-Bus round trips per
minute** to `gnome-shell`, which is not a load worth designing around. The cost that would have
justified a subscription is not there.

Against that small cost, polling buys:

- **No contract change.** `IdleFor(now) time.Duration` (`ports.go:24-25`) maps onto `GetIdletime`
  directly. The port, the event, and every existing table test keep their shape.
- **No subscription lifecycle.** Watches are registered by id and must be re-registered when the
  Shell restarts (C9) and re-issued when configured thresholds change. That is real bookkeeping in
  the adapter, and its failure mode is silent: a watch that was never re-registered produces no
  signal, which is indistinguishable from a user who is simply active.
- **The domain stays a total function of the sampled state.** With watches, idleness becomes an
  edge the daemon is told about once; if that edge is missed the domain has no way to recover. With
  polling, every tick re-derives the truth, so a missed sample costs at most 5 seconds.

The last point is the real argument. Polling is self-correcting and subscription is not, and this
codebase has already paid once for state that drifted because nobody republished it.

Latency cost: an idle edge is observed up to 5 seconds late. Against a 3-minute pause threshold that
is noise.

### Decision 2 — The credit branch is broken for any real idle source, and must latch

This is the find of the design phase. `applyTick`'s credit branch (`machine.go:97-98`) is:

```go
case ev.IdleFor >= s.Durations.IdleCredit:
    return creditBreak(s, now)
```

`IdleFor` from a real source grows monotonically while the user is away. It does not reset when a
break is credited — nothing in the domain can reset it, because it is the compositor's number. So
once idle passes the credit threshold:

| t | `IdleFor` | Branch | Result |
|---|---|---|---|
| 10m00s | 10m00s | credit | phase reset, persist + notify |
| 10m05s | 10m05s | credit | phase reset, persist + notify |
| 10m10s | 10m10s | credit | phase reset, persist + notify |

Every tick, forever, for as long as the user is away. A state reset, a disk write and a
`PropertiesChanged` every five seconds — violating `daemon-control` "No per-second traffic", and
burning the disk on a laptop left at lunch.

This is **unreachable today only because `IdleSource` is stubbed to zero**, which is precisely the
condition M4 removes. It is a latent defect that ships the moment idle becomes real, and it is not
covered by any existing test because `TestLongIdleCredits` applies exactly one event.

The fix is a latch in the domain: `State.IdleCredited`, set when a window credits a break, cleared
when the user returns. The credit branch becomes conditional on it.

**This also removes risk C8 structurally.** Research could not settle whether `GetIdletime` accrues
across a suspend, which raised the possibility of crediting one absence twice — once via
`EventSuspended`, once via the following tick. With the latch, `applySuspend` sets the same flags
when it credits, so the subsequent tick sees a window already credited and stays silent. **The
design is correct whether or not idle accrues across suspend.** C8 still gets measured, because a
design that is correct by argument and a system that is correct by observation are different
claims — but the measurement is now a confirmation rather than a load-bearing dependency.

### Decision 3 — `Idle` and `IdleCredited` replace `IdleSince`, which is deleted

`IdleSince` (`state.go:53-57`) was declared at M1 to hold the instant idle began, for edge detection
the daemon was assumed to need. It has never been read or written.

It is not needed. Edge detection requires knowing only *whether a window is open*, not when it
opened, because the domain never computes anything from the window's start. Two booleans carry
everything:

```go
Idle         bool // an idle window is open
IdleCredited bool // this window has already credited a break
```

`IdleSince` is deleted. The F2 obligation to "revive or remove" it is discharged by removal, and the
replacement is smaller than the field it replaces. A `time.Time` that nothing reads was an invitation
to write logic that keys on it.

### Decision 4 — Idle needs no frozen-remaining field, unlike `Paused`

`Paused` carries `PausedRemaining` because `EventResume` recomputes `ElapsedInPhase` from it.

Idle needs no equivalent, and the reason is worth stating so nobody adds one for symmetry: during an
idle window `ElapsedInPhase` is frozen, so `Remaining()` (`state.go:98-105`) is **already constant
for the window's whole duration**. The value that must be published frozen is the value the existing
method already returns. `publish` needs only to treat idle as a frozen interval:

```go
frozen := s.state.Paused || s.state.Idle
endsAt := int64(0)
if s.state.Active && !frozen {
    endsAt = now.Add(remaining).Unix()
}
```

with `remaining` still taken from `PausedRemaining` only when `Paused`.

This is the whole adapter change. The continuously-advancing deadline that made F2 hard is solved by
not publishing a deadline at all during the window — the same answer `Paused` reached in M1.

### Decision 5 — `EventPause` clears `Idle`

The spec requires `Paused` to take precedence, and `applyTick` returns early while paused
(`machine.go:91-93`), so a window open at the moment of pause would otherwise stay open and publish
both flags true. `EventPause` sets `Idle = false` and `IdleCredited = false` as part of the pause
transition. `EventStartSession` and `EventStopSession` build fresh states and need no change.

### Decision 6 — The adapter fails safe to zero, and says so once

Per `session-timer` "Idle Source Availability", an unreachable interface reports zero idle. In
practice that means a `GetIdletime` call whose error is swallowed into `0`.

Two details that are easy to get wrong:

- **No name watching.** A failed call is sufficient evidence; adding a `NameOwnerChanged`
  subscription to decide whether to call is more state for no gain, and it would need its own
  recovery path.
- **Log on transition, not per failure.** At 12 calls a minute, logging every failure fills the
  journal with 17,000 lines a day while the Shell is down. The adapter tracks its last availability
  and logs only when it changes.

The daemon already learned this lesson: finding F3 was a panic from calling into a dead connection
at logout, fixed in `fix-daemon-absence-handling`.

### Decision 7 — Assert the unit, because nothing upstream promises it

Research C5 found the upstream interface has one line of documentation and does not declare a unit.
Milliseconds is measurement, not contract (C4). The adapter converts explicitly
(`time.Duration(ms) * time.Millisecond`) and a test pins a known value's conversion, so a unit change
upstream fails a test rather than silently multiplying every threshold by 1000.

### Decision 8 — Idle flags are runtime state and are not persisted

`store/file.go` maps `State` onto an explicit record, so adding fields persists nothing by default.
That default is kept deliberately.

Idle is derived from a live compositor reading and is re-derived within one tick of startup. Persisting
it risks a stale latch suppressing a legitimate credit after a restart. The cost is the opposite edge:
a daemon restarted mid-window loses its latch and may credit one extra break on the next tick. That is
bounded at one credit per restart, and it errs toward giving the user a break they may have already
had rather than withholding one they earned.

## Data Flow

```
gnome-shell (Mutter IdleMonitor)
  └─ GetIdletime -> uint64 ms        [every 5s, error -> 0]
       └─ mutterIdleSource.IdleFor(now) -> time.Duration
            └─ Service.Tick -> EventTick{IdleFor, Tier}
                 └─ session.Apply -> applyTick
                      ├─ IdleFor >= IdleCredit && !IdleCredited -> creditBreak + latch  -> persist, notify
                      ├─ IdleFor >= IdlePause && !Idle          -> open window          -> persist, notify
                      ├─ IdleFor >= IdlePause &&  Idle          -> silent
                      └─ active after a window                  -> close window         -> persist, notify
                           └─ Service.publish -> PhaseEndsAt=0 + frozen RemainingSeconds while Idle
                                └─ PropertiesChanged -> extension -> frozen MM:SS
```

## File Changes

| File | Change |
|---|---|
| `daemon/internal/dbusapi/idle.go` | New: `mutterIdleSource`, fail-safe, transition logging |
| `daemon/cmd/cadenced/main.go` | Delete `noIdleSource`; wire the real source |
| `daemon/internal/session/state.go` | Delete `IdleSince`; add `Idle`, `IdleCredited` |
| `daemon/internal/session/machine.go` | Latch the credit branch; edge logic; `EventPause` clears idle; delete the false comment |
| `daemon/internal/dbusapi/service.go` | `Idle` in the property map, introspection, and `publish` |
| `daemon/internal/session/machine_test.go` | Invert `TestShortIdlePauses`; add edge, latch and re-credit tests |
| `daemon/internal/dbusapi/service_test.go` | Idle publication and quiet-window tests |
| `extension/extension.js` | `Idle` in the interface XML and the property cache |
| `extension/render.js` | Idle freeze in `computeDisplay`; warning suppressed; `menuSensitivity` untouched |
| `extension/test-render.js` | Idle freeze, resume, warning suppression, menu sensitivity |

## Interfaces / Contracts

`dev.ian.Cadence1` gains one read-only property, `Idle` (`b`). Additive: the five methods and six
existing properties are unchanged. An older client that does not know about `Idle` sees a session
whose `PhaseEndsAt` is `0` during idle, which is the condition it already handles for `Paused` — it
will freeze on a stale `RemainingSeconds` rather than render nonsense.

## Testing Strategy

Domain tests drive `Apply` directly, as the existing table tests do; no desktop session required.

Three mutation checks, because a regression test never seen to fail is a guess:

1. Remove the `IdleCredited` guard from the credit branch — the repeated-credit test must go red.
2. Remove the `!s.Idle` guard from the window-open branch — the quiet-window test must go red.
3. Remove `Idle` from `publish`'s frozen condition — the frozen-deadline test must go red.

Two checks cannot be unit tested and are owed live, carrying research C7 and C8 and proposal
criteria 5 and 6:

- **Locked screen**: lock, wait past the credit threshold, unlock, observe whether a break was
  credited and whether `Idle` tracked the lock.
- **Real suspend**: suspend past the credit threshold, resume, confirm **exactly one** break was
  credited — the direct observation of the C8 risk Decision 2 addresses by argument.

Both follow M2's pattern of live-session verification rather than inference.

## Migration / Rollout

No state migration: the persisted record is unchanged (Decision 8), so an existing state file loads
untouched and a downgrade reads it back. Rollback is restoring `noIdleSource`, which returns the
daemon to today's behavior exactly, with `Idle` published as a constant false — a value the
extension handles as "not idle" without a downgrade.

The daemon and extension halves must land together. A daemon publishing `Idle` to an extension that
does not read it would freeze `PhaseEndsAt` at `0` with no client-side branch to handle it.

## Open Questions

- Whether locked-and-idle should behave differently from idle-at-desk. The spec currently treats
  them identically and the live check (C7) may argue otherwise; if it does, that is a follow-up
  change, not a rescope of this one.
- Whether `IdlePause` and `IdleCredit` remain the right defaults once idle is real for the first
  time. This change deliberately does not touch them; the first week of real use is better evidence
  than any argument made now.
