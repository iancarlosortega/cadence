# Tasks: m5-tier-detection

## Review Workload Forecast

| Item | Value |
|---|---|
| Estimated changed lines | ~700 (logic ~280, tests ~420; tests weighted 1.5× after M2/M4 under-forecasts) |
| 400-line budget risk | High |
| Chained PRs recommended | No |
| Decision needed before apply | No — `delivery_strategy: exception-ok`, the user commits directly to `main` |

Daemon and extension halves are independent (design "Migration / Rollout"), but the change is small
enough to land as one commit.

## Phase 1 — Domain: T3 gating (session-timer "Tier Gating")

- [x] 1.1 In `machine_test.go`, add tests for "T1 starts break", "T2 starts break" and "T3 skips the break silently" at the focus deadline. Run and observe T1/T2 **fail** against today's non-T0 rule.
- [x] 1.2 In `transitionPhase`, narrow the no-break rule from `Tier != T0` to `Tier == T3`, and rewrite its comment to name the M5 rule and P3/P4. Run 1.1 green.
- [x] 1.3 Add a test for "Presenting during a break ends it": an active break, a tick with `T3`, giving focus with 0 elapsed and Persist + Notify. Observe red.
- [x] 1.4 In `applyTick`, after sampling the tier, end a break under T3 the way `EventSkipBreak` does, checked before the idle cases (design D5). Run green.
- [x] 1.5 Add a test that a share starting on the same tick as the focus deadline skips the break (design D5 ordering note).

## Phase 2 — Domain: publish a tier-only change (design D6)

- [x] 2.1 Add a test: an active focus, a tick whose only change is T0→T3 (before the deadline) emits exactly one `EffectNotify` and no `EffectPersist`. Observe red.
- [x] 2.2 Add the same test with an idle window already open (the `if s.Idle { return ns, nil }` path). Observe red.
- [x] 2.3 Add a test that a tick with an unchanged tier emits nothing.
- [x] 2.4 In `Apply`'s `EventTick` case, append `EffectNotify{Reason: "tier changed"}` when the tier changed and no Notify is present. Run 2.1–2.3 green.
- [x] 2.5 Mutation check: remove the append and confirm 2.1 and 2.2 fail, then restore.

## Phase 3 — Persistence: stop storing Tier (design D7)

- [x] 3.1 Remove `Tier` from the store record in `store/file.go` (both save and restore). Restore yields `TierT0`.
- [x] 3.2 Add a `file_test.go` case: a state file containing `"tier": "T3"` restores with `Tier == TierT0` and no error.

## Phase 4 — Adapters (session-timer "Tier Source Availability")

- [x] 4.1 `dbusapi/tier.go`: `TierDetector` implementing `session.TierSource` over a `probe` interface, with precedence T3 > T2 > T1 > T0, errors read as absent, and transition-only logging per probe (design D1).
- [x] 4.2 `tier_test.go`: precedence with fake probes. "The highest tier wins" scenario.
- [x] 4.3 `tier_test.go`: a failing probe reads absent and logs once across repeated ticks, then logs once more when it recovers. Capture via `log.SetOutput`.
- [x] 4.4 Screencast probe: Introspect `/org/gnome/Mutter/ScreenCast/Session` and report present when there's any child node (design D2). Split XML parsing into a pure function.
- [x] 4.5 Tests for 4.4: XML with no children reads absent, XML with `<node name="u2"/>` reads present, and an unreachable bus name reads as an error.
- [x] 4.6 Camera probe: walk `<root>/[0-9]*/fd/*`, `Readlink` each, match `^/dev/video[0-9]+$`, and skip unreadable pid directories (design D3).
- [x] 4.7 Tests for 4.6 on a temp-dir fixture: a symlink to `/dev/video0` reads present, symlinks to other targets read absent, an unreadable pid directory is skipped, and a missing root is an error.
- [x] 4.8 PipeWire probe: run `pw-dump` with a 2s timeout and decode the minimal fields. Report mic (`Stream/Input/Audio` running) and camera node (`Video/Source` running) from one snapshot (design D4). The camera counts as present from either D3 or D4.
- [x] 4.9 Tests for 4.8: decode trimmed fixtures from `~/tier-research/` captures (rest: neither; mic: mic only; a synthetic running `Video/Source`: camera). A missing binary reads as an error.
- [x] 4.10 Wire `NewTierDetector(conn, "/proc", "pw-dump")` in `cmd/cadenced/main.go` and delete `fixedT0Tier` (design D9). Update the `ports.go` and `state.go` comments that say M5 is pending.

## Phase 5 — Service

- [x] 5.1 `service_test.go`: a tick with only a tier change emits exactly one `PropertiesChanged` carrying only `Tier`, and the following ticks at the same tier emit none (daemon-control "A tier change alone emits once").

## Phase 6 — Extension (panel-indicator "Break Warning")

- [x] 6.1 `test-render.js`: no warning in focus at 60s with `tier: 'T3'`; the warning returns with `tier: 'T2'` and `tier: 'T1'`.
- [x] 6.2 `render.js` `computeDisplay`: add `state.tier !== 'T3'` to the warning condition, with a comment citing the spec. Run 6.1 green.

## Phase 7 — Verification gates

- [x] 7.1 `cd daemon && go vet ./... && go test -count=1 ./...`, with zero skips (`-v | rg -- '--- SKIP'` empty).
- [x] 7.2 `gjs -m extension/test-render.js` reports all checks pass.
- [x] 7.3 `gofmt -l` is clean on every touched Go file.

## Phase 8 — Live verification (install + extension reload)

- [x] 8.1 Install (`make -C packaging install`), restart `cadenced`, and log out and back in to reload the extension.
- [x] 8.2 At rest: `Tier = T0`. The journal logs no probe as unavailable on a healthy session.
- [x] 8.3 Join a call, mic only: `Tier = T1` within one tick (5s). `dbus-monitor` shows one signal carrying `Tier`.
- [x] 8.4 Camera on: `Tier = T2`. Let a short focus deadline pass (temporary config `focus_minutes = 1`): the break **starts** and the overlay shows (P4).
- [x] 8.5 Share the screen: `Tier = T3`, and the amber warning disappears if it was showing. At the focus deadline no break starts and a fresh focus block begins.
- [x] 8.6 Start a share while a break overlay is on screen: the overlay disappears within one tick and the phase is focus.
- [x] 8.7 Share a single window instead of the whole screen and confirm it also reads T3 (research U3).
- [x] 8.8 Leave the call: `Tier = T0` within one tick. Restore the normal config.
