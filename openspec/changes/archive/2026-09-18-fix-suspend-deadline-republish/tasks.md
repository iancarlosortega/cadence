# Tasks: Republish the deadline after a short suspend

## Review Workload Forecast

Estimated changed lines: **~45**.

| Artifact | Estimate |
|----------|----------|
| `daemon/internal/session/machine.go` | ~4 (one effect added, comment corrected) |
| `daemon/internal/session/machine_test.go` | ~40 (one test amended, one added) |

Delivery strategy: `single-pr`.

Decision needed before apply: **No**.
Chained PRs recommended: No
Chain strategy: null
400-line budget risk: **None** — an order of magnitude inside the budget.

M2's forecast was 35% low, so this one is deliberately generous. Even at triple the estimate it does
not approach the ceiling.

### Suggested Work Units

One. The fix, its tests and its verification are a single reviewable unit; splitting a four-line
domain change across PRs would cost more attention than it saves.

## Phase 1: Fix

- [x] 1.1 `daemon/internal/session/machine.go` — `applySuspend` short-suspend path: add `EffectNotify{Reason: "resumed from short suspend"}` to the returned effects, keeping the existing `EffectPersist` unchanged (design Decision 2)
- [x] 1.2 `daemon/internal/session/machine.go` — correct the short-suspend comment: it currently explains only the persist, and must now say why the notify is required (the deadline moved without a phase change). Keep it to one line per the project's comment policy

## Phase 2: Tests

- [x] 2.1 `machine_test.go` — amend `TestShortSuspendDoesNotCredit`: bind `effects` instead of discarding it with `_`, and assert an `EffectNotify` is present. This is the assertion whose absence hid the defect for the whole life of M1 (design Decision 3)
- [x] 2.2 `machine_test.go` — add a test pinning the *consequence*: after a short suspend, the deadline a client would derive (`resumeAt + next.Remaining()`) equals the daemon's own remaining, so the test fails even if a future edit emits the wrong effect
- [x] 2.3 Assert on the presence of the `EffectNotify` type, never on `len(effects)` — effect counts are brittle and say nothing about which effect is there (design Decision 3, rejected alternative)

## Phase 3: Verification

- [x] 3.1 `go test ./...` in `daemon/` — all packages pass, including the two tests above
- [x] 3.2 **Mutation check**: remove the `EffectNotify` line, confirm 2.1 and 2.2 both go red, restore it. A regression test never observed failing is a guess, not a test
- [x] 3.3 `git diff --stat` — only `machine.go` and `machine_test.go` changed; `dbusapi/`, `extension/` and the store schema untouched
- [x] 3.4 Live check — done 2026-09-18 on a real 9s suspend (`systemd-logind` 15:34:58 → 15:35:07), the same duration that produced the original defect. Offset measured at full precision across three samples: **+0.319s, +0.319s, +0.319s**. Sub-second and constant, which is the whole-second quantization of `PhaseEndsAt` (`now.Add(remaining).Unix()` truncates the fraction at publish), not drift. Before the fix the same suspend left a constant **3 whole seconds** that never recovered
- [x] 3.5 Confirm M2 finding F1 is resolved and M2 task 6.3 is no longer blocked by this defect (it remains blocked on a Wayland session restart, which is M2's own business)

## Note on the live check

3.4 is the same measurement that found the defect, run again. That is deliberate: the bug was found
by measuring a running daemon rather than by reading code, and the fix should be closed the same way.
The unit tests prove the effect is emitted; only 3.4 proves a real client stops drifting.
