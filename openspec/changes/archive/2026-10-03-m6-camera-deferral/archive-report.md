# Archive Report: m6-camera-deferral

**Change**: m6-camera-deferral (Milestone 6)
**Project**: cadence
**Archived**: 2026-10-03
**Verdict at verify**: pass with warnings — 9/9 requirements, 45/45 scenarios, 0 blockers, 4 warnings, 8/8 live checks

## What shipped

On camera, cadence no longer throws the full-screen overlay over the call. It holds the break and
asks quietly:

```
focus ends, camera on  -> break held, frozen; corner panel "A break is due, and you are on camera." [Skip break]
                          panel 15s, again every 5 min, 3 times; then a persistent "Break owed" pill
camera off             -> break starts right away (normal overlay)
Skip (panel or menu)   -> break dropped, fresh focus
```

That completes the sdd-init tier table: T3 skips silently (M5), T2 holds and asks (M6), and T1/T0
take the normal break.

| Area | Change |
|---|---|
| Domain | `Hold` stage (`none`/`prompt`/`pill`), `Prompts`, `SincePrompt`; `hold`/`release` helpers; lift, re-hold and retry-clock cases in `applyTick` |
| D-Bus | `Hold` (s), `Prompts` (i); frozen `PhaseEndsAt` while held |
| Persistence | `hold`, `prompts`, `since_prompt_ns`; unknown values load as not held |
| Extension | New `prompt.js` (panel + pill); `promptSurface` and `nextSuppression` predicates; overlay hidden while held |
| Cleanups | `config.go` gofmt; `godbus` direct in `go.mod` |

**1174 changed lines** against a ~1000 forecast: 17% over, after M4's 130% and M5's 52%.

## Spec changes merged

| Capability | Change |
|---|---|
| `session-timer` | Tier Gating: the T2 hold, prompts, the cap, the pill, the lift, and release on every end path |
| `daemon-control` | Control Surface (+ `Hold`, `Prompts`); **added** Hold Publication |
| `session-persistence` | Durable State: a held break survives a restart |
| `break-overlay` | Overlay Presence: no overlay while held; it appears on the lift |
| `panel-indicator` | Countdown Derivation: frozen while held |
| `camera-prompt` | **New capability**: Corner Panel, Pill, Prompt Surface Teardown |

## The defect review caught

Fullscreen suppression was latched on the first render with `phase === 'break'`, and for a held
break that's when the hold begins. A break held during a fullscreen call (the normal case on
camera) would have stayed suppressed after the call ended, giving **no overlay at all for a running
break**. The user would simply never have been told to move. Unit tests couldn't see it, because
the latch lived in `extension.js`. The independent verifier found it by reading. It now lives in a
pure `nextSuppression` with tests.

## Live verification notes

- Using a 30-second retry test build made the whole prompt → pill sequence observable in about two
  minutes. The source was reverted right after the build, so the test value never reached a commit.
- The overlay covers only the primary monitor and absorbs clicks, so the second monitor is the way
  to operate the call during a break.
- **Unexplained**: one held break ended at 19:22:17 as if skipped, while its panel was visible. The
  method-call recorder wasn't running yet. With the recorder on, every end was a recorded
  `SkipBreak`. Carried as a watch item (verify W1).

## Carried forward

- W1: watch for unexplained held-break ends in daily use.
- W2: the panel re-arms for 15s on connect or restart while held.
- W3: no automated tests for a short suspend while held, downtime replay of a held break, or panel
  teardown on disable.
- The retry interval, cap and panel duration are constants until config hot reload.

## Next milestone

Config hot reload (the original M6). It would also make `PromptRetry`, `PromptCap` and the panel
duration configurable.
