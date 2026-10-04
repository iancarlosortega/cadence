# daemon-control Specification (delta)

## MODIFIED Requirements

### Requirement: Control Surface

The daemon MUST expose `StartSession`, `StopSession`, `Pause`, `Resume`, `SkipBreak` on the session bus, and properties `SessionActive`, `Phase`, `PhaseEndsAt`, `RemainingSeconds`, `Paused`, `Tier`, `Idle`, `Hold`, `Prompts`.

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

### Requirement: Hold Publication

The daemon MUST publish a read-only string `Hold` and a read-only integer `Prompts`.

`Hold` MUST be `none` unless a break is held (`session-timer` "Tier Gating"). It MUST be `prompt`
while a held break has not passed its prompt cap, and `pill` once it has.

`Prompts` MUST be the number of prompts that have happened for the current break, and `0` when no
break is owed. It changes exactly once per prompt, so a client observes each new prompt as a change
of `Prompts` even when `Hold` stays `prompt`.

While `Hold` is not `none`, the daemon MUST publish `PhaseEndsAt` as `0` and `RemainingSeconds` as
the break's remaining time frozen at the hold. This is the same freeze convention as `Paused` and
`Idle`.

The ticks between prompts MUST NOT emit. Entering a hold, each prompt, entering the pill stage, and
lifting or releasing a hold are changes to published values and are each published in one
`PropertiesChanged`.

#### Scenario: Entering a hold

- GIVEN an active `focus` session reaching its deadline under tier `T2`
- WHEN the break becomes held
- THEN one `PropertiesChanged` carries `Phase` = `break`, `Hold` = `prompt`, `Prompts` = 1 and `PhaseEndsAt` = 0

#### Scenario: A further prompt

- GIVEN `Hold` = `prompt` and `Prompts` = 1
- WHEN the retry interval passes under `T2`
- THEN one `PropertiesChanged` carries `Prompts` = 2
- AND no `PropertiesChanged` is emitted by the ticks in between

#### Scenario: Entering the pill stage

- GIVEN `Hold` = `prompt` and `Prompts` = 3
- WHEN the retry interval passes under `T2`
- THEN one `PropertiesChanged` carries `Hold` = `pill`
- AND `Prompts` stays 3

#### Scenario: Lifting a hold

- GIVEN `Hold` = `prompt` with 10 minutes of break remaining
- WHEN the tier drops to `T0`
- THEN one `PropertiesChanged` carries `Hold` = `none` and `PhaseEndsAt` = the lift instant plus 10 minutes
