# panel-indicator Specification (delta)

## MODIFIED Requirements

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
