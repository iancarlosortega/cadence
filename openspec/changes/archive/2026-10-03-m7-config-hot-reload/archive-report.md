# Archive Report: m7-config-hot-reload

**Change**: m7-config-hot-reload (Milestone 7, the original plan's M6)
**Project**: cadence
**Archived**: 2026-10-03
**Verdict at verify**: pass with warnings — 5/5 requirements, 32/32 scenarios, 0 blockers, 3 warnings, 7/7 live checks

## What shipped

The config file is now what sdd-init said it would be: *owned and watched by the daemon, exposed
over D-Bus.*

- **Save to apply.** The daemon re-checks the file's identity (exists, mtime, size) on its 5s tick.
  A change is reloaded and applied to the running session, with no restart and no `stop`/`start`.
- **Applies now.** A phase keeps its elapsed time and runs against the new length. Shortening below
  the time worked starts the next phase on the next tick. A paused phase keeps its elapsed time too.
- **Tolerant reloads, strict startup.** An invalid save is ignored and logged once. A deleted file
  reverts to the defaults. Startup still refuses an invalid file.
- **Camera policy is configurable**: `[camera] prompt_every_minutes` (5) and `prompt_limit` (3)
  replace M6's constants.
- **`Config` on D-Bus** (`a{si}`): the active values, inspectable with `busctl`.

| File | Change |
|---|---|
| `internal/config/config.go` | `[camera]` keys, `Defaults()`, `IsDefined`-based validation |
| `internal/config/watch.go` + test | new, 78 + 158 |
| `internal/session/{state,event,machine}.go` | policy in `Durations`; `EventConfigChanged` |
| `internal/dbusapi/service.go` | `Config` property, `reflect.DeepEqual` diff, `ApplyConfig` |
| `cmd/cadenced/main.go` | watcher seeded at startup, polled before each tick |
| `daemon/README.md` | `[camera]`, live reload, the stricter zero rule |

**851 changed lines** against a ~550 forecast (55% over).

## Spec changes merged

| Capability | Change |
|---|---|
| `daemon-configuration` | Defaults And Validation (+ camera keys, zero rejected); **added** Live Reload |
| `daemon-control` | Control Surface (+ `Config`); **added** Configuration Publication |
| `session-timer` | Tier Gating: the prompt interval and limit are configured, not fixed |

## Two defects found at design, both fixed

- **`0` passed validation.** `if X != 0 { if X <= 0 … }` meant a literal `0` silently became the
  default, against a spec that has required rejecting non-positive values since M1. Fixed with
  `toml.MetaData.IsDefined`. **Behavior change**: a file containing `= 0` now fails startup, naming
  the key.
- **`publish` couldn't diff a map.** `prev.Value() == v.Value()` panics on map values, and the panic
  is recovered into an error, so `Config` would never have been emitted. That was caught in red by
  the new service test, and the diff now uses `reflect.DeepEqual`.

## Live verification notes

- Hot reload made its own test easier. Raising `pause_after_minutes` through a reload let elapsed
  time accrue while the user was away, so "shortening below the time worked" could be tested
  without anyone at the keyboard.
- The user missed two 15-second panels while watching a call on the other monitor (verify W2).
  The panel shows on the primary monitor only.

## Carried forward

- **W2**: the corner panel is easy to miss on a multi-monitor desk. Candidates are panel placement
  (`overlay_monitor`-style) or a configurable panel duration.
- Extension tunables (warning threshold, hold-to-skip, panel time) are still constants, by P3.
- M6 watch item: an unexplained held-break end.

## Project status

This closes the numbered plan from sdd-init (2026-09-14): daemon (M1), panel indicator (M2), break
overlay (M3), idle awareness (M4), tier detection (M5), camera deferral (M6, split out of M5), and
config hot reload (M7).

Decided at init but never put in a milestone: **auto-arm** (offer to start a session after sustained
typing), the **`overlay_monitor`** setting, and **stats** (explicitly deferred).
