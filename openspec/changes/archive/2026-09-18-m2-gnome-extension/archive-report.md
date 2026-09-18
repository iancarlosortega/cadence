# Archive Report: m2-gnome-extension

**Change**: m2-gnome-extension (Milestone 2)
**Project**: cadence
**Archived**: 2026-09-18
**Verdict at verify**: pass — 8/8 requirements, 16/16 scenarios, 0 blockers, 3 warnings

## What shipped

A GNOME Shell 48 extension that puts the daemon in the top bar. It reflects session state over
D-Bus, derives its countdown locally from the absolute `PhaseEndsAt`, warns amber two minutes before
a break, and drives the daemon through its five existing methods.

| File | Lines |
|------|-------|
| `extension/extension.js` | 272 |
| `extension/test-render.js` | 112 |
| `extension/render.js` | 58 |
| `packaging/Makefile` | 49 |
| `extension/stylesheet.css` | 24 |
| `extension/metadata.json` | 7 |

522 code lines against a 400 budget; `size:exception` accepted by the maintainer on 2026-09-18 and
recorded in the runtime ledger with attempt 1 preserved.

## Spec changes merged

`openspec/specs/panel-indicator/spec.md` — **new capability**, 8 requirements, 16 scenarios. The
four daemon capabilities are untouched: M2 shipped without a single change under `daemon/`, verified
by `git show --stat cf57273 -- daemon/` returning empty.

## Milestone thesis: proven

M2 existed to prove the two halves talk. Two observations establish it, and neither is inferable
from code:

- Clicking Pause in the panel froze the label **and** `busctl` reported `Paused b true`. The freeze
  came from the daemon, not a local decision — the extension applies no optimistic update.
- Stopping `cadenced` mid-session dimmed the indicator and the countdown stopped advancing, rather
  than continuing on its own authority.

## Invariant I1 — held

*The daemon decides, the tick only renders.* Enforced structurally by design Decision 1:
`computeDisplay(state, now)` is pure and holds no countdown between calls, so a drifting client-side
timer is unrepresentable rather than merely discouraged. Verified under a stopped daemon and across
a real suspend.

## Evidence at close

- 17/17 pure-logic checks under `gjs`; `go test ./...` in `daemon/` green; both JS files parse as ES
  modules under `node --check`.
- Verified on real hardware, GNOME Shell 48.8 on Wayland, on the extension's first ever load.
- Amber on at exactly `2:00`, off at the break transition.
- Post-suspend offset `+0.171s / +0.174s / +0.174s`, sub-second and stable across a phase change.
- Three disable/enable cycles including one with the daemon absent: no errors, no disposed-object
  warnings, no leaked sources.

## Deviations from design

1. **Pure logic split into `extension/render.js`.** The design placed it in `extension.js`; running
   the test proved that unimportable outside the Shell (`resource:///org/gnome/shell/...`). The split
   made design Decision 1's testability claim true rather than aspirational.
2. **One `stylesheet.css` with two classes** instead of `stylesheet-dark.css` / `stylesheet-light.css`.
   The `window-list` precedent is real but its selection mechanism was not confirmable locally;
   `St.Settings.color-scheme` was verified in `St-16.typelib`, so the class is chosen in JS.

## Warnings carried into the merged spec

1. Success criterion 8's literal "offset zero" is unachievable — `PhaseEndsAt` is a whole-second
   value, so ~0.17s is resolution, not drift.
2. **Light-theme amber was never observed.** `.cadence-warning-light` is implemented and installed
   but only dark mode was exercised. Untested colour, not verified colour.
3. Suspend and leak checks ran on a 3-minute test config, not the 50-minute default.

## Findings raised, owned elsewhere

- **F1 — daemon did not republish `PhaseEndsAt` after a short suspend.** RESOLVED by change
  `fix-suspend-deadline-republish`, archived the same day. Found by measuring M2's task 6.3 against a
  live daemon.
- **F3 — `cadenced` panics on logout.** `publish()` calls `prop.SetMust`, which panics on error;
  logout closes the session bus under the next `Tick`. Pre-existing from M1, self-healing via
  `Restart=on-failure`, which is why it stayed invisible. Open, daemon-side.
- **F4 — daemon downtime is charged as focus work.** Measured on the pure reducer: 8h as downtime
  yields `break`, the same 8h as suspend yields `focus`. Booting after an overnight shutdown starts
  the user on a break, contrary to M1 decision P1. Open, daemon-side.
- **F2 — idle-pause path shares the republish defect**, deferred to M4 by
  `fix-suspend-deadline-republish`.

Every one of F1, F3 and F4 was found by running the system, not by reading it.

## Process notes worth keeping

- Verification was gated on a Wayland logout for the whole milestone, and the gate was honoured
  rather than routed around. A verdict claiming the indicator renders would have been unfounded
  until somebody had seen it render.
- Two instrumentation bugs failed correct code during this milestone: integer truncation reported a
  0.319s offset as a constant 1s, and `check-offset.py` compared a break countdown against the focus
  length and reported the 120s difference as drift. In both cases the failure magnitude was
  suspiciously round. Suspect the ruler before the thing measured.
- An amber "bug" was chased and found not to exist. Two hypotheses were disproved before any code
  changed — the daemon does emit `Phase → "break"` (captured off the bus), and
  `st_widget_add_style_class_name` guards against duplicates (GNOME 48 source).

## Delivery

Committed as `cf57273` (feature) plus follow-ups. Push remains a separate human decision.
