# daemon-control Specification (delta)

## MODIFIED Requirements

### Requirement: Change Notification

A published property MUST emit `PropertiesChanged` whenever its correct value changes. This includes
`PhaseEndsAt` when the effective deadline moves without a phase change — any path that declines to
charge elapsed wall-clock time to the current phase moves that deadline and MUST republish it.

A tick that changes no published value MUST NOT emit. The daemon MUST NOT emit on a fixed interval.

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

#### Scenario: A client never drifts across suspends and transitions

- GIVEN a client deriving its countdown as `PhaseEndsAt` minus the current time
- WHEN any sequence of suspends and phase transitions occurs
- THEN the client's derived remaining time equals the daemon's elapsed-derived remaining time
- AND the two do not diverge by a constant offset

The requirement above is stated generally and deliberately so. Idle gating is not implemented —
`IdleSource` is stubbed to zero, so the idle-pause branch of the tick reducer is unreachable and
there is no idle path for this requirement to govern today. When M4 wires real idle detection it
MUST satisfy this requirement for the paths it makes reachable, which requires deciding what a
client displays while the user is idle, because the effective deadline advances continuously
throughout an idle window rather than moving once at its edges.
