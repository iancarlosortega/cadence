# panel-indicator Specification (delta)

## MODIFIED Requirements

### Requirement: Countdown Derivation

While a session is active and neither paused, idle nor held, the displayed countdown MUST be
recomputed on every tick as `PhaseEndsAt` minus the current wall-clock time, clamped at zero. The
extension MUST NOT decrement a locally held counter.

While `Paused` is true, the displayed countdown MUST be taken from `RemainingSeconds` and MUST NOT
be derived from `PhaseEndsAt`, which the daemon publishes as `0` while paused.

While `Idle` is true, the displayed countdown MUST be taken from `RemainingSeconds` and MUST NOT be
derived from `PhaseEndsAt`, which the daemon publishes as `0` while idle. The extension MUST NOT
infer idleness from its own input monitoring; `Idle` is the daemon's to declare.

While `Hold` is not `none`, the displayed countdown MUST be taken from `RemainingSeconds` and MUST
NOT be derived from `PhaseEndsAt`, which the daemon publishes as `0` while a break is held.

#### Scenario: Display self-corrects after suspend

- GIVEN an active `focus` session
- WHEN the machine suspends and resumes
- THEN the next tick renders `PhaseEndsAt` minus now
- AND the rendered value equals the daemon's elapsed-derived remaining
- AND that offset stays constant at zero across repeated samples

The comparison MUST be against the daemon's elapsed-derived remaining
(`elapsed_in_phase` plus time since `LastObserved`), NOT against the published `RemainingSeconds`
property. `RemainingSeconds` is republished only on transitions, so between transitions it is a
stale snapshot and a correct client will appear to diverge from it by exactly the elapsed time.

This scenario depends on the daemon republishing `PhaseEndsAt` whenever it changes the effective
deadline, which `daemon-control` "Change Notification" requires. Verified 2026-09-18 on a real
suspend: sub-second offset, stable across a phase transition.

#### Scenario: Paused freezes on RemainingSeconds

- GIVEN an active `focus` session with 12 minutes remaining
- WHEN `Pause` is invoked and the daemon publishes `Paused` = true, `PhaseEndsAt` = 0
- THEN the countdown displays 12 minutes and does not advance
- AND the countdown is not rendered as a negative value

#### Scenario: Idle freezes on RemainingSeconds

- GIVEN an active `focus` session with 12 minutes remaining
- WHEN the daemon publishes `Idle` = true, `PhaseEndsAt` = 0, `RemainingSeconds` = 720
- THEN the countdown displays 12 minutes and does not advance
- AND the countdown is not rendered as a negative value

#### Scenario: Countdown resumes on return from idle

- GIVEN a frozen countdown showing 12 minutes because `Idle` is true
- WHEN the daemon publishes `Idle` = false and a non-zero `PhaseEndsAt`
- THEN the countdown resumes from 12 minutes
- AND it is again derived from `PhaseEndsAt` on every tick

#### Scenario: A held break freezes on RemainingSeconds

- GIVEN the daemon publishes `Phase` = `break`, `Hold` = `prompt`, `PhaseEndsAt` = 0, `RemainingSeconds` = 600
- WHEN the indicator renders on successive ticks
- THEN the countdown displays 10 minutes and does not advance
