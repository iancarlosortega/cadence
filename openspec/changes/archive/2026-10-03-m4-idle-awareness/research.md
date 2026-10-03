# Research: the idle source for M4

Lane selected 2026-09-19 as product decision P2, because the idle source is the one material claim
in the exploration that cannot be settled from this repository, and because the answer can eliminate
design options on feasibility grounds.

Evidence is graded. **Empirical** means measured on the user's own machine (Fedora 42, GNOME Shell
48, Wayland) on 2026-09-19. **Upstream** means read from the project's source. **Unverified** means
it is still an assumption and is called out as such.

## C1 — `org.gnome.Mutter.IdleMonitor` is live on GNOME 48 under Wayland — **Empirical**

```
$ busctl --user list | rg IdleMonitor
org.gnome.Mutter.IdleMonitor   5516  gnome-shell  ian  :1.25  user@1000.service
```

Exported by the running `gnome-shell` process, on the **session** bus, at
`/org/gnome/Mutter/IdleMonitor/Core`. This is the candidate `ports.go:23` nominated at M1, and it
still exists on the target platform. The exploration's primary unknown is resolved in the
affirmative.

## C2 — It is reachable from a plain user process outside the Shell — **Empirical**

The probe above ran as uid `ian`, pid 673469, an ordinary shell — not inside `gnome-shell`, not a
Shell extension, no portal, no elevated access. `cadenced` is a user service on the same session bus
under the same `user@1000.service`, so it has strictly no less access than the probe did.

This is the finding that keeps M4's architecture as-is. The exploration flagged that a
Shell-only signal would force a Shell-side reporter and **invert the dependency direction**, making
the extension a producer rather than the pure consumer it has been since M2. That inversion is not
required. The extension stays a consumer.

## C3 — Interface shape — **Empirical** (introspection)

```
org.gnome.Mutter.IdleMonitor  interface
.AddIdleWatch        method  t  u
.AddUserActiveWatch  method  -  u
.GetIdletime         method  -  t
.RemoveWatch         method  u  -
.ResetIdletime       method  -  -
.WatchFired          signal  u  -
```

`GetIdletime` takes no arguments and returns `t` (uint64).

## C4 — `GetIdletime` returns milliseconds and tracks wall time — **Empirical**

Three samples, two seconds apart, with no input activity:

| Sample | Value | Delta |
|---|---|---|
| 1 | 19216 | — |
| 2 | 21220 | +2004 |
| 3 | 23224 | +2004 |

The unit is **milliseconds** and it advances 1:1 with elapsed real time while the user is idle. The
existing port signature `IdleFor(now time.Time) time.Duration` (`ports.go:24-25`) maps onto this
directly: `time.Duration(ms) * time.Millisecond`. No port change is needed, and `now` is not even
required by this adapter — the Shell tracks the reference point itself.

## C5 — The interface is officially undocumented — **Upstream**

The upstream `src/org.gnome.Mutter.IdleMonitor.xml` carries exactly one line of documentation — "This
interface is used by gnome-desktop to implement user activity monitoring" — and **no** per-method
docs, no declared unit, and no statement about lock or suspend behavior.

This is a real finding, not a gap in the search. **The unit in C4 is established by measurement
only.** Nothing upstream promises it, so M4 should not treat milliseconds as a contract; a defensive
adapter and a test that would catch a unit change are warranted.

## C6 — Mutter offers native edge detection — **Empirical** (introspection)

`AddIdleWatch(t) -> u` registers a watch that fires `WatchFired` once the given idle threshold is
reached; `AddUserActiveWatch() -> u` fires when the user becomes active again; `RemoveWatch(u)`
cancels.

This bears directly on the `IdleSince` question the exploration left open. The edge detection F2
assumed the daemon must implement — the reason `IdleSince` (`state.go:53-57`) exists — is **available
from the compositor**. Two shapes are now possible:

- **Polling**: `Tick` calls `GetIdletime` once per tick — every 5s, per `tickInterval` in
  `cmd/cadenced/main.go:25` — as the current `EventTick{IdleFor}` design already assumes. Keeps the domain pure and needs no new event type. `IdleSince` gets revived for
  edge detection inside `applyTick`.
- **Watch-driven**: subscribe to `AddIdleWatch(IdlePause)` / `AddUserActiveWatch` and feed discrete
  events, in the way `sleep.go` already turns login1 `PrepareForSleep` into `EventSuspended`. There
  is precedent in this codebase for exactly that adapter shape.

Polling fits the existing port and event with zero contract change; the watch shape matches an
existing adapter pattern but adds an event type. **This is a design decision, not a research one** —
recorded here so design has both options with evidence, and deliberately not settled.

## C7 — Behavior while the screen is locked — **Unverified**

Not testable without locking the user's session, and no upstream documentation states it (C5).
Circumstantial only: 1Password for Linux uses this interface to power "lock after the computer has
been idle for N minutes", which implies it keeps working across a lock, but that is a third-party
inference and **not evidence of what the value does**.

This matters for cadence specifically. A locked screen is unambiguously time away from the desk, so
it should reach `IdleCredit` and credit a break. If idle time froze or reset at lock, it would not.
**Must be verified empirically by M4**, in the same manner as M2's post-logout Shell checks —
observation on a live session, not reasoning.

## C8 — Interaction with the existing suspend path — **Unverified, and the highest risk found**

cadence already treats suspend independently: `sleep.go` turns login1 `PrepareForSleep` into
`EventSuspended`, and `applySuspend` (`machine.go:123-139`) credits a break when the suspend exceeds
`IdleCredit`. If `GetIdletime` **also** accrues across a suspend, then on resume the daemon can
observe both a large `EventSuspended` and a large `IdleFor`, and credit the same absence twice.

Reasoning, not measurement: Mutter derives idle time from input event timestamps, which on Linux use
`CLOCK_MONOTONIC`, and `CLOCK_MONOTONIC` does not advance across suspend (`CLOCK_BOOTTIME` does). If
that holds, idle time does **not** grow while suspended and there is no double-credit. That chain is
plausible and consistent with C4, but it is inference across two layers and is **not** established.

`creditBreak` is idempotent in isolation — it resets to a fresh focus phase, so crediting twice in
immediate succession is harmless. The danger is the ordering where a suspend credit is followed by
an idle credit that resets a phase the user has already legitimately begun.

**Must be measured by M4** with a real suspend, exactly as `fix-suspend-deadline-republish` measured
its own fix rather than trusting the unit tests.

## C9 — The Shell is not always there — **Empirical**, from the same probe

The name is owned by the `gnome-shell` process, so it disappears when the Shell restarts or has not
yet started. `cadenced` is a user service with no ordering guarantee against the Shell, so the
adapter must tolerate the name being absent at startup and vanishing at runtime.

The daemon already has both the precedent and the scar tissue for this: finding F3, fixed in
`fix-daemon-absence-handling`, was a panic from calling into a dead D-Bus connection at logout. An
idle adapter that assumes the Shell is present would reintroduce that class of defect.

**Failing safe means returning zero idle** — the current stub's value. Unavailable idle detection
degrades cadence to its present behavior rather than freezing a timer or crediting a phantom break.

## Consequences for the exploration's options

- **Option B is feasible.** C1 and C2 remove the only identified feasibility blocker.
- **Option C is not forced.** It was the exploration's fallback for a world where no usable idle
  source exists. That world does not obtain, so C should now be chosen on product grounds or not at
  all.
- **`IdleSince` may not need reviving.** C6 puts edge detection in the compositor. Design decides.
- **Two empirical checks are owed before M4 can be called verified**: locked-screen behavior (C7) and
  the suspend interaction (C8). Both are live-session observations, in the same category as M2's
  post-logout checks.

## Open items handed to design

1. Polling versus watch-driven adapter (C6).
2. Whether the unit is asserted defensively, given no upstream contract (C5).
3. The credit-ordering rule if C8 turns out to double-count.
4. Where the locked-screen check lands in the task list (C7).

## Sources

- Live session bus on the user's machine, 2026-09-19 (`busctl --user`) — C1, C2, C3, C4, C6, C9.
- [endlessm/mutter — `src/org.gnome.Mutter.IdleMonitor.xml`](https://github.com/endlessm/mutter/blob/master/src/org.gnome.Mutter.IdleMonitor.xml) — C5.
- [jwz — Wayland and screen savers](https://www.jwz.org/blog/2023/09/wayland-and-screen-savers/) — background on why Wayland has no screensaver-based idle path, supporting the choice of a compositor interface.
- [k4yt3x/idlemon](https://github.com/k4yt3x/idlemon) — third-party consumer of this same interface, corroborating C1/C2 as a usable public surface.
