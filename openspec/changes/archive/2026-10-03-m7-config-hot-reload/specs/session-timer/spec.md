# session-timer Specification (delta)

## MODIFIED Requirements

### Requirement: Tier Gating

Break start MUST consult the tier source. The tier is what other people can see: `T3` presenting
(the screen is being cast), `T2` on camera, `T1` listening (a microphone stream is open), `T0` free.
When several apply, the highest wins.

`T3` MUST skip the break silently. At the focus deadline under `T3`, the phase MUST stay `focus`
with 0 elapsed, and no break is started. A break that is in progress or held when `T3` begins MUST
end at that tick, the phase becoming `focus` with 0 elapsed, so that nothing break-related is
visible while presenting.

`T2` MUST hold the break. At the focus deadline under `T2`, the phase MUST become `break` in a held
state. A break that is running when `T2` begins MUST become held at that tick. While held, the
break's elapsed time MUST NOT advance, so the whole break is still owed when the hold lifts.

A held break prompts the user. The first prompt happens when the hold begins. While the tier stays
`T2`, a further prompt MUST happen each time the configured prompt interval (default 5 minutes)
of held, unpaused time passes, up to the configured prompt limit (default 3)
(`daemon-configuration`, `[camera]`). A changed interval or limit applies from the next tick; a
held break whose prompt count already meets a lowered limit enters its pill stage when the
interval next passes. When the retry interval passes after the last prompt, the hold
MUST enter its pill stage, and no further prompts happen. The prompt count MUST NOT reset when a
hold lifts and is re-entered within the same break, so a camera that turns off and on cannot
escape the cap.

When the tier drops below `T2` while a break is held, the hold MUST lift at that tick and the break
MUST run from the remaining time it was held at.

A held break MUST be released, with its prompt count cleared, on every path that ends or credits
the break: skipping it, `T3`, idle credit, suspend credit, downtime credit, and stopping the
session.

`T0` and `T1` MUST start the break normally. `T1` behaves as `T0` because the break overlay has
nothing to mute or shorten.

Any screencast counts as `T3`, including a local screen recording, because the daemon cannot tell
who will see it.

The tier MUST be sampled on every tick while a session is active and not paused. Outside that, the
tier is not sampled and nothing is emitted on its account.

#### Scenario: T0 starts break

- GIVEN `focus` reaching 0s and tier `T0`
- WHEN the phase would change
- THEN the phase becomes `break`

#### Scenario: T1 starts break

- GIVEN `focus` reaching 0s and tier `T1`
- WHEN the phase would change
- THEN the phase becomes `break`

#### Scenario: T2 holds the break

- GIVEN `focus` reaching 0s and tier `T2`
- WHEN the phase would change
- THEN the phase becomes `break`, held, with 0 elapsed
- AND one prompt has happened

#### Scenario: A held break does not advance

- GIVEN a held break with 0 elapsed
- WHEN 4 minutes pass with tier `T2`
- THEN the break still has 0 elapsed
- AND no further prompt has happened

#### Scenario: Prompts repeat every 5 minutes up to the cap

- GIVEN a break that became held under `T2`
- WHEN the tier stays `T2` for 10 minutes
- THEN three prompts have happened
- AND the hold is not yet in its pill stage

#### Scenario: Past the cap the hold becomes a pill

- GIVEN a held break with three prompts
- WHEN the tier stays `T2` for a further 5 minutes
- THEN the hold is in its pill stage
- AND no fourth prompt happens

#### Scenario: The camera turning off starts the break

- GIVEN a held break with 0 elapsed, in any stage
- WHEN a tick observes tier `T0` or `T1`
- THEN the break is no longer held
- AND its elapsed time advances from that tick

#### Scenario: The camera turning on mid-break holds it

- GIVEN a running break with 4 minutes elapsed
- WHEN a tick observes tier `T2`
- THEN the break is held with 4 minutes elapsed
- AND a prompt happens

#### Scenario: A flapping camera cannot escape the cap

- GIVEN a held break with three prompts that lifted when the camera turned off
- WHEN the camera turns on again within the same break
- THEN the break is held again
- AND the prompt count is still three

#### Scenario: Skipping a held break

- GIVEN a held break
- WHEN the break is skipped
- THEN the phase is `focus` with 0 elapsed
- AND the break is no longer held and its prompt count is cleared

#### Scenario: Idle credit releases a held break

- GIVEN a held break
- WHEN idle reaches the credit threshold
- THEN the phase is `focus` with 0 elapsed
- AND the break is no longer held

#### Scenario: T3 skips the break silently

- GIVEN `focus` reaching 0s and tier `T3`
- WHEN the phase would change
- THEN the phase is `focus` with 0 elapsed
- AND no break is started

#### Scenario: Presenting during a break ends it

- GIVEN an active or held `break`
- WHEN a tick observes tier `T3`
- THEN the phase is `focus` with 0 elapsed
- AND the break is no longer held

#### Scenario: The highest tier wins

- GIVEN the screen is being cast, the camera is open and a microphone stream is running
- WHEN the tier is sampled
- THEN the tier is `T3`

#### Scenario: A configured interval and limit

- GIVEN `prompt_every_minutes = 2` and `prompt_limit = 2`, and a break that became held under `T2`
- WHEN the tier stays `T2` for 4 minutes
- THEN two prompts have happened and the hold is in its pill stage
