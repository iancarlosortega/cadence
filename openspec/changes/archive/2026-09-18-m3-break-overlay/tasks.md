# Tasks: M3 — Break overlay with hold-to-skip

## Review Workload Forecast

Estimated changed lines: **~330**.

| Artifact | Estimate |
|----------|----------|
| `extension/overlay.js` | ~150 (actor construction, hold-to-skip, lifecycle) |
| `extension/extension.js` | ~45 (wiring, suppression edge, monitors-changed, teardown) |
| `extension/stylesheet.css` | ~45 (overlay, message, progress, both themes) |
| `extension/render.js` | ~20 (predicate and constant) |
| `extension/test-render.js` | ~55 (predicate cases) |
| `packaging/Makefile` | ~1 |
| runbook | ~50 (markdown, not code) |

Delivery strategy: `single-pr`.

Decision needed before apply: **No** — the estimate sits inside the 400-line budget.
Chained PRs recommended: No
Chain strategy: null
400-line budget risk: **Medium**

M2 forecast 340-390 and came in at 522, a 35% miss on a comparable GJS milestone. This estimate is
deliberately generous and still leaves only ~70 lines of headroom. **If the implementation crosses
400, the Review Workload Guard must stop before apply and require a recorded `size:exception`** —
`single-pr` does not permit a silent split. Task 5.1, the optional pure-logic cases beyond the spec
scenarios, is the designated drop-first item.

### Suggested Work Units

One. The overlay, its predicate and its teardown are a single reviewable unit; splitting the actor
from the predicate that drives it would make both halves unreviewable on their own.

## Phase 1: Predicate

Pure logic first, because it is the only part testable without a Shell — and it carries three of the
four Self-Owned Exit paths.

- [x] 1.1 `extension/render.js` — `HOLD_TO_SKIP_SECONDS = 3` as a named constant
- [x] 1.2 `extension/render.js` — `shouldShowOverlay(state, nowSeconds, suppressed)`: true only when available, session active, phase is `break`, derived remaining above zero, and not suppressed (design Decision 1)
- [x] 1.3 Confirm the predicate reads nothing from the Shell — no `Main`, no `St`, no `global` — so `render.js` still imports nothing (design Decision 1)

## Phase 2: Overlay actor

- [x] 2.1 `extension/overlay.js` — `CadenceOverlay` class with `show(monitor)`, `hide()`, `destroy()`; holds one actor reference and one hold state
- [x] 2.2 `show()` — full-screen `St.Widget` sized to the monitor geometry, added with `Main.layoutManager.addTopChrome()`, chrome params left at their defaults (design Decision 3)
- [x] 2.3 `show()` content — a message, the remaining time, and a hold-to-skip control with a progress element
- [x] 2.4 `hide()` — destroy the actor rather than hiding it, so chrome tracking and the input region go with it (design Decision 4); safe to call when nothing is shown
- [x] 2.5 Fade in with `ease()`, skipping the animation when `St.Settings.get().enableAnimations` is false

## Phase 3: Hold to skip

- [x] 3.1 `button-press-event` starts a `GLib.timeout_add` for `HOLD_TO_SKIP_SECONDS` and eases the progress element to full over the same duration
- [x] 3.2 `button-release-event` and `leave-event` both cancel: `GLib.Source.remove`, `remove_all_transitions()`, reset the progress element (spec: *Released early*, *Pointer leaves the control*)
- [x] 3.3 On completion, invoke the supplied callback and **do not dismiss the overlay** — dismissal is the daemon's property change arriving (spec: Hold To Skip; design Decision 5)
- [x] 3.4 Cancelling is idempotent and safe when no hold is in progress

## Phase 4: Wiring

- [x] 4.1 `extension.js` — construct the overlay in `enable()`, pass a callback that calls `this._client.call('SkipBreak')`
- [x] 4.2 `extension.js` — suppression edge in `_onStateChanged()`: when the phase becomes `break`, set `_suppressed = Main.layoutManager.primaryMonitor.inFullscreen`; clear it when the phase is not `break` (design Decision 2, spec: Fullscreen Suppression)
- [x] 4.3 `extension.js` — `_render()` calls `shouldShowOverlay(...)` and drives `show(primaryMonitor)` / `hide()`
- [x] 4.4 `extension.js` — connect `monitors-changed`; while shown, re-show against the current primary monitor (spec: Monitor Changes)
- [x] 4.5 `extension.js` — `disable()` tears the overlay down **first**, before the client and indicator, and disconnects `monitors-changed` (design Decision 6)
- [x] 4.6 `extension/stylesheet.css` — overlay background, message, and progress styling for dark and light
- [x] 4.7 `packaging/Makefile` — install `overlay.js`

## Phase 5: Tests

- [x] 5.1 *(Optional, drop first if over budget)* `test-render.js` — predicate cases beyond the spec scenarios: boundary at zero remaining, disconnected state, inactive session
- [x] 5.2 `test-render.js` — suppressed break shows nothing (spec: *Break begins during fullscreen*)
- [x] 5.3 `test-render.js` — phase other than `break` shows nothing (spec: *Break ends normally*)
- [x] 5.4 `test-render.js` — unavailable daemon shows nothing, which is the F3 path (spec: *Daemon disappears while the overlay is up*)
- [x] 5.5 `test-render.js` — zero remaining shows nothing even with no property change (spec: *No property change arrives*)

## Phase 6: Verification

- [x] 6.1 `gjs -m extension/test-render.js` — all cases pass, including M2's existing 17
- [x] 6.2 **Mutation check** — make `shouldShowOverlay` ignore `suppressed`, confirm 5.2 goes red, restore
- [x] 6.3 **Mutation check** — as written this was WRONG twice. First the mutation targeted the wrong function: `if (!state.available || !state.sessionActive)` appears in both `computeDisplay` and `shouldShowOverlay`, and a first-occurrence replace hit the wrong one. Second, once aimed correctly it still did not fail, because 5.4 uses `DISCONNECTED`, which clears `available` and `sessionActive` together — the test passed for the wrong reason. Fixed by adding an isolating case (`available: false` on an otherwise live break). Now observed failing. The same gap existed in M2's `computeDisplay` test and was closed the same way
- [x] 6.4 `node --check` on `overlay.js` and `extension.js`; `git diff --stat daemon/` empty; `go test ./...` in `daemon/` still green
- [x] 6.5 Write `verification-runbook.md` in fish, with a 3-minute test config
- [x] 6.6 Verified 2026-09-18: a break covered the primary monitor. **Defect found and fixed**: content rendered at the top-left rather than centred, because a plain `St.Widget` has no layout manager so the child box's `x_align`/`y_align` were ignored. Fixed with `layout_manager: new Clutter.BinLayout()`; the fix needs a logout to observe
- [x] 6.7 Verified 2026-09-18: Alt+Tab still switches windows while the overlay covers the screen, as product decision P1 intends
- [x] 6.8 Verified 2026-09-18: holding the control to completion ended the break and dismissed the overlay
- [x] 6.9 Verified 2026-09-18: releasing early cancelled without skipping; the overlay stayed
- [x] 6.10 Verified 2026-09-18: a break running to its natural end dismissed the overlay on its own
- [x] 6.11 Verified 2026-09-18: with the overlay up, `systemctl --user stop cadenced` removed it **instantly** and the screen was usable with no further action. Instant rather than faded is correct — `hide()` destroys the actor and only a fade-in was implemented. A vanished daemon cannot leave the screen covered
- [x] 6.12 Verified 2026-09-18: with the primary monitor fullscreen across a break boundary, no overlay appeared. Fullscreen suppression works
- [x] 6.13 Verified 2026-09-18: disabling and re-enabling while the overlay was on screen produced a brief flash — the actor destroyed and rebuilt — and the overlay resumed counting, because the phase was still `break` so the predicate stayed true. Correct teardown and correct re-entry
- [x] 6.14 Verified 2026-09-18: with the current build loaded, overlay content renders centred on the primary monitor. The first attempt had been wasted by a stale install, not a code fault
- [x] 6.15 Verified 2026-09-18: the progress bar now rests empty and fills as the control is held. The defect was found by the user, not by any test — `scale_x` is a Clutter property on an actor that cannot exist outside a Shell, so the 28 unit checks and the spec scenario for *Released early* all passed while the resting state was visibly wrong
- [x] 6.16 Verified 2026-09-18: the countdown renders centred
- [x] 6.17 Verified 2026-09-18 on the fixed build (Shell started 21:40:36, fix installed 21:39:49). The hold is 3s by construction — one constant drives both the `timeout_add` and the progress animation, and `enable-animations` carries no slow-down factor. The user reported it still *felt* longer; investigation found no defect, and the perception was resolved as a product decision to keep 3s deliberately. Two contributors to the feel are understood and intended: three seconds of active holding is genuinely long, and the overlay waits for the daemon to confirm the skip before dismissing, so perceived time is 3s plus a round trip. That wait is what keeps the daemon authoritative (design Decision 5) and was not traded away

## Process failure — install before logging out

The centring fix was written, syntax-checked, and then **not installed**. The user logged out to
verify it and the Shell loaded the previous build, so a correct fix looked broken and a logout was
spent for nothing.

Editing `extension/` changes nothing a running Shell can see. Two steps are required, in order:
`make -C packaging install-extension`, then log out. The runbook now states this and the verify
report carries it as a warning, because a Wayland logout is the most expensive unit of work in this
project and wasting one on a stale install is avoidable.

## Amber during break — resolved, not a bug

An earlier report of the panel indicator staying amber during a break was investigated twice. It is
**not** a defect. Confirmed 2026-09-18 by an unambiguous correlated observation: while the overlay
was on screen — which only happens when `phase === 'break'` — the panel indicator read **white**.
The first report came from an untimestamped screenshot during a fast 3-minute cycle, where `0:57`
is ambiguous between focus (amber correct) and break (amber wrong).

## Note on what only a Shell can prove

Phase 5 covers the predicate, which is three of the four dismissal paths. It cannot show that
`addTopChrome` actually covers windows, that clicks are absorbed, that Alt+Tab survives, or that a
three-second hold feels deliberate rather than tedious. Those are 6.6 through 6.13 and they need a
real session.

6.11 is the one to run first among the live checks. If a vanished daemon can leave the screen
covered, nothing else about this milestone matters.
