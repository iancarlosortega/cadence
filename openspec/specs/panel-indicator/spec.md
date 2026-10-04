# panel-indicator Specification

## Purpose

The GNOME Shell surface for cadence: a top-bar indicator that reflects daemon session state, renders
a countdown derived locally from the daemon's absolute deadline, warns before a break is due, and
drives the daemon through its existing control methods.

## Requirements

### Requirement: Indicator Presence

The extension MUST add exactly one indicator to the top bar on `enable()` and MUST remove it on
`disable()`. The indicator MUST remain present regardless of daemon availability or session state,
and MUST NOT be hidden as a way of representing an idle or disconnected condition.

#### Scenario: Enabled with no daemon

- GIVEN `cadenced` is not running
- WHEN the extension is enabled
- THEN the indicator is present in the top bar in its dimmed presentation

#### Scenario: Disabled

- GIVEN the extension is enabled
- WHEN the extension is disabled
- THEN no indicator remains in the top bar

### Requirement: State Authority

D-Bus properties MUST be the only authority for session state. The extension MUST update `Phase`,
`SessionActive`, `Paused`, and `Tier` only from the proxy's cached properties, refreshed on
`g-properties-changed`.

The local tick MUST NOT decide or infer a state transition. It MUST NOT change `Phase` when its
computed remainder reaches zero, MUST NOT clear `SessionActive`, and MUST NOT call any daemon method
on its own initiative.

#### Scenario: Countdown reaches zero without a daemon signal

- GIVEN an active `focus` session whose computed remainder has reached zero
- AND no `PropertiesChanged` has been received
- WHEN the next tick fires
- THEN the displayed countdown is `00:00`
- AND `Phase` still reads `focus`
- AND no daemon method is called

#### Scenario: Daemon announces the transition

- GIVEN a displayed countdown of `00:00` in `focus`
- WHEN the daemon emits `PropertiesChanged` with `Phase` = `break`
- THEN the indicator renders the break phase and its countdown

### Requirement: Countdown Derivation

While a session is active and neither paused, idle nor held, the displayed countdown MUST be
recomputed on every tick as `PhaseEndsAt` minus the current wall-clock time, clamped at zero. The
extension MUST NOT decrement a locally held counter.

While `Paused` is true, the displayed countdown MUST be taken from `RemainingSeconds` and MUST NOT
be derived from `PhaseEndsAt`, which the daemon publishes as `0` while paused.

While `Idle` is true, the displayed countdown MUST be taken from `RemainingSeconds` and MUST NOT be
derived from `PhaseEndsAt`, which the daemon publishes as `0` while idle. The extension MUST NOT
infer idleness from its own input monitoring; `Idle` is the daemon's to declare.

While `Hold` is not `none`, the displayed countdown MUST be taken from `RemainingSeconds` and MUST
NOT be derived from `PhaseEndsAt`, which the daemon publishes as `0` while a break is held.

#### Scenario: Display self-corrects after suspend

- GIVEN an active `focus` session
- WHEN the machine suspends and resumes
- THEN the next tick renders `PhaseEndsAt` minus now
- AND the rendered value equals the daemon's elapsed-derived remaining
- AND that offset stays constant at zero across repeated samples

The comparison MUST be against the daemon's elapsed-derived remaining
(`elapsed_in_phase` plus time since `LastObserved`), NOT against the published `RemainingSeconds`
property. `RemainingSeconds` is republished only on transitions, so between transitions it is a
stale snapshot and a correct client will appear to diverge from it by exactly the elapsed time.

This scenario depends on the daemon republishing `PhaseEndsAt` whenever it changes the effective
deadline, which `daemon-control` "Change Notification" requires. Verified 2026-09-18 on a real
suspend: sub-second offset, stable across a phase transition.

#### Scenario: Paused freezes on RemainingSeconds

- GIVEN an active `focus` session with 12 minutes remaining
- WHEN `Pause` is invoked and the daemon publishes `Paused` = true, `PhaseEndsAt` = 0
- THEN the countdown displays 12 minutes and does not advance
- AND the countdown is not rendered as a negative value

#### Scenario: Idle freezes on RemainingSeconds

- GIVEN an active `focus` session with 12 minutes remaining
- WHEN the daemon publishes `Idle` = true, `PhaseEndsAt` = 0, `RemainingSeconds` = 720
- THEN the countdown displays 12 minutes and does not advance
- AND the countdown is not rendered as a negative value

#### Scenario: Countdown resumes on return from idle

- GIVEN a frozen countdown showing 12 minutes because `Idle` is true
- WHEN the daemon publishes `Idle` = false and a non-zero `PhaseEndsAt`
- THEN the countdown resumes from 12 minutes
- AND it is again derived from `PhaseEndsAt` on every tick

#### Scenario: A held break freezes on RemainingSeconds

- GIVEN the daemon publishes `Phase` = `break`, `Hold` = `prompt`, `PhaseEndsAt` = 0, `RemainingSeconds` = 600
- WHEN the indicator renders on successive ticks
- THEN the countdown displays 10 minutes and does not advance

### Requirement: Presentation States

The indicator MUST render exactly one presentation for the current condition:

| Condition | Presentation |
|-----------|--------------|
| Daemon not on the bus | dimmed icon, no countdown label |
| `SessionActive` false | dimmed icon, no countdown label |
| `focus` or `break`, not paused, not idle | icon and `MM:SS` countdown |
| `Paused` true | icon and frozen `MM:SS` countdown |
| `Idle` true | icon and frozen `MM:SS` countdown |

`Paused` and `Idle` are never both true, per `daemon-control` "Idle Publication", so the table has no
ambiguous row.

#### Scenario: Daemon disappears mid-session

- GIVEN an active `focus` session with a running countdown
- WHEN `cadenced` stops
- THEN the indicator returns to its dimmed presentation
- AND the countdown stops advancing

#### Scenario: Daemon appears later

- GIVEN the extension is enabled and the indicator is dimmed because no daemon is present
- WHEN `cadenced` starts and takes the name `dev.ian.Cadence`
- THEN the indicator reflects the daemon's current state without the extension being re-enabled

### Requirement: Break Warning

While `Phase` is `focus` and the session is active and not paused, the indicator MUST apply a
warning style class when the computed remaining time is at most 120 seconds, and MUST remove it
otherwise. The threshold MUST be a named constant in the extension and MUST NOT be read from
configuration in this change.

The warning MUST NOT be applied during `break`, while paused, while idle, while `Tier` is `T3`, or
while no session is active. A warning exists to catch the user's attention before a break begins.
While the daemon reports the user idle, there is no attention to catch and the remaining time is
frozen. While presenting, no break will begin (`session-timer` "Tier Gating"), and the top bar is
visible to the audience of a full-screen share.

#### Scenario: Crossing the threshold

- GIVEN an active unpaused `focus` session with 121 seconds remaining
- WHEN one tick elapses and 120 seconds remain
- THEN the warning style class is applied to the indicator

#### Scenario: Cleared on the break transition

- GIVEN the warning style class is applied in `focus`
- WHEN the daemon publishes `Phase` = `break`
- THEN the warning style class is removed

#### Scenario: Not applied while paused

- GIVEN an active `focus` session paused with 60 seconds remaining
- WHEN the indicator renders
- THEN the warning style class is not applied

#### Scenario: Not applied while idle

- GIVEN an active `focus` session with `Idle` true and 60 seconds remaining
- WHEN the indicator renders
- THEN the warning style class is not applied

#### Scenario: Not applied while presenting

- GIVEN an active `focus` session with `Tier` = `T3` and 60 seconds remaining
- WHEN the indicator renders
- THEN the warning style class is not applied

#### Scenario: Removed when presenting starts

- GIVEN the warning style class is applied in `focus`
- WHEN the daemon publishes `Tier` = `T3`
- THEN the warning style class is removed

### Requirement: Control Actions

The indicator menu MUST offer actions invoking `StartSession`, `StopSession`, `Pause`, `Resume`, and
`SkipBreak` on `dev.ian.Cadence1`. Every D-Bus call MUST be asynchronous. The extension MUST NOT
make a synchronous D-Bus call, which would block the Shell's main loop.

The extension MUST NOT apply a local state change in anticipation of a method's effect; the
resulting state MUST arrive via `PropertiesChanged`.

Menu sensitivity MUST be derived from `Paused`, never from `Idle`. `Pause` MUST remain available
while `Idle` is true, and `Resume` MUST remain unavailable, because an idle freeze is not a pause:
the user never requested it, and `Resume` against a session that was never paused is not a
meaningful action.

#### Scenario: Pause round trip

- GIVEN an active unpaused `focus` session
- WHEN the user activates the pause action
- THEN `Pause` is called asynchronously on the daemon
- AND the frozen countdown is rendered only after the daemon publishes `Paused` = true

#### Scenario: Action with no daemon

- GIVEN the daemon is not on the bus
- WHEN a control action would otherwise be available
- THEN no method call is attempted and no error is raised into the Shell log

#### Scenario: Menu during an idle window

- GIVEN an active `focus` session with `Idle` true and `Paused` false
- WHEN the user opens the indicator menu
- THEN `Pause` is offered
- AND `Resume` is not offered

### Requirement: Connection Lifecycle

The extension MUST watch the bus name `dev.ian.Cadence` and construct its proxy asynchronously when
the name appears. It MUST tear down the proxy when the name vanishes, and MUST tolerate the daemon
appearing, vanishing, and reappearing repeatedly within one enabled session.

#### Scenario: Daemon restart

- GIVEN an enabled extension connected to a running daemon
- WHEN the daemon exits and is restarted by systemd
- THEN the indicator returns to its dimmed presentation while the name is gone
- AND reconnects and resumes rendering when the name reappears

### Requirement: Teardown Hygiene

On `disable()` the extension MUST remove its timeout source via `GLib.Source.remove`, disconnect
every connected signal handler, unwatch the bus name via `Gio.bus_unwatch_name`, destroy the
indicator, and null every retained reference.

No timeout source, signal handler, proxy, or actor may outlive `disable()`, including when the
extension is disabled while the daemon is absent or mid-reconnect.

#### Scenario: Enable and disable cycle leaves nothing behind

- GIVEN the extension has been enabled and connected to a running daemon
- WHEN it is disabled
- THEN no timeout source remains registered
- AND no proxy or signal handler is retained
- AND the Shell log records no error from the extension

#### Scenario: Disabled while disconnected

- GIVEN the extension is enabled and the daemon is not on the bus
- WHEN it is disabled
- THEN the name watch is removed and no source or handler remains
