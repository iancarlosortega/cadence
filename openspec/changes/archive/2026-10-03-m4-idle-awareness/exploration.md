# Exploration: M4 — Idle awareness

Deferred half of the archived change `fix-suspend-deadline-republish`, carried forward as finding
F2. That change fixed the suspend path and deliberately left the idle path alone, because the idle
path is not the same fix.

## Current State

### The idle pipeline exists end to end and is stubbed at exactly one point

| Link | Location | State |
|---|---|---|
| Port | `daemon/internal/session/ports.go:21-25` — `IdleSource.IdleFor(now)` | Defined; comment already names "the Mutter IdleMonitor adapter is M4" |
| Production adapter | `daemon/cmd/cadenced/main.go:31-34` — `noIdleSource` returns `0` | **Stub**; wired at `main.go:86` |
| Sampling | `daemon/internal/dbusapi/service.go:140-144` — `Tick` reads `IdleFor(now)` into `EventTick` | Real |
| Domain | `daemon/internal/session/machine.go:95-112` — `applyTick` | Real, three branches |

Only the adapter is missing. Because it returns a constant zero, `applyTick` always takes its
`default` branch in production: **both idle branches are unreachable today.** That unreachability is
the sole reason the daemon does not currently violate its own `daemon-control` Change Notification
requirement.

### The defect F2 hands to M4

`applyTick`'s idle-pause branch (`machine.go:100-105`):

```go
case ev.IdleFor >= s.Durations.IdlePause:
    // Idle beyond the pause threshold but short of the credit
    // threshold: elapsed time does not advance, nothing observable
    // changed, so no effect is emitted.
    ns.LastObserved = now
    return ns, nil
```

The comment is false, and M4 owes its deletion. `ElapsedInPhase` freezes while wall time keeps
moving, so `Remaining()` (`state.go:98-105`) stays constant while `now` advances. `publish` derives
`endsAt = now.Add(remaining).Unix()` (`service.go:205-213`), so **the correct `PhaseEndsAt` advances
continuously for the entire idle window**, not once at its edges. A client counting down from the
last published value drifts by the full width of the window.

With the configured defaults (`config.go:20-21`, `IdlePause` 3 min, `IdleCredit` 10 min) the window
is bounded at **7 minutes** — after that, `creditBreak` fires and republishes anyway. So edge
detection alone would *bound* the error at 7 minutes, not remove it. That is the finding that
rescoped this work out of the previous change.

### The freeze mechanism already exists on the wire

This is the most important thing exploration found, and it was not known when F2 was written.

`Paused` already solves the identical problem, and the solution is live in both halves:

- Daemon (`service.go:206-213`): when paused, `remaining` comes from the frozen `PausedRemaining`
  and `endsAt` is published as `0`.
- Client (`extension/render.js:30-39`): `computeDisplay` branches on `state.paused` and reads
  `remainingSeconds`, explicitly because "Paused publishes PhaseEndsAt as 0".
- Pinned by `extension/test-render.js:86-87`.

So "what does a client display during a frozen interval" is an **already-answered question** in this
codebase. M4's contract decision is not *whether* to freeze but *under which property*.

### Dead code

`IdleSince` (`state.go:53-57`) is declared with a comment describing precisely the edge-detection
semantics an idle implementation needs, and is **never read or written anywhere** — confirmed by
`rg 'IdleSince'` returning only the declaration. Dead since M1. M4 revives it or deletes it; which
one is decided by the option chosen below.

## Affected Areas

- `daemon/internal/session/machine.go` — the idle-pause branch and its false comment.
- `daemon/internal/session/state.go` — `IdleSince`, revive or delete.
- `daemon/internal/dbusapi/service.go` — `publish`, if a new property is added.
- `daemon/cmd/cadenced/main.go` — replace `noIdleSource` with a real adapter.
- `extension/extension.js:25-41` — the hardcoded introspection XML, **only** if a property is added.
- `extension/render.js` — `computeDisplay` and `menuSensitivity`, same condition.
- `openspec/specs/daemon-control/spec.md` — the Change Notification requirement M4 inherits.
- `openspec/specs/session-timer/spec.md` — Idle Credit requirement.

## Approaches

### Decision 1 — What a client displays during idle *(product decision, see below)*

**Option A — reuse the existing `Paused` property.**
Zero daemon property changes, zero extension changes, and the freeze renders correctly on day one
because `render.js:30-39` already handles it.

Its defect is not the countdown, it is the menu. `menuSensitivity` (`render.js:48-57`) derives the
menu from the same flag: `pause: live && !state.paused`, `resume: live && state.paused`. Publishing
`Paused=true` for an idle freeze would grey out **Pause** and offer **Resume** for a pause the user
never requested — and `Resume` maps to `EventResume` (`machine.go:57-68`), which recomputes
`ElapsedInPhase` from `PausedRemaining`. Firing it against an idle freeze that never set
`PausedRemaining` is a state-corruption path, not just a cosmetic wrong label. Rejecting this option
needs no further research.

**Option B — add an `Idle` boolean, reuse the freeze convention.**
Daemon publishes `endsAt = 0` and a frozen `RemainingSeconds` when idle, exactly as it does for
paused, but under a distinct property. Menu semantics stay truthful: Pause remains available, Resume
stays hidden.

Cost: the extension's interface XML is hardcoded (`extension.js:25-41`) and the proxy is built with
`Gio.DBusProxy.makeProxyWrapper(IFACE_XML)` (`extension.js:42`), so a property absent from that XML
is not surfaced on the proxy at all. Adding `Idle` therefore **requires an extension change** — it is
not transparently backward compatible. This makes M4 touch the `panel-indicator` capability, which
M4 would otherwise leave alone.

**Option C — drop the idle-pause freeze entirely.**
Remove the middle branch: idle below the credit threshold simply charges elapsed time like any other
tick. No new property, no extension change, no `IdleSince`, no edge detection, and the defect
disappears because there is no longer a path that declines to charge time without a transition.

This is a **product** change, not a technical one: it says a 4-minute coffee break counts as work.
It also makes `IdlePause` and its config key (`config.go:85-89`) dead, which is a user-facing
removal.

### Decision 2 — Where the real idle signal comes from *(needs research)*

`ports.go:23` already nominates `org.gnome.Mutter.IdleMonitor`. Candidates identified, none yet
verified against the installed system:

- `org.gnome.Mutter.IdleMonitor` — session-bus, `GetIdletime`, historically the GNOME answer.
- `org.freedesktop.login1` — already a daemon dependency for suspend (`dbusapi/sleep.go`), so no new
  bus connection, but its idle reporting is session/seat-level rather than input-level.
- A GNOME Shell-side reporter feeding the daemon — avoids the Mutter API question entirely but
  inverts the current dependency direction, where the extension is a pure consumer.

**Not verifiable from this repository.** Whether `org.gnome.Mutter.IdleMonitor` is still exported on
GNOME 48 under Wayland, whether it is reachable from a user daemon outside the Shell's own process,
and what it reports while the screen is locked, are all open. This is the natural research lane and
it should be run before design, because the answer can eliminate Option B or C on feasibility
grounds — for example, a signal only available inside the Shell would force the third candidate and
change M4's architecture.

### Decision 3 — `IdleSince`: revive or delete

Conditional on Decision 1. Options A and B need edge detection, so `IdleSince` gets revived. Option C
removes idle state from the domain entirely, so it gets deleted. Either way the M1 dead field stops
being dead — that obligation is discharged under every branch.

## Recommendation

**Option B**, with the research lane run first.

It is the only option that keeps the countdown truthful *and* the menu truthful, and it reuses a
freeze convention this codebase has already proved in production rather than inventing one. Option A
is disqualified by the `Resume`-against-a-non-pause corruption path, not by taste. Option C is
coherent and cheapest, but it silently reverses a product rule the user set at M1, so it must be
their call and not a design convenience.

The extension change Option B forces is real and should be priced into the proposal, not discovered
at apply.

## Risks

- **The idle source is an unknown, and it gates everything.** Every option except C is worthless
  without a working `IdleSource`; C is the only branch that ships value with the adapter still
  stubbed. Research before design.
- **M4 reaches into M2's capability.** Options A and B make `panel-indicator` a modified capability.
  M2 has been archived as consumer-only until now.
- **The 7-minute bound is not a bug the user has ever seen**, because the branch is unreachable. M4
  makes a latent defect reachable — the adapter and the notification fix must land together or M4
  ships the drift it exists to prevent.
- **`TestShortIdlePauses` (`machine_test.go:55-67`) asserts `len(effects) != 0` fails** — it pins
  "no effect for a non-observable idle pause". Options A and B invert that assertion. It is a
  correct test of an incorrect requirement, and the spec must move first.
- **Screen lock interacts with idle** and is unexamined. Locked-and-idle may need to behave
  differently from idle-at-desk; nothing in the current spec addresses it.

## Open Product Decisions

- **P1 — the idle display contract**: Option A, B, or C above. Blocks proposal.
- **P2 — research the idle source before design?** Recommended yes; the answer can eliminate options.

## Ready for Proposal

**No.** P1 is unresolved, and P2 governs whether design proceeds on verified or assumed facts.
