# Exploration: M3 — Break overlay with hold-to-skip

## Current State

M1 and M2 are archived and both daemon fixes have shipped. The panel indicator works on real
hardware: it tracks the daemon, warns amber at T-2min, round-trips Pause, and survives logout.

M3 is where cadence stops informing and starts **interrupting**. That changes the stakes: a bug in
M2 meant a wrong number in the panel. A bug in M3 can cover your screen at the wrong moment, or
refuse to go away.

### The daemon needs no changes

`SkipBreak` already exists and is exactly what hold-to-skip needs
(`daemon/internal/session/machine.go:62-71`): it is a no-op unless a session is active and the phase
is `break`, and it resets to a fresh `focus` with elapsed 0. Phase transitions already publish
`EffectNotify`. As with M2, this should be consumer-only.

### GNOME facts, verified against the installed Shell 48.8

Everything below was read out of `libshell-16.so` on this machine rather than taken from
documentation, because three claims from the initial research turned out to be wrong.

| Fact | Evidence |
|---|---|
| `addChrome()` places an actor **below** `global.top_window_group` — i.e. under normal windows | `layout.js` `addChrome` body |
| `addTopChrome()` places it at the top of `uiGroup` — **above** windows | `layout.js:250` comment and `addTopChrome` body |
| Chrome defaults are `trackFullscreen: false, affectsStruts: false, affectsInputRegion: true` | `layout.js:184-188` |
| `Monitor.inFullscreen` is real: `global.display.get_monitor_in_fullscreen(this.index)` | `layout.js:166-168` |
| `pushModal(actor, {actionMode})` returns a `Clutter.Grab` from `global.stage.grab(actor)` | `main.js` `pushModal` |
| **`pushModal` auto-releases the grab when the actor is destroyed** | `main.js`: it connects `actor.connect('destroy', ...)` and calls `popModal(grab)` from it |
| `popModal` **throws** `Error('incorrect pop')` if the grab is not on the stack | `main.js` `popModal` |
| `_trackActor` also untracks chrome on the actor's `destroy` | `layout.js` `_trackActor` |
| `St.Settings` exposes `enable-animations` and `slow-down-factor` | `St-16.typelib` |

**Three research claims were wrong and are corrected here**: `addTopChrome()` does exist; the Shell
*does* auto-release a modal grab on actor destroy; and `inFullscreen` is confirmed rather than
"reasonably but not fully confirmed".

That middle one materially changes the risk profile. The research concluded "nothing auto-releases
the grab, you are relying on your own defensive code". In fact **destroying the overlay actor
releases the grab**, so a `disable()` that destroys the actor is already safe, and a thrown exception
that reaches actor destruction is too. Defensive teardown is still correct, but it is a second line
rather than the only one.

## Affected Areas

- `extension/` — a new overlay module, plus wiring in `extension.js`. The likely shape is
  `overlay.js` (actor construction and lifecycle) and pure helpers in `render.js`.
- `extension/stylesheet.css` — overlay styling.
- `openspec/specs/panel-indicator/` — or a **new capability**; see Decision 1.
- `daemon/` — expected untouched.

## Approaches

### Decision 1 — New capability, or extend `panel-indicator`?

`panel-indicator` is named for a panel indicator. A fullscreen overlay is a different surface with
different requirements: presence, input, escape, monitor targeting. Folding it in would make the
capability's name a lie.

**Recommended:** a new `break-overlay` capability. `panel-indicator` stays as-is.

### Decision 2 — How much should the overlay block? *(product decision, see below)*

This is the real question of M3, and it is a product call rather than a technical one.

- **Visual only** — `addTopChrome`, no `pushModal`. Covers the primary monitor, and because
  `affectsInputRegion` defaults true it also **absorbs mouse clicks** in its region. But keyboard
  shortcuts keep working: Alt+Tab, Super, workspace switching all still function, so the overlay can
  be worked around without skipping.
- **Modal** — additionally `pushModal` with a `Shell.ActionMode`. Genuinely holds the session:
  keybindings are gated by the action mode, so the overlay is the only thing you can interact with.
  Strongest nudge, and the one that can actually annoy.

The safety gap between these is smaller than it first appears, given the verified auto-release on
destroy. The real difference is behavioural: whether cadence *asks* you to stand up or *makes* you.

### Decision 3 — Who ends the overlay

Invariant I1 from M2 applies with teeth here. The overlay must not depend on the daemon to tell it
the break is over, because **F3 proved the daemon can vanish** — and an overlay waiting forever on a
dead daemon is a covered screen with no exit.

**Recommended:** the overlay computes its own end from `PhaseEndsAt` on the same 1s tick that already
drives the panel, and dismisses itself when the remainder reaches zero **regardless of daemon
state**. A `PropertiesChanged` leaving `break` also dismisses it. The daemon remains authoritative for
*state*; the overlay owns its own *exit*. Additionally the client should dismiss if the daemon's name
vanishes from the bus, since a break it cannot verify is one it should not enforce.

### Decision 4 — Fullscreen suppression *(product decision, see below)*

`Main.layoutManager.primaryMonitor.inFullscreen` is available and is the conventional guard. Options:
suppress the overlay entirely during fullscreen, show it anyway, or show a reduced non-blocking form.
A tool that covers your screen mid video call is a tool that gets uninstalled.

### Decision 5 — Hold-to-skip mechanics

Standard Clutter: `button-press-event` starts a timer and a progress animation,
`button-release-event` and `leave-event` cancel it, completion calls `SkipBreak` and dismisses. A
keyboard equivalent is needed because a modal grab takes key focus and a pointer-only exit is a poor
escape hatch. `remove_all_transitions()` and `GLib.Source.remove()` on teardown.

Hold duration is a product choice — long enough to be deliberate, short enough not to be a punishment.

## Recommendation

Build it as a new `break-overlay` capability, consumer-only, with the overlay owning its own exit via
the local tick. Settle Decisions 2 and 4 with the user before proposing, since both are about how
much the tool is allowed to impose on its user.

## Risks

- **Trapping the user** is the headline risk. Mitigated by verified auto-release on actor destroy, a
  self-owned exit, a hard maximum lifetime, dismissal when the daemon disappears, and teardown in
  `disable()`.
- **Interrupting something that matters** — mitigated by Decision 4.
- **Not testable without a session cycle.** As with M2, overlay behaviour needs a real Shell. A
  3-minute test config makes breaks arrive quickly.
- `popModal` throwing on a double release means teardown must be idempotent, not merely present.

## Ready for Proposal

Yes, once Decisions 2 and 4 are settled.
