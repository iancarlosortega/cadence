# daemon-control Specification (delta)

## MODIFIED Requirements

### Requirement: Change Notification

A published property MUST emit `PropertiesChanged` whenever its correct value changes. This includes
`PhaseEndsAt` when the effective deadline moves without a phase change — any path that declines to
charge elapsed wall-clock time to the current phase moves that deadline and MUST republish it.

A tick that changes no published value MUST NOT emit. The daemon MUST NOT emit on a fixed interval.

An idle window is a path that declines to charge elapsed time, so it is governed by the sentence
above. Because the correct `PhaseEndsAt` would otherwise advance continuously for the whole window,
the daemon MUST satisfy this requirement by publishing the window's boundaries and freezing the
deadline between them, per "Idle Publication" — not by republishing a moving deadline per tick.

A tick on which only the tier changes is a change to a published value and MUST emit. Clients act
on `Tier` without waiting for a transition: the panel withholds its warning while presenting
(`panel-indicator` "Break Warning").

#### Scenario: No per-second traffic

- GIVEN an active unpaused `focus`
- WHEN 60s pass with no transition
- THEN no `PropertiesChanged` is emitted

#### Scenario: Short suspend republishes the deadline

- GIVEN an active unpaused `focus` session
- WHEN the system suspends for less than the idle-credit threshold and resumes
- THEN elapsed time is unchanged, per `session-timer` "Suspend Is Time Away"
- AND `PropertiesChanged` is emitted
- AND `PhaseEndsAt` is republished as the resume instant plus the remaining time

#### Scenario: An idle window emits exactly twice

- GIVEN an active unpaused `focus` session
- WHEN the user goes idle past the pause threshold and later returns, without reaching the credit threshold
- THEN exactly one `PropertiesChanged` is emitted as the window opens
- AND exactly one `PropertiesChanged` is emitted as the window closes
- AND no `PropertiesChanged` is emitted by any tick in between

#### Scenario: A client never drifts across suspends, idle windows and transitions

- GIVEN a client deriving its countdown as `PhaseEndsAt` minus the current time, or from
  `RemainingSeconds` while the daemon reports a frozen interval
- WHEN any sequence of suspends, idle windows and phase transitions occurs
- THEN the client's derived remaining time equals the daemon's elapsed-derived remaining time
- AND the two do not diverge by a constant offset

#### Scenario: A tier change alone emits once

- GIVEN an active unpaused `focus` session at tier `T0`
- WHEN screen sharing starts and the next tick observes `T3`, with no other change
- THEN exactly one `PropertiesChanged` is emitted, carrying `Tier`
- AND no `PropertiesChanged` is emitted by the following ticks while the tier stays `T3`
