# Proposal: Make idle detection real and honest to clients

## Intent

cadence has shipped four milestones without ever knowing whether the user is at the desk.
`IdleSource` has been stubbed to zero since M1 (`daemon/cmd/cadenced/main.go:31-34`), so both idle
branches of `applyTick` are unreachable in production and the timer charges a lunch break as work.

M4 makes idle real, and resolves the contract question the previous change deferred as finding F2:
what a client displays while the user is idle, given that the correct `PhaseEndsAt` advances
continuously across an idle window rather than at its edges.

Product decision **P1 = Option B** (2026-09-19): freeze the countdown as `Paused` already does, but
under a distinct `Idle` property. Product decision **P2 = research first** (2026-09-19): the lane ran
and is recorded in `research.md`.

## Scope

### In Scope

- A real `IdleSource` adapter over `org.gnome.Mutter.IdleMonitor`, replacing `noIdleSource`.
- A new read-only `Idle` D-Bus property, published using the freeze convention `Paused` already uses:
  `PhaseEndsAt = 0` and a frozen `RemainingSeconds`.
- Emitting `EffectNotify` at the idle edges, discharging the `daemon-control` Change Notification
  obligation for the paths M4 makes reachable.
- Deleting the false "nothing observable changed" comment at `machine.go:101-103`.
- Resolving `IdleSince` (`state.go:53-57`) — revive or delete, decided at design.
- Extension changes to consume `Idle`: the interface XML (`extension.js:25-41`), the property cache
  (`extension.js:120-131`), and `computeDisplay` / `menuSensitivity` (`render.js`).
- The two empirical checks research left open: locked-screen behavior (C7) and the suspend
  interaction (C8).

### Out of Scope

- Tier detection. `TierSource` stays stubbed; that is M5.
- Changing the configured thresholds. `IdlePause` 3 min and `IdleCredit` 10 min stand.
- Changing how long a suspend is charged. `applySuspend` keeps its M1 semantics; M4 only has to
  avoid crediting the same absence twice (C8).
- The break overlay. M3's behavior during idle is untouched beyond whatever follows from the phase
  not advancing.
- A non-GNOME idle backend. cadence targets this desk; `ext-idle-notify-v1` is not in scope.

## Capabilities

### New Capabilities

None. Idle is a property of the existing session timer, not a new capability.

### Modified Capabilities

- **`daemon-control`** — a new `Idle` property on the D-Bus surface, and the Change Notification
  requirement discharged for the idle paths.
- **`session-timer`** — the Idle Credit requirement gains real reachability, and the
  "Short idle pauses" scenario changes from "no effect" to "notify once at the edge".
- **`panel-indicator`** — the extension consumes `Idle`. M2 has been consumer-only until now; this
  is the first change to edit it, and it is a direct consequence of choosing Option B.

## Approach

### Why Option B and not the cheaper alternatives

Option A — reusing the existing `Paused` property — costs nothing and renders correctly on day one,
because `render.js:30-39` already handles the freeze. It was rejected for a specific defect, not on
taste: `menuSensitivity` (`render.js:48-57`) derives the menu from that same flag, so an idle freeze
would offer **Resume** for a pause the user never requested. `Resume` maps to `EventResume`
(`machine.go:57-68`), which recomputes `ElapsedInPhase` from `PausedRemaining` — a value an idle
freeze never sets. That is a state-corruption path, not a wrong label.

Option C — dropping the idle freeze entirely — was the exploration's fallback for a world in which no
usable idle source exists. Research C1/C2 established that world does not obtain, so C would now be a
product reversal chosen for convenience. The user declined it.

### The freeze convention is reused, not invented

This is the part that keeps M4 small. The daemon already publishes a frozen interval correctly
(`service.go:206-213`) and the client already reads one correctly (`render.js:30-39`), pinned by
`test-render.js:86-87`. M4 adds a second trigger for an existing, proven mechanism rather than
designing a new one.

### What research changed

`org.gnome.Mutter.IdleMonitor` is exported by `gnome-shell` on the session bus and is reachable from
a plain user process outside the Shell (C1, C2), so the extension stays a pure consumer and no
Shell-side reporter is needed. `GetIdletime` returns uint64 milliseconds advancing 1:1 with real
time (C4), which maps onto the existing `IdleFor(now)` port with no signature change.

Research also found that Mutter offers native edge detection — `AddIdleWatch`, `AddUserActiveWatch`,
`WatchFired` (C6) — which means the edge detection F2 assumed the daemon must build may not need to
be built at all. Polling versus watch-driven is left to design, with both options evidenced.

### Two checks are owed, and one is a real risk

Research could not settle locked-screen behavior (C7) or the suspend interaction (C8), and the
interface is undocumented upstream (C5), so neither can be settled by reading.

C8 is the one to watch. cadence already credits a long suspend via login1 and `applySuspend`. If
`GetIdletime` also accrues across a suspend, a resume could observe both a large `EventSuspended`
and a large `IdleFor` and credit the same absence twice. The `CLOCK_MONOTONIC` reasoning suggests it
does not, but that is inference across two layers. It gets measured, not assumed — the same
discipline that made `fix-suspend-deadline-republish` trustworthy.

## Affected Areas

| File | Change |
|---|---|
| `daemon/cmd/cadenced/main.go` | Replace `noIdleSource` with the real adapter |
| `daemon/internal/dbusapi/` | New idle adapter; `Idle` in the property map and in `publish` |
| `daemon/internal/session/machine.go` | Idle edges emit `EffectNotify`; delete the false comment |
| `daemon/internal/session/state.go` | `IdleSince` revived or deleted |
| `daemon/internal/session/machine_test.go` | `TestShortIdlePauses` inverts |
| `extension/extension.js` | Interface XML and property cache gain `Idle` |
| `extension/render.js` | `computeDisplay` and `menuSensitivity` honor idle |
| `openspec/specs/daemon-control/spec.md` | `Idle` property; Change Notification discharged |
| `openspec/specs/session-timer/spec.md` | Idle Credit becomes reachable |
| `openspec/specs/panel-indicator/spec.md` | Indicator behavior while idle |

## Risks

- **M4 makes a latent defect reachable.** The adapter and the notification fix must land together,
  or M4 ships the very drift it exists to prevent. They belong in one change, not two.
- **Double-crediting an absence** if C8 turns out to accrue across suspend.
- **The unit is unpromised.** C5 found no upstream contract for milliseconds; a silent unit change
  upstream would corrupt every threshold. Assert it defensively.
- **The Shell is not always there** (C9). The adapter must tolerate the name being absent at startup
  and vanishing at runtime, failing safe to zero idle — the current stub's value. Getting this wrong
  reintroduces the F3 defect class fixed in `fix-daemon-absence-handling`.
- **`TestShortIdlePauses` is a correct test of an incorrect requirement.** The spec must move before
  the test, or the change looks like a regression.
- **Screen lock is unspecified today.** Nothing in the current spec says what locked-and-idle means;
  M4 has to decide it rather than inherit it.
- **First edit to M2's capability.** `panel-indicator` stops being consumer-only.

## Rollback Plan

Revert to `noIdleSource`. Because idle is currently unreachable, restoring the stub restores exactly
today's behavior: both idle branches go dark again and the `Idle` property is published as a
constant false. The extension tolerates this without a downgrade, since a permanently-false `Idle`
is the same code path as "not idle".

## Dependencies

- Inherits finding **F2** and its four obligations from `fix-suspend-deadline-republish`.
- Requires GNOME Shell running and owning `org.gnome.Mutter.IdleMonitor` at runtime — degrades to
  zero idle when absent (C9), never fails.
- No dependency on M5 tier detection.

## Success Criteria

1. With the user idle past `IdlePause`, the daemon publishes `Idle = true`, `PhaseEndsAt = 0`, and a
   frozen `RemainingSeconds`, exactly once at the edge — not once per tick.
2. A live `dbus-monitor` across a full idle window shows **one** `PropertiesChanged` at entry and
   **one** at exit, preserving the "No per-second traffic" scenario.
3. The panel indicator shows a frozen countdown while idle and resumes from the correct value on
   return, with **Pause** still offered and **Resume** still hidden.
4. Idle past `IdleCredit` credits a break, as it always specified but never reachably did.
5. C7 answered by observation: locked-screen idle behavior is documented in the verify report.
6. C8 answered by measurement: a real suspend produces exactly one break credit, not two.
7. `IdleSince` is either read and written, or gone. No dead field survives M4.
8. `go vet` and `go test ./...` clean; the extension's `gjs` render tests pass.
