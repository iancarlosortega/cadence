# Archive Report: m3-break-overlay

**Change**: m3-break-overlay (Milestone 3)
**Project**: cadence
**Archived**: 2026-09-18
**Verdict at verify**: pass — 6/6 requirements, 15/15 scenarios, 0 blockers, 3 warnings

## What shipped

A full-screen break overlay on the primary monitor with hold-to-skip. Cadence stops informing and
starts interrupting.

| File | Lines |
|------|-------|
| `extension/overlay.js` | new, ~175 |
| `extension/test-render.js` | +59 |
| `extension/extension.js` | +43 / −2 |
| `extension/stylesheet.css` | +41 |
| `extension/render.js` | +22 |
| `packaging/Makefile` | +1 |

**329 changed lines against a 400 budget**, against a ~330 forecast — within a line, after M2's
forecast missed by 35%. No `size:exception` needed. `daemon/` untouched: `SkipBreak` already existed
and the D-Bus contract is unchanged, so the daemon cannot tell M3 from M2.

## Spec changes merged

`openspec/specs/break-overlay/spec.md` — **new capability**, 6 requirements, 15 scenarios, copied
whole. Nothing modified or removed. `panel-indicator` untouched: an overlay is a different surface,
and folding it into a capability named for a panel indicator would have made that name a lie.

## Product decisions that shaped it

- **No modal grab.** `addTopChrome` plus the default `affectsInputRegion` covers the screen and
  absorbs mouse clicks while Alt+Tab keeps working. A nudge, not a cage — and it deleted the entire
  trapping risk class: no grab to leak, no `ActionMode` gating rescue shortcuts, no `popModal`
  throwing on a double teardown.
- **Fullscreen suppression decided once, at the break edge.** No overlay over a video call, and no
  flicker as fullscreen toggles.
- **Hold stays at 3 seconds** after the user reported it feeling long and investigation found no
  defect. The friction is the point for a tool whose job is making you stand up.

## The requirement that earned its place

**Self-Owned Exit.** Four dismissal paths — the locally derived remainder reaching zero, the phase
leaving `break`, the daemon's bus name vanishing, and `disable()` — all converging on one pure
predicate returning false. One way down, reached four ways.

Verified live: `systemctl --user stop cadenced` with the overlay on screen removed it instantly and
left the screen usable with no user action.

That requirement exists only because **F3 was discovered the same day** and proved the daemon can
vanish. Written a day earlier it would have read as paranoia.

## Research corrected before it could mislead

A documentation pass was run and then checked against Shell 48.8 JS extracted from
`libshell-16.so`. **Three of its claims were wrong**:

- `addTopChrome()` was said not to exist. It does, and it is what puts an actor above windows —
  `addChrome()` places it *below* `top_window_group`, where it would have covered nothing.
- "Nothing auto-releases a modal grab on disable" — false. `pushModal` connects the actor's
  `destroy` and calls `popModal` from it.
- `Monitor.inFullscreen` was "not fully confirmed". It is real.

The middle one materially changed this milestone's risk profile before a line was written.

## Defects found during verification, all fixed

| Defect | Cause |
|---|---|
| Content rendered top-left | A plain `St.Widget` has no layout manager |
| Progress bar full at rest | `scale_x` defaults to 1, never zeroed at construction |
| Countdown not centred | `St.Label` defaults to `ActorAlign.FILL` |
| Hold finished before the bar | `timeout_add_seconds` coalesces to second boundaries |

**All four were found by the user looking at the screen. None was reachable by the test suite** —
they live in Clutter and St actor properties that cannot exist outside a running Shell. The
pure-predicate strategy that makes the safety logic provable offers nothing for appearance. That is
a limit of the approach, recorded rather than hidden.

## Investigated and found not to be defects

- **Hold feels longer than 3s** — one constant drives both timer and animation, no slow-down factor.
  Kept at 3s by product decision.
- **Panel amber during break** — investigated twice; resolved by correlated observation. While the
  overlay was on screen, which only happens when the phase is `break`, the panel read white.

Recording non-defects matters as much as recording fixes; otherwise they are re-investigated later.

## Carried forward

- **Monitor Changes is PARTIAL.** Single-monitor session, so the `monitors-changed` handler has
  never run against a real configuration change. The one requirement resting on inspection rather
  than observation.
- **F2** — the idle-pause republish path — remains open and owned by M4.

## Process notes

- A logout was wasted loading a stale build, because a fix was written and not installed. Editing
  `extension/` changes nothing a running Shell can see. The runbook now opens with
  `make install-extension` and a `diff` that must print nothing.
- Three mutation checks were run and each was observed failing. One of them was invalid twice
  before it worked: first it mutated the wrong function, because the same condition appears in two
  places and a first-occurrence replace hit the other one; then, aimed correctly, it still passed
  because the fixture cleared two fields at once. A safety test whose fixture sets several fields
  cannot prove any one of them is load-bearing.

## Delivery

Not committed at time of writing. Commit and push remain separate human decisions.
