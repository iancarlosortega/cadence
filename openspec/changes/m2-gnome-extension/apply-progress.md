# Apply Progress: M2 — GNOME Shell Extension

Status: **implementation complete, blocked on a changed-line budget decision**

All 29 tasks implemented. Runtime attempt 1 settled `passed`, but the ledger reports
`changed_line_budget_exceeded: true` and `decision_required: true`, so the work unit cannot close
without an explicit maintainer ruling.

## Budget accounting

| Measure | Value |
|---------|-------|
| Forecast at `tasks` | 340-390 lines, Medium risk |
| Actual code | **522** lines |
| Ledger total (`cumulative_changed_lines`) | **1548** |
| Declared budget | 400 (`max_changed_lines_source: explicit`) |

The forecast was wrong by roughly 35% on code alone. The ledger figure is larger still because it
counts every intended-untracked path, which includes ~1026 lines of this change's own OpenSpec
planning artifacts (exploration, proposal, spec, design, tasks, state).

Dropping the designated drop-first item (task 6.1, `test-render.js`, 112 lines) yields 410 code
lines — still over. The overrun is not recoverable by trimming the optional task.

| File | Lines |
|------|-------|
| `extension/extension.js` | 272 |
| `extension/test-render.js` | 112 |
| `extension/render.js` | 58 |
| `packaging/Makefile` | 49 |
| `extension/stylesheet.css` | 24 |
| `extension/metadata.json` | 7 |

## Tasks

### Phase 1: Install Path
- [x] 1.1 `packaging/Makefile` — `install-daemon` builds both binaries to `~/.local/bin`, installs the unit, runs `daemon-reload`
- [x] 1.2 `packaging/Makefile` — `install-extension` copies to `~/.local/share/gnome-shell/extensions/cadence@ian.dev/`
- [x] 1.3 `packaging/Makefile` — `uninstall`, `nested`, and `test` targets
- [x] 1.4 Verified: `busctl --user introspect` lists all five methods and six properties

### Phase 2: Extension Skeleton
- [x] 2.1 `extension/metadata.json` — uuid `cadence@ian.dev`, `shell-version: ["48"]`; validated as JSON, directory name matches uuid
- [x] 2.2 / 2.3 Stylesheets — **deviation, see below**: one `stylesheet.css` with two classes rather than two stylesheet files
- [x] 2.4 `extension/extension.js` — ESM imports, `CadenceExtension extends Extension`, `CadenceIndicator` registered via `GObject.registerClass`
- [ ] 2.5 Visual confirmation — **pending**, requires a Wayland session restart

### Phase 3: Render Function
- [x] 3.1 `computeDisplay(state, now)` and `formatMMSS` — **moved to `extension/render.js`**, see deviation
- [x] 3.2 `WARNING_THRESHOLD_SECONDS = 120`, warning iff `focus && !paused && remaining <= 120`
- [x] 3.3 Paused branch reads `remainingSeconds`, never `phaseEndsAt`
- [x] 3.4 `CadenceIndicator.render()` — applies classes and menu sensitivity; no clock access inside the indicator

### Phase 4: D-Bus Client
- [x] 4.1 Interface XML matching the daemon's introspection; `makeProxyWrapper`
- [x] 4.2 `Gio.bus_watch_name` + `CadenceProxy.newAsync`; guards against `disable()` landing mid-construction
- [x] 4.3 Property cache rebuilt whole on every `g-properties-changed`
- [x] 4.4 Tick started only while available, active and unpaused; source id stored and nulled
- [x] 4.5 Verified against the live daemon — see evidence below

### Phase 5: Controls and Teardown
- [x] 5.1 Five popup menu items with per-phase sensitivity via `menuSensitivity(state)`
- [x] 5.2 `${method}Async()` only; no synchronous D-Bus call; no optimistic update
- [x] 5.3 `call()` returns early when the proxy is absent
- [x] 5.4 `disable()` — `GLib.Source.remove`, disconnect, `bus_unwatch_name`, destroy, null
- [ ] 5.5 Pause round-trip in the panel — **pending**, requires a session restart

### Phase 6: Verification
- [x] 6.1 `extension/test-render.js` — 17 checks, all passing under `gjs -m`
- [ ] 6.2 I1 stopped-daemon check — **pending** (Shell-side)
- [ ] 6.3 I1 suspend check — **pending** (Shell-side)
- [ ] 6.4 Amber legibility in both themes — **pending** (Shell-side)
- [ ] 6.5 Leak check — **pending** (Shell-side)
- [x] 6.6 Daemon untouched — `git diff --stat daemon/` empty, `go test ./...` passes

## Deviations from design

1. **Pure logic split into `extension/render.js`.** The design placed `computeDisplay` in
   `extension.js`. Running the test proved that unworkable: importing `extension.js` outside the
   Shell fails with `ImportError: Unable to load file from:
   resource:///org/gnome/shell/extensions/extension.js`. The pure functions now live in
   `render.js`, which imports nothing, so Decision 1's testability claim is actually true rather
   than aspirational. `extension.js` imports from it; the Makefile installs both.

2. **One `stylesheet.css` with two classes, not two stylesheet files.** The design chose
   `stylesheet-dark.css` / `stylesheet-light.css` based on the shipped `window-list` extension's
   precedent. That precedent is real, but the selection mechanism could not be confirmed in any
   source available locally. Instead, `St.Settings` was verified to expose `color-scheme` with an
   `St.SystemColorScheme` enum (`PREFER_DARK` / `PREFER_LIGHT` / `DEFAULT`, confirmed in
   `/usr/lib64/gnome-shell/St-16.typelib`). The extension now picks `.cadence-warning-dark` or
   `.cadence-warning-light` explicitly in JS and re-renders on `notify::color-scheme`. This relies
   on a verified API instead of an unverified convention.

## Evidence

- `gjs -m extension/test-render.js` — 17/17 pass, including the warning boundary at exactly 120
  and 121 seconds, and the paused-reads-`RemainingSeconds` case.
- `go test ./...` in `daemon/` — all packages pass; `git diff --stat daemon/` empty.
- `node --check` — both `extension.js` and `render.js` parse as ES modules.
- `make -C packaging install` — succeeded; all four extension files installed, uuid matches
  directory name.
- Live daemon, session started: `PhaseEndsAt` 1789745092 with `now` 1789742094 → 2998s, matching
  `RemainingSeconds` 3000 less the 2s elapsed. `computeDisplay` rendered `49:58`.
- **Live confirmation of the paused trap:** pausing published `PhaseEndsAt: 0` with
  `RemainingSeconds: 2993`. `computeDisplay` rendered `49:53`. A naive
  `PhaseEndsAt - now` would have computed **-1789742100**. The trap identified at exploration is
  real and is handled.
- `gnome-extensions list` does not yet show the extension: the running Shell scanned extensions at
  session start and Wayland cannot reload it in place. Files, metadata and uuid are verified
  correct on disk; activation needs logout/login.

## Blocking decision

Under `single-pr`, a silent split is not permitted and the budget was exceeded. The runtime ledger
returns `state: blocked`, `reason: maintainer_decision`, `next_action: reset`. A maintainer
decision is required before this work unit can close or `verify` can run.
