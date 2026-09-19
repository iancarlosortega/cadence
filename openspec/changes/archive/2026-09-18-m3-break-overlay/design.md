# Design: M3 — Break overlay with hold-to-skip

## Technical Approach

One new module, `extension/overlay.js`, owning a single actor and its lifecycle. One new pure
predicate in `render.js`. `extension.js` gains the wiring and one extra teardown step. No modal grab,
no daemon change, no new D-Bus surface.

The overlay reuses the machinery M2 already built: `CadenceClient` supplies state, the existing 1s
tick supplies time, and the same `_render()` that updates the panel decides whether the overlay
should be up.

## Architecture Decisions

### Decision 1 — Visibility is a pure predicate, the actor is dumb

`render.js` gains:

```
shouldShowOverlay(state, nowSeconds, suppressed) -> bool
```

true only when the daemon is available, a session is active, the phase is `break`, the derived
remaining time is above zero, and the break is not suppressed. `overlay.js` never inspects
`state` — it is told `show()` or `hide()`.

This is the same split that made M2's countdown provable, and it is what makes three of the
Self-Owned Exit paths testable without a Shell: the remainder reaching zero, the phase leaving
`break`, and the daemon going away all reduce to this function returning false.

The fourth path, `disable()`, is structural and cannot be unit-tested here.

### Decision 2 — Suppression is decided at the edge, not per tick

Suppression must be evaluated **once**, when the break begins, per product decision P2. The predicate
therefore takes `suppressed` as an argument rather than reading the Shell itself — it stays pure, and
the edge detection lives in `extension.js`:

```
on state change:
    if phase became 'break':
        this._suppressed = Main.layoutManager.primaryMonitor.inFullscreen
    if phase is not 'break':
        this._suppressed = false
```

Reading `inFullscreen` inside the predicate would make it impure and would re-evaluate every second,
which is exactly the flicker P2 rejected.

### Decision 3 — `addTopChrome`, not `addChrome`

Verified in the Shell 48.8 source on this machine: `addChrome()` places the actor **below**
`global.top_window_group`, so it would sit under normal windows and cover nothing. `addTopChrome()`
places it at the top of `uiGroup`.

Chrome defaults are `trackFullscreen: false, affectsStruts: false, affectsInputRegion: true`. All
three are what this needs unchanged:

- `trackFullscreen: false` — the Shell must not auto-hide the overlay; suppression is our decision,
  taken once at break start.
- `affectsStruts: false` — an overlay is not a dock and must not resize anyone's workspace.
- `affectsInputRegion: true` — this is what absorbs mouse clicks without a grab, and is the entire
  mechanism behind product decision P1.

### Decision 4 — The actor is destroyed, not hidden

`hide()` destroys the actor rather than setting `visible = false`. `_trackActor` connects the actor's
`destroy` signal to `_untrackActor`, so destroying it removes the chrome tracking and the input
region in one step, verified in `layout.js`.

Keeping a hidden actor around would leave chrome tracked and an input region registered for something
invisible — a plausible way to end up with a screen region that silently eats clicks.

Consequence: `show()` constructs fresh each time. An overlay is created a handful of times an hour;
construction cost is irrelevant and the simpler lifecycle is worth more.

### Decision 5 — Hold-to-skip is a timeout plus a transition, cancelled everywhere

`button-press-event` starts a `GLib.timeout_add` for the hold duration and eases a progress bar's
width to full over the same period. `button-release-event` and `leave-event` both cancel: remove the
source, `remove_all_transitions()`, reset the bar.

On completion the handler calls `SkipBreak` through the existing `CadenceClient.call()` and **does
not dismiss the overlay itself**. Dismissal happens when the daemon's resulting `PropertiesChanged`
moves the phase out of `break` and the predicate goes false — the spec requires this, and it keeps
invariant I1 intact at the one control where shortcutting would be most tempting.

If the daemon is gone, `call()` already returns early and the overlay is dismissed by the
name-vanished path instead. The user is never stuck because a skip went unheard.

### Decision 6 — Teardown is idempotent and ordered

`disable()` gains overlay teardown **before** the existing client and indicator teardown, so the
overlay cannot be left behind if a later step throws. Teardown cancels any hold, removes the timeout
source, removes transitions, destroys the actor, and nulls the reference — each guarded so running it
twice, or with no overlay up, is a no-op.

The `monitors-changed` handler is connected in `enable()` and disconnected in `disable()` alongside
the existing `notify::color-scheme` handler.

## Data Flow

```
daemon publishes Phase = break
  → CadenceClient._refresh() rebuilds the cache
  → _onStateChanged()
        phase became break → _suppressed = primaryMonitor.inFullscreen   (once)
  → _render()
        shouldShowOverlay(state, now, suppressed)
              true  → overlay.show(primaryMonitor)   → addTopChrome
              false → overlay.hide()                 → actor.destroy()

1s tick → _render() → same predicate
        remainder hits zero → false → hide, with no daemon involvement

daemon name vanishes → client state becomes DISCONNECTED
        → available false → predicate false → hide

hold completes → client.call('SkipBreak')
        → daemon publishes Phase = focus
        → predicate false → hide
```

Every dismissal path converges on the same predicate returning false. There is one way the overlay
comes down, reached four ways.

## File Changes

| File | Change |
|------|--------|
| `extension/overlay.js` | New. `CadenceOverlay` with `show(monitor)`, `hide()`, `destroy()` |
| `extension/render.js` | `shouldShowOverlay`, `HOLD_TO_SKIP_SECONDS` |
| `extension/extension.js` | Wire the overlay, suppression edge, `monitors-changed`, teardown |
| `extension/stylesheet.css` | Overlay, message and progress styling for both themes |
| `extension/test-render.js` | Cases for the predicate |
| `packaging/Makefile` | Install `overlay.js` |
| `daemon/` | Untouched |

## Interfaces / Contracts

Unchanged. No new property, method or signal; `SkipBreak` is called through the existing proxy. The
daemon cannot tell M3 from M2.

## Testing Strategy

- **Pure, deterministic**: `shouldShowOverlay` is a function of `(state, now, suppressed)` and is
  tested in `test-render.js` beside `computeDisplay`, including the three no-Shell dismissal paths.
- **Not unit-testable**: that `addTopChrome` actually covers windows, that clicks are absorbed, that
  Alt+Tab still works, and that hold-to-skip feels right. All need a real Shell and a real break, so
  the change carries a runbook with a 3-minute test config.
- **The F3 check is manual and deliberate**: kill `cadenced` while the overlay is up and confirm the
  screen frees itself. That is the scenario this design exists to guarantee.
- **Mutation check**: make `shouldShowOverlay` ignore `suppressed` and confirm the suppression test
  goes red.

## Migration / Rollout

`make -C packaging install-extension` then a logout, since the Shell cannot reload an extension in
place on Wayland. No unit change, no daemon change, so nothing to re-enable. Rollback is a revert and
another logout; a stale `overlay.js` on disk is inert once nothing imports it.

## Open Questions

None blocking. Two deliberately deferred: a keyboard-held equivalent for skip, unnecessary while the
keyboard is already an escape hatch, and a configurable hold duration, which belongs with M6.
