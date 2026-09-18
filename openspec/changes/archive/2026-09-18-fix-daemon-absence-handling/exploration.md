# Exploration: Handling absences the daemon did not witness

## Current State

Two defects found while verifying M2 on real hardware, both daemon-side, neither caused by M2. They
look unrelated but they are the same shape: **something went away while the daemon was not
watching, and the daemon handles that badly.** F3 is the bus going away. F4 is the daemon itself
going away.

### F3 — a logout panics the daemon

```
panic: dbus: connection closed by user
  prop.(*Properties).SetMust           prop.go:346
  dbusapi.(*Service).publish           service.go:199
  dbusapi.(*Service).apply             service.go:176
  dbusapi.(*Service).Tick              service.go:142
  main.run                             main.go:105
cadenced.service: Main process exited, code=exited, status=2/INVALIDARGUMENT
```

Logging out tears down the session bus. `systemd --user` outlives the graphical session, so the next
tick calls `publish()`, which calls `prop.Properties.SetMust`. **`SetMust` panics on error by
design** — the "Must" in the name is the contract. A closed connection at logout is an entirely
expected condition, so an ordinary logout crashes the daemon.

The bitter detail: `main.go:104-107` already handles an error from `Tick`:

```go
case <-ticker.C:
    if err := svc.Tick(); err != nil {
        log.Printf("cadenced: tick: %v", err)
    }
```

The error path exists and is correct. `SetMust` simply never uses it — it panics instead of
returning, so the handling the daemon already has is bypassed. `Restart=on-failure` then restarts
the process, which is exactly why this stayed invisible for a month: **the failure erases its own
evidence.**

### F4 — daemon downtime is charged as focus work

Measured on the pure reducer, same 8h gap, two arrival paths:

| Gap arrives as | Result |
|---|---|
| Daemon downtime (a tick whose `LastObserved` is old) | `phase=break` |
| Suspend (`EventSuspended`) | `phase=focus` |

`applyTick`'s default branch (`machine.go:107-118`) charges `now.Sub(s.LastObserved)` to the current
phase unconditionally. On restart, `LastObserved` is whatever was last persisted, so the entire
downtime is billed as focus work — which blows through the phase and drops the user into a break.

Boot after an overnight shutdown and cadence opens with "take a break", before any work has
happened. M1 decision P1 says the opposite: an absence past the idle-credit threshold is time away
from the desk, which credits a break and resets to a **fresh focus**. `applySuspend` implements
exactly that. Downtime never learned it.

## Affected Areas

- `daemon/internal/dbusapi/service.go` — `publish()` and the `apply`/`Tick` error path (F3).
- `daemon/cmd/cadenced/main.go` — what the loop does when the bus is gone (F3).
- `packaging/cadenced.service` — possibly the restart policy (F3).
- `daemon/internal/session/machine.go` or `cmd/cadenced/main.go` — where the startup gap is
  interpreted (F4).
- `openspec/specs/session-timer` and/or `session-persistence` — F4 changes observable behaviour and
  needs a requirement.
- `extension/` — untouched. Neither fix changes the published contract.

## Approaches

### Decision 1 — F3: what `publish()` does when the bus is gone

- **Option A (recommended): `Set` instead of `SetMust`, propagate the error.** `prop.Properties.Set`
  returns an error where `SetMust` panics. `publish()` returns it, `apply()` returns it, and
  `main.go`'s existing `log.Printf("cadenced: tick: %v", err)` finally does its job. Minimal, and it
  makes the error path that already exists actually reachable.
- **Option B: guard on connection liveness before publishing.** Needs a liveness concept godbus does
  not really expose, and still races — the connection can close between the check and the call.
- **Option C: recover() around publish.** Converts a panic into a swallowed error at a distance.
  Hides the class of bug rather than fixing it.

### Decision 2 — F3: what the daemon does after the bus is gone

Separate from Decision 1, and the more interesting question. Once the bus is gone the daemon can
still count, still persist, but can tell nobody.

- **Option A (recommended): keep running, keep persisting, log once, stop trying to publish.** The
  timer state is the valuable thing and it stays correct on disk. Nothing crashes, nothing loops.
  The cost: after the user logs back in, this process is attached to a dead connection and will
  never publish again, so the panel stays dim until the service is restarted.
- **Option B: exit cleanly (code 0) on connection loss.** Honest — the daemon's purpose is gone. But
  `Restart=on-failure` does not restart on 0, and `WantedBy=default.target` only fires when the user
  manager starts, so a re-login would not bring it back. Would need `Restart=always`, which then
  loops every `RestartSec` while no bus exists.
- **Option C: reconnect.** Correct in principle, most work, and needs a re-export of the name,
  object and properties on the new connection.

This is a real product decision about what the user experiences after logging back in, and it is put
to the user below.

### Decision 3 — F4: how a startup gap should be interpreted

- **Option A (recommended): reuse the suspend rule.** At startup, the gap between the persisted
  `LastObserved` and now is exactly "time the daemon did not witness" — the same quantity
  `EventSuspended{From, To}` already describes. Synthesising that event on resume makes downtime and
  suspend behave identically, satisfies M1 decision P1 for free, and adds no new domain concept. The
  rule is already table-tested.
- **Option B: cap the charge.** Introduces an arbitrary constant and a second concept for the same
  thing.
- **Option C: leave it.** Every boot after an overnight shutdown starts on a break.

One honest wrinkle with Option A: `LastObserved` is persisted on transitions and a 60s heartbeat, so
after a clean run it can be up to 60s stale. Treating that as absence **under-charges** by up to a
minute. Today's behaviour **over-charges** without limit. A bounded under-charge of under a minute is
plainly the better error.

## Recommendation

Fix F3 with `Set` plus the existing error path, and F4 by synthesising the suspend event at startup.
Both are small. Decision 2 needs the user's call because it is about what they see after logging back
in, not about correctness.

## Risks

- F4 changes observable timer behaviour, so it needs a spec requirement rather than just a patch.
- F3's Decision 2 interacts with the systemd unit; changing `Restart=` has its own failure modes and
  should not be done casually.
- Neither fix is verifiable by unit test alone. F3 needs a real logout and F4 a real restart across a
  long gap, so the change must carry a runbook the way M2 did.

## Ready for Proposal

Yes, once Decision 2 is settled.
