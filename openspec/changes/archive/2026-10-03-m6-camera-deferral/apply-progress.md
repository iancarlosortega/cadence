# Apply progress: m6-camera-deferral

Status: Phases 1-8 complete. Phase 9 (live verification) not started, by instruction.

## Tasks done

1.1-1.2, 2.1-2.5, 3.1-3.4, 4.1-4.2, 5.1-5.2, 6.1-6.2, 7.1-7.5, 8.1-8.3 are all `[x]` in `tasks.md`.

## Files (diff lines, insertions + deletions)

| File | Lines |
|---|---|
| daemon/internal/session/state.go | 47 |
| daemon/internal/session/machine.go | 100 |
| daemon/internal/session/machine_test.go | 315 (T2 case in `TestT1AndT2StartBreak` rewritten as `TestT1StartsBreak`) |
| daemon/internal/store/file.go | 18 |
| daemon/internal/store/file_test.go | 46 |
| daemon/internal/dbusapi/service.go | 23 |
| daemon/internal/dbusapi/service_test.go | 144 |
| daemon/internal/config/config.go | 4 (gofmt only) |
| daemon/go.mod | 8 (`go mod tidy` only; go.sum unchanged) |
| extension/render.js | 48 |
| extension/test-render.js | 95 |
| extension/extension.js | 46 |
| extension/stylesheet.css | 38 |
| extension/prompt.js | 155 (new) |
| packaging/Makefile | 1 |

## Verification

- `cd daemon && go vet ./...`: clean.
- `cd daemon && go test -count=1 -race ./...`: ok for cmd/cadence, cmd/cadenced, config, dbusapi, session, store.
- `go test -count=1 -v ./... | rg -c -- '--- SKIP'`: no output (zero skips).
- `gjs -m extension/test-render.js`: ends `all checks passed`.
- `gofmt -l daemon`: empty.
- `node --check` on copies of prompt.js, extension.js, render.js, overlay.js renamed to .mjs: syntax ok (it parses ESM without resolving `gi://` imports). prompt.js imports only `gi://Clutter`, `gi://St` and `resource:///org/gnome/shell/ui/main.js`, the same specifiers overlay.js uses; extension.js imports `./prompt.js`, which exists.

## Red / green / mutation evidence

- Session domain (2.1, 2.3, 3.1, 3.4): tests were written after the first draft of the domain code, so red was obtained by restoring `machine.go` from HEAD (keeping the new `state.go` types so it compiled) and running the suite: 10 new tests failed (`TestT2HoldsTheBreak`, `TestAHeldBreakDoesNotAdvance`, `TestPromptsRepeatEveryFiveMinutesUpToTheCap`, `TestPastTheCapTheHoldBecomesAPill`, `TestTheCameraTurningOffStartsTheBreak`, `TestTheCameraTurningOnMidBreakHoldsIt`, `TestAFlappingCameraCannotEscapeTheCap`, `TestQuietRetryTicksEmitNothingAndTheEdgeEmitsOnce`, `TestEveryEndPathReleasesAHeldBreak`, `TestPauseDuringAHoldFreezesTheRetryClock`). The new `machine.go` was restored and the package went green.
- Mutation (3.3): removing `release` from `creditBreak` failed `TestEveryEndPathReleasesAHeldBreak/idle_credit` and `/suspend_credit`, and nothing else. Restored, green.
- Extension (6.1): before `render.js` changed, `test-render.js` failed to load (missing export `PANEL_SECONDS`), which is red by construction. Green after 6.2.
- Store and D-Bus tests (4.2, 5.2) were written after their implementations and passed on first run; no red was observed for them.

## Deviations

1. Domain red was obtained by reverting the implementation, not by writing tests first (see above).
2. `promptSurface` returns `null` for a panel while `paused` (the pill is unaffected). The render tick stops while paused, so a panel shown before a pause would otherwise never reach its 15 seconds. Not in design D9; covered by a test.
3. `isHeld(state)` in render.js is `hold === 'prompt' || hold === 'pill'` instead of `hold !== 'none'`, so a snapshot without the field (older daemon) reads as not held.
4. `State.Held()` helper added in `state.go`, and `holdString` in `service.go` normalises the zero `HoldStage` to `none`.
5. The pill is added with `{affectsInputRegion: false}` so it never absorbs a click; the panel uses the default. Not in the design; to confirm in Phase 9.
6. Placement reads `get_preferred_width(-1)` after `addTopChrome` (needs a stage for the theme node) instead of a fixed CSS width.
7. `TestT1AndT2StartBreak` was replaced by `TestT1StartsBreak`, since T2 no longer starts the break.
8. `Prompts` is published as int32 and the extension XML declares `i`, per design D7.

## Open

- Phase 9 live checks, including whether the `affectsInputRegion: false` pill and the `get_preferred_width` placement behave in the real Shell.
- `.atl/` modifications in the working tree predate this work and were not touched.

## Post-apply correction (orchestrator, 2026-10-03)

The independent verification (risk tier high) found no blocker and one major defect, now corrected.

- **Fullscreen suppression latched at hold entry, not at break start (major).** `_suppressed` was
  decided on the first render with `phase === 'break'`, and for a held break that's when the hold
  begins. With a fullscreen call (the usual case on camera), the later running break stayed
  suppressed after the call ended, so no overlay showed for a running break. The decision now lives
  in a pure `nextSuppression` in `render.js`, keyed on the break starting to *run* (not held). Five
  `test-render.js` checks cover it, observed red (missing export) then green.
- **Unknown `hold` in a hand-edited state file (minor).** `Load` now maps anything other than
  `prompt`/`pill` to `none`, clearing the count and the retry clock, so the published `Hold` always
  agrees with `State.Held()`. Pinned by `TestUnknownHoldLoadsAsNotHeld`.

Accepted as known, not fixed:

- The panel re-arms for 15s on first connect, extension enable or a daemon restart while already
  held, because `_lastPrompts` starts at 0. That's harmless: one extra short reminder.
- No dedicated tests for a short suspend while held, or for startup downtime replay of a held
  state. Correct by reading (`applySuspend` moves `LastObserved` and leaves `SincePrompt`; downtime
  runs through `creditBreak`, which releases).
- The `IdleCredited` latch isn't cleared while held. That needs a rare sequence and has no
  user-visible effect.
