# Design: M2 — GNOME Shell Extension

## Technical Approach

A single-file GJS extension plus two theme stylesheets and a packaging Makefile. No build step, no
bundler, no dependencies: GNOME Shell 48 loads ESM directly.

The extension is structured as three collaborators inside `extension.js`, kept separate because I1
demands it:

- **`CadenceClient`** — owns the bus name watch, the async proxy, and the property cache. It is the
  only thing that knows D-Bus exists, and the only writer of session state.
- **`CadenceIndicator`** — a `PanelMenu.Button` subclass. Pure presentation: it is handed a state
  object and renders it. It never reads a clock and never touches the proxy.
- **The extension class** — wires the two together, owns the 1s tick, and owns teardown.

The tick calls `indicator.render(state, now)`. That signature is the invariant made structural: the
renderer receives state it cannot modify and a timestamp it did not choose.

## Architecture Decisions

### Decision 1 — Rendering is a pure function of (state, now)

`render(state, now)` computes its output from the property cache plus a passed-in timestamp and
returns nothing. It holds no countdown variable between calls.

This is I1 enforced by shape rather than by discipline. A decrementing counter requires a field to
decrement; if no such field exists, the drift bug is not merely avoided, it is unrepresentable. It
also makes the countdown logic the one genuinely unit-testable piece of the extension: pure input,
pure output, no Shell, no bus.

Rejected: an indicator that owns a `_remaining` field updated by the tick. Conventional, and exactly
the second-timer failure I1 exists to prevent.

### Decision 2 — The tick runs only while it has something to render

`GLib.timeout_add_seconds` is started when a session becomes active and unpaused, and removed when
it stops being so. A dimmed indicator has no timer behind it.

Rationale: a 1s source that wakes the compositor to redraw nothing is a battery cost on a laptop
that will run this for eight hours a day. The source id is nulled on removal so teardown is
idempotent.

Consequence: paused is a stopped tick, not a tick that renders the same value repeatedly. The frozen
display comes from `RemainingSeconds` on the pause `PropertiesChanged`, rendered once.

### Decision 3 — Property cache, not per-render proxy reads

`CadenceClient` copies the six properties into a plain object on every `g-properties-changed` and
exposes that object. `render` reads the plain object.

Rationale: `g-properties-changed` delivers only the changed subset, so a renderer reading the proxy
directly would mix fresh and stale reads across the tick. A single cache updated atomically per
signal gives every render a coherent snapshot. It also keeps the renderer testable without a bus.

### Decision 4 — Amber ships as a local class in dark and light stylesheets

Settles open decision D4 with local evidence rather than documentation.

Extracting GNOME 48.8's own shipped theme
(`gresource extract /usr/share/gnome-shell/gnome-shell-theme.gresource`) shows the complete set of
St custom properties available to an extension is exactly five: `-st-accent-color`,
`-st-accent-fg-color`, `-st-hfade-offset`, `-st-icon-style`, `-st-vfade-offset`. A search for
`.warning`, `.error`, `.destructive`, or `.success` in both `gnome-shell-dark.css` and
`gnome-shell-light.css` returns **zero matches**. There is no warning token to inherit, and
`-st-accent-color` is the user's accent, not a semantic warning — using it would make the "you are
about to be interrupted" state indistinguishable from ordinary chrome, and would change meaning if
the user picks an amber accent.

So `.cadence-warning` is defined locally. It must ship twice, because the panel background differs
per theme: `#000000` in dark, `#fafafb` in light (both at `#panel`, line 1776 of each file). One
amber cannot be legible on both — a mid amber on near-white fails contrast badly.

The mechanism is GNOME's own: the shipped `window-list@gnome-shell-extensions.gcampax.github.com`
extension (also `shell-version: ["48"]`) ships `stylesheet-dark.css` and `stylesheet-light.css`
side by side. The Shell selects the variant. Cadence follows that precedent rather than inventing a
media-query approach.

### Decision 5 — Name watching owns the connection, `enable()` does not

`enable()` starts a `Gio.bus_watch_name` and returns. The proxy is built in `onAppeared`, destroyed
in `onVanished`. There is no connection attempt at enable time and no retry loop.

Rationale: the daemon's unit is `Restart=on-failure`, so vanish/appear is a normal event, not an
error path. Name watching is level-triggered and handles "not started yet", "restarted", and
"stopped for good" with the same two callbacks. A retry loop would be a second, worse
implementation of what the bus already provides.

### Decision 6 — Every D-Bus call is async, no exceptions

Proxy construction uses the `makeProxyWrapper` async callback form; methods use the generated
`...Async` wrappers.

Rationale: a synchronous D-Bus call in the Shell process blocks the compositor main loop. The
symptom is not a slow extension, it is a frozen desktop — including the mouse cursor. This is the
single highest-severity mistake available in this codebase, which is why the spec states it as a
prohibition rather than a preference.

### Decision 7 — No optimistic UI

Activating a menu item calls the method and returns. The display changes only when
`PropertiesChanged` arrives.

Rationale: M2's whole purpose is proving the round trip. An optimistic update would make the panel
look correct while proving nothing, and would actively lie if the daemon rejected the call — for
example `SkipBreak` outside a break, which the domain ignores
(`daemon/internal/session/machine.go:63-65`).

## Data Flow

```
cadenced (owner of dev.ian.Cadence)
    │  PropertiesChanged (transitions only — never per second)
    ▼
Gio.bus_watch_name ──► CadenceClient ──► property cache {SessionActive, Phase,
                          (only writer)      PhaseEndsAt, RemainingSeconds, Paused, Tier}
                                                   │
                                        ┌──────────┴──────────┐
                                        │   render(state, now)│ ◄── GLib 1s tick (now only)
                                        └──────────┬──────────┘
                                                   ▼
                                          CadenceIndicator
                                        (St.Icon + St.Label)

User clicks menu item ──► proxy.XAsync() ──► cadenced ──► PropertiesChanged ──► (loop above)
```

The tick contributes `now` and nothing else. No arrow runs from the tick back into the cache — that
absence is I1.

## File Changes

| File | Change |
|------|--------|
| `extension/metadata.json` | New. uuid `cadence@ian.dev`, `shell-version: ["48"]`, name, description, url |
| `extension/extension.js` | New. `CadenceClient`, `CadenceIndicator`, default `Extension` subclass |
| `extension/stylesheet-dark.css` | New. `.cadence-warning` amber legible on `#000000` |
| `extension/stylesheet-light.css` | New. `.cadence-warning` amber legible on `#fafafb` |
| `packaging/Makefile` | New. `install`, `uninstall`, `nested` targets |
| `packaging/cadenced.service` | Unchanged; installed by the Makefile |
| `daemon/**` | Unchanged |

## Interfaces / Contracts

Consumed, not defined — the full contract is M1's and is unchanged:

```
bus name   dev.ian.Cadence
path       /dev/ian/Cadence
interface  dev.ian.Cadence1
methods    StartSession() StopSession() Pause() Resume() SkipBreak()   (no args, no returns)
props      SessionActive b  Phase s  PhaseEndsAt x  RemainingSeconds x  Paused b  Tier s
signals    org.freedesktop.DBus.Properties.PropertiesChanged only
```

Interface XML for `makeProxyWrapper` is written by hand in `extension.js` to match the daemon's
introspection (`daemon/internal/dbusapi/service.go:89-109`). It is a copy of a contract owned
elsewhere, so it is a place drift can appear; the install-time smoke check below is the guard.

Internal render contract:

```
render(state, now) where state = {available, sessionActive, phase, phaseEndsAt,
                                  remainingSeconds, paused, tier}
  available false        → dimmed, no label
  sessionActive false    → dimmed, no label
  paused true            → icon + fmt(remainingSeconds),        no warning class
  otherwise              → icon + fmt(max(0, phaseEndsAt - now))
                           warning class iff phase === 'focus' && remaining <= 120
```

## Testing Strategy

Honest, and split by what is actually testable:

- **Daemon:** `go test ./...` in `daemon/` must still pass unchanged. M2 touches no Go code; a
  failure here means the change escaped its scope.
- **Pure logic:** the countdown/format/warning-threshold decision is a pure function of
  `(state, now)`. It is testable in principle by importing it under plain `gjs` outside the Shell.
  Whether M2 ships such a harness is a `tasks` decision, not a spec requirement — the value is that
  Decision 1 makes it *possible*, where the conventional design would not.
- **Integration:** manual, in a nested session — `dbus-run-session gnome-shell --nested --wayland`.
  There is no automated UI harness for Shell extensions and this design does not pretend otherwise.
- **The two I1 checks are manual and deliberate:** stop `cadenced` mid-session and confirm the
  countdown stops claiming progress; suspend mid-`focus` and confirm the resumed display agrees with
  `busctl --user get-property ... RemainingSeconds` within one tick.
- **Leak check:** enable/disable cycle with `journalctl -f -o cat /usr/bin/gnome-shell` open, asserting
  no error output and no surviving source.

## Migration / Rollout

Additive. `make install` builds `cadenced` to `~/.local/bin`, installs the unit, and installs the
extension to `~/.local/share/gnome-shell/extensions/cadence@ian.dev/`. Because Wayland cannot
restart the Shell in place, first activation requires logout/login; subsequent iteration uses the
nested session. Rollback is `gnome-extensions disable cadence@ian.dev` plus removing that directory;
the daemon is unaffected.

## Open Questions

None blocking. D4 is settled by Decision 4 with local evidence from the installed 48.8 theme.

Deferred deliberately:

- The exact amber hex values are a `tasks`-time choice; the constraint is a legible contrast against
  `#000000` and `#fafafb` respectively.
- Whether to ship a pure-logic GJS test harness — enabled by Decision 1, decided at `tasks`.
