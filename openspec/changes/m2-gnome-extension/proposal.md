# Proposal: M2 — GNOME Shell Extension

## Intent

Prove that the two halves of cadence talk. M1 built a correct, fully unit-tested timer domain that
nothing can see. M2 puts it in the top bar: a panel indicator that reflects daemon session state
over D-Bus, counts down locally, turns amber two minutes before a break is due, and drives the
daemon back through its five existing methods.

## Scope

### In Scope

- A new `extension/` tree: `metadata.json`, `extension.js`, `stylesheet.css`, uuid
  `cadence@ian.dev`, `shell-version: ["48"]`.
- A `PanelMenu.Button` indicator added via `Main.panel.addToStatusArea`, always present in the
  panel.
- An async `Gio.DBusProxy` against `dev.ian.Cadence` / `/dev/ian/Cadence` / `dev.ian.Cadence1`,
  with `Gio.bus_watch_name` handling a daemon that is absent, starting late, or restarting.
- Reading the six existing properties and reacting to `g-properties-changed`.
- A local 1s `GLib.timeout_add_seconds` source deriving MM:SS from `PhaseEndsAt`, and from
  `RemainingSeconds` while paused.
- An amber style class applied when remaining time crosses 120s during `focus`.
- A popup menu invoking `StartSession`, `StopSession`, `Pause`, `Resume`, `SkipBreak`.
- A `disable()` that removes the timeout source, disconnects every handler, unwatches the name,
  destroys the indicator, and nulls every reference.
- A `packaging/Makefile` installing the `cadenced` binary, the extension, and the systemd user unit.

### Out of Scope

- Any change to `daemon/`. M2 is a consumer of the M1 contract. If it turns out a daemon change is
  needed, that is a finding to surface, not work to absorb.
- The fullscreen break overlay and hold-to-skip (M3).
- Real idle gating and auto-credit (M4) — `IdleSource` stays stubbed.
- Real tier detection (M5) — `Tier` is displayed if useful but always reads `T0`.
- Config hot reload and any user-tunable threshold (M6). The 120s warning is a constant.
- Publishing to extensions.gnome.org. This is a personal install.

## Capabilities

### New Capabilities

- `panel-indicator` — the GNOME Shell surface: presence and presentation states, the client-side
  countdown, the amber warning transition, control actions, and extension lifecycle hygiene.

### Modified Capabilities

None. `session-timer`, `session-persistence`, `daemon-control`, and `daemon-configuration` are
unchanged.

## Approach

The amber state is derived client-side, and this is forced rather than chosen.
`specs/daemon-control` requires `PropertiesChanged` on transitions only and forbids per-second
emission; the daemon honours it by publishing only on an `EffectNotify`
(`daemon/internal/dbusapi/service.go:169-205`). So the daemon publishes `PhaseEndsAt` as an
absolute unix epoch second and the extension owns the countdown — already settled as M1 decision D1.

### Invariant I1 — The daemon decides, the tick only renders

The extension holds two sources of truth and MUST keep them separate:

- **D-Bus properties are authoritative for state** — which phase we are in, whether we are paused,
  whether a session exists at all. State changes only on `g-properties-changed`.
- **The local tick is authoritative for presentation only** — how the remaining time is drawn. It
  recomputes a display from an absolute deadline and does nothing else.

The tick therefore MUST NOT decide or infer a transition. It MUST NOT flip `focus` to `break` when
its own countdown reaches zero, MUST NOT clear `SessionActive`, and MUST NOT write any state the
daemon owns. When its computed remainder hits zero it renders `00:00` and waits for the daemon to
say what happens next.

This is not stylistic. A tick that is allowed to conclude anything becomes a second timer, and two
timers drift: the daemon's elapsed-in-phase survives suspend by design
(`daemon/internal/session/state.go:46-50`), a client-side countdown does not. Deriving the display
from the absolute `PhaseEndsAt` instead is self-correcting across missed signals and clock jumps,
because every tick recomputes from the deadline rather than decrementing its own counter.

**Self-correction has a precondition the daemon does not currently meet.** `PhaseEndsAt` is only
authoritative while it is republished every time the effective deadline moves. `applySuspend`
(`daemon/internal/session/machine.go:123-139`) declines to charge a short suspend to the phase —
correct per M1 decision P1 — but returns `EffectPersist` alone, with no `EffectNotify`. The deadline
therefore moves without a `PropertiesChanged`, and every client keeps counting to a stale
`PhaseEndsAt` until some later transition happens to republish it. Measured on 2026-09-18: a 9-second
suspend left the indicator a constant 3 seconds ahead of the daemon across three samples 20s apart.
The error is bounded by the idle-credit threshold, so a 9-minute suspend can leave the panel about
nine minutes wrong. This is a daemon defect surfaced by M2, tracked as its own change; M2 stays
consumer-only per Out of Scope above.

I1 binds the downstream phases: `specs` MUST encode it as a requirement with scenarios, `design`
MUST NOT introduce a code path where tick output feeds state, and `verify` MUST check it.

Paused is the one case where `PhaseEndsAt` is not the input: the daemon publishes `0` and moves the
live value to `RemainingSeconds` (`service.go:191-197`), so the display freezes on that instead.
This is consistent with I1 rather than an exception to it — the daemon still decides that we are
paused; only the field the renderer reads changes.

Presentation states:

| Condition | Indicator |
|-----------|-----------|
| Daemon absent | dimmed icon, no label |
| `SessionActive` false | dimmed icon, no label |
| `focus` / `break`, running | icon + `MM:SS` |
| `Paused` true | icon + frozen `MM:SS` from `RemainingSeconds` |
| `focus` and remaining ≤ 120s | amber class on icon and label |

## Affected Areas

- `extension/` — new, the whole GJS half.
- `packaging/Makefile` — new. `packaging/cadenced.service` unchanged but finally installed by it.
- `openspec/specs/panel-indicator/` — new capability spec, merged at archive.
- `daemon/` — untouched.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Leak in `disable()` (timeout source, proxy, handlers) | High | Explicit teardown task and a spec requirement; it is the defect this milestone is most likely to ship |
| Tick drifts into a second timer, violating I1 | Med | I1 stated as a binding invariant; spec scenarios and success criteria 7-8 test it under a stopped daemon and across suspend |
| Naive countdown from `PhaseEndsAt` breaks on pause | High | Spec scenario pinning the paused display to `RemainingSeconds` |
| Sync D-Bus construction janks the compositor | Med | Async proxy construction only; no sync call anywhere in the Shell process |
| Daemon not installed, so nothing verifiable | Certain today | `packaging/Makefile` is a task, not a follow-up |
| Wayland cannot reload the Shell in place | Certain | Nested session for the dev loop; logout/login for final verification |
| No automated test harness for the extension half | Certain | Honest split: Go tests unchanged, extension verified manually in a nested session |
| Amber styling token unconfirmed | Low | D4 settled at design after a direct doc check; local class is the fallback |

## Rollback Plan

`gnome-extensions disable cadence@ian.dev` and `rm -rf
~/.local/share/gnome-shell/extensions/cadence@ian.dev`. The daemon is unaffected — it ran without
any client for all of M1 and continues to. M1's own rollback (`systemctl --user disable --now
cadenced.service`) is unchanged. Code revert is `git reset`.

## Dependencies

- GNOME Shell 48 (verified: 48.8) on Wayland.
- A running `cadenced` on the session bus.
- No new Go modules. No npm, no build step for the extension — plain ESM loaded by the Shell.

## Success Criteria

1. The indicator appears in the top bar on `gnome-extensions enable cadence@ian.dev`.
2. With `cadenced` stopped, the indicator is dimmed; starting the daemon makes it live without
   re-enabling the extension.
3. `cadence start` from the CLI updates the panel countdown; clicking Pause in the panel freezes it
   and `busctl --user` confirms the daemon is the one that paused.
4. The label and icon go amber at 120s remaining in `focus`, and return to normal on the break
   transition.
5. Disabling the extension leaves no timeout source, no proxy, and no handler behind — verified by
   an enable/disable cycle with no errors in the Shell log.
6. `go test ./...` in `daemon/` still passes unchanged.
7. **I1 holds under a stopped daemon.** With a session active, stop `cadenced`. The countdown stops
   claiming progress and the indicator goes dimmed — it does not keep counting down on its own, and
   it never announces a phase change the daemon never made.
8. **I1 holds across suspend.** Suspend mid-`focus`, resume, then sample the rendered countdown
   against the daemon's true remaining at least three times, roughly 20s apart. True remaining is
   `focusSeconds - (elapsed_in_phase_ns/1e9 + now - last_observed)`, read from
   `~/.local/state/cadence/session.json`. The offset must be zero, and must *stay* zero across the
   samples — a constant non-zero offset is the failure this criterion exists to catch, because it
   means the client is counting to a deadline the daemon no longer agrees with.

   **Do not compare against the `RemainingSeconds` property.** It is republished only on transitions
   (`specs/daemon-control`, "Change Notification"), so between transitions it is a stale snapshot and
   a *correct* implementation will appear to diverge from it by exactly the time elapsed since the
   last transition. An earlier revision of this criterion said to compare against it; that was wrong
   and would have failed a working build.
