# session-persistence Specification

## Purpose

Durable session state across daemon restart and suspend.

## Requirements

### Requirement: Durable State

State MUST persist elapsed-in-phase, never an absolute deadline. It MUST be written on every transition, before suspend, and at least every 60s.

A held break MUST survive a restart: whether the break is held, its prompt count, and the held time
elapsed since the last prompt, stored as an elapsed duration and never as an absolute instant.

#### Scenario: Restart resumes position

- GIVEN `focus` with 30m elapsed persisted
- AND the daemon was down for less than the idle-credit threshold
- WHEN the daemon restarts
- THEN it resumes `focus` within 60s of 30m elapsed

#### Scenario: No session to resume

- GIVEN no persisted state
- WHEN the daemon starts
- THEN no session is active

#### Scenario: Restart keeps a held break

- GIVEN a held break with two prompts persisted
- AND the daemon was down for less than the idle-credit threshold
- WHEN the daemon restarts
- THEN the break is still held with two prompts

### Requirement: Downtime Is Time Away

Time during which the daemon was not running MUST be treated as idle time of the same duration, using the same thresholds as suspend (`session-timer`, "Suspend Is Time Away"). Downtime MUST NOT be charged to the current phase as elapsed work.

The duration is measured from the persisted `LastObserved` to the instant the daemon resumes. Because that value is written at most every 60s, the measured absence MAY exceed the true absence by up to 60s, which is within the tolerance the Durable State requirement already allows.

#### Scenario: Long downtime credits a break

- GIVEN `focus` with 12m elapsed persisted
- WHEN the daemon is stopped for 1h and started again
- THEN the phase is `focus` with 0m elapsed

#### Scenario: Short downtime does not credit

- GIVEN `focus` with 12m elapsed persisted
- WHEN the daemon is stopped for 90s and started again
- THEN elapsed is 12m

#### Scenario: Downtime and suspend agree

- GIVEN two sessions in the same phase with the same elapsed
- WHEN one is absent for a given duration as daemon downtime and the other as a suspend
- THEN both reach the same phase with the same elapsed

#### Scenario: Overnight shutdown does not open on a break

- GIVEN an active `focus` session when the machine is shut down
- WHEN the machine is booted the next morning
- THEN the session is a fresh `focus`, not a `break`
