# session-persistence Specification (delta)

## MODIFIED Requirements

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
