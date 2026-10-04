# camera-prompt Specification

## Purpose

The quiet surfaces that stand in for the break overlay while the user is on camera: a corner panel
that asks, and a pill that remembers.

## ADDED Requirements

### Requirement: Corner Panel

While `Hold` is `prompt`, the extension MUST show a small panel in the top-right corner of the
primary monitor, below the top bar, each time `Prompts` changes to a new value, and MUST remove it
15 seconds later or as soon as `Hold` is no longer `prompt`, whichever comes first.

The panel MUST say that a break is due and that the user is on camera, and MUST offer a Skip action.
Skip MUST call `SkipBreak` on the daemon and MUST NOT dismiss the panel locally; the panel leaves
when the resulting property change reports the break has ended.

The panel MUST NOT take a modal grab and MUST absorb pointer input only within its own bounds, so
the window behind it, typically the call, remains usable.

#### Scenario: A prompt shows the panel

- GIVEN no panel is shown
- WHEN the daemon publishes `Hold` = `prompt` and `Prompts` = 1
- THEN the panel is shown in the top-right corner of the primary monitor within one tick interval

#### Scenario: The panel leaves on its own

- GIVEN the panel was shown for `Prompts` = 1
- WHEN 15 seconds pass with no property change
- THEN the panel is removed

#### Scenario: A new prompt shows it again

- GIVEN the panel was shown for `Prompts` = 1 and has left
- WHEN the daemon publishes `Prompts` = 2
- THEN the panel is shown again

#### Scenario: Skip ends the break

- GIVEN the panel is shown
- WHEN the user activates Skip
- THEN `SkipBreak` is called asynchronously on the daemon
- AND the panel is removed only after the daemon publishes a phase other than `break`

#### Scenario: The rest of the screen stays usable

- GIVEN the panel is shown
- WHEN the user clicks outside its bounds
- THEN the click reaches the window beneath

### Requirement: Pill

While `Hold` is `pill`, the extension MUST show a small persistent indicator in the same corner
saying a break is owed, and MUST remove it as soon as `Hold` is no longer `pill`. The pill offers no
action; skipping remains available from the panel-indicator menu, which offers Skip during a break.

#### Scenario: The pill appears past the cap

- GIVEN `Hold` = `prompt` and `Prompts` = 3
- WHEN the daemon publishes `Hold` = `pill`
- THEN the pill is shown and stays shown

#### Scenario: The camera turning off removes the pill

- GIVEN the pill is shown
- WHEN the daemon publishes `Hold` = `none` with `Phase` = `break`
- THEN the pill is removed
- AND the break overlay appears (`break-overlay` "Overlay Presence")

### Requirement: Prompt Surface Teardown

The panel and the pill MUST be removed when the daemon's bus name disappears and when the extension
is disabled, and any pending panel timeout MUST be cancelled. Neither surface may outlive the
condition that shows it.

#### Scenario: Daemon disappears while the pill is shown

- GIVEN the pill is shown
- WHEN `cadenced` stops
- THEN the pill is removed

#### Scenario: Disabled while the panel is shown

- GIVEN the panel is shown with its timeout pending
- WHEN the extension is disabled
- THEN the panel is removed and the timeout does not fire afterwards
