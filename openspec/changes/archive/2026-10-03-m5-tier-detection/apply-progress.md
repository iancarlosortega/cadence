# Apply progress: m5-tier-detection (Phases 1-7)

Status: complete for Phases 1-7. Phase 8 (live verification) is not done by design.

## Tasks
1.1-1.5, 2.1-2.5, 3.1-3.2, 4.1-4.10, 5.1, 6.1-6.2, 7.1-7.3 are marked `[x]` in tasks.md.

## Files changed (insertions/deletions; new files by line count)
- daemon/internal/session/machine.go (+48/-): T3-only gate, T3 ends a break, tier-change Notify
- daemon/internal/session/machine_test.go (+132)
- daemon/internal/session/ports.go, state.go: stale M5-pending comments updated
- daemon/internal/store/file.go (+6/-): Tier dropped from record, Restore yields T0
- daemon/internal/store/file_test.go (+19)
- daemon/internal/dbusapi/tier.go (new, 299 lines)
- daemon/internal/dbusapi/tier_test.go (new, 380 lines)
- daemon/internal/dbusapi/service_test.go (+73): settableTier, newTestServiceWithSources, TestTierChangeAloneEmitsOnce
- daemon/cmd/cadenced/main.go: NewTierDetector wired, fixedT0Tier deleted
- extension/render.js (+7), extension/test-render.js (+18)

## Verification
- `cd daemon && go vet ./...`: clean
- `cd daemon && go test -count=1 ./...`: all packages ok
- `go test -count=1 -v ./... | rg -c -- '--- SKIP'`: no output (0 skips)
- `gjs -m extension/test-render.js`: ends with `all checks passed`
- `gofmt -l daemon`: only the pre-existing `daemon/internal/config/config.go`
- `git diff --stat`: 12 files, 301 insertions, 28 deletions (includes 2 unrelated pre-existing `.atl` registry files); new files 679 lines. Total about 975 changed lines excluding `.atl`.

## Red/green and mutation evidence
- 1.1: TestT1AndT2StartBreak/T1 and /T2 failed ("phase = focus, want break") before the fix; green after. TestT3SkipsBreakSilently passed before and after (today's rule already skipped T3).
- 1.3: TestPresentingDuringBreakEndsIt failed ("phase = break, want focus"); green after 1.4.
- 2.1/2.2: TestTierOnlyChangeNotifies and ...InsideIdleWindow failed ("got []"); green after 2.4.
- 2.5 mutation: replacing the append condition with `false` failed both again; restored, green.
- 1.5, 2.3, 3.2, 5.1, 6.1: written after or alongside the behavior they pin; 6.1's T3 case was observed red before render.js changed.

## Deviations from design
1. `probe.sample()` returns a `session.Tier` (highest tier that probe sees) instead of `(present bool, error)`. Reason: the PipeWire probe must answer T1 (mic) and T2 (camera node) from one pw-dump run; a bool probe would spawn it twice per tick or need a shared snapshot cache. Precedence and errors-as-absent are unchanged.
2. Task 5.1 / spec "carrying `Tier`": the real signal also carries `RemainingSeconds`, because the 5s the tick advances legitimately changes it. The test asserts Tier is present, that SessionActive/Phase/Paused/Idle/PhaseEndsAt are not, and that exactly one signal fires and none follow. The spec wording "carrying only Tier" is slightly stronger than reality; consider amending it.
3. PipeWire fixtures are hand-written JSON in the test file (raw pw-dump captures are not saved), as instructed; the screencast at-rest XML is a real gdbus capture, the with-child XML adds `<node name="u2"/>`.
4. The screencast "unreachable bus name" test uses an injected introspect func returning an error rather than a real bus, so it needs no session bus and cannot skip.
5. Store `Load` sets `Tier = TierT0` explicitly (zero value of Tier is "").

## Open
- Phase 8 live verification.
- `daemon/internal/config/config.go` gofmt failure is pre-existing and untouched.
- `internal/dbusapi/idle_test.go` still has its pre-existing t.Skip on a missing session bus (not triggered here).

## Post-apply correction (orchestrator, 2026-10-03)

Independent verification (risk tier high: `tier.go` spawns `pw-dump`) found no blocker or major
defect, and no failure path that can produce T3. One spec deviation and one hardening item were
corrected:

- **Tier sampled only while active and unpaused.** `Service.Tick` sampled the detector every tick,
  so an idle daemon spawned `pw-dump` and walked `/proc` every 5s against `session-timer` "Tier
  Gating". `Tick` now checks `Active && !Paused` under the mutex and runs the probes outside it.
  Pinned by `TestTierIsSampledOnlyWhileActiveAndUnpaused`, observed red before the fix.
- **`cmd.WaitDelay = 500ms`** on the `pw-dump` command, so a descendant holding stdout cannot stall
  the tick past the 2s timeout.

Accepted as known, not fixed:

- `EventResume` republishes the pre-pause tier until the next tick, at most 5s. That's cosmetic:
  gating reads the tick's fresh tier.
- "Every signal unavailable" is covered piecewise, by `TestFailingProbeReadsAbsent`,
  `TestProbeUnavailabilityLogsOnTransitionsOnly` and `TestT1AndT2StartBreak`, and has no end-to-end
  test. "A signal returns" is correct by construction (stateless `conn.Object`); live check 8.2
  covers both.

Gates after correction: `go vet` clean; `go test -count=1 -race ./...` all ok; 0 skips; gjs suite
all checks passed; gofmt clean on touched files.
