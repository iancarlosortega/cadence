```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:0b68c36199eac1ecb04eaf0f6c4fb315bd91b15d9e82694cc5d20dda5f6ba5d3
verdict: pass
blockers: 0
critical_findings: 0
requirements: 8/8
scenarios: 16/16
test_command: gjs -m extension/test-render.js
test_exit_code: 0
test_output_hash: sha256:01b5ca2b6a10f080ab1a4666ceed2853f81891b953796b08139c8e62110918ee
build_command: go test ./... (daemon, must remain green and untouched)
build_exit_code: 0
build_output_hash: sha256:b5882ab72d14151df018548625a8bbccfd4a68b5a4d9b2bded3f90b5f0eb59ad
```

# Verify Report: M2 — GNOME Shell Extension

**Verdict: PASS**

30/30 tasks. All 8 requirements and 16 scenarios of the `panel-indicator` spec are satisfied.
Verified on real hardware — GNOME Shell 48.8 on Wayland — after the extension's first ever load,
against a daemon carrying `fix-suspend-deadline-republish`.

## Requirement coverage — `panel-indicator`

| Requirement | Scenarios | Evidence | Result |
|---|---|---|---|
| Indicator Presence | 2 | `gnome-extensions info` reports `State: ACTIVE`; indicator present with `cadenced` stopped, and removed on disable | PASS |
| State Authority | 2 | Stopping `cadenced` mid-session dimmed the indicator and the countdown stopped advancing — the tick did not conclude a transition. Unit checks cover the zero-remainder case | PASS |
| Countdown Derivation | 2 | Post-suspend offsets `+0.171s / +0.174s / +0.174s`, sub-second and stable across a focus→break transition. Paused path unit-tested and observed frozen | PASS |
| Presentation States | 2 | All four rows observed: dimmed with no daemon, dimmed with no session, running countdown, frozen while paused | PASS |
| Break Warning | 3 | Amber appears at exactly `2:00` remaining in `focus` and clears at the break transition; legible on the dark panel | PASS |
| Control Actions | 2 | Pause froze the label **and** `busctl` reported `Paused b true`; no optimistic update. Menu actions inert with no daemon, nothing raised into the Shell log | PASS |
| Connection Lifecycle | 1 | Daemon stopped and restarted repeatedly within one enabled session; indicator dimmed and recovered each time without re-enabling | PASS |
| Teardown Hygiene | 2 | Three disable/enable cycles including one with `cadenced` absent; no errors, no disposed-object warnings, no leaked-source complaints | PASS |

## Automated evidence

- `gjs -m extension/test-render.js` — **17/17** pure-logic checks pass, covering the warning
  boundary at exactly 120 and 121, the paused branch reading `remainingSeconds`, and menu
  sensitivity per phase.
- `go test ./...` in `daemon/` — all packages pass.
- `node --check` on `extension.js` and `render.js` — both parse as ES modules.
- `git show --stat cf57273 -- daemon/` — **empty**. M2 shipped without touching the daemon, as its
  proposal required.

## Success criteria from the proposal

| # | Criterion | Result |
|---|---|---|
| 1 | Indicator appears on enable | PASS |
| 2 | Dimmed with daemon stopped; goes live when it starts, without re-enabling | PASS |
| 3 | CLI updates the panel; panel Pause freezes it and `busctl` confirms the daemon paused | PASS |
| 4 | Amber at 120s in focus, normal on the break transition | PASS |
| 5 | Disable leaves no source, proxy or handler; no Shell log errors | PASS |
| 6 | `go test ./...` in `daemon/` passes unchanged | PASS |
| 7 | I1 under a stopped daemon | PASS |
| 8 | I1 across suspend | PASS — sub-second offset (see warning 1) |

## Warnings

1. **Criterion 8's literal wording remains unachievable.** It asks for an offset of zero; the
   measured offset is `~0.17s` because `PhaseEndsAt` is published as a whole-second unix value. This
   is contract resolution, not drift. Read it as "no persisting whole-second offset", as recorded in
   the `fix-suspend-deadline-republish` verify report.
2. **Light-theme amber was not observed.** `.cadence-warning-light` (`#8f5300` on `#fafafb`) is
   implemented and installed but was only exercised in dark mode. Low risk, but it is untested
   colour rather than verified colour.
3. **The suspend and leak checks ran against a 3-minute test config**, not the 50-minute default.
   The logic is duration-independent, but the default timings have not themselves been observed
   end-to-end.

## Findings raised during verification, outside M2's scope

- **F3 — `cadenced` panics on logout.** `publish()` calls `prop.SetMust`, which panics on error by
  design; logging out closes the session bus underneath the next `Tick`. Pre-existing from M1,
  unrelated to M2's code (`internal/dbusapi/` unchanged by this milestone), and self-healing via
  `Restart=on-failure`, which is why it stayed invisible. Daemon-side, needs its own change.
- **F4 — daemon downtime is charged as focus work.** Measured on the pure reducer: an 8h gap
  arriving as daemon downtime yields `phase=break`, while the same 8h as a suspend yields
  `phase=focus`. Booting after an overnight shutdown therefore starts the user on a break. Contrary
  to M1 decision P1, which treats a long absence as time away. Daemon-side, needs its own change.

Both were found by running the system, not by reading it, and neither is caused by M2.

## Corrections made during verification

- An early report of amber persisting into `break` was a **false alarm**: with the 3-minute test
  config the cycle turns over every 4 minutes, so an untimestamped `0:57` reading is ambiguous
  between focus (amber correct) and break (amber wrong). Resolved by reading the daemon phase at the
  same instant as the panel. Two hypotheses were disproved before any code changed — the daemon does
  emit `Phase → "break"` (captured off the bus), and `st_widget_add_style_class_name` guards against
  duplicates (GNOME 48 source).
- `packaging/check-offset.py` failed a correct build by reading `Phase` once at startup and
  comparing a break countdown against the focus length, reporting the 120s difference as drift.
  Fixed to re-read the phase per sample.

## Ready for Archive

Yes. No blockers. Commit and push remain separate human decisions.
