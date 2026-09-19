# Proposal: M3 — Break overlay with hold-to-skip

## Intent

Make the break visible enough to obey. M2 put the timer in the panel, where it is easy to ignore.
M3 covers the primary monitor when a break begins, and offers one deliberate way out: hold to skip.

## Scope

### In Scope

- A new `extension/overlay.js` building a full-screen actor on the primary monitor via
  `Main.layoutManager.addTopChrome()`.
- Shown while the daemon reports `break`, hidden otherwise.
- **No modal grab.** The overlay absorbs mouse input because `affectsInputRegion` defaults true, but
  keyboard navigation keeps working.
- Hold-to-skip: press and hold, with visible progress, then `SkipBreak` and dismiss.
- Suppression when the primary monitor is in fullscreen at the moment the break begins.
- Self-owned exit: the overlay dismisses itself when its locally computed remainder reaches zero, when
  the phase leaves `break`, or when the daemon leaves the bus.
- `monitors-changed` handling so the overlay follows the primary monitor.
- A new `break-overlay` capability spec.

### Out of Scope

- **Any modal grab or input capture.** Chosen deliberately: see Decision 1.
- The daemon. `SkipBreak` already exists and does exactly what is needed
  (`machine.go:62-71`); `daemon/` is untouched.
- A global keyboard shortcut for skip. Without a grab the keyboard is already an escape hatch, so a
  pointer-held skip is sufficient for this milestone.
- Re-showing a suppressed overlay after fullscreen ends. See Decision 2.
- Idle gating (M4), tier detection (M5), config hot reload (M6). F2 remains M4's.

## Capabilities

### New Capabilities

- `break-overlay` — presence, suppression, the self-owned exit, hold-to-skip, and teardown.

### Modified Capabilities

None. `panel-indicator` is untouched: an overlay is a different surface with different requirements,
and folding it into a capability named for a panel indicator would make that name a lie.

## Approach

### Decision 1 — Visual and mouse blocking, no modal grab

`addTopChrome()` places the actor above `global.top_window_group`, so it covers windows, and the
default `affectsInputRegion: true` means it swallows clicks in its region. Keyboard navigation —
Alt+Tab, Super, workspace switching — keeps working.

This is a nudge, not a cage. For a personal stand-up reminder that is the honest level: cadence asks
you to stand up and you retain the ability to overrule it without ceremony.

It also deletes the entire risk class that made this milestone worth worrying about. With no
`pushModal` there is no grab to leak, no `ActionMode` gating rescue shortcuts, and no `popModal`
throwing `'incorrect pop'` on a double teardown.

### Decision 2 — Suppress for the whole break if fullscreen when it starts

`Main.layoutManager.primaryMonitor.inFullscreen` is evaluated **once, when the break begins**. If the
primary monitor is fullscreen at that moment, no overlay appears for that break. The daemon still
runs the break and the panel still shows it, so the cycle stays honest — you simply are not
interrupted during a call, a film or a presentation.

Evaluated once rather than continuously, so the overlay cannot appear partway through a call that
started before the break, and cannot flicker as fullscreen toggles. The rejected alternative —
re-showing for the remaining time once fullscreen ends — surfaces a nearly expired break at an
arbitrary moment and needs more state to get right.

### Decision 3 — The overlay owns its own exit

Invariant I1 from M2, with consequences. The overlay must never depend on the daemon to tell it the
break is over, because F3 proved the daemon can vanish, and an overlay waiting on a dead daemon is a
covered screen with no exit.

It dismisses on whichever comes first:

1. Its locally computed remainder from `PhaseEndsAt` reaches zero.
2. A `PropertiesChanged` reports a phase other than `break`.
3. The daemon's name leaves the bus — a break it cannot verify is one it should not enforce.
4. `disable()`.

The daemon stays authoritative for *state*; the overlay owns its *exit*.

### Decision 4 — Visibility is a pure predicate

Whether the overlay should be up is a function of state, time and one boolean:

```
shouldShowOverlay(state, nowSeconds, suppressed) -> bool
```

Living in `render.js` alongside `computeDisplay`, it is unit-testable outside the Shell — the same
property that made M2's countdown provable. `overlay.js` holds only actor construction and lifecycle.

## Affected Areas

- `extension/overlay.js` — new.
- `extension/render.js` — `shouldShowOverlay` and hold-progress helpers.
- `extension/extension.js` — wiring, `monitors-changed`, teardown.
- `extension/stylesheet.css` — overlay styling, both themes.
- `extension/test-render.js` — new cases.
- `packaging/Makefile` — install `overlay.js`.
- `daemon/` — untouched.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Overlay outlives the break and covers the screen | Low | Four independent dismissal paths, one of which needs no daemon at all |
| Overlay leaks on `disable()` | Med | Destroy the actor in `disable()`; chrome untracks on destroy, verified in `layout.js` `_trackActor` |
| Hold-to-skip leaves a dangling timer or transition | Med | `GLib.Source.remove` and `remove_all_transitions()` on release, leave, and teardown |
| Appears over a call | Low | Decision 2, with the check at break start |
| Not verifiable without a session cycle | Certain | Runbook, 3-minute test config, same as M2 |
| Multi-monitor and hotplug | Med | `monitors-changed` reconnects to the current primary |

## Rollback Plan

`git revert`, `make -C packaging install-extension`, then log out and back in. The daemon is
untouched, so nothing to roll back there, and a stale `overlay.js` left on disk is inert once
`extension.js` no longer imports it.

## Dependencies

GNOME Shell 48 (verified 48.8), a running `cadenced`. No new Go modules, no npm, no build step.

## Success Criteria

1. When a break begins, the primary monitor is covered within one tick, and mouse clicks do not reach
   windows underneath.
2. Alt+Tab still works while the overlay is up — this is deliberate, not a defect.
3. Holding the skip control for its full duration calls `SkipBreak`, the overlay dismisses, and
   `busctl` confirms the daemon moved to `focus`.
4. Releasing early cancels: no skip, progress resets, the overlay stays.
5. When the break ends naturally, the overlay dismisses on its own.
6. **Killing `cadenced` while the overlay is up dismisses it** rather than leaving the screen covered.
7. Starting a break while the primary monitor is fullscreen shows no overlay, and the break still
   runs and completes.
8. Disabling the extension while the overlay is up removes it and leaves no errors in the Shell log.
9. `go test ./...` in `daemon/` still passes and `git diff --stat daemon/` is empty.
