# Proposal: Handling absences the daemon did not witness

## Intent

Two defects found while verifying M2 on real hardware. They look unrelated but share a shape:
something went away while the daemon was not watching, and the daemon handled it badly. F3 is the
session bus going away at logout, which panics the process. F4 is the daemon itself going away,
whose downtime is then billed to the user as focus work.

## Scope

### In Scope

- **F3a** — `publish()` uses `prop.Properties.Set` instead of `SetMust`, returning the error through
  `apply()` and `Tick()` into the error path `main.go:104-107` already has.
- **F3b** — `packaging/cadenced.service` binds to the graphical session:
  `PartOf=graphical-session.target` and `WantedBy=graphical-session.target`. Logout stops the daemon
  cleanly through the SIGTERM path it already implements; login starts it again.
- **F4** — at startup, the gap between the persisted `LastObserved` and now is applied as
  `EventSuspended{From: LastObserved, To: now}`, so downtime and suspend obey one rule.
- A delta on `session-persistence` for F4's observable behaviour change.
- A runbook, because neither fix is provable by unit test alone.

### Out of Scope

- The extension. Neither fix changes the published D-Bus contract; `extension/` is untouched.
- Reconnecting to a new bus in-process. Made unnecessary by F3b: the daemon no longer outlives its
  session, so there is no dead connection to recover from.
- The idle-pause republish path (F2), still owned by M4.
- Real idle detection. `IdleSource` stays stubbed.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `session-persistence` — a requirement for how a startup gap is interpreted.

## Approach

### F3 — stop panicking, and stop outliving the session

`SetMust` panics by contract. The daemon already handles a `Tick` error correctly; the panic simply
bypasses that handling. Switching to `Set` makes the existing path reachable.

That alone stops the crash but leaves the daemon attached to a dead bus. The unit change is the real
fix: a session-scoped service should be owned by the session. `PartOf` propagates the target's stop,
so logout delivers SIGTERM and the daemon takes its existing clean shutdown path.

**`Restart=always` was considered and rejected on evidence.** With the unit's `RestartSec=2` and
systemd's defaults on this machine — `StartLimitBurst=5`, `StartLimitIntervalUSec=10s` — a daemon
exiting immediately for want of a bus would exhaust the limit within ten seconds and be marked
failed, and would then not return at login. It would be worse than the bug.

### F4 — one rule for time the daemon did not witness

The gap between the persisted `LastObserved` and startup is exactly the quantity
`EventSuspended{From, To}` already describes. Applying that event at startup makes downtime inherit
the suspend rule: past the idle-credit threshold it credits a break and resets to a fresh focus;
below it, elapsed does not advance. M1 decision P1 is satisfied without inventing a second concept,
and the rule is already table-tested.

**Known trade, stated plainly.** `LastObserved` is persisted on transitions and a 60s heartbeat, so
after a clean run it can be up to a minute stale. Treating that as absence under-charges the phase by
up to 60s. Today's behaviour over-charges without limit — an overnight shutdown bills eight hours of
sleep as focus. A bounded under-charge under a minute is the better error, and it matches the
existing "within 60s" tolerance already written into `session-persistence`.

## Affected Areas

- `daemon/internal/dbusapi/service.go` — `publish`, `apply`, `Tick` signatures and error flow.
- `daemon/cmd/cadenced/main.go` — apply the startup gap before serving.
- `packaging/cadenced.service` — session binding.
- `openspec/specs/session-persistence/` — F4 requirement.
- `daemon/internal/session/` — expected untouched; F4 reuses `applySuspend` as it stands.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Unit change does not stop the daemon at logout on this desktop | Med | `graphical-session.target` is active here with ten-plus services already bound to it; verified by runbook after a real logout |
| F4 under-charges after a clean restart | Certain, bounded | Bounded at 60s by the heartbeat; the spec already allows a 60s tolerance |
| F4's startup event fires with no persisted state | Low | `applySuspend` returns early when the session is inactive |
| Changing `Tick`'s error flow disturbs the tick loop | Low | `main.go` already logs and continues on a `Tick` error; this only makes it reachable |
| Neither fix is unit-testable end to end | Certain | Runbook with a real logout and a real long-gap restart, as M2 did |

## Rollback Plan

`git revert`, then `make -C packaging install-daemon` to restore the previous unit and binary. No
persisted shape changes, no D-Bus contract change, so a rolled-back daemon and the shipped extension
still interoperate.

## Dependencies

None. No new Go modules, no schema change, no extension change.

## Success Criteria

1. `go test ./...` in `daemon/` passes, including new coverage for the startup gap.
2. A real logout no longer panics: no `panic:` in the journal, and the unit stops rather than
   restarting.
3. A real login starts the daemon again automatically, and the panel is live without manual
   intervention.
4. After stopping the daemon for longer than the idle-credit threshold and starting it again, the
   session is in a **fresh focus**, not a break — the same outcome a suspend of that length produces.
5. After a brief stop, elapsed does not jump by the downtime.
6. The extension is unchanged and still passes its 17 render checks.
