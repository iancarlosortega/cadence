# Design: Handling absences the daemon did not witness

## Technical Approach

Three small changes, no new domain concepts. `publish()` stops panicking and returns its error into
the handling `main.go` already has. The unit binds to the graphical session so the daemon stops
before its bus does. Startup applies the gap since `LastObserved` through the suspend rule that
already exists.

## Architecture Decisions

### Decision 1 — `publish()` returns an error by catching prop's panic

**Revised during apply. The first version of this decision was wrong.**

It originally said to swap `SetMust` for `prop.Properties.Set` and rejected `recover()` as hiding the
bug class. Implementing it broke `New()` immediately:

```
dbusapi: seed properties: publish SessionActive:
org.freedesktop.DBus.Properties.Error.ReadOnly
```

Reading `prop.go` explains why, and closes off the option:

| API | Behaviour |
|-----|-----------|
| `Set` | Exported, but returns `ErrReadOnly` unless `Writable` is true — it is false for all six properties, deliberately |
| `set` | Non-panicking internal setter — **unexported**, not callable from here |
| `SetMust` | `set` plus `panic(err)` |

There is no exported non-panicking internal write. Making the properties `Writable` would let any bus
client set them, which is a worse defect than the one being fixed. So the panic is caught at this
boundary and converted:

```go
func (s *Service) publish() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("publish: %v", r)
		}
	}()
	...SetMust calls unchanged...
	return nil
}
```

`apply()` wraps the returned error in `dbus.MakeFailedError` the same way its `EffectPersist` arm
already wraps a store failure, and `New()` fails loudly if seeding fails. `Tick()` already returns
`*dbus.Error` and `main.go:104-107` already logs it and continues.

This is a narrow, documented `recover` at a boundary where a third-party API panics by contract — not
a blanket one around unknown code. The earlier rejection stands for that broader shape; it simply did
not apply once the library's surface was actually read.

### Decision 2 — The unit is owned by the graphical session

```ini
[Unit]
PartOf=graphical-session.target
After=graphical-session.target

[Install]
WantedBy=graphical-session.target
```

`PartOf` propagates the target's stop, so logout delivers SIGTERM and the daemon takes the clean
shutdown path it already implements at `main.go:112-114`. Login starts it again. The daemon never
outlives its bus, so there is no dead connection to recover from and no reconnection logic to write.

`Restart=on-failure` stays as-is: a clean stop at logout is not a failure, and a genuine crash should
still be restarted.

**`Restart=always` was rejected on measured evidence.** This machine reports `StartLimitBurst=5` over
`StartLimitIntervalUSec=10s` with `RestartUSec=2s`. A daemon exiting for want of a bus would exhaust
that limit within ten seconds, be marked `failed`, and not return at login — worse than the defect
it was meant to fix.

### Decision 3 — Startup applies the gap through `ApplySuspend`

After `New()` seeds the service from persisted state, and only when a session was resumed:

```go
if found {
    svc.ApplySuspend(session.EventSuspended{From: initial.LastObserved, To: now})
}
```

The gap between the persisted `LastObserved` and startup **is** the quantity `EventSuspended`
describes: wall-clock time the daemon did not witness. Routing it through the existing entry point
means downtime inherits the table-tested rule, emits the same effects, and persists and publishes
through the same path. No new event type, no second copy of the threshold logic.

Rejected: a distinct `EventAbsence` type. It would duplicate `applySuspend`'s body to express the
same rule, and two copies of a threshold drift apart.

Not done here: renaming `EventSuspended` to something absence-shaped. It would be more honest
vocabulary but touches four files for no behaviour change, and this is a bugfix. One line at the call
site names the reason instead.

Edge cases, all already handled by `applySuspend`:

| Case | Behaviour |
|------|-----------|
| No persisted session (`found == false`) | Not called |
| Persisted session inactive | `applySuspend` returns early on `!s.Active` |
| Clock stepped backwards | `max(To.Sub(From), 0)` yields a zero gap |
| Gap below idle-credit | Elapsed does not advance; deadline republished (the earlier fix) |
| Gap at or above idle-credit | Break credited, fresh focus |

### Decision 4 — The two fixes compose, deliberately

They are not independent, and the interaction is the point. Decision 2 makes the daemon **stop at
logout**, which converts "logged out for three hours" into three hours of daemon downtime. Decision 3
then interprets that downtime as time away and credits a break, resetting to a fresh focus.

Before this change the same three hours would have been billed as focus work. The unit change would
have made F4 *more* visible, not less — fixing only F3 would have made the product worse.

## Data Flow

```
logout
  → graphical-session.target stops
  → PartOf propagates → SIGTERM
  → main.go sig case → clean return → state persisted, exit 0
  (no tick against a dead bus, no panic)

login
  → graphical-session.target starts → cadenced starts
  → store.Load() → initial, LastObserved = T0
  → dbusapi.New(...) → name exported, properties seeded
  → svc.ApplySuspend({From: T0, To: now})
        gap >= IdleCredit → creditBreak → fresh focus, elapsed 0
        gap <  IdleCredit → elapsed unchanged, deadline republished
  → effects: persist + notify → PropertiesChanged
  → panel is live and correct on the first tick
```

## File Changes

| File | Change |
|------|--------|
| `daemon/internal/dbusapi/service.go` | `publish() error` via `Set`; `apply()` wraps the failure |
| `daemon/cmd/cadenced/main.go` | Apply the startup gap when a session was resumed |
| `packaging/cadenced.service` | `PartOf` and `WantedBy` the graphical session |
| `daemon/internal/session/` | Untouched — the rule already exists |
| `extension/` | Untouched — no contract change |

## Interfaces / Contracts

Unchanged. Same bus name, path, interface, five methods, six properties, same persisted JSON shape.
`publish()` and `apply()` are internal; their signature change is invisible outside the package.

## Testing Strategy

- **Unit, deterministic**: the startup gap reduces to `Apply(state, EventSuspended{...}, now)`, which
  is pure. Table tests cover the long gap crediting a break, the short gap leaving elapsed alone, and
  parity between a downtime gap and a suspend of equal length — the spec's "Downtime and suspend
  agree" scenario asserted directly rather than by inspection.
- **The publish error path is unit-testable** by exporting on a connection that is then closed, which
  is what `internal/dbusapi` tests already set up connections for.
- **Not unit-testable**: that logout actually stops the unit, and that login starts it. Both need a
  real session cycle, so the change carries a runbook as M2 did.
- **Mutation check** on the F4 test, as with the previous fix: remove the startup call and confirm the
  long-gap test goes red.

## Migration / Rollout

`make -C packaging install-daemon` rewrites the unit; `systemctl --user daemon-reload` is already in
that target. The unit must also be re-enabled, because its `WantedBy` changes:
`systemctl --user disable cadenced.service` then `enable`. That is a one-time step and belongs in the
runbook.

Behaviour change users will notice: after a long logout or shutdown, cadence opens on a fresh focus
instead of a break. That is the fix.

## Open Questions

None. Renaming `EventSuspended` to absence-shaped vocabulary is noted as a possible follow-up, not a
question blocking this change.
