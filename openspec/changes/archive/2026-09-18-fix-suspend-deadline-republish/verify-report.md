```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:2d10dfad10161b3f9757b4e3d90f4778fcd1f82714a9b0228df3329e49a5b6ad
verdict: pass
blockers: 0
critical_findings: 0
requirements: 1/1
scenarios: 3/3
test_command: go test ./...
test_exit_code: 0
test_output_hash: sha256:b5882ab72d14151df018548625a8bbccfd4a68b5a4d9b2bded3f90b5f0eb59ad
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verify Report: Republish the deadline after a short suspend

**Verdict: PASS**

10/10 tasks complete. Every requirement in the delta spec is satisfied by evidence gathered against
the running daemon, not by inspection alone. No blockers, no critical findings, one warning.

## Requirement coverage — `daemon-control`, Change Notification

| Scenario | Evidence | Result |
|----------|----------|--------|
| No per-second traffic | 60s of `dbus-monitor` on `/dev/ian/Cadence` during an active unpaused `focus` with no transition: **0** `PropertiesChanged`. Plus `TestNoEffectOnQuietTick` passing | PASS |
| Short suspend republishes the deadline | `TestShortSuspendDoesNotCredit` and `TestShortSuspendRepublishesDeadline` pass; both fail when the fix is removed | PASS |
| A client never drifts across suspends and transitions | Real 9s suspend, full-precision offset `+0.319s` constant across three samples; sub-second and attributable to whole-second publish quantization | PASS |

The requirement's general clause — *any* path that declines to charge elapsed time MUST republish —
is satisfied because the only other such path, the idle-pause branch, is unreachable (`IdleSource`
stubbed to zero). Recorded as finding F2, owned by M4.

## Design conformance

| Decision | Check | Result |
|----------|-------|--------|
| D1 — emit from the domain, not the adapter | `git diff --name-only` shows `machine.go` and `machine_test.go` only; `internal/dbusapi/` has 0 changed files | PASS |
| D2 — `EffectNotify` only, `EffectPersist` unchanged | Diff shows the persist effect byte-identical, notify appended | PASS |
| D3 — assert on the effect, not `len(effects)` | Tests use a `hasNotify(effects)` type check; no length assertion added | PASS |
| D4 — idle-pause path untouched | Branch verified unchanged in source, including its (now known-false) comment | PASS |

## Scope conformance

- Files changed: exactly the two the design named. `internal/dbusapi/`, `internal/store/` and
  `extension/` show 0 changed files.
- D-Bus surface after the change: 5 methods and 6 properties, identical to before. No property added,
  none removed, no signature altered.
- Persisted JSON shape unchanged.

## Test evidence

- `go test ./...` in `daemon/` — all packages pass.
- `go vet ./...` — clean.
- Mutation check: with `EffectNotify` removed, `TestShortSuspendDoesNotCredit` and
  `TestShortSuspendRepublishesDeadline` both fail; restored, both pass. The regression tests have
  been observed failing, which is what distinguishes them from the assertion that let this defect
  ship in the first place.

## Success criteria from the proposal

| # | Criterion | Result |
|---|-----------|--------|
| 1 | `go test ./...` passes including the new tests | PASS |
| 2 | `TestShortSuspendDoesNotCredit` fails if the fix is removed | PASS — observed |
| 3 | Live: offset zero and stays zero after a real suspend | PASS — `+0.319s` constant, sub-second publish quantization rather than drift |
| 4 | M2 task 6.3 no longer blocked by this defect | PASS — M2 finding F1 resolved; 6.3 now gated only on a Wayland session restart |

## Warnings

1. **`PhaseEndsAt` has whole-second resolution.** `now.Add(remaining).Unix()` truncates the
   fractional second, so a client is permanently up to 1s below the daemon's exact remaining. This is
   contract resolution, not drift — it does not accumulate and it resets at every publish. It is
   harmless for a minutes-scale break reminder and is recorded rather than fixed. Success criterion 3
   says "offset zero", which at full precision is unachievable by design; read it as "no whole-second
   offset that persists".

## Findings carried forward

- **F2 (deferred, owner M4)** — the idle-pause branch shares the defect class, is unreachable today,
  and cannot be fixed correctly without deciding what a client displays during an idle window. Its
  comment asserting "nothing observable changed" is false and should die with the fix. `IdleSince`
  (`state.go:57`) remains declared-but-never-used.

## Observed, deliberately not fixed

`daemon/internal/session/state.go` fails `gofmt -l` on a pre-existing comment-alignment nit. Unrelated
to this change and not bundled into a bugfix diff.

## Ready for Archive

Yes. No blockers. Commit and push remain separate human decisions.
