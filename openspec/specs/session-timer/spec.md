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

The timer MUST NOT advance while the user is idle beyond the pause threshold. Idle reaching the credit threshold MUST credit a break and restart `focus`.

#### Scenario: Short idle pauses

- GIVEN `focus` with 20m elapsed
- WHEN idle exceeds the pause threshold by 1m
- THEN elapsed remains 20m

#### Scenario: Long idle credits

- GIVEN `focus` with 20m elapsed
- WHEN idle reaches the credit threshold
- THEN the phase is `focus` with 0m elapsed

### Requirement: Pause Semantics

Pause MUST store remaining duration, not a deadline. A paused phase MUST NOT expire.

#### Scenario: Paused phase does not expire

- GIVEN `focus` paused with 10m remaining
- WHEN the clock advances 3h
- THEN the phase is still `focus`, paused, 10m remaining

### Requirement: Suspend Is Time Away

System suspend MUST be treated as idle time of the suspended duration, using the same thresholds.

#### Scenario: Long suspend credits a break

- GIVEN `focus` with 12m elapsed
- WHEN the system suspends for 1h and resumes
- THEN the phase is `focus` with 0m elapsed

#### Scenario: Short suspend does not credit

- GIVEN `focus` with 12m elapsed
- WHEN the system suspends for 90s and resumes
- THEN elapsed is 12m

### Requirement: Tier Gating

Break start MUST consult the tier source. `T0` MUST start the break.

#### Scenario: T0 starts break

- GIVEN `focus` reaching 0s and tier `T0`
- WHEN the phase would change
- THEN the phase becomes `break`
