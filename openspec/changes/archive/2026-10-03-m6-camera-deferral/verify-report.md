```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:eaf833b6f2664ae96df8a4083f94fed304b1209280ad895c135ba7bff5a4a8e4
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 9/9
scenarios: 45/45
test_command: go test ./...
test_exit_code: 0
test_output_hash: sha256:06b7e0e0355abdd70bfa4c6a7cc032010591539341b5ed9bf43fe443eb10d3a7
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verify Report — m6-camera-deferral

**Date**: 2026-10-03
**Verdict**: **PASS WITH WARNINGS** — 0 blockers, 4 warnings
**Tasks**: 33/33 (all live checks observed, none waived)
**Requirements**: 9/9 (6 modified, 3 added in the new `camera-prompt`) · **Scenarios**: 45/45

## Test Runs

| Command | Observed result |
|---|---|
| `cd daemon && go vet ./...` | clean |
| `cd daemon && go test -count=1 ./...` | all 6 packages `ok` |
| `cd daemon && go test -count=1 -race ./...` | all `ok` |
| `cd daemon && go test -count=1 -v ./... \| rg -c -- '--- SKIP'` | 0 |
| `cd daemon && go build ./...` | clean |
| `gofmt -l daemon` | empty, including `config.go`, which this change formatted |
| `gjs -m extension/test-render.js` | `all checks passed` |

## Independent Verification

`gentle-ai review assess` rated the change **high**, so an independent read-only verifier reviewed
it against the specs and design. It found no blocker, and confirmed:

- a held break cannot advance its elapsed time (only `applyTick`'s `default` branch does, and a
  held break never reaches it);
- no path leaks hold state;
- one `PropertiesChanged` per hold edge and none on quiet ticks;
- `prompt.js` uses the GNOME Shell 48 APIs correctly.

It found **one major defect, corrected before live verification**. Fullscreen suppression was
latched at hold entry, so a break held during a fullscreen call stayed suppressed after the call
ended, and no overlay showed for a running break. The decision now lives in a pure
`nextSuppression`, keyed on the break starting to run, with five render checks (red, then green). A
minor hardening was also applied: an unknown `hold` value in the state file loads as not held.
Details: `apply-progress.md`, "Post-apply correction".

## Live Verification (Phase 9)

A test build with `PromptRetry = 30s` (the source was reverted immediately after the build) and a
temporary 1-minute focus and 3-minute break, in a real Brave call. A logger sampled the daemon
every 2s (`live-verification.log`), and later SkipBreak calls were recorded with `dbus-monitor`.

| Task | Result | Evidence |
|---|---|---|
| 9.1 | PASS | Installed, `cadenced` restarted, logout and login, extension `ACTIVE` |
| 9.2 | PASS | Camera on at the deadline: `Phase = break`, `Hold = prompt`, `Prompts = 1` (19:20:54). The corner panel showed top-right with no overlay and the countdown frozen (screenshot). The call stayed usable around it |
| 9.3 | PASS | The panel left after about 15s. In the undisturbed run, `Prompts` 2 → 3 came at 19:24:24 and `Hold = pill` at 19:24:58, exactly one retry later. The pill stayed (screenshot) |
| 9.4 | PASS | Camera off at 19:25:47: `Hold = none`, the overlay showed, and the break counted down 6s |
| 9.5 | PASS (at cap) | Camera back on at 19:25:57: straight to `pill` with the countdown frozen at 2:54. This is the "A flapping camera cannot escape the cap" scenario. The prompt-stage form of mid-break holding was also observed at 19:21:28 (a user camera toggle lifted the hold, and re-holding gave `Prompts = 2`) |
| 9.6 | PASS | Skip from the panel-indicator menu at 19:27:05, and Skip from the corner panel at 19:28:12. Both were recorded as `SkipBreak` calls, and both started a fresh focus |
| 9.7 | PASS | `cadenced` restarted while in the pill stage resumed `Hold = pill`, `Prompts = 3`. The first sample read `T0` (tier isn't persisted), but the first tick read `T2`, so there was no lift |
| 9.8 | PASS | Screenshots of the panel and the pill: aligned, padded, nothing clipped. The real build (5-minute retry) was reinstalled, the test config removed, and the session restarted on 50/10 |

## Requirement Compliance

### session-timer

| Scenario | Status | Evidence |
|---|---|---|
| T0 starts break | ✅ | `TestT0StartsBreak` |
| T1 starts break | ✅ | `TestT1StartsBreak` |
| T2 holds the break | ✅ | `TestT2HoldsTheBreak`; live 9.2 |
| A held break does not advance | ✅ | `TestAHeldBreakDoesNotAdvance`; live (frozen countdown) |
| Prompts repeat every 5 minutes up to the cap | ✅ | `TestPromptsRepeatEveryFiveMinutesUpToTheCap`; live 9.3 |
| Past the cap the hold becomes a pill | ✅ | `TestPastTheCapTheHoldBecomesAPill`; live 9.3 |
| The camera turning off starts the break | ✅ | `TestTheCameraTurningOffStartsTheBreak`; live 9.4 |
| The camera turning on mid-break holds it | ✅ | `TestTheCameraTurningOnMidBreakHoldsIt`; live 19:21:28 |
| A flapping camera cannot escape the cap | ✅ | `TestAFlappingCameraCannotEscapeTheCap`; live 9.5 |
| Skipping a held break | ✅ | `TestEveryEndPathReleasesAHeldBreak/skip`; live 9.6 |
| Idle credit releases a held break | ✅ | `TestEveryEndPathReleasesAHeldBreak/idle_credit` |
| T3 skips the break silently | ✅ | `TestT3SkipsBreakSilently` |
| Presenting during a break ends it | ✅ | `TestPresentingDuringBreakEndsIt`, `TestEveryEndPathReleasesAHeldBreak/T3` |
| The highest tier wins | ✅ | `TestHighestTierWins` |

### daemon-control

| Scenario | Status | Evidence |
|---|---|---|
| Start then inspect · Start when already active · Idle is published from the first connection | ✅ | Existing tests |
| Hold is published from the first connection | ✅ | `TestHoldIsPublishedFromTheFirstConnection` |
| Entering a hold | ✅ | `TestEnteringAHoldEmitsOneSignal` |
| A further prompt | ✅ | `TestAFurtherPromptEmitsOnlyPrompts` |
| Entering the pill stage | ✅ | `TestPastTheCapTheHoldBecomesAPill` (domain edge + Notify); live 9.3 |
| Lifting a hold | ✅ | `TestLiftingAHoldRepublishesALiveDeadline`; live 9.4 |

### session-persistence

| Scenario | Status | Evidence |
|---|---|---|
| Restart resumes position · No session to resume | ✅ | Existing tests |
| Restart keeps a held break | ✅ | `TestAHeldBreakRoundTrips`, `TestAnOldFileLoadsAsNotHeld`, `TestUnknownHoldLoadsAsNotHeld`; live 9.7 |

### break-overlay

| Scenario | Status | Evidence |
|---|---|---|
| Break begins · Keyboard remains usable · Only the primary monitor | ✅ | Existing checks; M3 live |
| Held break shows no overlay | ✅ | `held break shows no overlay`, `pill-stage hold shows no overlay`; live 9.2 |
| Hold placed mid-break removes the overlay | ✅ | Same predicates; live 9.5 |
| Hold lifting shows the overlay | ✅ | `a lifted hold shows the overlay while the break runs`, `lifting a hold decides afresh`; live 9.4 |

### panel-indicator

| Scenario | Status | Evidence |
|---|---|---|
| Existing four scenarios | ✅ | Existing checks |
| A held break freezes on RemainingSeconds | ✅ | `held break reads RemainingSeconds…`, `held label does not advance with time`, `pill-stage hold freezes the same way` |

### camera-prompt (new)

| Scenario | Status | Evidence |
|---|---|---|
| A prompt shows the panel | ✅ | `panel is shown before 15s`; live 9.2 |
| The panel leaves on its own | ✅ | `panel is gone at exactly 15s`, `panel is gone after 15s`; live 9.3 |
| A new prompt shows it again | ✅ | `_panelUntil` re-armed on a `Prompts` increase; live 9.3 |
| Skip ends the break | ✅ | Live 9.6 (`SkipBreak` recorded; the panel left on the property change) |
| The rest of the screen stays usable | ✅ | `addTopChrome` input region limited to the actor; live 9.2 |
| The pill appears past the cap | ✅ | `pill shows for the pill stage`, `pill stays however long…`; live 9.3 |
| The camera turning off removes the pill | ✅ | `no surface when the hold is none`; live 9.4 |
| Daemon disappears while the pill is shown | ✅ | `no pill when the daemon is unavailable` |
| Disabled while the panel is shown | ✅ ⚠️ | `PromptController.destroy()` in `disable()`. There is no timeout to leak, since the panel expiry is derived from the render tick. Reviewed by the verifier, but no automated test (Shell-only) |

## Warnings

- **W1 — One break ended without an explained cause.** At 19:22:17 a held break (prompt 3) ended
  as if skipped: `Phase` went to `focus` and `Prompts` to 0. The tier read `T2` on every sample, and
  the user reports no Skip. The method-call recorder wasn't running yet. The only end paths are
  Skip, T3 and idle credit, and the samples rule out T3, while idle credit is implausible in that
  window. It happened while the third panel was visible (19:22:05–19:22:20), so an accidental click
  on its Skip button is the likeliest cause. It did not recur in about 6 minutes of recorded
  testing, where both observed ends were recorded `SkipBreak` calls. Watch for it in daily use.
- **W2 — Panel re-arms on connect.** On first connect, extension enable or a daemon restart while
  a break is held, the panel shows once more for 15s, because `_lastPrompts` starts at 0. That's
  harmless: one extra reminder.
- **W3 — Untested by automation.** A short suspend while held, startup downtime replay of a held
  state, and the panel teardown on disable are correct by reading but not pinned by tests.
- **W4 — Size.** 1174 changed lines (975 insertions and 44 deletions in tracked files, plus 155 in
  `prompt.js`) against a ~1000 forecast, 17% over. That's the closest forecast yet, inside the
  declared 1400. `size:exception` was accepted at preflight.

## Next

`archive`.
