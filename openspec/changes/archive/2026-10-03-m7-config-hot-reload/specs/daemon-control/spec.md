# daemon-control Specification (delta)

## MODIFIED Requirements

### Requirement: Control Surface

The daemon MUST expose `StartSession`, `StopSession`, `Pause`, `Resume`, `SkipBreak` on the session bus, and properties `SessionActive`, `Phase`, `PhaseEndsAt`, `RemainingSeconds`, `Paused`, `Tier`, `Idle`, `Hold`, `Prompts`, `Config`.

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

#### Scenario: Hold is published from the first connection

- GIVEN a client that has just connected to the daemon
- WHEN it reads the property set without waiting for a signal
- THEN `Hold` and `Prompts` are present and carry the daemon's current hold state

## ADDED Requirements

### Requirement: Configuration Publication

The daemon MUST publish a read-only property `Config` of type `a{si}` mapping each configuration
key, as written in the file with its section (for example `timer.focus_minutes`), to its active
value. It MUST carry every key, including those at their defaults, and MUST be present from the
first connection.

`Config` MUST be republished in one `PropertiesChanged` when a reload changes the active values,
and MUST NOT be emitted otherwise. It is read-only: the file is the configuration's only writer.

#### Scenario: Config is published from the first connection

- GIVEN no config file
- WHEN a client reads `Config`
- THEN it maps `timer.focus_minutes` to 50 and `camera.prompt_limit` to 3, and carries all six keys

#### Scenario: A reload republishes Config

- GIVEN `Config` maps `timer.focus_minutes` to 50
- WHEN the file is saved with `focus_minutes = 40`
- THEN one `PropertiesChanged` carries `Config` with `timer.focus_minutes` = 40
