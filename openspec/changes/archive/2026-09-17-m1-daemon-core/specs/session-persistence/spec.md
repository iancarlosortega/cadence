# session-persistence Specification

## Purpose

Durable session state across daemon restart and suspend.

## Requirements

### Requirement: Durable State

State MUST persist elapsed-in-phase, never an absolute deadline. It MUST be written on every transition, before suspend, and at least every 60s.

#### Scenario: Restart resumes position

- GIVEN `focus` with 30m elapsed persisted
- WHEN the daemon restarts
- THEN it resumes `focus` within 60s of 30m elapsed

#### Scenario: No session to resume

- GIVEN no persisted state
- WHEN the daemon starts
- THEN no session is active
