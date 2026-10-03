# Proposal: Detect what others can see, and never break while presenting

## Intent

`TierSource` has been stubbed to `T0` since M1 (`daemon/cmd/cadenced/main.go:31-35`). cadence has
no idea whether the user is in a call or sharing the screen. A break that fires mid-presentation
puts a full-screen "Time to move" overlay in front of the audience, which is the most embarrassing
thing this app can do.

M5 makes tier detection real and ships the policy that matters most: **while the screen is being
shared, breaks are skipped silently.** Camera and microphone are detected and published truthfully,
but they gate nothing yet.

## Product decisions (all 2026-10-03)

| ID | Decision |
|---|---|
| P1 = A | Detection for every signal + T3 silent skip. The T2 policy and its corner panel and pill move to M6. |
| P2 = A | Research first. Done: `research.md`, lane `tier-signals`, measured live in a Brave call. |
| P3 = A | T1 collapses into T0. The mic is detected so `Tier` reads truthfully, but a T1 break behaves like T0. |
| P4 = A | T2 behaves like T0 until M6. The overlay shows on camera. |
| P5 = A | Any Mutter screencast is T3, including the GNOME recorder and OBS. |

Net effect in M5: **only T3 changes behavior.** T0, T1 and T2 all take the normal break.

## Scope

### In Scope

- **A real `TierSource`** composed of independent probes. Highest tier wins: T3 > T2 > T1 > T0.
  - **Screencast (T3)**: any child under `/org/gnome/Mutter/ScreenCast/Session` (research C4).
  - **Camera (T2)**: any of the user's processes holding `/dev/video*` open (C2, C3). Also the
    PipeWire `Video/Source` node `running`, for apps that reach the camera through PipeWire (U1).
  - **Microphone (T1)**: any PipeWire `Stream/Input/Audio` node `running` (C1).
  - Each probe fails safe to "absent", so a broken probe can only lower the tier, never cause a
    skip. Unavailability is logged on transition only, like `MutterIdleSource`.
- **T3 silent skip at the focus deadline**: no break, no overlay, and a fresh focus block. That
  replaces today's accidental rule (`machine.go:223-237`), which applies the same restart to *every*
  non-T0 tier. T1 and T2 now start the break.
- **T3 beginning during a break ends the break.** Silent skip means nothing visible while
  presenting. An overlay already on screen when sharing starts would be shown to the audience.
- **No pre-break warning while T3.** The amber panel label at T-2 min sits in the top bar, which a
  full-screen share shows. "Nothing before" covers it.
- **Fix latent defect D1**: a tick whose only change is the tier must publish `Tier`
  (`daemon-control` "Change Notification"). Without the fix, the extension would keep showing a
  warning or an overlay after sharing starts.
- **Stop persisting `Tier`**, or ignore it on restore. It is sampled live, and a stored T3 restored
  at startup would be wrong until the first tick.
- **Specs**: extend `session-timer` "Tier Gating" with T1–T3 scenarios. Add a tier-source
  availability requirement like M4's idle one. Add the no-warning rule for T3 to `panel-indicator`.
  `break-overlay` is unchanged: the daemon ends the break, and the overlay already leaves when the
  phase leaves `break` (its "Self-Owned Exit" requirement).

### Out of Scope

- **T2 policy** (defer, retry every 5 min, cap, pill) and the **corner panel UI**. That's M6.
- **A sound or a 10s lockout** on the normal overlay, and therefore anything T1 would mute or shorten.
- **Telling a recording apart from a call share.** P5 counts both as T3.
- **Per-app allowlists** or configuration of tier rules.

## Approach

The approach mirrors M4's `MutterIdleSource`:

- A `dbusapi` adapter per signal behind the unchanged `TierSource` port, composed in `cmd/cadenced`.
- `Tick` already samples sources **outside** the service mutex (`service.go:167-171`), so probe cost
  delays the tick loop but blocks no D-Bus method.
- Measured probe costs are about 10ms for `pw-dump` (C7) and about 14ms for the `/proc` walk (C8),
  well inside the 5s tick. Design decides between polling `pw-dump` and a long-lived
  `pw-dump --monitor` reader, and whether the mic probe can run less often, since under P3 it only
  feeds a label.

## Success Criteria

1. Sharing the screen in a real call publishes `Tier = T3` within one tick, and stopping it returns
   the tier to the correct lower value within one tick.
2. A focus deadline reached while sharing starts no break: no overlay, and a fresh focus block.
3. Starting a share during a break removes the overlay within one tick.
4. The panel shows no amber warning while T3.
5. Camera on, no share: `Tier = T2`, and the break **does** start, with the overlay shown (P4).
6. Mic only: `Tier = T1`, and the break starts normally (P3).
7. With every probe failing (no PipeWire, no Mutter), the daemon runs, reports `T0`, and logs each
   unavailability once.
8. A tier change with no other change emits exactly one `PropertiesChanged` carrying `Tier`, and
   ticks with no change emit nothing.

## Risks

| Risk | Mitigation |
|---|---|
| A false T3 silently eats breaks, and the user never finds out | Probes fail safe to absent. T3 needs a positive Mutter session. Live checks cover rest and after-call. |
| Other call apps reach the camera through PipeWire, not V4L2 (U1) | The camera probe checks both paths |
| `pw-dump` spawn every 5s | Measured at about 10ms; design may move to a monitor stream |
| Recording silently skips breaks (P5) | Accepted by the user; stated in the spec so it isn't a surprise |
| Size: M4 ran 130% over forecast | `size:exception` accepted. Forecast below weights tests at 1.5× |

**Size forecast**: about 700 changed lines (adapters ~200, adapter tests ~250, domain + D1 ~80,
domain tests ~120, extension ~30).

## Capabilities

- **Modified**: `session-timer`, `daemon-control`, `panel-indicator`.
- **New**: none.
