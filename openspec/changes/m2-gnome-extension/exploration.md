# Exploration: M2 — GNOME Shell Extension (panel indicator, amber at T-2min)

## Current State

M1 is complete and archived at `openspec/changes/archive/2026-09-17-m1-daemon-core/`. Its four
capability specs are merged into `openspec/specs/`. The daemon builds clean (`go build ./...` in
`daemon/`) and the whole repository is committed at `bec60bc`.

The D-Bus contract the extension must consume is already fixed and verified in source:

| Fact | Value | Evidence |
|------|-------|----------|
| Bus name | `dev.ian.Cadence` | `daemon/internal/dbusapi/service.go:20` |
| Object path | `/dev/ian/Cadence` | `daemon/internal/dbusapi/service.go:21` |
| Interface | `dev.ian.Cadence1` | `daemon/internal/dbusapi/service.go:22` |
| Methods | `StartSession`, `StopSession`, `Pause`, `Resume`, `SkipBreak` — all no-arg, no return value | `daemon/internal/dbusapi/service.go:54-60`, `:127-131` |
| Properties | `SessionActive` (b), `Phase` (s), `PhaseEndsAt` (x), `RemainingSeconds` (x), `Paused` (b), `Tier` (s) | `daemon/internal/dbusapi/service.go:68-77` |
| Signals | None custom. Only `org.freedesktop.DBus.Properties.PropertiesChanged` | `daemon/internal/dbusapi/service.go:188-205` |
| Introspection | Exported explicitly; `busctl --user introspect` works | `daemon/internal/dbusapi/service.go:89-109` |

`Phase` is one of `none` / `focus` / `break` (`daemon/internal/session/state.go:13-16`). `Tier` is
one of `T0`–`T3`, and M1 only ever observes `T0` (`daemon/internal/session/state.go:24-29`).

Environment verified on this machine: GNOME Shell **48.8**, session type **wayland**.

Two gaps that bound M2's scope:

1. `cadenced` is **not installed** (`~/.local/bin/cadenced` does not exist) and **not on the session
   bus** (`busctl --user list` shows no `dev.ian.Cadence`). The systemd unit at
   `packaging/cadenced.service` points at `%h/.local/bin/cadenced`, so nothing has ever been
   installed. M2 cannot be verified at all until the daemon is installed and running.
2. There is no `extension/` directory. The GJS half is greenfield.

## Affected Areas

- `extension/` — new. GJS sources: `metadata.json`, `extension.js`, `stylesheet.css`.
- `packaging/` — an install path for the extension into
  `~/.local/share/gnome-shell/extensions/<uuid>/`, and an install path for the `cadenced` binary
  that the existing unit already assumes.
- `openspec/specs/` — one new capability. The daemon's existing four specs are **unchanged**: M2
  consumes the M1 contract, it does not alter it.
- `daemon/` — expected to be untouched. If M2 needs a daemon change, that is a finding, not a plan.

## Approaches

### Decision 1 — Where the amber state comes from

This is the load-bearing decision of M2, and the daemon has already constrained it.

`specs/daemon-control:27` requires that `PropertiesChanged` is emitted **on transitions only** and
**MUST NOT** be emitted per second. The implementation honours this: `publish()` runs only from
`apply()`, only when the domain returns an `EffectNotify`
(`daemon/internal/dbusapi/service.go:169-188`), and the domain emits `EffectNotify` only on real
transitions (`daemon/internal/session/state.go:78-81`). A quiet tick produces no bus traffic.

Therefore **there is no "T-2min" signal and there will not be one.** The daemon publishes
`PhaseEndsAt` as an absolute unix-epoch second (`service.go:194-196, 201`), and the extension is
expected to derive the countdown itself. This was already settled in M1 as open decision D1:
"emit on transitions only and let the client tick locally"
(`openspec/changes/archive/2026-09-17-m1-daemon-core/state.yaml:108-112`).

- **Option A — local tick from `PhaseEndsAt` (recommended).** The extension runs its own
  1s `GLib.timeout_add_seconds` source, computes `PhaseEndsAt - now`, renders the countdown, and
  applies the amber style class when the remainder crosses 120s during `focus`. Zero added bus
  traffic. Survives missed signals because the absolute deadline is self-correcting.
- **Option B — ask the daemon to emit a warning.** Rejected. It would either violate the
  no-per-second-traffic requirement, or add a second timing authority that can disagree with the
  first. The domain is deliberately GNOME-free (`openspec/config.yaml:26`, and
  `daemon/internal/session/state.go:1-5`).

Two edge cases Option A must handle, both already visible in `publish()`:

- When `Paused` is true, `PhaseEndsAt` is published as **0** and the live value is
  `RemainingSeconds` (`service.go:191-197`). The extension must freeze its display on
  `RemainingSeconds` while paused rather than counting down from a zero deadline.
- When the session is inactive, `PhaseEndsAt` is also 0 and `Phase` is `none`. The indicator needs
  a defined idle presentation.

### Decision 2 — Proxy lifecycle and a daemon that is not running

`cadenced` is not running right now, and its unit is `Restart=on-failure`, so the extension will
routinely outlive and re-meet the daemon. A proxy constructed once at `enable()` against an absent
name is the obvious failure mode.

- **Recommended:** `Gio.bus_watch_name(Gio.BusType.SESSION, 'dev.ian.Cadence', ...)` and build the
  proxy in `onAppeared`, tear it down in `onVanished`, showing a defined disconnected state in the
  panel. Unwatch with `Gio.bus_unwatch_name(id)` in `disable()`.
- The proxy itself must be constructed **asynchronously**. A synchronous D-Bus call in the Shell
  blocks the compositor main loop — this janks the whole desktop, not just the extension.
  `Gio.DBusProxy.makeProxyWrapper(xml)` with the async callback form is the documented path
  (https://gjs.guide/guides/gio/dbus.html).

### Decision 3 — Shell 48 module and lifecycle shape

GNOME Shell 45 replaced the legacy GJS `imports` system with standard ESM, so every 42-era example
on the web is wrong for this target. For Shell 48:

- `metadata.json` requires `uuid`, `name`, `description`, `shell-version: ["48"]`, `url`.
  `session-modes` is not needed for a normal desktop indicator.
- `extension.js` must `export default class ... extends Extension`, imported from
  `resource:///org/gnome/shell/extensions/extension.js`; GObject/Gio/GLib/St/Clutter come from
  `gi://`, and `Main`/`PanelMenu`/`PopupMenu` from `resource:///org/gnome/shell/ui/*.js`.
- The indicator is a `GObject.registerClass`-ed subclass of `PanelMenu.Button`, added with
  `Main.panel.addToStatusArea(this.uuid, indicator)`.
- `disable()` is a hard contract, not a courtesy: every signal handler disconnected, every GLib
  source removed via `GLib.Source.remove(id)` even if it would self-remove, every actor destroyed,
  every reference nulled. This is exactly where a 1s timer and a D-Bus proxy leak.
  (https://gjs.guide/extensions/review-guidelines/review-guidelines.html)

### Decision 4 — Where the 2-minute threshold lives

No warning threshold exists anywhere today. `config.toml` defines only `timer.focus_minutes`,
`timer.break_minutes`, `idle.pause_after_minutes`, `idle.credit_break_after_minutes`
(`daemon/internal/config/config.go:26-35`), and the daemon has no way to hand a preference to the
extension other than a new D-Bus property.

- **Recommended for M2:** hardcode 120s as a named constant in the extension. It is the
  milestone's stated behaviour, and M6 (config hot reload) is the milestone that earns the right to
  make it tunable.
- Alternative — a GSettings schema for the extension — adds a schema compile step to the install
  path for one integer. Deferred, not rejected.

### Decision 5 — Styling the amber state

`stylesheet.css` beside `metadata.json` is auto-loaded, and state is toggled with
`add_style_class_name()` / `remove_style_class_name()`. GNOME 48 exposes `-st-accent-color` for
theme-following accent, but the research found **no documented standard warning/amber token** for
extensions. Recommendation: define a local `.cadence-warning` class rather than assume a shared
token exists. Flagged as unconfirmed — worth one direct check during design.

## Recommendation

Build M2 as a consumer-only change: a new `extension/` tree plus an install path, with zero daemon
edits. The indicator reads the six existing properties, re-reads them on `g-properties-changed`,
and runs one local 1s source that derives the countdown from `PhaseEndsAt` and flips a
`.cadence-warning` class at 120s remaining in `focus`. Name watching handles the
daemon-not-running case. Start/Stop/Pause/Resume/SkipBreak land as popup menu items, which is what
actually proves "both halves talk" in both directions — properties inbound, methods outbound.

The milestone's own test is a round trip: click Pause in the panel, and the label freezes because
the daemon said so, not because the extension decided to stop counting.

## Risks

- **Verification requires a running daemon.** `cadenced` is not built into `~/.local/bin` and not
  on the bus. The install step is a prerequisite of M2, not a footnote.
- **Wayland cannot reload the Shell in place.** No Alt+F2 `r`. The dev loop is either logout/login
  or a nested session (`dbus-run-session gnome-shell --nested --wayland`), and the nested instance
  is documented as *not* fully isolated from the host session.
- **No automated test harness exists for Shell extensions.** Confirmed across the research: no
  Playwright/Jest equivalent in 2026, and e.g.o itself states extensions are reviewed but not
  always tested for functionality. Honest verification for M2 is: pure-logic GJS functions extracted
  and unit-testable in principle, daemon-side Go tests unchanged, and the extension half verified
  manually in a nested session. `strict_tdd` stays `false`; M2 is not the milestone that changes it.
- **Leak-in-`disable()` is the likeliest real defect.** A 1s timeout source and a D-Bus proxy are
  exactly the two things the review guidelines call out.
- **Paused publishes `PhaseEndsAt: 0`.** An extension that naively counts down from `PhaseEndsAt`
  will render a wildly negative countdown the moment the user pauses.
- **Stale research details.** The `-st-accent-color` property and the exact `journalctl` invocation
  for Shell logs came from search snippets rather than direct doc fetches. Re-check at design time.

## Ready for Proposal

Yes. The D-Bus contract is fixed and evidenced in source, the amber mechanism is determined by an
existing spec requirement rather than open to preference, and the Shell 48 API shape is researched.

Open items the proposal must settle:

- **P1 (product):** what the indicator shows when the daemon is absent or no session is active —
  hidden entirely, a dimmed icon, or a placeholder label.
- **D1:** confirm the 120s threshold is hardcoded for M2 rather than made configurable.
- **D2:** confirm the extension `uuid` (e.g. `cadence@ian.dev`), which is durable across packaging
  and the install path in the same way the bus name is.
- **D3:** decide whether the install path is a `Makefile`/script in `packaging/` or documented
  manual steps.
