# break-overlay Specification (delta)

## MODIFIED Requirements

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
