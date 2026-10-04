# Design: m7-config-hot-reload

## Context

- `config.Load(path) (session.Durations, error)` parses the TOML once at startup
  (`cmd/cadenced/main.go:38-46`). The `select` loop (`main.go:100-114`) drives `svc.Tick()` every 5s
  and `svc.Heartbeat()` every 60s.
- Domain policy lives in `session.Durations` (Focus, Break, IdlePause, IdleCredit). M6's
  `PromptRetry`/`PromptCap` are package constants (`session/state.go:47-53`), read at
  `machine.go:187,191,314`.
- `publish` diffs each desired property with `prev.Value() == v.Value()` (`dbusapi/service.go:314`).

## Decisions

### D1 — The prompt policy joins `Durations`

`session.Durations` gains `PromptRetry time.Duration` and `PromptCap int`. The constants are deleted,
and `machine.go` reads `s.Durations.PromptRetry` / `s.Durations.PromptCap`.

The struct name is now slightly narrow, since `PromptCap` is a count. Renaming it would touch every
domain test fixture for no behavior gain, so a comment on the type records that it is "the
configured policy". A rename can ride a future refactor.

`config.Defaults()` returns all six values. Tests that build `Durations` by hand get the prompt
defaults from a shared helper, so a zero cap can't silently turn every hold into a pill.

### D2 — Validation distinguishes absent from zero (fixes D1)

`config.Load` decodes into the schema as today, then for each key asks
`meta.IsDefined("timer", "focus_minutes")`:

- **defined** → the value must be `> 0`, else
  `config: timer.focus_minutes must be positive, got 0`;
- **absent** → the default.

That replaces the `if X != 0 { if X <= 0 … }` gating that let `0` through. The new `[camera]` keys
follow the same rule. Unknown keys are still rejected first, via `meta.Undecoded()`.

### D3 — Watching by polling the file's identity on the tick

A small `config.Watcher`:

```go
type Watcher struct { path string; last fileID }        // fileID{exists bool; mod time.Time; size int64}
func NewWatcher(path string, active fileID) *Watcher
func (w *Watcher) Poll() (d session.Durations, changed bool, err error)
```

`Poll` stats the path and compares `fileID` with the last one seen:

| Situation | Result |
|---|---|
| Same identity | `changed = false`, no read |
| Different, file missing | `config.Defaults()`, `changed = true` |
| Different, file present | `Load` it: success → `changed = true`; failure → `err` |

The new identity is recorded in every case, so a broken file is reported once and not re-parsed on
every tick. The next save changes the identity and retries.

**Why polling, not fsnotify.**

- No new dependency and no goroutine.
- It handles rename-on-save editors (vim, many IDEs) for free, because `stat` follows the path,
  whereas an inotify watch on the old inode goes silent.
- The cost is a `stat` every 5s.
- The 5s latency equals the spec's "within one tick interval".

**Rejected — SIGHUP / `cadence reload`.** P1 = A, automatic.

### D4 — Wiring in the main loop

`main.go` builds the watcher after the startup load, seeded with the file identity just read. The
ticker case becomes:

```go
case <-ticker.C:
	if d, changed, err := watcher.Poll(); err != nil {
		log.Printf("cadenced: config reload ignored: %v", err)
	} else if changed {
		if err := svc.ApplyConfig(d); err != nil { log… }
	}
	if err := svc.Tick(); …
```

Reloading before the tick means a shortened phase ends on that very tick, well within the
spec's "next tick". `ApplyConfig` mirrors `ApplySuspend`: a Go-only entry point, not exported on
D-Bus.

### D5 — `EventConfigChanged` in the pure domain

```go
type EventConfigChanged struct{ Durations Durations }
```

`Apply`:

1. If the new value equals `s.Durations`, return `s, nil`. A no-op save emits nothing.
2. `ns.Durations = ev.Durations`.
3. **Paused**: elapsed at the pause was `old.PhaseDuration(phase) − PausedRemaining`. Recompute
   `PausedRemaining = max(new.PhaseDuration(phase) − thatElapsed, 0)`. Elapsed is preserved, per
   "A paused phase keeps its elapsed time".
4. Return `EffectPersist` + `EffectNotify`. `PhaseEndsAt` and `Config` both changed.

There is no transition inside the event. A new length at or below the elapsed time is caught by the
next tick's `default` branch, which already does `ElapsedInPhase >= PhaseDuration → transition`.
That keeps one transition path. An idle-frozen focus keeps its frozen remainder semantics: it ends
when the idle window closes and the next advancing tick runs. Inactive sessions take the new value
too, so the next `StartSession` copies it.

### D6 — `Config` publication, and a comparison fix

`publish` adds `"Config": dbus.MakeVariant(configMap(s.state.Durations))`, a
`map[string]int32` keyed `timer.focus_minutes`, `timer.break_minutes`, `idle.pause_after_minutes`,
`idle.credit_break_after_minutes`, `camera.prompt_every_minutes`, `camera.prompt_limit`. It is
seeded in the property map as well.

**The diff must change.** `prev.Value() == v.Value()` panics at runtime when the values are maps
("comparing uncomparable type"). `publish` recovers panics into errors, so the symptom would be a
publish error on every change, not a crash, but no signal would go out. The comparison becomes
`reflect.DeepEqual(prev.Value(), v.Value())`, which handles every existing scalar property
identically.

### D7 — Persistence is unchanged

The store record already holds the four duration minutes, and `main.go:62` overwrites them from
config on resume. The prompt policy isn't persisted, because config wins on every start. No record
change.

### D8 — Startup behavior

Startup keeps failing on an invalid file. A user who breaks the file and restarts finds out
immediately (`daemon-configuration` "Defaults And Validation"). Only reloads are tolerant.

### D9 — Docs

`daemon/README.md` documents the `[camera]` section and live reload: within 5s, invalid saves
ignored and logged, deleting reverts to defaults.

## Data Flow

```
tick (5s)
  watcher.Poll(): stat → identity changed? → Load → Durations
    error → log once, keep active
    ok    → svc.ApplyConfig(d) → Apply(EventConfigChanged)
              equal → nothing
              else  → Durations replaced (paused remainder recomputed) → Persist + Notify
                       publish: PhaseEndsAt, RemainingSeconds, Config (DeepEqual diff)
  svc.Tick() → default branch ends a phase now at/over its new length
```

## Testing

| Layer | Approach |
|---|---|
| Config | Table tests: every key absent → default; `0` and negative → error naming `section.key`; new `[camera]` keys; unknown key still rejected |
| Watcher | Temp dir: unchanged → no reload; content change (bump mtime via `os.Chtimes` for same-second writes) → reload; invalid → error once, and not again until the next change; delete → defaults; rename-over (write temp, `os.Rename`) → reload |
| Domain | `EventConfigChanged`: equal → no effects; running focus keeps elapsed and `Remaining` uses the new length; shortened below elapsed → next tick transitions; paused remainder recomputed elapsed-preserving; held break uses the new interval and limit (pill at a lowered limit) |
| Service | `ApplyConfig` emits one signal carrying `Config` and `PhaseEndsAt`; equal config emits nothing; `Config` readable on first connect with six keys |
| Live | Edit the file while a session runs: focus length change, an invalid save, a fix, a deletion, and a short `prompt_every_minutes` on camera |

## Migration / Rollout

Daemon-only. Existing config files stay valid, since all new keys are optional. A file that
contains a literal `0` today (silently the default until now) will now fail at startup with a
diagnostic naming the key. That's the spec's existing rule, finally enforced, and it's called out in
the commit and the README.

## Open Questions

None blocking.
