# Design: M1 — Daemon Core

## Technical Approach

The repository is empty — no existing code, patterns, or tests to follow. Conventions are established here.

`internal/session` holds the state machine and its ports. It imports no D-Bus, GNOME, or `time.Now`. Everything outside it is an adapter. The machine is a pure reducer: `Apply(state, event, now) -> (state, effects)`. All five rules (phase cycle, idle-credit, pause, suspend, tier gating) are expressed as events, so `specs/session-timer` maps one-to-one onto table tests.

## Architecture Decisions

| Decision | Options | Tradeoff | Chosen |
|---|---|---|---|
| Countdown transport | per-second property; poll; deadline + local tick | per-second wakes the shell ~3600×/h; polling adds sync round-trips on the UI loop | **deadline + local tick** — publish `PhaseEndsAt`, emit on transitions only |
| Clock | `time.Now` inline; injected port | inline makes a 50m rule untestable without sleeping | **injected `Clock` port** |
| Elapsed measurement | monotonic; wall; both | Go's monotonic clock may stop across suspend (system-dependent) | **wall clock for elapsed, monotonic only within a tick** |
| Suspend detection | infer from clock jump; `login1 PrepareForSleep` | inference cannot distinguish suspend from NTP correction | **`PrepareForSleep`**, clock-jump as fallback |
| Persisted quantity | deadline; elapsed-in-phase | a stored deadline resumes already-expired | **elapsed-in-phase** |
| TOML | `pelletier/go-toml/v2`; `BurntSushi/toml` | both adequate | **BurntSushi 1.6.0** — `MetaData.Undecoded()` rejects unknown keys |
| Bus access control | none; peer filtering | session bus is already per-user | **none** — documented boundary, not a control |

**D-Bus trust boundary**: any process running as this user may call `StopSession`. That is equivalent to it editing `~/.config/cadence`, so no additional control is added. Recorded deliberately, not overlooked.

## Data Flow

```
login1 ──PrepareForSleep──┐
IdleSource ──idle event───┤
Clock ──tick──────────────┤
                          ▼
CLI ──D-Bus──▶ dbusapi ──▶ session (pure reducer) ──▶ effects
                   ▲            │
                   │            ├──▶ Store  (elapsed-in-phase)
                   └─PropsChanged◀── (transitions only)
```

## File Changes

| File | Action | Description |
|---|---|---|
| `daemon/go.mod` | Create | Module, Go 1.26 |
| `daemon/internal/session/state.go` | Create | Phase, State, Event types |
| `daemon/internal/session/machine.go` | Create | `Apply` reducer |
| `daemon/internal/session/ports.go` | Create | Clock, TierSource, IdleSource, Store |
| `daemon/internal/config/config.go` | Create | TOML load, defaults, validation |
| `daemon/internal/store/file.go` | Create | Atomic state file |
| `daemon/internal/dbusapi/service.go` | Create | Export, props, methods |
| `daemon/internal/dbusapi/sleep.go` | Create | `PrepareForSleep` subscriber |
| `daemon/cmd/cadenced/main.go` | Create | Wiring, event loop |
| `daemon/cmd/cadence/main.go` | Create | CLI client |
| `packaging/cadenced.service` | Create | systemd user unit |

## Interfaces / Contracts

```go
type Clock interface{ Now() time.Time }

type Event interface{ isEvent() }
// Tick, StartSession, StopSession, Pause, Resume, SkipBreak,
// IdleObserved(d), Suspended(from, to)

// Apply is total and side-effect free; effects are returned, not performed.
func Apply(s State, e Event, now time.Time) (State, []Effect)
```

State file (`~/.local/state/cadence/session.json`) stores `phase`, `elapsed_in_phase`, `paused`, `paused_remaining`, `updated_at` — never a deadline. Writes are write-temp-then-rename.

`PhaseEndsAt` is derived at publish time (`now + remaining`), never stored.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | All 14 spec scenarios | Table tests over `Apply` with a fake clock; no sleeping, no desktop |
| Unit | Config defaults, unknown-key rejection | Golden TOML fixtures |
| Unit | Store round-trip, partial-write recovery | Temp dir |
| Integration | Export, methods, props, no per-second emission | Real daemon on the session bus; count `PropertiesChanged` over 60s |
| E2E | Not applicable in M1 | No UI exists |

Suspend is tested as an ordinary `Suspended(from,to)` event — no real suspend needed.

## Threat Matrix

No routing, shell, subprocess, VCS/PR automation, or executable-file classification boundary exists in M1.

| Boundary | Applicability |
|---|---|
| Documentation-like paths | N/A — no file classification or execution |
| Git repository selection | N/A — no VCS invocation |
| Commit state | N/A — no VCS invocation |
| Push state | N/A — no VCS invocation |
| PR commands | N/A — no PR automation |

No tasks or RED tests are generated from this matrix.

## Migration / Rollout

No migration required — no prior version or persisted data. Install is `systemctl --user enable --now cadenced.service`.

## Open Questions

- [ ] Does the monotonic clock advance across suspend on Fedora 42 / kernel 6.19? Untested. The design does not depend on the answer (wall clock plus `PrepareForSleep`), but the answer determines whether clock-jump fallback is reachable in practice.
- [ ] Should `StopSession` clear persisted state or retain it for inspection? Defaulting to clear; no spec scenario covers it.
