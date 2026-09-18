# Exploration: M1 — Daemon Core (timer state machine, CLI, D-Bus)

## Current State

The repository is empty: `git init` on `main`, no commits, no `go.mod`, no source files, no CI, no lint or test configuration. There is no existing code to read, so this exploration is grounded in the target environment and the confirmed design rather than in an existing codebase.

Confirmed and verified environment facts:

| Fact | Value | Verification |
| --- | --- | --- |
| Go toolchain | 1.26.0 | `go version` |
| Session bus | `unix:path=/run/user/1000/bus` | `$DBUS_SESSION_BUS_ADDRESS` |
| `godbus/dbus/v5` | latest v5.2.2 | `go list -m -versions` |
| `BurntSushi/toml` | latest 1.6.0 | `go list -m -versions` |
| `pelletier/go-toml/v2` | latest 2.4.3 | `go list -m -versions` |
| Idle source | `org.gnome.Mutter.IdleMonitor.GetIdletime` | live call returned 11126 ms |
| systemd | 257, user services active | `systemctl --user` |
| `strict_tdd` | false (zero project roots discovered) | sdd-init |

M1 scope is the Go daemon only: timer state machine, CLI, and D-Bus interface. No GNOME extension, no UI, no pixels.

## Affected Areas

Nothing exists yet; all paths below are created by this change.

- `daemon/go.mod` — new Go module. First build marker in the repo; its arrival makes `strict_tdd` re-evaluable.
- `daemon/internal/session/` — the domain. Timer state machine covering the five interacting rules: focus phase, break phase, idle-credit, tiers (T0–T3), deferral. Must not import D-Bus, GNOME, or the OS clock.
- `daemon/internal/config/` — TOML load and validation for `~/.config/cadence/config.toml`.
- `daemon/internal/dbusapi/` — D-Bus adapter exporting the domain on the session bus.
- `daemon/cmd/cadence/` — CLI adapter (`start|stop|status|skip`), a thin D-Bus client.
- `daemon/cmd/cadenced/` — daemon entry point.
- `packaging/cadenced.service` — systemd user unit.
- `openspec/config.yaml` — already written; `rules.apply.guidelines` forbids domain code importing D-Bus, GNOME, or the OS clock.

## Approaches

### Decision 1 — What the D-Bus interface exposes for the countdown

This is the load-bearing interface decision, because the extension renders a per-second countdown.

1. **Push per-second state** — daemon emits a signal or property change every second with `remaining_seconds`.
   - Pros: extension is trivially dumb; single source of truth for the number.
   - Cons: ~3,600 bus messages per hour, forever, for a value the client can compute. Wakes the shell process every second even when nothing meaningful changed. Any bus hiccup shows as a stuttering clock.
   - Effort: Low
2. **Push transitions only, expose an absolute deadline** — daemon exposes `Phase`, `PhaseEndsAt` (absolute Unix timestamp), `Paused`, `Tier`, and emits `PropertiesChanged` only when one of those actually changes. The extension runs its own 1-second `GLib` timer and renders `PhaseEndsAt - now`.
   - Pros: bus traffic proportional to real state changes (a handful per hour). The extension can keep rendering correctly even if the daemon is briefly unreachable. `prop.Export` with `Emit: prop.EmitTrue` provides `org.freedesktop.DBus.Properties.PropertiesChanged` with no hand-written signal code — verified in the godbus docs.
   - Cons: two clocks exist, so clock skew and suspend/resume must be reasoned about explicitly. Pause must be represented as remaining-duration, not as a deadline, or a paused timer silently keeps expiring.
   - Effort: Medium
3. **Client polls `Status()` on a timer** — no signals; the extension calls a method every second.
   - Pros: simplest possible daemon.
   - Cons: strictly worse than option 2 — same per-second cost, plus a synchronous round-trip on the shell's main loop, which risks visible jank.
   - Effort: Low

### Decision 2 — Clock handling in the domain

1. **Call `time.Now()` directly in the state machine.**
   - Pros: least code.
   - Cons: makes the five interacting rules testable only in real time. A 50-minute focus rule cannot be unit-tested without either sleeping or exposing seams anyway. This is the single most likely cause of the domain becoming untestable.
   - Effort: Low
2. **Inject a `Clock` interface (`Now()`, plus a tick source).**
   - Pros: the whole state machine becomes table-testable — advance a fake clock, assert phase transitions, idle-credit, and deferral behavior deterministically, with no desktop session and no sleeping. Directly serves the hexagonal split already chosen.
   - Cons: one extra interface and a fake implementation.
   - Effort: Low

### Decision 3 — Session persistence across daemon restart

The agreed behavior is resume-on-restart.

1. **Write state on every transition only.**
   - Pros: minimal I/O.
   - Cons: a crash 40 minutes into a focus block restores a 40-minute-stale position.
   - Effort: Low
2. **Write state on every transition plus a periodic heartbeat (~60 s).**
   - Pros: worst-case loss is bounded at one minute, which is invisible against a 50-minute block. Cheap: one small file write per minute.
   - Cons: periodic writes while otherwise idle.
   - Effort: Low
3. **Persist elapsed-in-phase rather than a deadline.**
   - Independent of 1/2 and required for correctness: storing an absolute deadline means a daemon restarted after a suspend resumes with a deadline already in the past, firing a break immediately on wake.
   - Effort: Low

### Decision 4 — TOML library

1. **`BurntSushi/toml` 1.6.0** — the long-standing reference implementation; `DecodeFile` plus `MetaData.Undecoded()` makes rejecting unknown keys straightforward, which matters for a hand-edited config.
2. **`pelletier/go-toml/v2` 2.4.3** — faster, `encoding`-style API, strict mode available.

Both are maintained and adequate; this is a low-stakes choice.

## Recommendation

- **Decision 1 → Approach 2.** Expose `Phase`, `PhaseEndsAt`, `RemainingSeconds` (valid only while paused), `Paused`, `Tier`, and `SessionActive` as D-Bus properties via `prop.Export` with `Emit: prop.EmitTrue`; emit on transitions only and let the extension tick locally. Verified against godbus documentation: `prop.Export` handles `PropertiesChanged` without hand-written signal plumbing.
- **Decision 2 → Approach 2.** Inject a `Clock`. This is the difference between a testable domain and a domain that can only be exercised by waiting 50 minutes.
- **Decision 3 → Approaches 2 + 3.** Transition writes plus a 60-second heartbeat, storing elapsed-in-phase rather than an absolute deadline.
- **Decision 4 → `BurntSushi/toml`**, for `Undecoded()`-based rejection of unknown keys in a file the user is expected to hand-edit.

Proposed interface identity:

```
bus name:   dev.ian.Cadence
object path: /dev/ian/Cadence
interface:   dev.ian.Cadence1
methods:     StartSession, StopSession, Pause, Resume, SkipBreak, PostponeBreak, GetConfig, SetConfig
properties:  SessionActive, Phase, PhaseEndsAt, RemainingSeconds, Paused, Tier
signals:     (PropertiesChanged only, via the standard Properties interface)
```

Suggested module layout:

```
daemon/
├── go.mod
├── cmd/cadenced/       # daemon entry point
├── cmd/cadence/        # CLI, thin D-Bus client
└── internal/
    ├── session/        # DOMAIN — state machine, Clock port, no D-Bus/GNOME imports
    ├── config/         # TOML load + validate
    ├── store/          # session state persistence
    └── dbusapi/        # D-Bus adapter
```

M1 deliberately stubs the tier port: a `TierSource` interface returning `T0` always. Real detection is M5, and its riskiest input is unverified (see Risks).

## Risks

- **Two-clock skew and suspend/resume.** With an absolute `PhaseEndsAt` on the bus, a laptop suspend or a system clock change can make the extension render a deadline that has already passed. The domain must treat suspend as idle time rather than elapsed focus time, and the spec must state the expected behavior across suspend explicitly. This is the highest-value thing for the spec phase to pin down.
- **Pause representation.** If pause is stored as a deadline it keeps expiring while paused. Pause must convert to remaining-duration and back. Easy to get wrong, easy to test once the `Clock` is injected.
- **`strict_tdd` is currently false** only because the repo was empty at init. Once `daemon/go.mod` exists, `go test ./...` becomes a real workspace command — but `extension/` (GJS) has no standard runner, which may permanently prevent one workspace-level command from covering every in-scope project. Worth deciding deliberately rather than inheriting.
- **Well-known bus name choice is durable.** `dev.ian.Cadence` propagates into the extension, the systemd unit, and any future packaging. Changing it later is a coordinated rename across components.
- **Carried from design, not introduced here:** PipeWire mic detection is unverified and belongs to M5; if it proves unreliable, tier T1 collapses into T0. M1's stubbed `TierSource` keeps that risk fully out of this milestone.
- **Review budget.** A state machine plus config, store, D-Bus adapter, CLI, and their tests will plausibly exceed the 400-line PR budget. `delivery_strategy` is `ask-on-risk`, so the tasks phase forecast should be expected to raise this.

## Ready for Proposal

**Yes.** Product decisions were confirmed in the design interview and require no further discovery. The four open technical decisions above have clear recommendations grounded in verified library and environment facts.

The proposal should record M1 scope as: Go module, domain state machine with injected clock, TOML config, session persistence, D-Bus adapter, CLI, and systemd user unit — with a stubbed `TierSource` and explicitly no GNOME extension, no UI, and no real tier detection.
