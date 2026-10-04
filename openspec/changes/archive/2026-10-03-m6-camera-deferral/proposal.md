# Proposal: Hold the break while on camera, and ask quietly from a corner

## Intent

Since M5, a break that comes due while the camera is on puts the full-screen overlay over the call.
The user is visibly interrupted in front of other people, and the only escape is a hold-to-skip on
a screen the camera can see them fumbling with.

M6 gives T2 its own policy, the one fixed at sdd-init: **hold the break, ask from a small corner
panel, retry every 5 minutes up to a cap, then leave a persistent pill.** The break is never lost.
It starts the moment the camera turns off.

## Product decisions (all 2026-10-03)

| ID | Decision |
|---|---|
| P1 = A | When the camera turns off while a break is held, the break starts right away (normal overlay). |
| P2 = A | The corner panel shows a message and a Skip action. |
| P3 = A | After the cap, a pill stays until the camera turns off or the break is skipped. No more panels. |
| P4 = A | No research lane. M3 verified the Shell APIs, and M5 measured the camera signal live. |

## Behavior

```
focus deadline, tier T2
  -> break HELD: no overlay, break timer frozen
     corner panel: "Break due — you're on camera. I'll ask again in 5 min."  [Skip]
     +5 min still T2 -> panel again (2/3)
     +10 min still T2 -> panel again (3/3)
     +15 min still T2 -> pill: "Break owed" (stays; no more panels)
  camera off (T0/T1) at any point -> hold lifts, break runs, overlay shows
  Skip (panel or panel-indicator menu) -> break dropped, fresh focus
  share starts (T3) -> break ends, as in M5
  idle >= 10 min / long suspend / long downtime -> credited, fresh focus, hold cleared
```

Defaults: retry interval **5 min**, cap **3 prompts**, each panel visible for **15 s**. These are
named constants. Configuring them waits for config hot reload.

## Scope

### In Scope

- **Daemon: a held break.** Model it as `Phase = break` with a new held flag (exploration
  Option A). The break's elapsed time does not advance while held, the same frozen-interval
  convention as `Paused` and `Idle`.
  - **Hold** at the focus deadline under T2.
  - **Hold again** when T2 begins during a running break, so an overlay already up when the camera
    turns on gives way to the panel. The prompt count continues and does not reset, so a flapping
    camera can't escape the cap.
  - **Lift** when the tier drops below T2 (P1). The break then runs from where it was frozen.
  - **Retry timer** as an elapsed quantity advanced only while held and unpaused. Each 5 minutes
    under T2 is a new prompt, until the cap. Past the cap, the hold is in its pill stage.
  - **Clear** the held state on every path that ends or credits a break: skip, T3, idle credit,
    suspend credit, downtime credit, stop.
- **D-Bus.** Two new read-only properties:
  - `Hold` (string): `none`, `prompt` or `pill`;
  - `Prompts` (integer): incremented once per prompt, so clients can see each new prompt as an edge.

  While held, `PhaseEndsAt` is `0` and `RemainingSeconds` is frozen, as with `Paused`.
- **Extension: two new surfaces** in a new module, modelled on `OverlayController`.
  - **Corner panel:**
    - top-right of the primary monitor, below the top bar;
    - shown for 15 s on each new prompt;
    - a message and a **Skip** button calling `SkipBreak`;
    - absorbs clicks only over its own rectangle.
  - **Pill:** a small persistent "Break owed" chip in the same corner while `Hold = pill`. No
    actions. Skipping stays available from the panel-indicator menu, which already offers Skip
    during a break.
  - **No overlay while held.** `shouldShowOverlay` learns `Hold`.
  - **Panel indicator:** the frozen countdown, like paused. The warning already applies only in
    `focus`, so a held break never shows it.
- **Persistence.** Held state survives a daemon restart (new record fields). A restart with long
  downtime still credits through the existing path and clears it.

### Out of Scope

- Configurable retry interval, cap or panel duration. That waits for config hot reload.
- T1 policy, a sound, and the 10s lockout.
- A "Take break now" action (P2 = A).
- Fullscreen suppression changes. A held break shows no overlay anyway. Once the hold lifts, the
  existing break-edge fullscreen check applies as before.

## Success Criteria

1. A focus deadline with the camera on shows the corner panel, not the overlay. `Phase = break`,
   `Hold = prompt`, and the break countdown is frozen.
2. The panel disappears after about 15 s, and appears again 5 min later if the camera is still on.
3. After the third prompt, the next retry shows the pill, which stays.
4. Turning the camera off at any stage removes the panel or pill and shows the overlay within one
   tick, with the break running.
5. Skip on the panel ends the break: a fresh focus, and the panel gone.
6. Starting a share while held ends the break (M5 behavior), and nothing stays on screen.
7. The camera turning on mid-break replaces the overlay with the panel within one tick.
8. Restarting `cadenced` while held keeps the hold and its prompt count.

## Risks

| Risk | Mitigation |
|---|---|
| A missed clearing path leaves a phantom held break | One helper clears held state, called from every credit and end path; domain tests per path |
| A new St surface has the visual defects M3 hit, none reachable by tests | Reuse the M3 patterns (`BinLayout`, explicit `x_align`, destroy not hide); live check with screenshots |
| A flapping camera bounces between hold and overlay | The 5s tick bounds it; the count never resets, so the cap still applies |
| Size | Daemon state + D-Bus + persistence + a new extension module. Forecast below; `size:exception` accepted |

**Size forecast**: about 1000 changed lines. Domain about 120 plus tests about 250; D-Bus and
persistence about 60 plus tests about 120; extension module about 200, predicates about 50, render
tests about 120, CSS about 40.

## Capabilities

- **Modified**: `session-timer` (Tier Gating), `daemon-control` (Control Surface, new Hold
  Publication), `session-persistence` (Durable State), `break-overlay` (Overlay Presence),
  `panel-indicator` (Countdown Derivation).
- **New**: `camera-prompt` (corner panel and pill).
