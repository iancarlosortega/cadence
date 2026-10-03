# Design: m5-tier-detection

## Context

The `TierSource` port (`daemon/internal/session/ports.go`) is synchronous: `CurrentTier() Tier`. It
is sampled once per 5s tick in `Service.Tick` (`daemon/internal/dbusapi/service.go:167-171`),
**before** `apply` takes the service mutex. A slow probe therefore delays the tick loop but blocks
no D-Bus method call.

Research (`research.md`) fixed the signals:

| Tier | Signal | Claim |
|---|---|---|
| T3 | A child under `/org/gnome/Mutter/ScreenCast/Session` | C4 |
| T2 | A user process holding `/dev/video*` open; or the PipeWire `Video/Source` node `running` | C2, C3, U1 |
| T1 | A PipeWire `Stream/Input/Audio` node `running` | C1 |

## Decisions

### D1 — Keep the port; compose probes in the adapter layer

`TierSource` is unchanged. A new `dbusapi.TierDetector` implements it from three probes:

```go
type probe interface {
	name() string
	sample() (present bool, err error)
}
```

`CurrentTier` runs every probe, maps errors to "absent", and returns the highest tier present.
Probes are independent, so one failing never suppresses another.

Availability logging follows `MutterIdleSource.setAvailable`: one line per probe on each
available/unavailable transition, never per tick (`session-timer` "Tier Source Availability").

**Rejected — one PipeWire adapter for everything.** Research C2 showed Brave's camera never touches
the PipeWire node, and T3 is cheapest and most precise from Mutter. One source would be wrong for
the camera and slower for T3.

### D2 — Screencast probe: introspect the Mutter session directory

The probe calls `org.freedesktop.DBus.Introspectable.Introspect` on `org.gnome.Mutter.ScreenCast`
at `/org/gnome/Mutter/ScreenCast/Session` and reports present when the returned node has at least
one child `<node>`. godbus's `introspect.Node` parses the XML.

At rest the path exists with no children; during a share `Session/u2` appeared and was removed when
the share ended (C4). Any error, including the service being absent, means not presenting.

**Rejected — portal sessions.** One `webrtc_session` object stayed after the call ended (C6). A
stale T3 would skip breaks indefinitely, which is the one failure the user cannot see.

**Rejected — the gnome-shell `meta-screen-cast-src` PipeWire node** (C5). It's correct but redundant
with the Mutter object, and it would make T3, the only tier that changes behavior, depend on
spawning `pw-dump`.

### D3 — Camera probe: scan `/proc/*/fd` in-process

The probe walks `/proc/[0-9]*/fd/*`, calls `os.Readlink` on each entry, and reports present when
any target matches `/dev/video[0-9]+`. Unreadable process directories (other users, races with
exiting processes) are skipped, not errors. Only a failure to read `/proc` itself is an error.

The walk takes about 14ms (C8) and spawns nothing. The root is a constructor parameter, so tests
use a temp directory of symlinks.

The PipeWire half of the camera signal (`Video/Source` running, for U1) comes from the D4 snapshot.
The camera counts as present if either half reports it.

**Rejected — `fuser`/`lsof` subprocess.** Same information, plus a process spawn and output parsing.

### D4 — PipeWire probe: poll `pw-dump` each tick, decode only what is needed

Each tick runs `pw-dump` under a 2s `context.WithTimeout` and decodes the JSON array into a minimal
struct (`type`, `info.state`, `info.props["media.class"]`). One snapshot answers two questions:

- **Mic (T1)**: any `PipeWire:Interface:Node` with `media.class == "Stream/Input/Audio"` and state
  `running`.
- **Camera (T2, U1 half)**: any node with `media.class == "Video/Source"` and state `running`.

The measured cost is about 10ms and 6.5MB RSS per run (C7), once every 5s. A missing binary, a
timeout, a non-zero exit or unparseable output all make the probe unavailable, and every PipeWire
signal reads absent.

**Rejected — a long-lived `pw-dump --monitor` reader.** It would cut per-tick work to near zero, but
needs process supervision, restart on PipeWire restart, and an incremental parse of a stream of
JSON arrays carrying added, changed and removed objects. Under P3 and P4 the PipeWire signals gate
nothing in M5, so that complexity buys a smaller number on a probe that only feeds a label.
Revisit if M6's T2 policy needs faster camera edges.

**Rejected — sampling PipeWire less often than the tick.** It saves about 8ms every 5s at the cost
of a staleness rule and a second clock in the adapter. It isn't worth it at the measured cost.

### D5 — Domain: gate on T3 only, and end a break that T3 interrupts

In `machine.go`:

- `transitionPhase`: focus → break unless `s.Tier == TierT3`. Under T3 the phase stays focus with
  `ElapsedInPhase = 0`, which is today's non-T0 behavior narrowed to T3. T1 and T2 now start the
  break (P3, P4).
- `applyTick`: after `ns.Tier = ev.Tier`, if `s.Phase == PhaseBreak && ev.Tier == TierT3`, end the
  break exactly as `EventSkipBreak` does: phase focus, elapsed 0, Persist + Notify. That case is
  checked before the idle cases, so presenting wins over everything else on the tick.

Ordering note: the T3 check reads the *new* tier from the event, so a share that starts on the same
tick the focus deadline is reached skips the break rather than starting it for one tick.

### D6 — Publish a tier-only change (latent defect D1)

`Service.apply` publishes only on `EffectNotify`. A tick that changes only the tier currently
returns no effects (`applyTick`'s default branch, and the `if s.Idle { return ns, nil }` path
inside an open idle window).

Fix at the single choke point: in `Apply`'s `EventTick` case, after `applyTick` returns, if
`ns.Tier != s.Tier` and the effects contain no `EffectNotify`, append
`EffectNotify{Reason: "tier changed"}`. Nothing is persisted, by D7.

That covers every branch of `applyTick` without touching each one, including paths a future
milestone adds.

### D7 — Tier is not persisted

`Tier` is sampled live. A stored `T3` restored at startup would be wrong until the first tick, and a
stored tier has no consumer. The `tier` field is dropped from the store record. `Restore` leaves
`State.Tier` at its zero value, normalized to `TierT0`. Old state files that still carry `"tier"`
decode fine, because `encoding/json` ignores unknown fields.

### D8 — Extension: withhold the warning while T3

`render.js` `computeDisplay`: the warning condition gains `&& state.tier !== 'T3'`. `state.tier` is
already mapped from the proxy (`extension.js:130`). No overlay change is needed: when the daemon
ends a break under T3 (D5), the overlay's existing "phase left break" exit removes it
(`break-overlay` "Self-Owned Exit").

### D9 — Wiring

`cmd/cadenced/main.go` replaces `fixedT0Tier{}` with
`dbusapi.NewTierDetector(conn, "/proc", "pw-dump")` and deletes `fixedT0Tier`. Both the constructor
and the probes fail safe, with no startup error path, like `NewMutterIdleSource`.

## Data Flow

```
tick (5s, outside mutex)
  TierDetector.CurrentTier()
    screencastProbe  -- D-Bus Introspect --> Mutter        -> T3?
    cameraProbe      -- /proc/*/fd readlink               -> T2?
    pipewireProbe    -- pw-dump JSON --> mic / video node -> T1? / T2?
  -> EventTick{Tier}
apply (mutex)
  Apply: applyTick (T3 gate, T3 ends break) + tier-change Notify
  -> publish one batched PropertiesChanged if anything changed
extension
  Tier T3 -> no warning; Phase leaves break -> overlay exits
```

## Testing

| Layer | Approach |
|---|---|
| Domain | Table tests in `machine_test.go`: T1/T2/T3 at the focus deadline, T3 during a break, a tier-only change emits Notify (incl. inside an idle window), no Notify when the tier is unchanged |
| Composite | Fake probes: precedence, an error maps to absent, a transition-only log |
| Screencast | Parse of captured Introspect XML with and without a child node; an absent service reads as absent |
| Camera | Temp-dir `/proc` fixture with `fd` symlinks to `/dev/video0` and to other targets; an unreadable pid directory is skipped |
| PipeWire | Decode of trimmed real `pw-dump` captures from research (`1-mic`, `2-camera`, `0-rest`); a missing binary reads as unavailable |
| Service | `dbusapi` bus test: a tier-only tick emits exactly one signal carrying `Tier` |
| Extension | `test-render.js`: no warning under T3 at 60s; warning cleared on the T3 change |
| Live | Real call: share → T3 within a tick and no break at the deadline; share during a break removes the overlay; camera only → T2 with the overlay shown; leave → T0 |

## Migration / Rollout

Daemon and extension changes are independent. The new daemon behaves correctly with the old
extension, except that the amber warning still shows during a share (cosmetic). The extension's
`tier !== 'T3'` check is inert against the old daemon, which always reports `T0`. They can land in
one commit or two. Rollback is reverting the commit; no state migration is needed (D7).

## Open Questions

None blocking. U3 (window versus full-screen share) is checked in live verification.
