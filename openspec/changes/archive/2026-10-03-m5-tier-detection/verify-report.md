```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:e146c3aae0f870f55c3b558a25114e08cb967da753d07a2dcc8c406ca95efcd3
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 4/4
scenarios: 19/19
test_command: go test ./...
test_exit_code: 0
test_output_hash: sha256:bad1fc5062a82353971b6b18bbdc3de2d01ed4bfda8f7218edacb1ea8f5724c9
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verify Report — m5-tier-detection

**Date**: 2026-10-03
**Verdict**: **PASS WITH WARNINGS** — 0 blockers, 3 warnings
**Tasks**: 36/36 checked, with all 8 live checks observed and none waived
**Requirements**: 4/4 (3 modified, 1 added) · **Scenarios**: 19/19

## Test Runs

| Command | Observed result |
|---|---|
| `cd daemon && go vet ./...` | clean |
| `cd daemon && go test -count=1 ./...` | all 6 packages `ok` |
| `cd daemon && go test -count=1 -race ./...` | all `ok` (run after the post-apply correction) |
| `cd daemon && go test -count=1 -v ./... \| rg -c -- '--- SKIP'` | 0 |
| `cd daemon && go build ./...` | clean |
| `gjs -m extension/test-render.js` | `all checks passed` |
| `gofmt -l daemon` | only the pre-existing `internal/config/config.go` |

## Independent Verification

`gentle-ai review assess` rated the change **high** (`process_boundary`: `tier.go` spawns
`pw-dump`). An independent read-only verifier then reviewed the diff against the specs and design
and found no blocker or major defect. In particular, **no failure path produces T3**: T3 comes only
from `screencastActive`, which needs a successfully parsed Introspect document with at least one
child node. Every error, timeout, empty or malformed document, and missing bus name reads T0.

Of its five minor findings, two were corrected before this report (`apply-progress.md`,
"Post-apply correction"):

- **Tier sampling outside a session.** `Service.Tick` sampled the tier every tick, against "Tier
  Gating". It now samples only while active and unpaused, pinned by
  `TestTierIsSampledOnlyWhileActiveAndUnpaused` (observed red first).
- **`pw-dump` hardening.** The command now sets `WaitDelay`.

The remaining three are carried as warnings W1–W3.

## Live Verification (Phase 8)

Real Brave call, temporary config of 1-minute focus and 3-minute break. A logger sampled the
daemon's properties every 2s (`live-verification.log`).

| Task | Result | Evidence |
|---|---|---|
| 8.1 | PASS | `make -C packaging install`, `cadenced` restarted, logout and login, extension `ACTIVE` |
| 8.2 | PASS | At rest `T0`; no tier or unavailable lines in the journal for the whole session |
| 8.3 | PASS | Mic only → `T1` (17:35). The break also started under T1 (17:35:11) |
| 8.4 | PASS | Camera → `T2` at 17:35:42; the break **started** under T2 at 17:36:16 (P4) |
| 8.5 | PASS | Full-screen share → `T3` at 17:37:00. At the deadline the phase stayed `focus` and restarted at 59s (17:37:26). The user saw no amber warning |
| 8.6 | PASS | Break at 17:38:31; a share started from the second monitor → `T3` and `focus` at 60s on the same tick (17:38:45); the overlay left |
| 8.7 | PASS | Single-window share → `T3` (Mutter `Session/u7`). Research U3 answered: window and screen shares look the same |
| 8.8 | PASS | Left the call → `T0` at 17:40:15, within one tick |

The test config was removed afterwards, and the session restarted on the 50/10 defaults.

## Requirement Compliance

### session-timer

| Requirement / Scenario | Status | Evidence |
|---|---|---|
| **Tier Gating** (modified) | ✅ | |
| T0 starts break | ✅ | Existing `TestT0StartsBreak` |
| T1 starts break | ✅ | `TestT1AndT2StartBreak`; live 8.3 |
| T2 starts break | ✅ | `TestT1AndT2StartBreak`; live 8.4 |
| T3 skips the break silently | ✅ | `TestT3SkipsBreakSilently`, `TestShareStartingAtTheDeadlineSkipsTheBreak`; live 8.5 |
| Presenting during a break ends it | ✅ | `TestPresentingDuringBreakEndsIt`; live 8.6 |
| The highest tier wins | ✅ | `TestHighestTierWins`, `TestPipewireProbeMapsSignalsToTiers` |
| Sampled only while active and unpaused | ✅ | `TestTierIsSampledOnlyWhileActiveAndUnpaused` |
| **Tier Source Availability** (added) | ✅ ⚠️ | |
| Every signal unavailable | ✅ ⚠️ | Covered piecewise by `TestFailingProbeReadsAbsent`, `TestProbeUnavailabilityLogsOnTransitionsOnly` and `TestT1AndT2StartBreak` (W2) |
| A signal returns | ✅ ⚠️ | `TestProbeUnavailabilityLogsOnTransitionsOnly` (recovery log). The bus side is correct by construction: `conn.Object` addresses the well-known name on every call (W2) |

### daemon-control

| Requirement / Scenario | Status | Evidence |
|---|---|---|
| **Change Notification** (modified) | ✅ | |
| No per-second traffic | ✅ | `TestNoPropertiesChangedOnQuietTick`, `TestUnchangedTierEmitsNothing` |
| Short suspend republishes the deadline | ✅ | `TestShortSuspendRepublishesDeadline` (unchanged) |
| An idle window emits exactly twice | ✅ | `TestIdleEdgesEachEmitExactlyOneSignal` (unchanged) |
| A client never drifts | ✅ | Unchanged tests; tier changes do not touch the deadline |
| A tier change alone emits once | ✅ | `TestTierOnlyChangeNotifies`, `TestTierOnlyChangeNotifiesInsideIdleWindow`, D-Bus-level `TestTierChangeAloneEmitsOnce` |

### panel-indicator

| Requirement / Scenario | Status | Evidence |
|---|---|---|
| **Break Warning** (modified) | ✅ | |
| Crossing the threshold, Cleared on the break transition, Not applied while paused, Not applied while idle | ✅ | Existing `test-render.js` checks |
| Not applied while presenting | ✅ | `no warning in focus at 60s under T3`; live 8.5 |
| Removed when presenting starts | ✅ | `computeDisplay` is recomputed on every `g-properties-changed` from `p.Tier` (`extension.js:130`); the T3 check covers it; live 8.5 |

## Warnings

- **W1 — Stale tier for at most one tick after resume.** `EventResume` republishes the tier from
  before the pause, because the tier isn't sampled while paused. That's cosmetic: the next tick
  (≤5s) corrects it, and break gating reads the tick's fresh tier.
- **W2 — Two availability scenarios have no end-to-end test.** "Every signal unavailable" and "A
  signal returns" are covered by component tests, construction and live 8.2, but no single test
  drives a session to a break with every probe failing.
- **W3 — Size.** About 1067 changed lines (356 insertions and 27 deletions in tracked files, plus
  684 in the new `tier.go` and `tier_test.go`) against a ~700 forecast, 52% over. `size:exception`
  was accepted at preflight, and the ledger objective was reset by the owner.

Also noted, not warnings:

- Transitions land up to about 5s after the deadline. That's the existing tick granularity, seen in
  the live log as `left=-4s`.
- A screencast session that exists but hasn't started yet, such as an open portal picker, also
  reads T3. That's consistent with P5 ("any screencast").

## Next

`archive`.
