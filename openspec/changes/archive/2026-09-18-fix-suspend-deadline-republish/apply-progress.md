# Apply Progress: Republish the deadline after a short suspend

Status: **complete — 10/10 tasks, verified end-to-end on a live daemon.**

## Change

| File | Diff |
|------|------|
| `daemon/internal/session/machine.go` | +6 / -3 |
| `daemon/internal/session/machine_test.go` | +38 / -1 |

Total 2 files, +44 / -4. Budget was 150 tracked lines; openspec artifacts were excluded from the
attempt candidate so the budget measured code, not planning prose.

`dbusapi/`, `extension/`, the store schema and the D-Bus contract are untouched, as designed.

## The fix

```go
// Declining to charge the suspend moves the phase deadline, so clients
// counting from the published PhaseEndsAt must be told (specs/daemon-control).
ns := s
ns.LastObserved = ev.To
return ns, []Effect{
    EffectPersist{Reason: "resumed from short suspend"},
    EffectNotify{Reason: "resumed from short suspend"},
}
```

## Tasks

### Phase 1: Fix
- [x] 1.1 `EffectNotify` added to the short-suspend return, `EffectPersist` unchanged
- [x] 1.2 Comment replaced: it now states why the notify is required, in one line

### Phase 2: Tests
- [x] 2.1 `TestShortSuspendDoesNotCredit` binds `effects` instead of discarding it and asserts the notify is present
- [x] 2.2 `TestShortSuspendRepublishesDeadline` added — pins the consequence, asserting the derived deadline equals resume plus remaining, and explicitly that it is not the stale pre-suspend deadline
- [x] 2.3 Assertions use a `hasNotify(effects)` type check, never `len(effects)`

### Phase 3: Verification
- [x] 3.1 `go test ./...` in `daemon/` — all packages pass
- [x] 3.2 Mutation check — see below
- [x] 3.3 `git diff --stat` — only the two intended files
- [x] 3.4 Live check on a real suspend — see below
- [x] 3.5 M2 finding F1 resolved; M2 task 6.3 no longer blocked by this defect

## Evidence

### Mutation check (3.2)

`EffectNotify` removed, suite re-run:

```
--- FAIL: TestShortSuspendDoesNotCredit (0.00s)
    machine_test.go:144: want EffectNotify: elapsed did not advance, so the deadline moved
--- FAIL: TestShortSuspendRepublishesDeadline (0.00s)
    machine_test.go:158: want EffectNotify so PhaseEndsAt is recomputed on resume
FAIL
```

Both assertions go red without the fix, and green with it. These tests have been observed failing,
so they are regression tests rather than guesses — which is precisely what the original
`TestShortSuspendDoesNotCredit` was not.

### Live check (3.4)

Rebuilt daemon installed (`~/.local/bin/cadenced`, 15:32:31), session started, baseline offset **0s**.
Real suspend of 9 seconds — `systemd-logind` 15:34:58 → 15:35:07, the same duration that produced the
original defect.

Full-precision sampling after resume:

| sample | daemon true | client derived | offset |
|--------|-------------|----------------|--------|
| 1 | 2685.413s | 2685.094s | **+0.319s** |
| 2 (+15s) | 2670.409s | 2670.091s | **+0.319s** |
| 3 (+30s) | 2655.406s | 2655.087s | **+0.319s** |

Sub-second and constant. `PhaseEndsAt` is published as a whole-second unix value
(`now.Add(remaining).Unix()`), so it truncates the fractional second at publish time and a client
sits a fixed fraction below truth until the next publish. That is the resolution of the contract,
not drift, and it does not grow.

Before the fix, an identical 9-second suspend left a constant **3 whole seconds** that never
recovered. The error was bounded by the idle-credit threshold, so a suspend just under 10 minutes
could have left the panel roughly nine minutes wrong.

## Deviations

None. The change matches the design exactly.

## Observed but not touched

- `daemon/internal/session/state.go` fails `gofmt -l` on a pre-existing comment-alignment nit
  (`PhaseNone` has a double space before its trailing comment). Unrelated to this change and left
  alone rather than bundled into a bugfix diff.
- `IdleSince` (`state.go:57`) remains dead — declared, documented, never read or written. Recorded
  as part of finding F2 for M4.
