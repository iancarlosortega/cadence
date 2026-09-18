# daemon-configuration Specification

## Purpose

TOML configuration, defaults, and validation.

## Requirements

### Requirement: Defaults And Validation

Missing config MUST yield defaults: focus 50m, break 10m, idle pause 3m, idle credit 10m. Unknown keys and non-positive durations MUST be rejected with a diagnostic naming the key.

#### Scenario: No config file

- GIVEN no config file
- WHEN the daemon starts
- THEN focus is 50m and break is 10m

#### Scenario: Unknown key rejected

- GIVEN a config containing `focus_mins = 45`
- WHEN the daemon loads it
- THEN startup fails naming `focus_mins`
