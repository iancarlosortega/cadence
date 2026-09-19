# Apply Progress: M3 — Break overlay with hold-to-skip

Status: **29/37 complete.** Everything provable without a Shell is proven. The eight open tasks
(6.6–6.13) are the live checks and need a logout, as M2's did.

## Change

| File | Diff |
|------|------|
| `extension/overlay.js` | new, 163 |
| `extension/test-render.js` | +59 |
| `extension/extension.js` | +43 / −2 |
| `extension/stylesheet.css` | +41 |
| `extension/render.js` | +22 |
| `packaging/Makefile` | +1 |

**329 changed lines against a 400 budget.** The forecast said ~330; after M2 missed by 35% this one
landed within a line. No `size:exception` needed.

`daemon/` is untouched — `git diff --stat -- daemon/` is empty and five Go packages stay green.

## Tasks

- **Phase 1 Predicate** — 1.1–1.3 done. `shouldShowOverlay(state, now, suppressed)` added to
  `render.js`, which still imports nothing.
- **Phase 2 Overlay actor** — 2.1–2.5 done. `CadenceOverlay` plus an `OverlayController` that owns at
  most one actor, adds it with `addTopChrome` at default chrome params, and destroys rather than
  hides.
- **Phase 3 Hold to skip** — 3.1–3.4 done. Timeout plus a `scale_x` transition, cancelled on release,
  on leave, and on destroy.
- **Phase 4 Wiring** — 4.1–4.7 done. Suppression edge, `monitors-changed`, overlay torn down first in
  `disable()`, styling, Makefile.
- **Phase 5 Tests** — 5.1–5.5 done. Ten overlay cases; 28 checks total.
- **Phase 6 Verification** — 6.1–6.5 done; 6.6–6.13 need a logout.

## Corrections during apply

**Task 6.3's mutation check was wrong twice, and both failures were mine.**

First, the mutation targeted the wrong function. The condition
`if (!state.available || !state.sessionActive)` appears in *both* `computeDisplay` and
`shouldShowOverlay`, and a first-occurrence string replace silently hit `computeDisplay`. The check
reported nothing and looked like a pass.

Second, once aimed correctly it *still* did not fail — because test 5.4 uses `DISCONNECTED`, which
clears `available` and `sessionActive` together. Deleting the availability check left the test
passing on `sessionActive` alone. **The F3 safety test passed for the wrong reason.**

Fixed by adding an isolating case: `available: false` on an otherwise live break, a state the client
does not currently produce but which pins the property the check actually guards. The mutation now
fails as intended.

**The same weakness existed in M2's `computeDisplay` test** and was closed the same way, since this
change's own mutation check is what exposed it.

## Evidence

- `gjs -m extension/test-render.js` — **28/28**, including ten overlay-predicate cases covering
  three of the four Self-Owned Exit paths.
- **Mutation, suppression**: predicate ignoring `suppressed` fails `suppressed break shows nothing`.
- **Mutation, availability (the F3 path)**: predicate ignoring `available` fails
  `unavailable alone is enough to hide, even mid-break`.
- **Mutation, `computeDisplay` availability**: fails `unavailable alone is enough to dim`.
- `node --check` — all four JS files parse as ES modules.
- `git diff --stat -- daemon/` empty; `go test ./...` five packages green.
- `make -C packaging install-extension` succeeded; all five installed files byte-identical to source.

## Left open — needs a logout

6.6 coverage and click absorption · 6.7 Alt+Tab still works · 6.8 hold completes and the daemon
agrees · 6.9 release cancels · 6.10 natural end · **6.11 the F3 check** · 6.12 fullscreen
suppression · 6.13 teardown while covered.

Runbook section 3 has them in fish, and puts 6.11 first: if a vanished daemon can leave the screen
covered, nothing else about this milestone matters.

## Observed, not touched

`daemon/internal/session/state.go` and `internal/dbusapi/service_test.go` still fail `gofmt -l` on
pre-existing comment-alignment nits, unrelated to this change.
