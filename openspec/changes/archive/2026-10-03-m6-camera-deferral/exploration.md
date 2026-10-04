# Exploration: m6-camera-deferral

**Date**: 2026-10-03
**Milestone**: M6, the T2 half of "meetings don't break it"

## Intent

Stop the full-screen overlay from taking over the screen mid-call when the user is on camera. The
rule fixed at sdd-init (2026-09-14) is:

> T2 On camera | Camera open | Corner panel only, defer and retry every 5 min, cap ~3, then a
> persistent pill

M5 shipped detection and explicitly left T2 behaving as T0 (P4): today the overlay shows on camera.

## Current State (verified)

### Domain

- The break decision is one branch: `transitionPhase`, `if s.Tier != TierT3`
  (`daemon/internal/session/machine.go:389`).
- `State` has no defer counter, retry timer or pill flag. Time is modelled as `ElapsedInPhase`
  advanced by `now.Sub(LastObserved)`, never as a stored deadline (`state.go:46-50`,
  `session-persistence` "Durable State"). Any retry timer must follow the same rule: an elapsed
  quantity, converted to an instant only when publishing.
- Things that touch a pending break:
  - `creditBreak` (idle ≥10 min, long suspend, long downtime) always resets to a fresh focus. It
    would naturally clear any owed break, which is correct: the user was away.
  - Pause stops `applyTick` entirely, so a retry timer freezes with it.
  - T3 starting ends a break (M5), and must also clear an owed one.
  - `EventSkipBreak` only acts in `PhaseBreak`.
- `Durations` carries every configured threshold. Config hot reload is not built, so the retry
  interval and cap would be constants or new config keys read at startup.

### D-Bus

Properties: `SessionActive`, `Phase` (`none`/`focus`/`break`), `PhaseEndsAt`, `RemainingSeconds`,
`Paused`, `Tier`, `Idle`. Nothing can express "a break is owed" or "the next retry is at …".

A new property needs four touch points: `service.go` (`propsMap` seed, `desired` map), the
extension's `IFACE_XML`, and `CadenceClient._refresh`.

### Extension

- `shouldShowOverlay` (`render.js`) is true for any `Phase == break` with time left, unless the
  fullscreen flag captured at the break edge suppresses it. It knows nothing about tier.
- `OverlayController` (`overlay.js`) is the pattern for a second surface:
  - `addTopChrome`, so it sits above windows;
  - destroyed rather than hidden, so no stale input region is left;
  - a `BinLayout` with explicit `x_align`, per M3's St pitfalls;
  - "request only": actions call the daemon, and property changes dismiss.
- No corner panel or pill exists, and `stylesheet.css` has no styles for one.
- `addTopChrome` defaults to `affectsInputRegion: true`. For a small actor that absorbs clicks only
  over its own rectangle, so a corner panel doesn't block the call window behind it.

### Persistence

The store record holds `active`, `phase`, `elapsed_in_phase_ns`, `paused`, `paused_remaining_ns`,
`last_observed` and the durations. `Idle`, `IdleCredited` and `Tier` are deliberately not persisted.
Downtime on restart already runs through `creditBreak`.

## Modelling the owed break

| Option | Shape | Pro | Con |
|---|---|---|---|
| **A. Break phase, held** | `Phase = break` with a new `Deferred` flag. The break's elapsed time does not advance while it is held. | `SkipBreak` and T3-ends-break already work, since it is a break. The overlay's "phase left break" exit is unchanged. When the camera turns off, the hold lifts and the break simply runs. | Every client branching on `phase === 'break'` must learn `Deferred` (overlay predicate, menu, warning). |
| B. Focus overtime | `Phase = focus` with elapsed past the duration, plus an "owed" flag | No break semantics touched | `Remaining()` is 0 and `PhaseEndsAt` sits in the past, which the warning and countdown would misread. `SkipBreak` doesn't apply. A new path to start the break later. |
| C. New phase value | `Phase = deferred` | Explicit | Breaks the extension's `none`/`focus`/`break` assumptions everywhere; widest blast radius |

**A** is the natural fit. The break is owed and simply isn't running yet, which is exactly the
held-time pattern `Paused` and `Idle` already use. Design confirms.

## What the rule leaves undecided

The init rule says *what* (corner panel, retry every 5 min, cap about 3, pill) but not:

1. What happens when the **camera turns off** while a break is owed.
2. What the **corner panel offers**: just a message, or actions.
3. What happens **after the cap**: a pill until when, and whether panels keep coming.

Those are product decisions (below).

## Research

M3 already verified the GNOME Shell 48 APIs a corner panel needs (`addTopChrome`, input regions,
St layout pitfalls, destroy semantics) against Shell 48.8 source. M5 measured the camera signal
live. Nothing in M6 depends on an unverified external behavior, so a research lane would add little.

## Risks

- **A new UI surface** is the area where M3's four visual defects lived, and none were reachable by
  tests. Live verification needs a real call with the camera on.
- **A camera signal flapping** (a call app briefly releasing the device) could bounce the hold on
  and off. The 5s tick bounds that, but the spec should state what a short release does.
- **Size.** It touches the daemon state, D-Bus, the extension and a new module, and M5 ran 52% over.
  It's likely the largest milestone yet.
- **`Deferred` must be cleared everywhere a break ends or is credited**: skip, T3, idle credit,
  suspend, downtime and stop. A missed path leaves a phantom owed break.
