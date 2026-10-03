# Exploration: m5-tier-detection

**Date**: 2026-10-03
**Milestone**: M5, "meetings don't break it"

## Intent

Replace the `fixedT0Tier` stub with real detection of what other people can see, and give each tier
its break policy. These are the rules fixed at sdd-init (2026-09-14):

| Tier | Signal | Policy |
|---|---|---|
| T3 Presenting | ScreenCast active | Pure silent skip: nothing before or after |
| T2 On camera | Camera open | Corner panel only, defer and retry every 5 min, cap ~3, then a persistent pill |
| T1 Listening | Mic stream, no camera, no share | Overlay fires, muted, 3s friction instead of 10s |
| T0 Free | none | Normal overlay |

## Current State (verified)

### Port and stub

- Port: `session.TierSource { CurrentTier() Tier }` at `daemon/internal/session/ports.go:14-19`.
  It is synchronous, takes no arguments and returns no error.
- Stub: `fixedT0Tier` at `daemon/cmd/cadenced/main.go:31-35`, wired at `main.go:83`.
- Sampling: once per 5s tick in `Service.Tick` (`daemon/internal/dbusapi/service.go:165-172`), passed
  in as `EventTick.Tier`. `Tick` runs under the service mutex, so a slow probe blocks the daemon.

### Domain

- `applyTick` copies `ev.Tier` into state (`machine.go:99`), but only when the session is active and
  not paused.
- The only branch on tier is `transitionPhase` (`machine.go:223-237`). For any non-T0 tier, focus
  stays focus, **`ElapsedInPhase` is zeroed**, and Persist and Notify are emitted. So a non-T0 tier
  at the focus deadline restarts a full focus block. That is accidentally close to the T3 policy
  ("silent skip"), and wrong for T2 (no defer, no retry) and T1 (no overlay at all).
- `State` has no defer counter, retry deadline or pill flag.
- `Tier` is persisted (`store/file.go:26`) and restored on start. The first tick then overwrites it,
  so the stored value is meaningless at best.

### Latent defect: a tier change alone is never published

`Service.apply` publishes only on `EffectNotify` (`service.go:198-206`). A tick whose only change is
the tier takes `applyTick`'s default branch, which returns no effects. So the `Tier` property on the
bus stays at its last-transition value until the next phase change, idle edge or pause. Unreachable
today only because the stub never changes, which is the same condition M4 hit with Decision 2.
`daemon-control` "Change Notification" already requires the publish: *"A published property MUST
emit `PropertiesChanged` whenever its correct value changes."*

### D-Bus and extension

- `Tier` is already a published, read-only string property, `"T0"`..`"T3"` (`service.go:99`,
  `service.go:274`). It is declared in the extension XML (`extension/extension.js:39`) and mapped to
  `state.tier` (`extension.js:130`).
- **No extension code branches on tier.**
- The only suppression is fullscreen, decided once at the break edge (`extension.js:261-272`,
  `render.js:77-99`).

### What T1 would change today: nothing

- **"Muted"**: the overlay plays no sound. `extension/` has no audio, sound or mute code.
- **"3s friction instead of 10s"**: `HOLD_TO_SKIP_SECONDS = 3` (`render.js:10`) is the only
  friction, and it is already 3s. The sdd-init rule "no dismiss for the first ~10s" was never built.
  At M3 the user kept the hold at 3s after it felt long (M3 archive report).

So T1 has no observable difference from T0 on the current overlay. It becomes meaningful only if a
sound or a 10s lockout is added to T0 first, which reverses an M3 decision.

### T2 needs new UI

There is no corner panel and no pill anywhere in `extension/`, and `stylesheet.css` has no styles
for one. T2 is the largest piece by far: a new presentation surface, plus daemon-side defer state
with a retry counter and a cap.

## Detection Signals (measured on this machine, 2026-10-03, no call in progress)

| Signal | Observation |
|---|---|
| `/dev/video0`, `/dev/video1` | Present. `fuser` reports no holder at rest. |
| PipeWire camera | Exposed as a `Video/Source` node (`v4l2_input.pci-0000_13_00.0-usb-0_1.1_1.0`), state `suspended` |
| PipeWire mics | Three `Audio/Source` nodes, all `suspended` |
| App streams | One `Stream/Output/Audio` (Brave playback); no `Stream/Input/*` |
| Portals | `org.freedesktop.portal.Desktop` and `impl.portal.desktop.gnome` on the session bus |
| Mutter ScreenCast | `org.gnome.Mutter.ScreenCast` present; introspection exposes no session listing |
| Tooling | `pw-dump`, `pw-cli`, `wpctl`, `fuser`, `lsof` installed |

**Unknown, and needed before design:** what each signal looks like during a real Teams call in
Brave, which was the M1-deferred risk ("PipeWire mic detection unverified"):

1. Does joining a call create a `Stream/Input/Audio` node, and does an `Audio/Source` go `running`?
2. Does Brave's camera go through the PipeWire `Video/Source` node, open `/dev/video*` directly, or
   both? The answer decides whether `fuser` or PipeWire node state is the camera signal. If
   PipeWire also holds the device, `fuser` is positive whenever *anything* uses the camera.
3. What marks a screen share: a `gnome-shell` screencast `Video/Source` node, a portal session, or
   something on `org.gnome.Mutter.ScreenCast`?
4. How to read PipeWire from Go without blocking the tick: polling `pw-dump` every 5s (a process
   spawn plus a full-graph JSON parse, under the service mutex) versus a long-lived
   `pw-dump --monitor` reader updating a cached tier.

## Approaches

### Detection architecture

| Option | Shape | Pro | Con |
|---|---|---|---|
| A. One PipeWire adapter | Single `TierSource` reading the PipeWire graph (camera node state, input streams, screencast node) | One source of truth; matches the port | Depends on the unknowns above |
| B. Composite of probes | `TierSource` combining a PipeWire probe, a `/dev/video` probe and a portal probe, highest tier wins | Each probe testable alone; tolerant of one failing | More adapters; the probes can disagree |

Both keep the port unchanged and fail safe to T0, like `MutterIdleSource` fails safe to zero idle.
Choose after research.

### Slicing

M4 came in at about 990 lines against a ~430 forecast. All four tiers in one change would be well
past that. Natural cuts:

- **Detection plus T3**: the adapter, the publication fix, and silent skip. Daemon-only, and
  small.
- **T2**: defer state in the daemon plus the corner panel and pill in the extension.
- **T1**: only meaningful after a T0 sound or lockout exists.

## Product Decisions Needed Before Proposal

- **P1 — Scope of M5.** Which tiers ship in this change.
- **P2 — Research first?** Measure the signals live during a real call before design.
- **P3 — T1 fate.** Drop it (collapse to T0), or first add a sound or lockout so "muted" and
  "friction" mean something.

## Risks

- Detection is only as good as the live measurement. A false T3 silently eats breaks, which is the
  worst failure for this app: the user never finds out.
- Camera via PipeWire versus direct V4L2 may differ per app (Brave, Teams PWA, Zoom).
- A synchronous PipeWire probe on the 5s tick holds the service mutex.
- The unpublished tier change above must be fixed in the same change that makes it reachable.
