```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:46d57cecfd9792f4c41b30449f68205aadd2073b71a23585916535601513144b
verdict: pass
blockers: 0
critical_findings: 0
requirements: 2/2
scenarios: 6/6
test_command: go test ./...
test_exit_code: 0
test_output_hash: sha256:6e1a1a448c429c6cb07e44f01740f2ced129eb817ab9c28e55bdeaabebe2e665
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verify Report: Handling absences the daemon did not witness

**Verdict: PASS**

21/21 tasks. Both requirements and all six scenarios of the `session-persistence` delta are
satisfied, and both fixes were observed working on a real logout/login cycle rather than inferred.

## Requirement coverage — `session-persistence`

| Requirement | Scenarios | Evidence | Result |
|---|---|---|---|
| Durable State (MODIFIED) | 2 | Short restart resumes position: 20s downtime left remaining at 180s. No-session case covered by `TestStartupGapSkippedWithoutASession` | PASS |
| Downtime Is Time Away (ADDED) | 4 | See below | PASS |

| Scenario | Evidence | Result |
|---|---|---|
| Long downtime credits a break | `TestLongDowntimeCreditsBreak`; live 70s stop against a 60s credit gave elapsed 0s | PASS |
| Short downtime does not credit | `TestShortDowntimeDoesNotCredit`; live 20s stop left remaining at 180s | PASS |
| Downtime and suspend agree | `TestDowntimeAndSuspendAgree` — contrasts the suspend path against the tick path and asserts they still differ, pinning why the startup indirection exists | PASS |
| Overnight shutdown does not open on a break | Live 12m04s logout against a 10m credit: resumed `focus`, `RemainingSeconds=180`, elapsed 0s | PASS |

## F3 — verified on a real logout

```
17:04:18  cadenced: shutting down
17:04:18  Stopped cadenced.service
17:04:18  Stopped target graphical-session.target
17:16:22  Reached target graphical-session.target
17:16:22  Started cadenced.service
```

| Check | Result |
|---|---|
| New panics across the cycle | **0** — count unchanged at 1, which is the pre-fix crash at 16:04:50 |
| `NRestarts` | **0** — the unit was stopped, not crashed and restarted |
| `PartOf` propagation | Directly observed: `cadenced` stops immediately **before** the target finishes stopping |
| Automatic start at login | Yes, with no intervention |

The daemon no longer outlives its bus, so the dead-connection state that produced F3 cannot arise.

## Automated evidence

- `go test ./...` — all five packages pass, including `cmd/cadenced`, which had no tests before.
- `go vet ./...` — clean.
- **Mutation, F3**: removing `recover` from `publish` fails
  `TestPublishOnClosedConnectionErrorsNotPanics` with "publish panicked on a closed connection
  instead of returning an error".
- **Mutation, F4**: making `startupGap` always skip fails `TestStartupGapReplaysTheAbsence`.
- `gjs -m extension/test-render.js` — 17/17; the extension is untouched and unaffected.

## Success criteria from the proposal

| # | Criterion | Result |
|---|---|---|
| 1 | `go test ./...` passes including new startup-gap coverage | PASS |
| 2 | A real logout no longer panics; the unit stops rather than restarting | PASS |
| 3 | A real login starts the daemon again; panel live without intervention | PASS |
| 4 | Downtime past idle-credit yields a fresh focus, not a break | PASS — 12m04s live |
| 5 | A brief stop does not jump elapsed by the downtime | PASS — 20s live |
| 6 | Extension unchanged and still passing | PASS |

## Deviations recorded during apply

1. **Design Decision 1 was wrong and was corrected.** `prop.Properties.Set` enforces the `Writable`
   flag, false for all six properties, so it broke `New()` with `ErrReadOnly`. The non-panicking
   `p.set` is unexported. `publish()` keeps `SetMust` and converts the panic with a narrow `recover`
   at the boundary — the option the design had rejected before the library surface was read. The
   design was amended.
2. **Task 5.2's mutation check was invalid as written.** Removing the startup `ApplySuspend` broke no
   test, because the domain tests call `Apply` directly and never touch `main.go`'s wiring. Fixed by
   extracting `startupGap()` with its own tests. The check found a hole in its own plan.
3. **A tautological test was rewritten.** `TestDowntimeAndSuspendAgree` originally compared `Apply`
   against the same event on both sides and could never fail.

## Warnings

1. **F4 under-charges by up to 60s after a clean restart.** `LastObserved` is persisted on a 60s
   heartbeat, so a portion of genuinely worked time can be treated as absence. Bounded, intentional,
   and within the tolerance `Durable State` already allows. The behaviour it replaces over-charged
   without limit.
2. **The live checks ran against a 3-minute test config** for focus and break, with idle-credit at
   its 10-minute default for the logout test and lowered to 60s for the earlier bounded test. The
   rules are duration-independent but the 50-minute defaults were not themselves observed.
3. **`recover` in `publish` will convert any future panic there into an error**, not only a closed
   connection. That is the intended contract at this boundary, but it means a genuinely unexpected
   panic surfaces as a logged tick error rather than a crash.

## Findings

F3 and F4 are both resolved. No new findings. F2 (idle-pause republish) remains owned by M4.

## Ready for Archive

Yes. No blockers. Commit and push remain separate human decisions.
