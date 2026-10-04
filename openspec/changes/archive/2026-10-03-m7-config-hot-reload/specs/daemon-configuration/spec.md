# daemon-configuration Specification (delta)

## MODIFIED Requirements

### Requirement: Defaults And Validation

Missing config MUST yield defaults: focus 50m, break 10m, idle pause 3m, idle credit 10m, camera
prompt interval 5m, camera prompt limit 3. Unknown keys and non-positive values MUST be rejected
with a diagnostic naming the key. A key set to `0` is non-positive and MUST be rejected; only an
absent key takes its default.

The configuration keys are `[timer] focus_minutes`, `break_minutes`; `[idle] pause_after_minutes`,
`credit_break_after_minutes`; and `[camera] prompt_every_minutes`, `prompt_limit`.

#### Scenario: No config file

- GIVEN no config file
- WHEN the daemon starts
- THEN focus is 50m and break is 10m
- AND the camera prompt interval is 5m and the limit is 3

#### Scenario: Unknown key rejected

- GIVEN a config containing `focus_mins = 45`
- WHEN the daemon loads it
- THEN startup fails naming `focus_mins`

#### Scenario: Zero rejected

- GIVEN a config containing `focus_minutes = 0`
- WHEN the daemon loads it
- THEN startup fails naming `timer.focus_minutes`

#### Scenario: Camera policy configured

- GIVEN a config containing `[camera]` with `prompt_every_minutes = 2` and `prompt_limit = 5`
- WHEN the daemon loads it
- THEN a held break prompts every 2 minutes, up to 5 prompts

## ADDED Requirements

### Requirement: Live Reload

The daemon MUST notice a change to the config file within one tick interval of it being saved, and
apply the new values to the running session without a restart.

A change applies to the phase in progress. The phase keeps the elapsed time it has, and is measured
against the new length from the next tick. A phase whose new length is at or below its elapsed time
ends on the next tick. A paused phase keeps its elapsed time, and its frozen remaining time is
recomputed from the new length.

A file that fails to load on reload (unknown key, non-positive value, malformed TOML, unreadable)
MUST leave the active configuration unchanged and MUST be logged once, naming the problem. The next
change to the file is loaded afresh.

A deleted file MUST revert the configuration to the defaults, as a missing file does at startup.

A reload that yields the same effective values as the active configuration changes nothing and
emits nothing.

#### Scenario: Saving a new focus length

- GIVEN an active `focus` with 20m elapsed and focus 50m
- WHEN the file is saved with `focus_minutes = 30`
- THEN within one tick interval the phase has 20m elapsed of 30m
- AND `PhaseEndsAt` is republished as 10m from that tick

#### Scenario: Shortening below the time worked

- GIVEN an active `focus` with 20m elapsed
- WHEN the file is saved with `focus_minutes = 15`
- THEN the phase becomes `break` on the next tick

#### Scenario: A paused phase keeps its elapsed time

- GIVEN a paused `focus` with 20m elapsed of 50m
- WHEN the file is saved with `focus_minutes = 30`
- THEN `RemainingSeconds` is republished as 10 minutes
- AND resuming continues from 20m elapsed of 30m

#### Scenario: An invalid file is ignored

- GIVEN the active configuration has focus 50m
- WHEN the file is saved with `focus_minutes = 0`
- THEN the active configuration still has focus 50m
- AND one log line names `timer.focus_minutes`
- AND no `PropertiesChanged` is emitted on account of the file

#### Scenario: Fixing the file applies it

- GIVEN an invalid file was ignored
- WHEN the file is saved again with `focus_minutes = 40`
- THEN the active configuration has focus 40m

#### Scenario: Deleting the file reverts to defaults

- GIVEN the active configuration has focus 30m from the file
- WHEN the file is deleted
- THEN within one tick interval the active configuration has focus 50m

#### Scenario: Saving without a change emits nothing

- GIVEN the active configuration matches the file
- WHEN the file is saved again with the same values
- THEN no `PropertiesChanged` is emitted
