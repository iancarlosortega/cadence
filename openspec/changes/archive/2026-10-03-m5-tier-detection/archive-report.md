# Archive Report: m5-tier-detection

**Change**: m5-tier-detection (Milestone 5)
**Project**: cadence
**Archived**: 2026-10-03
**Verdict at verify**: pass with warnings — 4/4 requirements, 19/19 scenarios, 0 blockers, 3 warnings, 8/8 live checks

## What shipped

cadence now knows what other people can see. Screen sharing silently skips breaks. Camera and
microphone are detected and published, but gate nothing yet.

| Tier | Signal | Behavior in M5 |
|---|---|---|
| T3 presenting | A Mutter ScreenCast session exists | No break at the deadline (fresh focus); a break in progress ends; no amber warning |
| T2 on camera | A process holds `/dev/video*`, or the PipeWire `Video/Source` is running | As T0: the overlay shows (P4, until M6) |
| T1 listening | A PipeWire `Stream/Input/Audio` is running | As T0 (P3: nothing to mute) |
| T0 free | none | Normal break |

| File | Lines |
|---|---|
| `daemon/internal/dbusapi/tier.go` | new, ~305 |
| `daemon/internal/dbusapi/tier_test.go` | new, ~380 |
| `daemon/internal/dbusapi/service.go` + test | Tick samples only while active; tier-only signal test |
| `daemon/internal/session/machine.go` + test | T3 gate, T3 ends a break, tier-only Notify |
| `daemon/internal/store/file.go` + test | `Tier` no longer persisted |
| `daemon/cmd/cadenced/main.go` | `fixedT0Tier` deleted, `TierDetector` wired |
| `extension/render.js` + test | No warning under T3 |

**About 1067 changed lines** against a ~700 forecast: 52% over, down from M4's 130%. Weighting tests
at 1.5× helped, but the adapter test file alone was 380 lines.

## Spec changes merged

| Capability | Modified | Added |
|---|---|---|
| `session-timer` | Tier Gating (T1–T3, ending a break, highest wins, sampling window) | Tier Source Availability |
| `daemon-control` | Change Notification (+ "A tier change alone emits once") | — |
| `panel-indicator` | Break Warning (none while T3) | — |

`break-overlay` is unchanged: when the daemon ends the break, the overlay's existing "Self-Owned
Exit" removes it.

## Research changed the design before a line was written

The plan from sdd-init assumed PipeWire would expose the camera. The live call showed **Brave opens
`/dev/video0` directly**, and the PipeWire camera node stayed `suspended` the whole time. A
PipeWire-only adapter would have never seen the camera in the user's own call app.

The call also showed that **portal session objects outlive the call**. A probe built on them would
have reported "presenting" forever after the first share, silently skipping every later break.

## Latent defects fixed

- **D1**: a tier-only change was never published, because `apply` publishes only on Notify and
  that tick returned no effects. Fixed once, in `Apply`'s tick case.
- **D2**: any non-T0 tier restarted focus at the deadline. Narrowed to T3.
- Independent review found the detector sampled even with no session, spawning `pw-dump` every 5s
  all day. Fixed and pinned before verify.

## Product decisions

P1 Detection + T3 · P2 research first · P3 T1 → T0 · P4 T2 → T0 until M6 · P5 any screencast is
T3, including recordings.

## Carried forward

- **M6**: the T2 policy (defer, retry every 5 min, cap, pill) and the corner panel.
- Tier stale for at most one tick after resume (verify W1).
- No end-to-end test for "every signal unavailable" (verify W2).
- `config.go` gofmt nit; `godbus` marked indirect in `go.mod`.

## Next milestone

M6: the T2 policy and the corner panel. The original plan's M6, config hot reload, moves later.
