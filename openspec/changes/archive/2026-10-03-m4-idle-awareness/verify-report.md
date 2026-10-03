```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:a9b2b20049c3218eb7ea5291dd4fc2c60f605b402af2413dc05044162ec6a6e4
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 10/10
scenarios: 38/38
test_command: go test ./...
test_exit_code: 0
test_output_hash: sha256:40a374a2995e4736e1660fea26ce6a098eab1d1d4c50a066c0c4054cb7578f61
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verify Report — m4-idle-awareness

**Date**: 2026-10-03
**Verdict**: **PASS WITH WARNINGS** — 0 blockers, 4 warnings
**Tasks**: 55/55 checked (8.6 waived by the user, see W1)
**Requirements**: 10/10 satisfied (8 modified, 2 added)

## Test Runs

| Command | Observed result |
|---|---|
| `cd daemon && go vet ./...` | clean |
| `cd daemon && go test -count=1 ./...` | all packages `ok` (`cmd/cadenced`, `config`, `dbusapi` 2.8s, `session`, `store`) |
| `cd daemon && go test -count=1 -v ./... \| rg -- '--- SKIP'` | 0 skips (F9 fix holds while a real `cadenced` owns the bus name) |
| `gjs -m extension/test-render.js` | `all checks passed` |

## Post-Apply Fixes (found in Phase 8, newer than apply-progress.md)

All in `daemon/internal/dbusapi/service.go` and its test.

| ID | Defect | Fix | Pinned by |
|---|---|---|---|
| F7 | Each idle edge emitted 7 `PropertiesChanged` instead of one | Edge properties batched into one signal carrying only what changed | `TestIdleEdgesEachEmitExactlyOneSignal`, `TestIdleEdgeSignalCarriesOnlyWhatChanged` |
| F8 | Countdown jumped back 1s at idle exit | Deadline republished from the frozen remainder | `TestCountdownDoesNotJumpBackwardsLeavingIdle` |
| F9 | dbusapi tests `t.Skip`'ed whenever a real `cadenced` held the production bus name | Tests no longer export under the production name | 0 skips in the run above |

## Live Verification (Phase 8)

| Task | Result | Evidence |
|---|---|---|
| 8.1–8.4 | PASS (2026-09-19) | One `PropertiesChanged` per window edge; countdown frozen during idle and resumed from the frozen value; `Pause` offered while idle |
| 8.5 (C7) | PASS (2026-10-03) | Locked 11:48:53–12:01:06. Mutter `GetIdletime` climbed monotonically to 703s across the lock. Window opened at 3 min (`Idle` true, `PhaseEndsAt` 0, `RemainingSeconds` 2791), exactly one credit at 10 min (`RemainingSeconds` 3000), closed at unlock |
| 8.6 (C8) | **WAIVED** | See W1 |
| 8.7 | PASS (2026-10-03) | `cadenced` on a private `dbus-run-session` bus (no Mutter, `ServiceUnknown`) with an active focus session for 60s: alive throughout, `Idle` false, `PhaseEndsAt` constant, exactly one `idle source unavailable` log line, clean shutdown |
| 8.8 | PASS (2026-10-03) | Locked 16:37:31–16:53:58. One credit at 16:47:38. `session.json` mtime changed only on the 60s heartbeat (`heartbeatInterval`, `daemon/cmd/cadenced/main.go:29`, documented in `daemon/README.md`) plus exactly 3 edge writes; ~200 ticks at 5s with no per-tick write |

**C7 answer**: Mutter's IdleMonitor tracks the lock screen. No screensaver or logind source is needed. An earlier 2026-09-19 run that showed fragmented windows was contaminated by input.

## Requirement Compliance

### session-timer

| Requirement / Scenario | Status | Evidence |
|---|---|---|
| **Idle Credit** | ✅ | |
| Short idle pauses | ✅ | `TestShortIdlePauses`; live 8.1 |
| Long idle credits | ✅ | `TestLongIdleCredits`; live 8.5, 8.8 |
| A long absence credits exactly one break | ✅ | `TestLongAbsenceCreditsExactlyOneBreak`, `TestTicksInsideAnIdleWindowAreSilent`; live 8.8 |
| A break advances while the user is idle | ✅ | `TestBreakDoesNotFreezeWhileIdle` |
| A break completes while the user is away | ✅ | `TestBreakCompletesWhileTheUserIsAway` |
| A second absence credits again | ✅ | `TestSecondAbsenceCreditsAgain` |
| Returning before the credit threshold resumes the same phase | ✅ | `TestReturnBeforeCreditResumesSamePhase`; live 8.3 |
| Real idle source (no zero stub) | ✅ | `MutterIdleSource` wired in `main.go`; live 8.5 |
| **Suspend Is Time Away** | ✅ ⚠️ | |
| Long suspend credits a break | ✅ | `TestLongSuspendCreditsBreak` |
| Short suspend does not credit | ✅ | `TestShortSuspendDoesNotCredit` |
| A suspend is not credited twice | ✅ ⚠️ | `TestSuspendAndIdleCreditTheSameAbsenceOnce`; not observed live (W1) |
| **Idle Source Availability** (added) | ✅ ⚠️ | |
| Idle source absent at startup | ✅ | `TestUnavailableIdleSourceReportsZero`; live 8.7 |
| Idle source disappears mid-session | ✅ ⚠️ | Same zero path per call; not exercised mid-session (W2) |
| Idle source returns | ✅ ⚠️ | Inspection only (W2) |

### daemon-control

| Requirement / Scenario | Status | Evidence |
|---|---|---|
| **Control Surface** | ✅ | |
| Start then inspect | ✅ | `TestStartThenInspect` |
| Start when already active | ✅ | `TestStartWhenAlreadyActiveNoop`, `TestStartSessionWhenAlreadyActiveIsNoop` |
| Idle is published from the first connection | ✅ | `Idle` exported in the initial property map (`service.go:100`) |
| **Change Notification** | ✅ | |
| No per-second traffic | ✅ | `TestNoPropertiesChangedOnQuietTick`, `TestNoEffectOnQuietTick` |
| Short suspend republishes the deadline | ✅ | `TestShortSuspendRepublishesDeadline`; live accidental 9s suspend on 2026-09-19 republished `PhaseEndsAt` by about +10s |
| An idle window emits exactly twice | ✅ | `TestIdleEdgesEachEmitExactlyOneSignal`, `TestNoPropertiesChangedInsideAnIdleWindow`; live 8.2, 8.5, 8.8 |
| A client never drifts | ✅ | `TestCountdownDoesNotJumpBackwardsLeavingIdle`, `TestLeavingIdleRepublishesALiveDeadline`; live 8.3 |
| **Idle Publication** (added) | ✅ | |
| Entering an idle window | ✅ | `TestIdleWindowPublishesAFrozenInterval`; live 8.5 |
| Leaving an idle window | ✅ | `TestLeavingIdleRepublishesALiveDeadline` |
| Pause during an idle window | ✅ | `TestPauseDuringIdleWindowClearsIdle` |
| Idle during a break | ✅ | `TestBreakDoesNotFreezeWhileIdle`; render check `idle during a break still counts down from PhaseEndsAt` |
| Idle while no session is active | ✅ | `applyTick` returns unchanged when `!s.Active` (`machine.go:94`); live 8.5 first run (no session, zero signals) |

### panel-indicator

| Requirement / Scenario | Status | Evidence |
|---|---|---|
| **Countdown Derivation** | ✅ | |
| Display self-corrects after suspend | ✅ | `late tick self-corrects`; carried from M2 (verified 2026-09-18) |
| Paused freezes on RemainingSeconds | ✅ | `paused reads RemainingSeconds, not PhaseEndsAt` |
| Idle freezes on RemainingSeconds | ✅ | `idle reads RemainingSeconds, not PhaseEndsAt`, `idle label does not advance with time` |
| Countdown resumes on return from idle | ✅ | `countdown resumes from PhaseEndsAt once idle clears`; live 8.3 |
| **Presentation States** | ✅ | Dimmed/inactive/unavailable checks; idle row covered by the idle checks above |
| **Break Warning** | ✅ | Boundary checks at 120s/121s, `no warning during break`, `paused never warns`, `idle never warns even under the threshold` |
| **Control Actions** | ✅ | `idle offers Pause and withholds Resume`, menu checks; live 8.4 |

## Warnings

- **W1 — Real-suspend double credit not observed live (8.6 waived).** Four suspend attempts (`systemctl suspend`, `rtcwake -m mem -s 660` ×3) all woke within 7–9s per `journalctl -k` (`PM: suspend entry` → `exit`), on wake IRQ 7 `pinctrl_amd`. The early wake persisted with ACPI `XH00` and USB device wakeup disabled, so it's the hardware, not cadence. The design Decision 2 risk (an absence counted once as suspend and again as idle) is covered only by `TestSuspendAndIdleCreditTheSameAbsenceOnce` and the `applySuspend` latch. Re-test on hardware that holds S3.
- **W2 — Idle source loss and return mid-session not exercised.** 8.7 covered absence at startup only. Recovery relies on `conn.Object` addressing the well-known name, so each call routes to the current owner; this was confirmed by inspection, with no test. A test that swaps the owner, or a live `gnome-shell` restart, would close it.
- **W3 — Change size.** `git diff --stat`: 837 insertions, 61 deletions across 12 tracked files, plus 152 lines in the untracked `idle.go`/`idle_test.go`. That's above the 400-line review budget; `size:exception` was accepted at preflight, and the user commits directly to `main`.
- **W4 — F10 follow-up (pre-existing, not a spec violation).** `cadence status` prints the stored `RemainingSeconds`, which is stale between transitions. `panel-indicator` "Countdown Derivation" explicitly allows that staleness, and the panel derives from `PhaseEndsAt` correctly. The CLI should derive the same way. Track it as its own fix.

## Next

`archive`.
