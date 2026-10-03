# session-timer Specification (delta)

## MODIFIED Requirements

### Requirement: Tier Gating

Break start MUST consult the tier source. The tier is what other people can see: `T3` presenting
(the screen is being cast), `T2` on camera, `T1` listening (a microphone stream is open), `T0` free.
When several apply, the highest wins.

`T3` MUST skip the break silently. At the focus deadline under `T3`, the phase MUST stay `focus`
with 0 elapsed, and no break is started. A break that is in progress when `T3` begins MUST end at
that tick, the phase becoming `focus` with 0 elapsed, so that nothing break-related is visible
while presenting.

`T0`, `T1` and `T2` MUST start the break. `T1` behaves as `T0` because the break overlay has nothing
to mute or shorten. `T2` behaves as `T0` until its own policy is specified.

Any screencast counts as `T3`, including a local screen recording, because the daemon cannot tell
who will see it.

The tier MUST be sampled on every tick while a session is active and not paused. Outside that, the
tier is not sampled and nothing is emitted on its account.

#### Scenario: T0 starts break

- GIVEN `focus` reaching 0s and tier `T0`
- WHEN the phase would change
- THEN the phase becomes `break`

#### Scenario: T1 starts break

- GIVEN `focus` reaching 0s and tier `T1`
- WHEN the phase would change
- THEN the phase becomes `break`

#### Scenario: T2 starts break

- GIVEN `focus` reaching 0s and tier `T2`
- WHEN the phase would change
- THEN the phase becomes `break`

#### Scenario: T3 skips the break silently

- GIVEN `focus` reaching 0s and tier `T3`
- WHEN the phase would change
- THEN the phase is `focus` with 0 elapsed
- AND no break is started

#### Scenario: Presenting during a break ends it

- GIVEN an active `break` with time remaining
- WHEN a tick observes tier `T3`
- THEN the phase is `focus` with 0 elapsed

#### Scenario: The highest tier wins

- GIVEN the screen is being cast, the camera is open and a microphone stream is running
- WHEN the tier is sampled
- THEN the tier is `T3`

## ADDED Requirements

### Requirement: Tier Source Availability

Each tier signal is provided by a system component (the compositor, PipeWire, the kernel's device
files) that is not guaranteed to be present or readable. The daemon MUST tolerate any of them being
unavailable at startup or becoming unavailable while it runs.

An unavailable signal MUST be treated as absent. Absent is the safe value: a broken signal can only
lower the tier, so it can never cause a break to be skipped.

An unavailable signal MUST NOT terminate the daemon and MUST NOT raise an error to clients. Its
unavailability MUST be logged when it changes, not on every tick.

#### Scenario: Every signal unavailable

- GIVEN no screencast service, no PipeWire and no readable video devices
- WHEN a session runs through a focus deadline
- THEN the tier is `T0`
- AND the phase becomes `break`
- AND each unavailable signal is logged once

#### Scenario: A signal returns

- GIVEN a running daemon whose screencast signal is unavailable
- WHEN the compositor's screencast service returns to the bus
- THEN the daemon observes screencasts again without being restarted
