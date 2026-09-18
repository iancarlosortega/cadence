# daemon-control Specification

## Purpose

D-Bus interface and CLI for controlling a session.

## Requirements

### Requirement: Control Surface

The daemon MUST expose `StartSession`, `StopSession`, `Pause`, `Resume`, `SkipBreak` on the session bus, and properties `SessionActive`, `Phase`, `PhaseEndsAt`, `RemainingSeconds`, `Paused`, `Tier`.

#### Scenario: Start then inspect

- GIVEN no active session
- WHEN `StartSession` is called
- THEN `SessionActive` is true and `Phase` is `focus`

#### Scenario: Start when already active

- GIVEN an active session
- WHEN `StartSession` is called
- THEN the existing session is unchanged

### Requirement: Change Notification

Property changes MUST emit `PropertiesChanged` on transitions only, and MUST NOT emit per second.

#### Scenario: No per-second traffic

- GIVEN an active unpaused `focus`
- WHEN 60s pass with no transition
- THEN no `PropertiesChanged` is emitted
