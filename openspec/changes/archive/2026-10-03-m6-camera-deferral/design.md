# Design: m6-camera-deferral

## Context

- The break decision lives in `transitionPhase` (`daemon/internal/session/machine.go:257`). The
  tick reducer is `applyTick`, whose case order since M5 is: T3 ends a break → idle credit → idle
  pause (focus only) → default (advance, transition).
- Time is elapsed-only (`state.go:46-50`), and publishing derives instants (`service.go` `publish`).
- In the extension, `_render` drives the overlay from pure predicates in `render.js`, and a 1s
  `timeout_add_seconds` tick runs while `available && sessionActive && !paused`
  (`extension.js:273-282`).

## Decisions

### D1 — A held break is `Phase = break` with a hold stage

`State` gains:

```go
type HoldStage string
const (HoldNone HoldStage = "none"; HoldPrompt = "prompt"; HoldPill = "pill")

Hold        HoldStage     // "" is normalised to HoldNone
Prompts     int           // prompts so far for the current break
SincePrompt time.Duration // held, unpaused time since the last prompt
```

`HoldStage` values map 1:1 to the D-Bus `Hold` string, so `publish` needs no translation.

**Why a break phase** (exploration Option A): `SkipBreak`, "T3 ends a break", the menu's Skip
during a break, and the overlay's "phase left break" exit all keep working unchanged. While held,
`ElapsedInPhase` doesn't advance, the same frozen interval as `Paused` and `Idle`. So when the hold
lifts, the full remaining break runs.

**Rejected — focus overtime (Option B).** `Remaining()` would be 0 and `PhaseEndsAt` in the past.
`SkipBreak` wouldn't apply. And a second path to start a break later would be needed.

**Rejected — a new phase value (Option C).** It breaks every `none`/`focus`/`break` assumption in
the extension.

### D2 — Policy constants live in the session package

```go
const (
	PromptRetry = 5 * time.Minute
	PromptCap   = 3
)
```

They are not in `Durations`, so neither the config file nor the store record carries them.
Configuring them waits for config hot reload, which would move them into `Durations` then.

### D3 — Entering and re-entering a hold share one helper

```go
func hold(ns State) State // called with Phase already break
	if ns.Prompts < PromptCap { ns.Prompts++; ns.Hold = HoldPrompt } else { ns.Hold = HoldPill }
	ns.SincePrompt = 0
```

- `transitionPhase`, focus → break: under `TierT2`, the break starts held via `hold`. T3 still
  stays in focus; T0 and T1 start normally.
- `applyTick`, a running break that sees `T2`: `hold`. The count is not reset, so a flapping camera
  re-enters at the cap straight into the pill. That's the "A flapping camera cannot escape the cap"
  scenario.

### D4 — `applyTick` case order

| # | Case | Effect |
|---|---|---|
| 1 | break, any hold, tier T3 | End the break (M5), via `release` |
| 2 | idle ≥ credit, latch unset | `creditBreak` (now calls `release`) |
| 3 | break, held, tier < T2 | **Lift**: `Hold = none`, `SincePrompt = 0`, `LastObserved = now` (the held gap is not charged). Persist + Notify |
| 4 | break, not held, tier T2 | **Re-hold**: `hold(ns)`, `LastObserved = now`. Persist + Notify |
| 5 | break, held, tier T2 | **Retry clock**: `SincePrompt += now − LastObserved`, `LastObserved = now`, elapsed untouched. On reaching `PromptRetry`: at prompt stage below the cap, `Prompts++` and `SincePrompt = 0`; at the cap, `Hold = pill`. Persist + Notify on those edges only; otherwise no effects |
| 6 | idle ≥ pause, focus | Unchanged |
| 7 | default | Unchanged (advance, transition) |

Idle credit (2) is checked before the hold cases. A user idle for 10 minutes with the camera on
(say, a webinar) is credited like any absence, and the hold is released. That's consistent with
"Idle Credit": the absence is the break.

### D5 — One release helper on every end and credit path

```go
func release(ns State) State { ns.Hold = HoldNone; ns.Prompts = 0; ns.SincePrompt = 0; return ns }
```

It's called from:

- `creditBreak`, which covers idle, suspend and downtime credit;
- `EventSkipBreak`;
- the T3 case;
- `EventStopSession`, which builds a fresh state and so releases implicitly — asserted by test;
- `transitionPhase` break → focus, a normal completion.

One helper means one place to forget. Each path gets a domain test asserting `Hold == none` and
`Prompts == 0` afterwards.

### D6 — Pause and resume need nothing new

`applyTick` returns early while paused, so the retry clock freezes. `EventPause` stores
`PausedRemaining = Remaining()`, which is the held break's frozen remainder, and `EventResume`
restores it. The hold stage and count survive a pause untouched.

### D7 — Publishing

- New properties: `Hold` (s) and `Prompts` (i, an int32 on the wire). Each is added to the
  `propsMap` seed and the `desired` map.
- Frozen interval: `publish` treats `Hold != none` like `Paused`. `PhaseEndsAt = 0`, and
  `RemainingSeconds` is the frozen `Remaining()`.
- Emission: the edges in D4 (3, 4, 5-on-edge) return `EffectNotify`, and the quiet retry ticks
  return nothing. M5's tier-only Notify in `Apply` still covers a T2 ↔ T1 change that doesn't move
  the hold, for example during focus.

### D8 — Persistence

The store record gains `hold`, `prompts` and `since_prompt_ns`. `Load` normalises an empty `hold`
to `none`, so old files load as "not held". Downtime replay on start runs through `creditBreak`,
which releases.

### D9 — Extension: one new module, pure predicates, no new timer

- **`render.js`:**
  - `computeDisplay` treats `hold !== 'none'` like `paused`: frozen label, no warning.
  - `shouldShowOverlay` additionally requires `hold === 'none'`.
  - New `promptSurface(state, panelUntil, now)` returns `'panel'`, `'pill'` or `null`:
    - `'pill'` when `hold === 'pill'`;
    - `'panel'` when `hold === 'prompt' && now < panelUntil`;
    - otherwise `null`.
- **`extension.js`:**
  - Maps `Hold` and `Prompts` in `_refresh` and the interface XML, defaulting to `'none'` and `0`.
  - In `_onStateChanged`, when `prompts` increases while `hold === 'prompt'`, it sets
    `_panelUntil = now + PANEL_SECONDS` (15).
  - `_render` asks `promptSurface` and drives the controller.
  - **No dedicated timeout.** The 1s render tick already runs while active and unpaused, so the
    panel leaves within a second of its 15s, with nothing extra to cancel at teardown. A one-second
    imprecision is fine for an informational panel. M3's `timeout_add` precision rule was about the
    3s hold, which must match its progress bar.
- **New `prompt.js` `PromptController`**, mirroring `OverlayController`:
  - `showPanel(monitor)`, `showPill(monitor)`, `hide()`, `destroy()`. At most one actor, swapped
    when the surface changes, and destroyed rather than hidden.
  - Added with `Main.layoutManager.addTopChrome(actor)`, so input is absorbed only over its own
    rectangle and there is no modal grab.
  - Positioned at `monitor.x + monitor.width − width − 16`, `monitor.y + Main.panel.height + 16`.
    The width is read after allocation; a fixed CSS width avoids that dance.
  - The panel is an `St.BoxLayout` (vertical) holding an `St.Label` message and an `St.Button`
    "Skip" that calls `client.call('SkipBreak')` — request only.
  - The pill is an `St.Label` in a rounded `St.Bin`.
  - Every child gets an explicit `x_align`, per M3.
  - Repositioned on `monitors-changed`, and destroyed in `disable()` before the client.
- **`stylesheet.css`:** `.cadence-prompt`, `.cadence-prompt-button`, `.cadence-pill`.

## Data Flow

```
tick: tier T2 at focus deadline
  transitionPhase -> break, hold(): Hold=prompt Prompts=1   -> PropertiesChanged
  extension: prompts 0->1 -> panelUntil=now+15 -> panel shown; overlay predicate false
  +15s render tick -> promptSurface null -> panel destroyed
  +5m retry clock -> Prompts=2 -> panel again ... Prompts=3 ... +5m -> Hold=pill -> pill
tick: tier T0
  lift: Hold=none, break runs                                -> PropertiesChanged
  extension: promptSurface null, shouldShowOverlay true -> overlay
```

## Testing

| Layer | Approach |
|---|---|
| Domain | One test per session-timer scenario. Release asserted on skip, T3, idle credit, suspend credit, downtime credit, stop and normal completion. Quiet retry ticks emit nothing |
| Store | Round-trip of hold, prompts and since-prompt; an old file without the fields loads as `none` |
| Service | A D-Bus test entering a hold publishes `Hold`, `Prompts`, `PhaseEndsAt = 0` in one signal; a retry tick emits `Prompts` only |
| Render | `computeDisplay` frozen while held; `shouldShowOverlay` false while held and true after a lift; `promptSurface` for every stage and the 15s boundary |
| Live | Real call with the camera on and a temporary 1-minute focus. The retry interval is a code constant, so a test build with `PromptRetry = 30s` exercises the cap within minutes — built, installed, then reverted |

## Migration / Rollout

The daemon and extension must land together. A new daemon with the old extension would publish
`Phase = break` for a held break, and the old extension would show the overlay on camera: no worse
than M5. A new extension with the old daemon reads `Hold` as missing, defaulting to `none`, which is
M5 behavior. Old state files load as not held. Rollback is a revert.

## Open Questions

None blocking.
