# Research: m5-tier-detection — lane `tier-signals`

**Date**: 2026-10-03
**Method**: live capture on the user's machine (Fedora 42, GNOME Shell 48, Wayland) during a real
call in Brave. `~/tier-snap.sh` snapshotted three things at each step: `/dev/video*` holders
(`fuser`), the PipeWire node and client graph (`pw-dump`), and the Mutter ScreenCast and portal
object trees (`busctl --user tree`).

| Step | Time | State |
|---|---|---|
| 0-rest | 17:14:14 | No call |
| 1-mic | 17:15:33 | In call, mic on, camera off, no share |
| 2-camera | 17:16:04 | Mic + camera |
| 3-share | 17:16:27 | Mic + screen share, camera off |
| 4-camera-share | 17:16:37 | Mic + camera + screen share |
| 5-after | 17:16:49 | Call left |

## Observations

| Signal | rest | mic | camera | share | cam+share | after |
|---|---|---|---|---|---|---|
| `Stream/Input/Audio` node `running` (Brave input, `RecordStream`) | — | ✅ ×2 | ✅ ×2 | ✅ ×2 | ✅ ×2 | — |
| `Audio/Source` HyperX mic `running` | — | ✅ | ✅ | ✅ | ✅ | — |
| `/dev/video0` held by `brave` (pid 1699768) | — | — | ✅ | — | ✅ | — |
| PipeWire camera `Video/Source` | suspended | suspended | **suspended** | suspended | **suspended** | suspended |
| `Stream/Output/Video` `meta-screen-cast-src` (gnome-shell) `running` | — | — | — | ✅ | ✅ | — |
| `Stream/Input/Video` `webrtc-consume-stream` (brave) | — | — | — | ✅ | ✅ | — |
| `/org/gnome/Mutter/ScreenCast/Session/u2` object | — | — | — | ✅ | ✅ | — |
| Portal `/org/freedesktop/portal/desktop/session/1_123/webrtc_session*` | — | — | — | 2 | 2 | **1 (lingers)** |

## Claims

| # | Claim | Evidence | Grade |
|---|---|---|---|
| C1 | Joining a call with the mic on creates `Stream/Input/Audio` nodes in state `running`, and they disappear when the call ends | 1-mic through 4, absent in 0 and 5 | **Verified** |
| C2 | Brave opens the camera **directly through V4L2**. `/dev/video0` gains a `brave` holder, and the PipeWire `Video/Source` node stays `suspended` throughout | 2-camera and 4: `fuser` shows brave; node suspended | **Verified** |
| C3 | Therefore PipeWire node state is **not** a camera signal for Brave. An open file descriptor on `/dev/video*` is | Follows from C2 | **Verified** (for Brave) |
| C4 | A screen share is marked by a Mutter ScreenCast session object under `/org/gnome/Mutter/ScreenCast/Session/`, which appears with the share and is removed when it ends | 3, 4 present; 0, 1, 2, 5 absent | **Verified** |
| C5 | The same share also shows as a running gnome-shell `Stream/Output/Video` node, `meta-screen-cast-src` | 3, 4 | **Verified**, a second, redundant signal |
| C6 | Portal session objects are **not** a reliable share signal. One `webrtc_session` still existed after the call was left | 5-after | **Verified** |
| C7 | `pw-dump` of the full graph costs about 10ms and 6.5MB RSS, producing about 290KB of JSON | `/usr/bin/time`, three runs | **Measured** |
| C8 | A walk of `/proc/*/fd` takes about 14ms. Only the user's own processes are readable, which includes Brave | `time` over the glob | **Measured** (glob only; `readlink` adds cost) |
| C9 | Every signal returns to baseline when the call ends, apart from the portal session (C6) | 5-after vs 0-rest | **Verified** |

## Unverified

- **U1, other apps.** Only Brave was observed. A Flatpak or native Zoom or Teams client might reach
  the camera through PipeWire (portal camera access) rather than V4L2, which `fuser` would miss and
  the `Video/Source` node state would catch. Checking both covers either path.
- **U2, false T3.** Any Mutter screencast creates a session object, including GNOME's built-in
  screen recorder and OBS's PipeWire capture. Those would read as "presenting" and silently skip
  breaks while recording. That's arguably right (someone may watch the recording) but unconfirmed
  with the user.
- **U3, unshared monitor.** Sharing a single window versus the whole screen was not compared. Both
  should create a Mutter session, but it wasn't observed.

## Implications for Design

- **T3 (presenting)**: check whether `/org/gnome/Mutter/ScreenCast/Session` has any child node.
  That's one cheap D-Bus introspection per tick, the same shape as `MutterIdleSource`, with a
  fail-safe of "not presenting".
- **T2 (camera)**: any process of the user holding `/dev/video*` open (C2/C3), or the PipeWire
  `Video/Source` node `running` (U1). It's read in-process from `/proc`, with no subprocess spawn.
- **T1 (mic)**: any `Stream/Input/Audio` node `running` (C1). PipeWire is the only source for it, so
  reading it means `pw-dump` or a native client. Under P3, T1 only feeds the `Tier` property, so the
  design can price a cheaper, coarser read.
- **Precedence**: T3 > T2 > T1 > T0, evaluated from independent probes. Each probe fails safe to
  "absent", so a broken probe can only lower the tier, never invent a skip.
