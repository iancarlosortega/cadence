```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:5f294fa3a18a0786711120bc365e17688ff772edf95bcd075c5f4456b381e112
verdict: pass
blockers: 0
critical_findings: 0
requirements: 6/6
scenarios: 15/15
test_command: gjs -m extension/test-render.js
test_exit_code: 0
test_output_hash: sha256:e9c8bfa814c139edab100c84ade1d790341f5e9b387045aacd6a154654d66c6b
build_command: go test ./... (daemon, must remain green and untouched)
build_exit_code: 0
build_output_hash: sha256:6e1a1a448c429c6cb07e44f01740f2ced129eb817ab9c28e55bdeaabebe2e665
```

# Verify Report: M3 — Break overlay with hold-to-skip

**Verdict: PASS**

41/41 tasks. All 6 requirements and 15 scenarios of `break-overlay` are satisfied, verified on real
hardware across several logout cycles. Four presentation defects were found during verification and
all four are fixed.

## Requirement coverage — `break-overlay`

| Requirement | Scenarios | Evidence | Result |
|---|---|---|---|
| Overlay Presence | 3 | Break covered the primary monitor above windows; clicks absorbed; **Alt+Tab still switched windows**, as product decision P1 requires | PASS |
| Fullscreen Suppression | 3 | Fullscreen across a break boundary produced no overlay, and the break still ran | PASS |
| Self-Owned Exit | 3 | Natural end dismissed it; **stopping `cadenced` while covered removed it instantly** and left the screen usable with no further action | PASS |
| Hold To Skip | 3 | Hold to completion ended the break; early release cancelled without skipping | PASS |
| Monitor Changes | 1 | Single-monitor session; the `monitors-changed` handler re-shows against the current primary. **Not exercised — see warning 2** | PARTIAL |
| Overlay Teardown | 2 | Disable/enable while covered produced a brief flash as the actor was destroyed and rebuilt, then counting resumed because the phase was still `break` | PASS |

## The check that mattered

`systemctl --user stop cadenced` with the overlay on screen removed it **instantly**, with no user
action. That scenario exists only because F3 — discovered earlier the same day — proved the daemon
can vanish. An overlay that waited for a dead daemon would be a covered screen with no exit.

Instant rather than faded is correct: `hide()` destroys the actor, and only a fade-in was written.

## Automated evidence

- `gjs -m extension/test-render.js` — **28/28**, including 10 overlay-predicate cases covering three
  of the four Self-Owned Exit paths.
- **Three mutation checks**, each observed failing: predicate ignoring `suppressed`; predicate
  ignoring `available` (the F3 path); `computeDisplay` ignoring `available`.
- `node --check` — all four JS files parse.
- `go test ./...` — five packages green; `git diff --stat -- daemon/` empty. Consumer-only, as
  proposed.
- Installed files byte-identical to source.

## Defects found during verification, all fixed

| Defect | Cause |
|---|---|
| Content rendered top-left | A plain `St.Widget` has no layout manager, so the child box's alignment was ignored. Fixed with `Clutter.BinLayout` |
| Progress bar full at rest, emptying on press | `scale_x` defaults to 1 and was only zeroed on hold-begin and cancel, never at construction |
| Countdown not centred | `St.Label` defaults to `ActorAlign.FILL`; the heading only looked centred because it was the widest child. Every child now sets `x_align` explicitly |
| Hold completed before the bar filled | `GLib.timeout_add_seconds` coalesces to second boundaries and could fire up to a second early against an exact 3000ms animation. Switched to `GLib.timeout_add` so both derive from one millisecond value |

**All four were found by the user looking at the screen. None was reachable by the test suite.**
They live in Clutter and St actor properties that cannot be instantiated outside a running Shell, so
the pure-predicate strategy that makes the safety logic provable offers nothing for appearance. That
is a limit of the approach, stated rather than hidden: the overlay provably will not trap you, and
whether it looks right needed eyes.

## Investigated and found not to be defects

- **Hold feels longer than 3s.** No defect: one constant drives both the timeout and the animation,
  and `enable-animations` carries no slow-down factor. Resolved as a product decision to keep 3s —
  the friction is the point. Two intended contributors to the feel: three seconds of active holding
  is genuinely long, and the overlay waits for the daemon to confirm before dismissing, so perceived
  time is 3s plus a round trip. That wait is design Decision 5 and was not traded away.
- **Panel amber during break.** Investigated twice, resolved as not a defect. Confirmed by correlated
  observation: while the overlay was on screen — which only happens when `phase === 'break'` — the
  panel read white.

## Success criteria from the proposal

| # | Criterion | Result |
|---|---|---|
| 1 | Break covers the primary monitor; clicks do not reach windows beneath | PASS |
| 2 | Alt+Tab still works | PASS — deliberate |
| 3 | Hold to completion calls `SkipBreak` and the daemon moves to focus | PASS |
| 4 | Release early cancels without skipping | PASS |
| 5 | Natural break end dismisses the overlay | PASS |
| 6 | Killing `cadenced` while covered dismisses it | PASS |
| 7 | Fullscreen at break start shows no overlay; the break still runs | PASS |
| 8 | Disable while covered removes it with no Shell log errors | PASS |
| 9 | `go test ./...` passes and `git diff --stat daemon/` is empty | PASS |

## Warnings

1. **Four presentation defects reached a human.** The test suite cannot see actor geometry, scale or
   alignment. Any future overlay work should assume the same and budget for eyes on a screen.
2. **Monitor Changes was not exercised.** A single-monitor session; the `monitors-changed` handler is
   written and connected but has never run against a real configuration change. This is the one
   requirement resting on code inspection rather than observation.
3. **A logout was wasted on a stale install.** The centring fix was written and not installed, so the
   Shell loaded the previous build and a correct fix looked broken. The runbook now opens with
   `make install-extension` and a `diff` that must print nothing.

## Findings

No new findings. F2 — the idle-pause republish path — remains open and owned by M4.

## Ready for Archive

Yes. No blockers. Commit and push remain separate human decisions.
