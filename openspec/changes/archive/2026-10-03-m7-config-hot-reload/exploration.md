# Exploration: m7-config-hot-reload

**Date**: 2026-10-03
**Milestone**: M7, the original plan's last numbered milestone ("Config hot reload — tunable
without rebuild")

## Intent

The sdd-init decision (2026-09-14): *"TOML at `~/.config/cadence/config.toml`, owned and watched by
the daemon, exposed over D-Bus. Explicitly NOT GSettings."*

Today the file is read once at startup. Every timing change in M5 and M6 live testing needed a
daemon restart, plus `cadence stop`/`start` for a running session.

## Current State (verified)

### Loading

- `config.Load(path) (session.Durations, error)` (`daemon/internal/config/config.go:50`) parses
  four integer-minute keys with BurntSushi/toml:
  - `[timer] focus_minutes`, `break_minutes`
  - `[idle] pause_after_minutes`, `credit_break_after_minutes`
- A missing file yields the defaults (50/10/3/10). An unknown key is an error naming the key.
- Called once in `cmd/cadenced/main.go:38-46`. On resume, config overwrites the persisted durations
  (`main.go:62`), so a daemon restart already applies new values.

### Defect: zero passes validation

`daemon-configuration` "Defaults And Validation" says non-positive durations MUST be rejected,
naming the key. Each check is `if X != 0 { if X <= 0 { error } }` (`config.go:73-96`), so `0`
skips the check and silently means "use the default". Only negatives are rejected. The fix needs
to distinguish "absent" from "zero": BurntSushi's `MetaData.IsDefined`, or pointer fields.

### Applying a change

- `Service.state` changes only through `apply(Event)` under the mutex (`dbusapi/service.go:206`).
- The event set (`session/event.go`) is a closed interface with an `isEvent()` marker.
- An `EventConfigChanged{Durations}` fits the pattern exactly: a pure `Apply` case, a
  `Service.ApplyConfig`, and a new `case` in the main loop's `select` (`main.go:100-114`).
- **Mid-phase semantics:** `ElapsedInPhase` is kept, and a phase ends when
  `Elapsed >= PhaseDuration`. Shrinking focus below the elapsed time ends the phase on the next
  tick; growing it extends the phase. Nothing clamps or defers.

### What else could become configurable

| Value | Where | Owner |
|---|---|---|
| `PromptRetry` 5m, `PromptCap` 3 | `session/state.go` | daemon (domain) |
| `WARNING_THRESHOLD_SECONDS` 120 | `extension/render.js:9` | extension |
| `HOLD_TO_SKIP_SECONDS` 3 | `extension/render.js:10` | extension |
| `PANEL_SECONDS` 15 | `extension/render.js:14` | extension |
| `tickInterval` 5s, `heartbeatInterval` 60s | `main.go:25,29` | daemon internals, not user policy |

The extension values can't reach the daemon's config without a new D-Bus channel. M2's
exploration noted there is *"no way to hand a preference to the extension other than a new D-Bus
property."*

### D-Bus

No configuration is exposed today. The interface carries only session state.

### Watching

There is no file-watch dependency (`go.mod`: toml, godbus; x/sys indirect).

| Option | Pro | Con |
|---|---|---|
| fsnotify | Immediate | New dependency; editors that save by atomic rename need a watch on the parent directory and re-adding |
| Poll mtime on the existing 5s ticker | No dependency, no new goroutine; rename-safe because it re-stats the path | Up to 5s latency; a `stat` every tick |
| SIGHUP / `cadence reload` | Explicit, no polling | Not "watched"; the user must remember to run it |

## Product Decisions Needed

- **P1 — How a change applies:** automatically on save (the init decision says "watched"), or
  only with an explicit `cadence reload`.
- **P2 — A change mid-phase:** applies to the current phase now, or only from the next phase.
- **P3 — Scope:** the four durations only; plus the daemon's prompt policy; or plus the extension's
  tunables as well.

**Stated defaults, no question needed:**

- **An invalid file on reload is ignored.** The daemon keeps the last good config and logs the key
  once. Failing the running daemon on a typo would be absurd. Startup keeps today's
  fail-with-diagnostic behavior.
- **No research lane.** Nothing external to measure: TOML parsing and file stat are local and
  deterministic.

## Risks

- **Editor save patterns:** truncate-then-write can expose a half-written file to a reader. A parse
  error there must be treated like any invalid file: keep the last good config and retry on the
  next change.
- **A shrinking focus below the elapsed time** starts a break at the next tick. Correct under "apply
  now", but surprising if unannounced.
- **Extension tunables** would add a D-Bus property set and an extension refresh path. That's the
  bulk of the size if P3 includes them.
