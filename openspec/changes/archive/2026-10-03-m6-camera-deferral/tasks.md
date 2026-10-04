# Tasks: m6-camera-deferral

## Review Workload Forecast

| Item | Value |
|---|---|
| Estimated changed lines | ~1000 (domain ~120 + tests ~250; D-Bus/store ~60 + tests ~120; `prompt.js` ~200; render/extension ~80 + tests ~120; CSS ~40) |
| 400-line budget risk | High |
| Chained PRs recommended | No |
| Decision needed before apply | No — `delivery_strategy: exception-ok`; the user commits directly to `main` |

Daemon and extension land together (design "Migration / Rollout").

## Phase 1 — Domain state and helpers

- [x] 1.1 `state.go`: add `HoldStage` (`none`/`prompt`/`pill`) and the `Hold`, `Prompts` and `SincePrompt` fields. Add the `PromptRetry = 5m` and `PromptCap = 3` constants (design D1, D2).
- [x] 1.2 `machine.go`: add the `hold(ns)` and `release(ns)` helpers (design D3, D5), each with a comment citing the spec.

## Phase 2 — Domain: entering, retrying, lifting (session-timer "Tier Gating")

- [x] 2.1 Tests: "T2 holds the break", "A held break does not advance", "Prompts repeat every 5 minutes up to the cap", "Past the cap the hold becomes a pill". Observe red.
- [x] 2.2 `transitionPhase`: focus → break under T2 enters `hold`. Add `applyTick` case 5, the retry clock (design D4). Run 2.1 green.
- [x] 2.3 Tests: "The camera turning off starts the break" (from the prompt and the pill stages), "The camera turning on mid-break holds it", "A flapping camera cannot escape the cap". Observe red.
- [x] 2.4 `applyTick` cases 3 (lift) and 4 (re-hold) in the order of design D4. Run 2.3 green.
- [x] 2.5 Test: quiet retry ticks inside a hold emit no effects. A tick that crosses the retry interval emits exactly Persist + Notify.

## Phase 3 — Domain: release on every end path (design D5)

- [x] 3.1 Tests asserting `Hold == none` and `Prompts == 0` after each of: skip, T3 during a hold, idle credit during a hold, suspend credit during a hold, stop, and normal break completion after a lift. Observe red where release is missing.
- [x] 3.2 Call `release` from `creditBreak`, `EventSkipBreak`, the T3 case and `transitionPhase` break → focus. Run 3.1 green.
- [x] 3.3 Mutation check: remove `release` from `creditBreak`, confirm the idle-credit and suspend-credit tests fail, then restore.
- [x] 3.4 Test: pause during a hold freezes the retry clock, and resume keeps the stage, the count and the frozen remainder (design D6).

## Phase 4 — Persistence (session-persistence "Durable State")

- [x] 4.1 `store/file.go`: add the `hold`, `prompts` and `since_prompt_ns` record fields. `Load` normalises an empty `hold` to `none` (design D8).
- [x] 4.2 `file_test.go`: a held break round-trips; an old file without the fields loads as `none`/0.

## Phase 5 — D-Bus (daemon-control "Control Surface", "Hold Publication")

- [x] 5.1 `service.go`: add the `Hold` (s) and `Prompts` (i) properties to the seed and the `desired` map. Freeze `PhaseEndsAt`/`RemainingSeconds` while `Hold != none` (design D7).
- [x] 5.2 `service_test.go`: entering a hold emits one signal carrying `Phase`, `Hold`, `Prompts` = 1 and `PhaseEndsAt` = 0. A retry emits `Prompts` = 2 with no signal between. A lift emits `Hold` = `none` with a live `PhaseEndsAt`. `Hold` and `Prompts` are readable on first connect.

## Phase 6 — Extension predicates (break-overlay, panel-indicator, camera-prompt)

- [x] 6.1 `test-render.js`: frozen countdown while held; `shouldShowOverlay` false while held, true after a lift with the break running; `promptSurface` returns `panel` before 15s, `null` at or after 15s, `pill` for the pill, and `null` when no session is active or the daemon is unavailable. Observe red.
- [x] 6.2 `render.js`: update `computeDisplay` and `shouldShowOverlay`, and add `promptSurface` and `PANEL_SECONDS = 15` (design D9). Run 6.1 green.

## Phase 7 — Extension surfaces

- [x] 7.1 `extension.js`: add `Hold` and `Prompts` to `IFACE_XML` and `_refresh` (defaults `'none'`, `0`). Track `_lastPrompts` and set `_panelUntil` on an increase while `hold === 'prompt'`.
- [x] 7.2 New `extension/prompt.js` `PromptController` per design D9: panel (message + Skip `St.Button` calling `SkipBreak`) and pill, `addTopChrome`, top-right below the top bar, destroy rather than hide, explicit `x_align` on every child.
- [x] 7.3 Wire it into `enable()` and `_render()`, reposition on `monitors-changed`, and destroy in `disable()` before the client.
- [x] 7.4 `stylesheet.css`: `.cadence-prompt`, `.cadence-prompt-button`, `.cadence-pill`, matching the overlay's palette.
- [x] 7.5 `packaging/Makefile`: install `prompt.js` alongside the other extension files.

## Phase 8 — Verification gates

- [x] 8.1 `cd daemon && go vet ./... && go test -count=1 -race ./...`, with zero skips.
- [x] 8.2 `gjs -m extension/test-render.js` reports all checks pass.
- [x] 8.3 `gofmt -l` is clean on touched files. Fold in the pre-existing cleanups: `gofmt -w daemon/internal/config/config.go`, and `go mod tidy` to make `godbus` a direct dependency.

## Phase 9 — Live verification (test build with `PromptRetry = 30s`, then reverted)

- [x] 9.1 Build and install with `PromptRetry = 30s` and a temporary config `focus_minutes = 1`, `break_minutes = 3`. Restart `cadenced`, then log out and back in.
- [x] 9.2 Camera on at the focus deadline: the corner panel appears top-right, with no overlay, and the panel countdown is frozen. The call window stays clickable around the panel.
- [x] 9.3 The panel leaves after about 15s and comes back at each retry, for 3 prompts in total. The next retry shows the pill, which stays.
- [x] 9.4 Camera off while the pill is shown: the pill goes, the overlay appears within one tick, and the break counts down.
- [x] 9.5 Camera on mid-break: the overlay is replaced by the panel within one tick.
- [x] 9.6 Skip on the panel: the panel goes and a fresh focus starts.
- [x] 9.7 Restart `cadenced` while held: still held with the same prompt count.
- [x] 9.8 Screenshot the panel and the pill for visual review (alignment, padding, no clipping), then restore `PromptRetry = 5m`, remove the test config and reinstall.
