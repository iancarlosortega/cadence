# break-overlay Specification

## Purpose

The full-screen break surface: a cover on the primary monitor while the daemon reports a break, with
one deliberate way out, and with an exit that never depends on the daemon still being there.

## Requirements

### Requirement: Overlay Presence

The extension MUST cover the primary monitor while the daemon reports `break`, `Hold` is `none`,
and the overlay is not suppressed, and MUST remove the cover otherwise. The overlay MUST be placed
above normal windows.

A held break shows no overlay. Its presence is the corner prompt's (`camera-prompt`). When a hold
lifts, the overlay MUST appear within one tick interval, because the break is now running.

The overlay MUST NOT take a modal grab. Mouse input within its region is absorbed; keyboard
navigation such as window switching MUST continue to work.

#### Scenario: Break begins

- GIVEN an active `focus` session and no overlay
- WHEN the daemon publishes `Phase` = `break` with `Hold` = `none`
- THEN the primary monitor is covered within one tick interval
- AND mouse clicks over the covered area do not reach windows beneath it

#### Scenario: Held break shows no overlay

- GIVEN an active `focus` session and no overlay
- WHEN the daemon publishes `Phase` = `break` with `Hold` = `prompt`
- THEN the primary monitor is not covered

#### Scenario: Hold placed mid-break removes the overlay

- GIVEN the overlay is covering the primary monitor
- WHEN the daemon publishes `Hold` = `prompt`
- THEN the cover is removed within one tick interval

#### Scenario: Hold lifting shows the overlay

- GIVEN a held break and no overlay
- WHEN the daemon publishes `Hold` = `none` while `Phase` stays `break`
- THEN the primary monitor is covered within one tick interval

#### Scenario: Keyboard remains usable

- GIVEN the overlay is covering the primary monitor
- WHEN the user switches windows with the keyboard
- THEN the switch succeeds

#### Scenario: Only the primary monitor

- GIVEN more than one monitor and an overlay covering the primary
- WHEN a break is in progress
- THEN other monitors are not covered

### Requirement: Fullscreen Suppression

Whether the overlay is suppressed MUST be decided once, at the moment the break begins, from whether
the primary monitor is in fullscreen. A suppressed break MUST show no overlay for its whole duration,
and MUST otherwise run and complete normally.

#### Scenario: Break begins during fullscreen

- GIVEN the primary monitor is in fullscreen
- WHEN the daemon publishes `Phase` = `break`
- THEN no overlay appears
- AND the break runs to completion

#### Scenario: Fullscreen ends mid-break

- GIVEN a break that was suppressed at its start
- WHEN fullscreen ends before that break is over
- THEN no overlay appears for the remainder of that break

#### Scenario: Next break is evaluated afresh

- GIVEN a break that was suppressed
- WHEN a later break begins and the primary monitor is not in fullscreen
- THEN the overlay appears for that break

### Requirement: Self-Owned Exit

The overlay MUST dismiss itself on whichever of these occurs first, and MUST NOT depend on the daemon
to end it:

1. The remaining time it derives locally from `PhaseEndsAt` reaches zero.
2. A property change reports a phase other than `break`.
3. The daemon's bus name is no longer present.
4. The extension is disabled.

#### Scenario: Daemon disappears while the overlay is up

- GIVEN the overlay is covering the primary monitor
- WHEN `cadenced` stops and its bus name vanishes
- THEN the overlay is dismissed
- AND the screen is usable without any further action

#### Scenario: Break ends normally

- GIVEN the overlay is covering the primary monitor
- WHEN the daemon publishes `Phase` = `focus`
- THEN the overlay is dismissed

#### Scenario: No property change arrives

- GIVEN the overlay is covering the primary monitor
- AND no property change has been received since the break began
- WHEN the locally derived remaining time reaches zero
- THEN the overlay is dismissed

### Requirement: Hold To Skip

The overlay MUST offer a control that ends the break when held continuously for a fixed duration, and
MUST show the progress of that hold. Completing the hold MUST call `SkipBreak` on the daemon and
dismiss the overlay. Releasing or leaving the control before completion MUST cancel without calling
the daemon.

The overlay MUST NOT end the break itself; ending it is the daemon's decision, requested by
`SkipBreak` and observed through the resulting property change.

#### Scenario: Hold to completion

- GIVEN the overlay is covering the primary monitor
- WHEN the skip control is held for the full duration
- THEN `SkipBreak` is called on the daemon
- AND the daemon reports `Phase` = `focus`
- AND the overlay is dismissed

#### Scenario: Released early

- GIVEN the skip control has been held for less than the full duration
- WHEN it is released
- THEN `SkipBreak` is not called
- AND the progress indication returns to its starting state
- AND the overlay remains

#### Scenario: Pointer leaves the control

- GIVEN the skip control is being held
- WHEN the pointer leaves the control
- THEN the hold is cancelled as if released

### Requirement: Monitor Changes

The overlay MUST follow the primary monitor across monitor configuration changes while it is
displayed.

#### Scenario: Primary monitor changes while covered

- GIVEN the overlay is covering the primary monitor
- WHEN the monitor configuration changes and a different monitor becomes primary
- THEN the overlay covers the new primary monitor

### Requirement: Overlay Teardown

On `disable()` the extension MUST remove the overlay actor, cancel any hold in progress, remove any
timeout or transition it created, and disconnect every signal it connected. Teardown MUST be safe to
run when no overlay is displayed and when a hold is in progress.

#### Scenario: Disabled while covered

- GIVEN the overlay is covering the primary monitor
- WHEN the extension is disabled
- THEN the overlay is removed
- AND no error is raised into the Shell log

#### Scenario: Disabled mid-hold

- GIVEN the skip control is being held
- WHEN the extension is disabled
- THEN the overlay is removed
- AND `SkipBreak` is not called
- AND no timeout or transition outlives the extension
