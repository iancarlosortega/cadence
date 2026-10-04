# Proposal: Reload the config on save, and make the camera prompts tunable

## Intent

The sdd-init decision says the config file is *"owned and watched by the daemon, exposed over
D-Bus."* Today it's read once at startup. Changing a timing means restarting `cadenced`, and for a
running session also `cadence stop`/`start`, as every M5 and M6 live test did. The camera prompt
policy shipped in M6 is hardcoded.

M7 makes the daemon pick up a saved config within one tick, apply it to the running session, and
publish the active values. It also makes the camera prompt interval and cap configurable, and fixes
a validation gap where `0` silently meant "default".

## Product decisions (all 2026-10-03)

| ID | Decision |
|---|---|
| P1 = A | A change applies **automatically on save**. |
| P2 = A | A change **applies to the phase in progress**. Shortening focus below the time already worked starts the break on the next tick. |
| P3 = A | Configurable: the four durations **plus the prompt policy** (retry interval, cap). Extension tunables stay constants. |

**Stated defaults:**

- An invalid file on reload is ignored: the last good config stays active, and the problem is
  logged once.
- A deleted file reverts to the defaults, the same rule as a missing file at startup.
- Startup still refuses an invalid file with a diagnostic.

## Config file after M7

```toml
[timer]
focus_minutes = 50
break_minutes = 10

[idle]
pause_after_minutes = 3
credit_break_after_minutes = 10

[camera]
prompt_every_minutes = 5   # new: held-break retry interval
prompt_limit = 3           # new: prompts before the pill
```

## Scope

### In Scope

- **Watching**: the daemon re-checks the file's modification time and size on its existing 5s
  tick and reloads when either changes. That needs no new dependency or goroutine, and it is safe
  with editors that save by renaming, because it re-stats the path each time.
- **Applying**: a new `EventConfigChanged` in the pure domain sets the new policy.
  - A running phase keeps its elapsed time and is measured against the new length (P2).
  - A paused phase keeps its elapsed time too: its frozen remainder is recomputed from the new
    length.
  - A held break uses the new retry interval and cap on its next check.
- **Validation fix**: `0` and negatives are rejected with a diagnostic naming the key, as the spec
  already requires. The new keys follow the same rule.
- **Publication**: a read-only `Config` property (`a{si}`, key → value) carrying the active values,
  republished when a reload changes them. The new values are inspectable from `busctl` without
  reading the file.
- **Prompt policy moves into configuration**: `PromptRetry` and `PromptCap` become fields of the
  domain's configured policy, replacing the package constants.

### Out of Scope

- Extension tunables: the warning threshold, the hold-to-skip time and the panel duration (P3).
- Changing config over D-Bus. The file is the only writer, and the property is read-only.
- The tick and heartbeat intervals, which are daemon internals, not user policy.
- `overlay_monitor` and auto-arm, which are separate future changes.

## Success Criteria

1. Saving a new `focus_minutes` while focus runs changes `PhaseEndsAt` within one tick, with
   elapsed time unchanged.
2. Saving a `focus_minutes` below the elapsed time starts the break on the next tick.
3. Saving an invalid file (unknown key, `0`, a negative, broken TOML) leaves the active config
   unchanged and logs one line naming the problem. Fixing the file applies it.
4. Deleting the file reverts to the defaults within one tick.
5. `prompt_every_minutes = 1` makes a held break re-prompt after one minute, without a restart.
6. `Config` reads the active values on first connect and emits once per effective change. A save
   that changes nothing emits nothing.
7. `focus_minutes = 0` at startup fails with a diagnostic naming the key.

## Risks

| Risk | Mitigation |
|---|---|
| An editor writes in two steps and a half-written file is read | A parse error is "invalid": the last good config is kept and the next change retries |
| Shortening focus below the time worked surprises the user | Stated in the spec as the P2 rule; it only happens when the user edits the file |
| A `stat` every 5s | Trivial cost; no file read unless the modification time or size changed |
| Paused-phase arithmetic | Elapsed-preserving recomputation, pinned by a test |

**Size forecast**: about 550 changed lines. Config about 120 plus tests about 160; domain about 40
plus tests about 90; service and main about 50 plus tests about 60; spec-driven CLI and README
touch-ups about 30.

## Capabilities

- **Modified**: `daemon-configuration` (Defaults And Validation), `session-timer` (Tier Gating),
  `daemon-control` (Control Surface).
- **Added requirements**: `daemon-configuration` Live Reload; `daemon-control` Configuration
  Publication.
