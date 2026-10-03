# session-timer Specification

## Purpose

Phase model and transition rules for a cadence session.

## Requirements

### Requirement: Phase Cycle

An active session MUST alternate `focus` and `break`. Durations MUST come from configuration.

#### Scenario: Focus elapses

- GIVEN an active session in `focus` with 0s remaining
- WHEN the clock advances
- THEN the phase becomes `break`

#### Scenario: Break elapses

- GIVEN an active session in `break` with 0s remaining
- WHEN the clock advances
- THEN the phase becomes `focus`

### Requirement: Idle Credit

The `focus` timer MUST NOT advance while the user is idle beyond the pause threshold. Idle reaching
the credit threshold MUST credit a break and restart `focus`.

A `break` MUST continue to advance while the user is idle. A break is time away from the desk by
design, so idling through one is its intended use: the break MUST be able to finish while the user
is away. Freezing it would leave the remainder owed on their return, making a break taken properly
the one that never completes.

Idle MUST be observed from a real input-activity source. A stub that reports constant zero satisfies
none of the scenarios below.

Entering and leaving an idle window are observable changes and MUST be published, per
`daemon-control` "Idle Publication". The ticks inside a window MUST NOT be.

A single idle window MUST credit at most one break. Observed idle time grows for as long as the user
is away and is not reset by crediting a break, so the credit MUST be conditioned on the window not
having credited already — not on the idle reading alone. Crediting per tick would reset the phase,
write state and emit a signal every tick for the whole absence.

#### Scenario: Short idle pauses

- GIVEN `focus` with 20m elapsed
- WHEN idle exceeds the pause threshold by 1m
- THEN elapsed remains 20m
- AND the idle window has been published as open

#### Scenario: Long idle credits

- GIVEN `focus` with 20m elapsed
- WHEN idle reaches the credit threshold
- THEN the phase is `focus` with 0m elapsed

#### Scenario: A long absence credits exactly one break

- GIVEN `focus` with 20m elapsed
- WHEN the user stays idle for one hour, well past the credit threshold
- THEN exactly one break is credited for that absence
- AND no state is persisted and no `PropertiesChanged` is emitted by the ticks that follow the credit

#### Scenario: A break advances while the user is idle

- GIVEN an active `break` with 2m elapsed
- WHEN the user is idle past the pause threshold for a further minute
- THEN elapsed is 3m
- AND no idle window is opened
- AND no `PropertiesChanged` is emitted

#### Scenario: A break completes while the user is away

- GIVEN an active `break` with one minute remaining
- WHEN the user is idle past the pause threshold and that minute passes
- THEN the phase becomes `focus`

#### Scenario: A second absence credits again

- GIVEN a break was credited for an earlier idle window
- WHEN the user returns, works, and later goes idle past the credit threshold again
- THEN a second break is credited for the second absence

#### Scenario: Returning before the credit threshold resumes the same phase

- GIVEN `focus` with 20m elapsed and an open idle window
- WHEN the user becomes active again before reaching the credit threshold
- THEN the phase is still `focus` with 20m elapsed
- AND the timer advances again from the moment of return

### Requirement: Pause Semantics

Pause MUST store remaining duration, not a deadline. A paused phase MUST NOT expire.

#### Scenario: Paused phase does not expire

- GIVEN `focus` paused with 10m remaining
- WHEN the clock advances 3h
- THEN the phase is still `focus`, paused, 10m remaining

### Requirement: Suspend Is Time Away

System suspend MUST be treated as idle time of the suspended duration, using the same thresholds.

A single absence MUST credit at most one break. Where a period away from the desk is observable both
as a suspend and as input idleness, the daemon MUST NOT credit it twice.

#### Scenario: Long suspend credits a break

- GIVEN `focus` with 12m elapsed
- WHEN the system suspends for 1h and resumes
- THEN the phase is `focus` with 0m elapsed

#### Scenario: Short suspend does not credit

- GIVEN `focus` with 12m elapsed
- WHEN the system suspends for 90s and resumes
- THEN elapsed is 12m

#### Scenario: A suspend is not credited twice

- GIVEN `focus` with 12m elapsed
- WHEN the system suspends past the credit threshold and resumes
- THEN exactly one break is credited
- AND a subsequent tick does not credit a second break on account of the same absence

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

### Requirement: Idle Source Availability

The idle source is provided by the desktop compositor and is not guaranteed to be present. The
daemon MUST tolerate it being unavailable when the daemon starts and becoming unavailable while the
daemon runs.

When the idle source is unavailable the daemon MUST report zero idle. Zero idle is the safe value:
it degrades cadence to charging all elapsed time as work, which is its behavior in every milestone
before this one, rather than freezing a timer or crediting a break that was never earned.

An unavailable idle source MUST NOT terminate the daemon and MUST NOT raise an error to clients.

#### Scenario: Idle source absent at startup

- GIVEN the compositor's idle interface is not on the bus
- WHEN the daemon starts and a session is started
- THEN the daemon runs normally
- AND the timer advances on every tick
- AND `Idle` is false

#### Scenario: Idle source disappears mid-session

- GIVEN an active session with a working idle source
- WHEN the compositor restarts and the interface leaves the bus
- THEN the daemon continues running
- AND idle is reported as zero until the interface returns

#### Scenario: Idle source returns

- GIVEN a running daemon whose idle source is unavailable
- WHEN the compositor's idle interface returns to the bus
- THEN the daemon observes real idle again without being restarted

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
