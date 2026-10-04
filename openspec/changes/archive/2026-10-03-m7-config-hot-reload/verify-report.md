```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:e4981e082a6498ee303b2c0085ab4e0084a8f8dbc49bd297f5fa0051fb176dd4
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 5/5
scenarios: 32/32
test_command: go test ./...
test_exit_code: 0
test_output_hash: sha256:a31303bfa1a2f55d1340df4db06b597ec77692df2954db5ace7a36d7e04e8369
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verify Report — m7-config-hot-reload

**Date**: 2026-10-03
**Verdict**: **PASS WITH WARNINGS** — 0 blockers, 3 warnings
**Tasks**: 22/22 (all live checks observed, none waived)
**Requirements**: 5/5 (3 modified, 2 added) · **Scenarios**: 32/32

## Test Runs

| Command | Observed result |
|---|---|
| `cd daemon && go vet ./...` | clean |
| `cd daemon && go test -count=1 ./...` | all 6 packages `ok` |
| `cd daemon && go test -count=1 -race ./...` | all `ok` |
| `cd daemon && go test -count=1 -v ./... \| rg -c -- '--- SKIP'` | 0 |
| `cd daemon && go build ./...` | clean |
| `gofmt -l daemon` | empty |

## Review

`gentle-ai review assess` rated the change **medium**. The only flagged path was the unrelated
`.atl` registry, and the daemon change spawns no processes. Under the medium tier, the writer's
self-verification (red observed for the validation, event and map-comparison tests) plus the
parent's re-run of every gate and read of the domain event and main-loop wiring is the recorded
verification. Both design-time findings are confirmed fixed:

- **Zero passing validation (D2).** `TestZeroAndNegativeRejectedForEveryKey` and
  `TestZeroRejectionMessageFormat` failed before the `IsDefined` change.
- **`publish` panicking on map values (D6).** `StartSession` failed with `comparing uncomparable
  type map[string]int32` under the old `==` diff, and passes with `reflect.DeepEqual`.

## Live Verification (Phase 8)

These used the installed build, with `~/.config/cadence/config.toml` edited while the daemon ran
and no restarts between steps.

| Task | Result | Evidence |
|---|---|---|
| 8.1 | PASS | No file: `Config` = `a{si} 6`, all keys at defaults |
| 8.2 | PASS | Saved `focus_minutes = 30` at 19:43:18. Within 5s: `config reloaded (focus=30m0s …)`, `Config` focus 30, and the countdown went 50:00 → 30:00 |
| 8.3 | PASS | About 70s elapsed of 30m (idle threshold raised by reload so time accrued while the user was away). Saved `focus_minutes = 1` at 19:44:46, and the next tick showed `break — 9m58s` |
| 8.4 | PASS | Saved `focus_minutes = 0`: one line, `config reload ignored: config: timer.focus_minutes must be positive, got 0`, across three ticks; `Config` unchanged. Saved `focus_minutes = 40`: applied |
| 8.5 | PASS | Deleted the file: `config reloaded (focus=50m0s …)`, and `Config` returned to all six defaults within 5s |
| 8.6 | PASS | `prompt_every_minutes = 1` applied by reload. On camera, held at 19:48:11, prompt 2 at 19:49:18, prompt 3 at 19:50:23, pill at 19:51:21: one-minute intervals within the 5s tick, no restart. The user saw prompt 3 and the pill |
| 8.7 | PASS | Test config removed, logger stopped by PID, session restarted on defaults (50m, prompt interval 5) |

## Requirement Compliance

### daemon-configuration

| Scenario | Status | Evidence |
|---|---|---|
| No config file | ✅ | `TestNoConfigFileYieldsDefaults`, `TestDefaultsCarryAllSixValues`; live 8.1 |
| Unknown key rejected | ✅ | `TestUnknownKeyRejected`, `TestUnknownCameraKeyRejected` |
| Zero rejected | ✅ | `TestZeroAndNegativeRejectedForEveryKey`, `TestZeroRejectionMessageFormat` |
| Camera policy configured | ✅ | `TestCameraKeysLoad`, `TestAConfiguredIntervalAndLimit`; live 8.6 |
| Saving a new focus length | ✅ | `TestNewFocusLengthKeepsElapsed`, `TestWatcherChangedContentReloads`, `TestAReloadRepublishesConfig`; live 8.2 |
| Shortening below the time worked | ✅ | `TestShorteningBelowElapsedEndsOnTheNextTick`; live 8.3 |
| A paused phase keeps its elapsed time | ✅ | `TestPausedRemainderIsRecomputedElapsedPreserved`, `TestPausedRemainderFloorsAtZero` |
| An invalid file is ignored | ✅ | `TestWatcherInvalidSaveErrorsOnce`; live 8.4 |
| Fixing the file applies it | ✅ | `TestWatcherFixAfterInvalidReloads`; live 8.4 |
| Deleting the file reverts to defaults | ✅ | `TestWatcherDeletionRevertsToDefaults`; live 8.5 |
| Saving without a change emits nothing | ✅ | `TestEqualConfigChangesNothing`, `TestAnEqualConfigEmitsNothing`, `TestWatcherUnchangedFileDoesNotReload` |

Also covered: rename-over saves (`TestWatcherRenameOverSaveReloads`), and a file created after
absence (`TestWatcherCreationAfterAbsenceReloads`).

### daemon-control

| Scenario | Status | Evidence |
|---|---|---|
| Existing four Control Surface scenarios | ✅ | Existing tests |
| Config is published from the first connection | ✅ | `TestConfigIsPublishedFromTheFirstConnection`; live 8.1 |
| A reload republishes Config | ✅ | `TestAReloadRepublishesConfig`; live 8.2 |

### session-timer (Tier Gating)

| Scenario | Status | Evidence |
|---|---|---|
| The 14 M6 scenarios | ✅ | Unchanged tests, now reading the policy from `Durations` |
| A configured interval and limit | ✅ | `TestAConfiguredIntervalAndLimit`, `TestHeldBreakAtALoweredLimitBecomesAPill`; live 8.6 |

## Warnings

- **W1 — A literal `0` now fails startup.** A config file containing `… = 0` used to start with the
  default silently, and now refuses with a diagnostic naming the key. That's the spec's existing
  rule finally enforced, and it is documented in `daemon/README.md`. It's called out here because it
  is a user-visible behavior change. No config file exists on this machine, so nothing is affected
  today.
- **W2 — The panel is easy to miss on a second screen.** During 8.6 the user missed prompts 1 and 2,
  each visible for 15s on the primary monitor while they watched the call on the other one. The
  daemon prompted correctly, and the extension showed prompt 3 and the pill. This is a `camera-prompt`
  UX observation outside M7's scope. It's a candidate for `overlay_monitor`-style placement or a
  configurable panel duration later.
- **W3 — Size.** 851 changed lines (555 insertions and 60 deletions in tracked files, plus 236 in
  `watch.go`/`watch_test.go`) against a ~550 forecast, 55% over, inside the declared 900. The tests
  are about 60% of it again.

## Next

`archive`.
