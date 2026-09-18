# Tasks: M2 — GNOME Shell Extension

## Review Workload Forecast

Estimated changed lines: ~340-390.

| Artifact | Estimate |
|----------|----------|
| `extension/metadata.json` | ~12 |
| `extension/extension.js` | ~240-280 (interface XML, `CadenceClient`, `CadenceIndicator`, Extension class) |
| `extension/stylesheet-dark.css` | ~12 |
| `extension/stylesheet-light.css` | ~12 |
| `packaging/Makefile` | ~50 |
| `extension/test-render.js` (optional, task 6.1) | ~60 |

Delivery strategy: `single-pr`.

Decision needed before apply: **No** — the estimate sits inside the 400-line budget.
Chained PRs recommended: No
Chain strategy: null
400-line budget risk: **Medium**

The estimate is close enough to the ceiling that it can cross it. If the implementation exceeds 400
changed lines, the Review Workload Guard MUST stop before apply and require a recorded
`size:exception`, because `single-pr` does not permit a silent split. The optional pure-logic test
harness in task 6.1 is the natural thing to drop first if the budget is tight — it is the only task
not required by a spec requirement.

### Suggested Work Units

1. **Install path** — `packaging/Makefile`. Test: `make install` places all three artifacts. Harness: `systemctl --user status cadenced`. Rollback: `make uninstall`.
2. **Extension skeleton** — metadata, stylesheets, empty lifecycle. Test: `gnome-extensions enable` shows a dimmed indicator. Harness: nested session. Rollback: `rm -rf extension/`.
3. **Client and render** — proxy, cache, pure render. Test: countdown tracks a real session. Harness: `cadence start` + `busctl --user`. Rollback: revert `extension.js`.
4. **Controls and teardown** — menu actions, `disable()` hygiene. Test: pause round trip; enable/disable leaves nothing. Harness: `journalctl -f -o cat /usr/bin/gnome-shell`.

All four ship as one PR under `single-pr`.

## Phase 1: Install Path

The daemon has never been installed, so nothing downstream is verifiable until this exists.

- [x] 1.1 `packaging/Makefile` — `install` target: `go build -o ~/.local/bin/cadenced ./cmd/cadenced` and `./cmd/cadence`, install `cadenced.service` to `~/.config/systemd/user/`, `systemctl --user daemon-reload`
- [x] 1.2 `packaging/Makefile` — extension install: copy `extension/` to `~/.local/share/gnome-shell/extensions/cadence@ian.dev/`
- [x] 1.3 `packaging/Makefile` — `uninstall` target (binaries, unit, extension dir) and `nested` target running `dbus-run-session gnome-shell --nested --wayland`
- [x] 1.4 Verify: `make install`, then `systemctl --user enable --now cadenced` and `busctl --user introspect dev.ian.Cadence /dev/ian/Cadence` lists all five methods and six properties

## Phase 2: Extension Skeleton

- [x] 2.1 `extension/metadata.json` — uuid `cadence@ian.dev`, name, description, `shell-version: ["48"]`, url
- [x] 2.2 ~~`extension/stylesheet-dark.css`~~ → `extension/stylesheet.css` `.cadence-warning-dark` (`#ffb454`, legible on `#000000`). **Deviation:** one stylesheet with two classes; the Shell's `stylesheet-dark.css`/`stylesheet-light.css` selection could not be confirmed locally, whereas `St.Settings.color-scheme` is verified in `St-16.typelib`
- [x] 2.3 ~~`extension/stylesheet-light.css`~~ → `.cadence-warning-light` (`#8f5300`, legible on `#fafafb`) in the same file, selected in JS
- [x] 2.4 `extension/extension.js` — ESM imports; `export default class CadenceExtension extends Extension` with `enable()`/`disable()`; `CadenceIndicator` as a `GObject.registerClass`-ed `PanelMenu.Button`
- [x] 2.5 Verified 2026-09-18 after logout: `gnome-extensions info` reports `State: ACTIVE`, indicator present, and it renders dimmed with no countdown while `cadenced` is stopped

## Phase 3: Render Function

Pure logic first, because it is the only part testable without the Shell.

- [x] 3.1 `extension/render.js` — `formatMMSS(seconds)` and `computeDisplay(state, now)` returning `{dimmed, label, warning}`; holds no countdown field between calls (design Decision 1, invariant I1). **Deviation:** moved out of `extension.js`, which cannot be imported outside the Shell (`ImportError: resource:///org/gnome/shell/extensions/extension.js`). `render.js` imports nothing, which is what makes I1 testable
- [x] 3.2 `extension/render.js` — `WARNING_THRESHOLD_SECONDS = 120` as a named constant; warning iff `phase === 'focus' && !paused && remaining <= 120` (spec: Break Warning)
- [x] 3.3 `extension/render.js` — paused branch reads `remainingSeconds`, never `phaseEndsAt` (spec: Countdown Derivation / *Paused freezes on RemainingSeconds*)
- [x] 3.4 `CadenceIndicator.render(display, sensitivity, warningClass)` — apply/remove the warning class, set label text, toggle dimmed style; no clock access inside the indicator

## Phase 4: D-Bus Client

- [x] 4.1 `extension/extension.js` — interface XML literal matching `daemon/internal/dbusapi/service.go:89-109`; `Gio.DBusProxy.makeProxyWrapper`
- [x] 4.2 `CadenceClient` — `Gio.bus_watch_name` on `dev.ian.Cadence`; proxy built with `CadenceProxy.newAsync`, dropped in `onVanished`; guards against `disable()` landing mid-construction (spec: Connection Lifecycle; design Decision 5, 6)
- [x] 4.3 `CadenceClient` — property cache rebuilt whole on `g-properties-changed`; exposed via a `state` getter; the only writer of session state (design Decision 3)
- [x] 4.4 Extension class — 1s `GLib.timeout_add_seconds` started only while available, active and unpaused; source id stored and nulled (design Decision 2)
- [x] 4.5 Verify: live daemon — `PhaseEndsAt` 1789745092 at now 1789742094 → 2998s, matching `RemainingSeconds` 3000 less 2s elapsed; `computeDisplay` rendered `49:58`. Paused published `PhaseEndsAt: 0` / `RemainingSeconds: 2993` → rendered `49:53` where a naive derivation gives -1789742100

## Phase 5: Controls and Teardown

- [x] 5.1 `CadenceIndicator` — popup menu items for Start, Stop, Pause, Resume, Skip Break; sensitivity from `menuSensitivity(state)`
- [x] 5.2 `CadenceClient.call()` — `${method}Async()` only; no synchronous D-Bus call anywhere; no optimistic local state update (spec: Control Actions; design Decision 7)
- [x] 5.3 `call()` returns early when the proxy is absent, raising nothing into the Shell log (spec: Control Actions / *Action with no daemon*)
- [x] 5.4 `disable()` — `GLib.Source.remove(tickId)`, disconnect every handler, `Gio.bus_unwatch_name(watchId)`, `indicator.destroy()`, null every reference; correct when disabled while disconnected (spec: Teardown Hygiene)
- [x] 5.5 Verified 2026-09-18: clicking Pause froze the label **and** `busctl` reported `Paused b true`. The freeze came from the daemon, not a local decision — this is the round trip M2 exists to prove

## Phase 6: Verification

- [x] 6.1 `extension/test-render.js` — 17 checks under `gjs -m`, all passing: zero-remainder holds phase, paused reads `remainingSeconds`, warning boundary inclusive at 120 and absent at 121, menu sensitivity per phase. *(Kept despite the budget overrun; see 6.7)*
- [x] 6.2 Verified 2026-09-18: stopping `cadenced` mid-session dimmed the indicator and the countdown stopped displaying. The tick did not keep counting on its own authority — I1 holds under the failure it was written for
- [x] 6.3 Verified 2026-09-18 after a real 9s suspend, on the daemon carrying `fix-suspend-deadline-republish`: offsets `+0.171s` / `+0.174s` / `+0.174s`, sub-second throughout and holding across a focus/break transition mid-measurement. Run via `python3 packaging/check-offset.py`
- [x] 6.4 Verified 2026-09-18 on a 3-minute test config: amber appears at exactly 2:00 remaining in `focus` and disappears at the break transition. Legible on the dark panel (`#ffb454` on `#000000`). An earlier report of amber persisting into break was a misattributed screenshot — the fast 3-minute cycle makes an untimestamped reading of `0:57` ambiguous between focus and break; resolved by reading the daemon phase at the same instant as the panel
- [x] 6.5 Verified 2026-09-18: three disable/enable cycles including one with `cadenced` stopped (the path where the bus name watch can outlive `disable()`). No extension errors, no disposed-object warnings, no leaked-source complaints in the Shell journal; indicator returned each time
- [x] 6.6 Daemon untouched — `go test ./...` in `daemon/` passes and `git diff --stat daemon/` is empty
- [x] 6.7 Budget ruling — actual 522 code lines against a 400 budget (forecast 340-390 was ~35% low). Dropping 6.1 reaches only 410, so the overrun was not trimmable. Maintainer accepted `size:exception` on 2026-09-18; objective reset recorded as `rst-m2-20260918-01`, actor "ian (maintainer)", with attempt 1 preserved in ledger history

## Finding — daemon defect surfaced by 6.3 (2026-09-18, RESOLVED)

**Resolved** by change `fix-suspend-deadline-republish`, archived 2026-09-18. Task 6.3 now passes
against a daemon carrying that fix. The original analysis is kept below for the record.



Task 6.3 does not merely lack evidence, it **fails**. `applySuspend`
(`daemon/internal/session/machine.go:123-139`) returns `EffectPersist` with no `EffectNotify` on the
short-suspend path, so the daemon moves the effective deadline without republishing `PhaseEndsAt`.

Measured: a 9-second suspend (`systemd-logind` 15:03:57 -> 15:04:06) left the indicator a constant
3 seconds ahead of the daemon across three samples 20s apart. The gap never closes on its own. The
error is bounded by the idle-credit threshold, so a 9-minute suspend can leave the panel roughly
nine minutes wrong.

This is a daemon bug, not an extension bug — the extension does exactly what I1 and the spec
require. M2 stays consumer-only; the fix belongs to its own change. Task 6.3 stays open until that
change lands.

## Pending work requires a session restart

Tasks 2.5, 5.5, 6.2, 6.3, 6.4 and 6.5 are all Shell-side. The running GNOME Shell scanned its
extension directory at session start, and Wayland cannot restart the Shell in place, so none of
them can be completed from this session. They are not failures and nothing about them is unknown-
risky; they are simply gated on:

```
# log out and back in, then
gnome-extensions enable cadence@ian.dev
cadence start
```

`verify` will remain blocked until they are done, which is correct — a verdict claiming the
indicator renders would be unfounded until somebody has actually seen it render.
