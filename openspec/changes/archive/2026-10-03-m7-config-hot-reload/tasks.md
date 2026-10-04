# Tasks: m7-config-hot-reload

## Review Workload Forecast

| Item | Value |
|---|---|
| Estimated changed lines | ~550 (config ~120 + tests ~160; domain ~40 + tests ~90; service/main ~50 + tests ~60; README ~30) |
| 400-line budget risk | High |
| Chained PRs recommended | No |
| Decision needed before apply | No — `delivery_strategy: exception-ok`; the user commits directly to `main` |

Daemon-only; no extension change.

## Phase 1 — Validation and the new keys (daemon-configuration "Defaults And Validation")

- [x] 1.1 `config_test.go`: tests for `focus_minutes = 0` rejected (naming `timer.focus_minutes`), each of the six keys at `0` and negative rejected, absent keys taking defaults, the new `[camera]` keys loaded, and an unknown key still rejected. Observe the zero cases **fail**.
- [x] 1.2 `config.go`: add the `[camera]` schema, a `Defaults()` with all six values, and `meta.IsDefined`-based validation (design D2). Run 1.1 green.

## Phase 2 — Policy into the domain (design D1)

- [x] 2.1 `session/state.go`: add the `PromptRetry` and `PromptCap` fields to `Durations` and delete the constants. Comment the type as the configured policy.
- [x] 2.2 `machine.go`: read the policy from `s.Durations`. Update the test fixtures so held-break tests get the default prompt policy from one helper.
- [x] 2.3 Test "A configured interval and limit": interval 2m, limit 2 → two prompts, then the pill at 4m.

## Phase 3 — `EventConfigChanged` (daemon-configuration "Live Reload", design D5)

- [x] 3.1 Tests: equal config → no effects; running focus keeps elapsed and the new length shows in `Remaining()`; shortened below elapsed → the next tick transitions to break; paused remainder recomputed with elapsed preserved; held break at a lowered limit → pill at the next interval; inactive session takes the new value. Observe red.
- [x] 3.2 `event.go` + `machine.go`: implement the event per design D5. Run 3.1 green.

## Phase 4 — Watcher (design D3)

- [x] 4.1 `config/watch_test.go`: unchanged file → no reload; changed content (use `os.Chtimes` to force a distinct mtime) → reload with the new values; invalid save → error once, then nothing on repeated polls; a fix → reload; deletion → defaults; rename-over save → reload.
- [x] 4.2 `config/watch.go`: `Watcher`, `NewWatcher`, `Poll`, recording the identity on every outcome. Run 4.1 green.

## Phase 5 — Service and wiring (daemon-control "Configuration Publication", design D4, D6)

- [x] 5.1 `service_test.go`: `Config` readable on first connect with six keys and default values; `ApplyConfig` with a changed focus emits one signal carrying `Config` and `PhaseEndsAt`; an equal config emits nothing. Observe red, including the map-comparison failure.
- [x] 5.2 `service.go`: add the `Config` property (seed + `desired`), switch the diff to `reflect.DeepEqual`, and add `ApplyConfig`. Run 5.1 green.
- [x] 5.3 `cmd/cadenced/main.go`: build the watcher after the startup load and poll it in the ticker case before `Tick`, logging ignored reloads and successful ones.

## Phase 6 — Docs (design D9)

- [x] 6.1 `daemon/README.md`: document the `[camera]` section, live reload (within 5s; invalid saves ignored and logged; deleting reverts to defaults) and the stricter `0` rule.

## Phase 7 — Verification gates

- [x] 7.1 `cd daemon && go vet ./... && go test -count=1 -race ./...`, with zero skips.
- [x] 7.2 `gofmt -l daemon` is empty.

## Phase 8 — Live verification (no logout needed: daemon only)

- [x] 8.1 Install and restart `cadenced`, then start a session. `busctl --user get-property dev.ian.Cadence /dev/ian/Cadence dev.ian.Cadence1 Config` shows six keys at their defaults.
- [x] 8.2 With focus running, save `focus_minutes = 30`: within 5s `PhaseEndsAt` moves, the elapsed time is unchanged, and the journal logs the reload.
- [x] 8.3 Save `focus_minutes = 1` with more than a minute elapsed: the break starts on the next tick.
- [x] 8.4 Save `focus_minutes = 0`: the config is unchanged, one journal line names `timer.focus_minutes`, and repeated ticks log nothing more. Fix the file and the fix applies.
- [x] 8.5 Delete the file: `Config` returns to defaults within 5s.
- [x] 8.6 On camera with `prompt_every_minutes = 1`: a held break re-prompts after about one minute, with no restart.
- [x] 8.7 Leave no config file behind (defaults), and restart the session on defaults.
