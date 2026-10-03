# daemon-control Specification

## Purpose

D-Bus interface and CLI for controlling a session.

## Requirements

### Requirement: Control Surface

The daemon MUST expose `StartSession`, `StopSession`, `Pause`, `Resume`, `SkipBreak` on the session bus, and properties `SessionActive`, `Phase`, `PhaseEndsAt`, `RemainingSeconds`, `Paused`, `Tier`, `Idle`.

#### Scenario: Start then inspect

- GIVEN no active session
- WHEN `StartSession` is called
- THEN `SessionActive` is true and `Phase` is `focus`

#### Scenario: Start when already active

- GIVEN an active session
- WHEN `StartSession` is called
- THEN the existing session is unchanged

#### Scenario: Idle is published from the first connection

- GIVEN a client that has just connected to the daemon
- WHEN it reads the property set without waiting for a signal
- THEN `Idle` is present and carries the daemon's current idle condition

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

### Requirement: Idle Publication

The daemon MUST publish a read-only boolean `Idle`, true exactly while the user has been idle at
least the configured pause threshold, the session is active and not paused, and the phase is
`focus`. A `break` does not freeze (`session-timer`, "Idle Credit"), so `Idle` MUST be false
throughout one.

While `Idle` is true the daemon MUST publish `PhaseEndsAt` as `0` and MUST publish
`RemainingSeconds` as the remaining time frozen at the instant the window opened. This is the same
freeze convention already used for `Paused`, and it exists because the correct deadline advances
continuously across an idle window, so no single published deadline can remain true for its
duration.

`Paused` MUST take precedence over `Idle`. While `Paused` is true, `Idle` MUST be false, regardless
of input activity, so that exactly one frozen-interval condition is ever published at a time.

`Idle` MUST NOT be conflated with `Paused` on the wire. They are distinct properties because they
authorize different user actions — see `panel-indicator` "Control Actions".

#### Scenario: Entering an idle window

- GIVEN an active unpaused `focus` session with 12 minutes remaining
- WHEN the user's idle time reaches the pause threshold
- THEN `Idle` is published as true
- AND `PhaseEndsAt` is published as `0`
- AND `RemainingSeconds` is published as 12 minutes

#### Scenario: Leaving an idle window

- GIVEN an idle window opened with 12 minutes remaining
- WHEN the user becomes active again before the credit threshold
- THEN `Idle` is published as false
- AND `PhaseEndsAt` is published as the return instant plus 12 minutes
- AND elapsed time is unchanged from when the window opened

#### Scenario: Pause during an idle window

- GIVEN an active `focus` session with `Idle` true
- WHEN `Pause` is invoked
- THEN `Paused` is published as true
- AND `Idle` is published as false

#### Scenario: Idle during a break

- GIVEN an active `break`
- WHEN the user is idle past the pause threshold
- THEN `Idle` is false
- AND `PhaseEndsAt` continues to be published as a live deadline

#### Scenario: Idle while no session is active

- GIVEN no active session
- WHEN the user is idle past the pause threshold
- THEN `Idle` is false
- AND no `PropertiesChanged` is emitted on account of idle
